// Copyright 2026 The pandaemonium Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package codex

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/zchee/gows"

	llm "github.com/zchee/pandaemonium/pkg/llm"
)

// Transport represents a bidirectional JSON message Transport between the client and the app-server.
type Transport interface {
	io.Closer
	WriteJSON(ctx context.Context, data []byte) error
	ReadJSON(ctx context.Context) ([]byte, error)
}

const defaultCloseTimeout = 10 * time.Second

type deadlineTransport interface {
	closeByDeadline(deadline time.Time) error
}

// stdioTransport represents a bidirectional JSON message transport over the app-server process's standard input and output streams.
type stdioTransport struct {
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

var _ Transport = (*stdioTransport)(nil)

// Close implements [Transport].
func (t *stdioTransport) Close() error {
	if t.stdin != nil {
		return t.stdin.Close()
	}
	return nil
}

// WriteJSON implements [Transport].
func (t *stdioTransport) WriteJSON(_ context.Context, data []byte) error {
	return llm.WriteJSONLine(
		t.stdin,
		data,
		func() error { return &TransportClosedError{Message: "app-server is not running"} },
		func(err error) error { return &TransportClosedError{Message: err.Error()} },
	)
}

// ReadJSON implements [Transport].
func (t *stdioTransport) ReadJSON(ctx context.Context) ([]byte, error) {
	// The goroutine in ReadJSONLine is orphaned when ctx is cancelled; it
	// exits naturally when [stdioTransport.Close] closes stdoutCloser (the raw
	// stdout pipe), which causes ReadBytes to return an error.
	return llm.ReadJSONLine(
		ctx,
		t.stdout,
		func() error { return &TransportClosedError{Message: "app-server is not running"} },
	)
}

// websocketTransport represents a bidirectional JSON message transport over a websocket connection to the app-server.
type websocketTransport struct {
	raw             net.Conn
	conn            *gows.Conn
	writeGate       chan struct{}
	readGate        chan struct{}
	done            chan struct{}
	terminalOnce    sync.Once
	terminalErr     *TransportClosedError
	shutdownOnce    sync.Once
	shutdownErr     error
	deadlineMu      sync.Mutex
	closeTimeout    time.Duration
	now             func() time.Time
	redactor        transportRedactor
	beforeTerminate func()
}

var _ Transport = (*websocketTransport)(nil)

// Close implements [Transport].
func (t *websocketTransport) Close() error {
	if t == nil || t.conn == nil || t.raw == nil {
		return nil
	}
	timeout := t.closeTimeout
	if timeout <= 0 {
		timeout = defaultCloseTimeout
	}
	now := t.now
	if now == nil {
		now = time.Now
	}
	return t.closeByDeadline(now().Add(timeout))
}

func (t *websocketTransport) closeByDeadline(deadline time.Time) error {
	t.shutdownOnce.Do(func() {
		t.markTerminal(newTransportClosedError("websocket transport closed", net.ErrClosed))
		t.deadlineMu.Lock()
		_ = t.raw.SetDeadline(deadline)
		t.deadlineMu.Unlock()

		if t.acquireUntil(t.writeGate, deadline) {
			_ = t.conn.WriteClose(gows.CloseNormalClosure, "")
			t.writeGate <- struct{}{}
		}
		if t.acquireUntil(t.readGate, deadline) {
			ctx, cancel := context.WithDeadline(context.Background(), deadline)
			t.shutdownErr = t.conn.CloseContext(ctx, gows.CloseNormalClosure, "")
			cancel()
			t.readGate <- struct{}{}
		}
		_ = t.closeRaw()
		if errors.Is(t.shutdownErr, net.ErrClosed) || errors.Is(t.shutdownErr, io.ErrClosedPipe) || errors.Is(t.shutdownErr, os.ErrDeadlineExceeded) || errors.Is(t.shutdownErr, gows.ErrCloseTimeout) || errors.Is(t.shutdownErr, context.DeadlineExceeded) {
			t.shutdownErr = nil
		}
	})
	return t.shutdownErr
}

// WriteJSON implements [Transport].
func (t *websocketTransport) WriteJSON(ctx context.Context, data []byte) error {
	if t == nil || t.conn == nil {
		return &TransportClosedError{Message: "app-server is not running"}
	}
	if err := t.acquire(ctx, t.writeGate); err != nil {
		return err
	}
	defer func() { t.writeGate <- struct{}{} }()
	if err := t.closedError(); err != nil {
		return err
	}
	err := t.contextIO(ctx, t.conn.SetWriteDeadline, func() error {
		return t.conn.WriteMessage(gows.OpcodeText, data)
	})
	if err != nil {
		closed := newTransportClosedError("websocket write failed", t.sanitizedTransportCause(err))
		t.terminate(closed)
		return t.terminalErr
	}
	return nil
}

// ReadJSON implements [Transport].
func (t *websocketTransport) ReadJSON(ctx context.Context) ([]byte, error) {
	if t == nil || t.conn == nil {
		return nil, &TransportClosedError{Message: "app-server is not running"}
	}
	if err := t.acquire(ctx, t.readGate); err != nil {
		return nil, err
	}
	defer func() { t.readGate <- struct{}{} }()
	if err := t.closedError(); err != nil {
		return nil, err
	}
	for {
		var typ gows.Opcode
		var payload []byte
		err := t.contextIO(ctx, t.conn.SetReadDeadline, func() error {
			var err error
			typ, payload, err = t.conn.ReadMessage()
			return err
		})
		if err != nil {
			var closeErr *gows.CloseError
			if errors.As(err, &closeErr) && closeErr.Code == gows.CloseNormalClosure {
				t.terminate(newTransportClosedError("websocket closed normally", t.sanitizedCloseError(closeErr)))
				return nil, io.EOF
			}
			closed := newTransportClosedError("websocket read failed", t.sanitizedTransportCause(err))
			if errors.As(err, &closeErr) {
				sanitized := t.sanitizedCloseError(closeErr)
				closed = newTransportClosedError(fmt.Sprintf("websocket closed with code %d", closeErr.Code), sanitized)
			}
			t.terminate(closed)
			return nil, t.terminalErr
		}

		switch typ {
		case gows.OpcodeText:
			owned := make([]byte, len(payload)+1)
			copy(owned, payload)
			owned[len(payload)] = '\n'
			return owned, nil
		case gows.OpcodeBinary:
			t.terminate(newTransportClosedError("unexpected binary websocket message", net.ErrClosed))
			return nil, &AppServerError{Message: "unexpected binary websocket message"}
		}
	}
}

func newWebsocketTransport(raw net.Conn, hs *gows.Handshake, redactors ...transportRedactor) *websocketTransport {
	raw = &onceCloseConn{Conn: raw}
	var redactor transportRedactor
	if len(redactors) != 0 {
		redactor = redactors[0].clone()
	}
	t := &websocketTransport{
		raw: raw,
		conn: gows.NewClientConn(
			raw,
			gows.WithBuffered(hs.Buffered),
			gows.WithReadLimit(32<<10),
		),
		writeGate:    make(chan struct{}, 1),
		readGate:     make(chan struct{}, 1),
		done:         make(chan struct{}),
		closeTimeout: defaultCloseTimeout,
		now:          time.Now,
		redactor:     redactor,
	}
	t.writeGate <- struct{}{}
	t.readGate <- struct{}{}
	return t
}

func (t *websocketTransport) closeRaw() error {
	return t.raw.Close()
}

func (t *websocketTransport) acquire(ctx context.Context, gate chan struct{}) error {
	if err := t.closedError(); err != nil {
		return err
	}
	select {
	case <-t.done:
		return t.terminalErr
	case <-ctx.Done():
		if err := t.closedError(); err != nil {
			return err
		}
		closed := newTransportClosedError("websocket operation canceled", ctx.Err())
		if t.beforeTerminate != nil {
			t.beforeTerminate()
		}
		t.terminate(closed)
		return t.terminalErr
	case <-gate:
		return nil
	}
}

func (t *websocketTransport) acquireUntil(gate chan struct{}, deadline time.Time) bool {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return false
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-gate:
		return true
	case <-timer.C:
		return false
	}
}

func (t *websocketTransport) closedError() error {
	select {
	case <-t.done:
		return t.terminalErr
	default:
		return nil
	}
}

func (t *websocketTransport) markTerminal(err *TransportClosedError) {
	t.terminalOnce.Do(func() {
		t.terminalErr = err
		close(t.done)
	})
}

func (t *websocketTransport) terminate(err *TransportClosedError) {
	t.markTerminal(err)
	_ = t.closeRaw()
}

func (t *websocketTransport) sanitizedTransportCause(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if closeErr, ok := errors.AsType[*gows.CloseError](err); ok {
		return t.sanitizedCloseError(closeErr)
	}
	return net.ErrClosed
}

func (t *websocketTransport) sanitizedCloseError(err *gows.CloseError) *gows.CloseError {
	reason := t.redactor.sanitize(err.Reason)
	return &gows.CloseError{Code: err.Code, Reason: reason, Sent: err.Sent}
}

func (t *websocketTransport) contextIO(ctx context.Context, setDeadline func(time.Time) error, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Time{}
	}
	t.deadlineMu.Lock()
	if closed := t.closedError(); closed != nil {
		t.deadlineMu.Unlock()
		return closed
	}
	err := setDeadline(deadline)
	t.deadlineMu.Unlock()
	if err != nil {
		return err
	}
	canceled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		t.deadlineMu.Lock()
		if t.closedError() == nil {
			_ = setDeadline(time.Now())
		}
		t.deadlineMu.Unlock()
		close(canceled)
	})
	err = fn()
	if !stop() {
		<-canceled
	}
	t.deadlineMu.Lock()
	if t.closedError() == nil {
		_ = setDeadline(time.Time{})
	}
	t.deadlineMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

type transportRedactor struct {
	secrets []string
}

func newTransportRedactor(u *url.URL, token string) transportRedactor {
	secrets := []string{token}
	if u != nil {
		secrets = append(secrets, urlSecrets(u)...)
	}
	return transportRedactor{secrets: secrets}.clone()
}

func (r *transportRedactor) addURL(u *url.URL) {
	if u != nil {
		r.secrets = append(r.secrets, urlSecrets(u)...)
		r.secrets = r.clone().secrets
	}
}

func urlSecrets(u *url.URL) []string {
	secrets := []string{u.RawQuery}
	for key, values := range u.Query() {
		secrets = append(secrets, key)
		secrets = append(secrets, values...)
	}
	if u.User != nil {
		username := u.User.Username()
		secrets = append(secrets, username, u.User.String())
		if password, ok := u.User.Password(); ok {
			pair := username + ":" + password
			secrets = append(secrets, password, pair, base64.StdEncoding.EncodeToString([]byte(pair)))
		}
	}
	return secrets
}

func (r transportRedactor) clone() transportRedactor {
	cloned := transportRedactor{secrets: make([]string, 0, len(r.secrets))}
	for _, secret := range r.secrets {
		if secret = strings.TrimSpace(secret); secret != "" {
			cloned.secrets = append(cloned.secrets, secret)
		}
	}
	return cloned
}

func (r transportRedactor) sanitize(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	lower := strings.ToLower(reason)
	for _, secret := range r.secrets {
		if strings.Contains(reason, secret) {
			return "[redacted peer reason]"
		}
	}
	if !utf8.ValidString(reason) || len(reason) > 96 || strings.ContainsAny(reason, "\r\n\x00@?=&:") || strings.Contains(lower, "bearer") || strings.Contains(lower, "proxy") || strings.Contains(lower, "token") || strings.Contains(lower, "://") {
		return "[redacted peer reason]"
	}
	return reason
}

type onceCloseConn struct {
	net.Conn
	once sync.Once
	err  error
}

func (c *onceCloseConn) Close() error {
	c.once.Do(func() { c.err = c.Conn.Close() })
	return c.err
}

func validateWebSocketConfig(cfg *WebSocketConfig, tokenSource bool) error {
	if cfg == nil {
		return nil
	}
	switch cfg.AuthMode {
	case WebSocketAuthNone, WebSocketAuthCapabilityToken, WebSocketAuthSignedBearerToken:
	default:
		return fmt.Errorf("invalid websocket auth mode %q", cfg.AuthMode)
	}
	if cfg.AuthMode == WebSocketAuthNone {
		if websocketAuthFieldsSet(cfg) {
			return fmt.Errorf("ws-auth none cannot include websocket auth fields")
		}
		return nil
	}
	if err := validateWebSocketSecretFormat(cfg); err != nil {
		return err
	}
	switch cfg.AuthMode {
	case WebSocketAuthCapabilityToken:
		if err := validateWebSocketCapabilityToken(cfg, tokenSource); err != nil {
			return err
		}
	case WebSocketAuthSignedBearerToken:
		if err := validateWebSocketSignedBearerToken(cfg, tokenSource); err != nil {
			return err
		}
	}
	if cfg.Issuer != "" && strings.TrimSpace(cfg.Issuer) == "" {
		return fmt.Errorf("ws-issuer cannot be empty if set")
	}
	if cfg.Audience != "" && strings.TrimSpace(cfg.Audience) == "" {
		return fmt.Errorf("ws-audience cannot be empty if set")
	}
	return nil
}

// validateWebSocketSecretFormat checks the auth fields shared by every non-"none"
// websocket auth mode: clock skew sign, the token-file/token-sha256 mutual
// exclusion, and the token-sha256 hex digest format.
func validateWebSocketSecretFormat(cfg *WebSocketConfig) error {
	if cfg.MaxClockSkewSeconds != nil && *cfg.MaxClockSkewSeconds < 0 {
		return fmt.Errorf("ws-max-clock-skew-seconds must be non-negative")
	}
	if cfg.TokenFile != "" && cfg.TokenSHA256 != "" {
		return fmt.Errorf("invalid websocket auth: --ws-token-file and --ws-token-sha256 are mutually exclusive")
	}
	if tokenSHA256 := strings.TrimSpace(cfg.TokenSHA256); tokenSHA256 != "" {
		if len(tokenSHA256) != 64 {
			return fmt.Errorf("ws-token-sha256 must be a 64-character hex digest")
		}
		if _, err := hex.DecodeString(tokenSHA256); err != nil {
			return fmt.Errorf("ws-token-sha256 must be a 64-character hex digest")
		}
	}
	return nil
}

// validateWebSocketCapabilityToken validates capability-token auth: a token file
// or token-sha256 is required, a sha256-only digest needs a client bearer token
// source, and a configured token file must have content.
func validateWebSocketCapabilityToken(cfg *WebSocketConfig, tokenSource bool) error {
	if cfg.TokenSHA256 != "" && !tokenSource {
		return fmt.Errorf("capability-token auth with --ws-token-sha256 requires a client bearer token source")
	}
	if cfg.TokenFile == "" && cfg.TokenSHA256 == "" {
		return fmt.Errorf("capability-token auth requires either ws token file or ws-token-sha256")
	}
	if cfg.TokenFile != "" {
		return validateTokenFileHasContent(cfg.TokenFile, "websocket token file")
	}
	return nil
}

// validateWebSocketSignedBearerToken validates signed-bearer-token auth: a
// shared-secret file with content and a client bearer token source are required.
// The issuer stays optional, matching the app-server CLI.
func validateWebSocketSignedBearerToken(cfg *WebSocketConfig, tokenSource bool) error {
	if cfg.SharedSecretFile == "" {
		return fmt.Errorf("signed-bearer-token auth requires --ws-shared-secret-file")
	}
	if !tokenSource {
		return fmt.Errorf("signed-bearer-token auth requires a client bearer token source")
	}
	return validateTokenFileHasContent(cfg.SharedSecretFile, "websocket shared secret file")
}

func validateUnixWebSocketConfig(cfg *WebSocketConfig) error {
	if cfg == nil {
		return nil
	}
	if cfg.AuthMode != WebSocketAuthNone || websocketAuthFieldsSet(cfg) {
		return fmt.Errorf("unix websocket listen does not support websocket auth fields; use unix socket file permissions or a ws:// listen endpoint with websocket auth")
	}
	return nil
}

func websocketAuthFieldsSet(cfg *WebSocketConfig) bool {
	if cfg == nil {
		return false
	}
	return cfg.TokenFile != "" ||
		cfg.TokenSHA256 != "" ||
		cfg.SharedSecretFile != "" ||
		cfg.Issuer != "" ||
		cfg.Audience != "" ||
		cfg.ClientBearerToken != "" ||
		cfg.ClientBearerTokenFile != "" ||
		cfg.MaxClockSkewSeconds != nil
}

func wsLaunchArgs(cfg *WebSocketConfig) []string {
	if cfg == nil || cfg.AuthMode == WebSocketAuthNone {
		return nil
	}
	args := []string{"--ws-auth", string(cfg.AuthMode)}
	if cfg.TokenFile != "" {
		args = append(args, "--ws-token-file", cfg.TokenFile)
	}
	if cfg.TokenSHA256 != "" {
		args = append(args, "--ws-token-sha256", cfg.TokenSHA256)
	}
	if cfg.SharedSecretFile != "" {
		args = append(args, "--ws-shared-secret-file", cfg.SharedSecretFile)
	}
	if cfg.Issuer != "" {
		args = append(args, "--ws-issuer", cfg.Issuer)
	}
	if cfg.Audience != "" {
		args = append(args, "--ws-audience", cfg.Audience)
	}
	if cfg.MaxClockSkewSeconds != nil {
		args = append(args, "--ws-max-clock-skew-seconds", fmt.Sprintf("%d", *cfg.MaxClockSkewSeconds))
	}
	return args
}

func ensureWebSocketListenAllowed(parsed *url.URL, cfg ListenConfig) error {
	if parsed == nil {
		return nil
	}
	if parsed.Scheme != "ws" {
		return nil
	}
	if parsed.Hostname() == "" {
		return nil
	}
	if cfg.AllowInsecureRemoteWebSocket {
		return nil
	}
	if host := parsed.Hostname(); host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("insecure websocket listen %q is allowed only for loopback hosts unless explicit opt-in", parsed.String())
	}
	return nil
}

func websocketHasClientBearerToken(cfg *WebSocketConfig) bool {
	if cfg == nil {
		return false
	}
	if strings.TrimSpace(cfg.ClientBearerToken) != "" {
		return true
	}
	if strings.TrimSpace(cfg.ClientBearerTokenFile) != "" {
		return true
	}
	return cfg.AuthMode == WebSocketAuthCapabilityToken && strings.TrimSpace(cfg.TokenFile) != ""
}

func dialWebSocketWithWait(ctx context.Context, procDone <-chan error, listen string, cfg *WebSocketConfig, env map[string]string, cwd string) (*websocketTransport, error) {
	mode, socketPath, err := websocketListenMode(listen, env, cwd)
	if err != nil {
		return nil, err
	}

	attemptLimit := 50
	for attempt := range attemptLimit {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case waitErr := <-procDone:
			return nil, appServerExitedBeforeReadyError(mode, socketPath, waitErr)
		default:
		}

		dialCtx := ctx
		var cancel context.CancelFunc
		if cfg != nil && cfg.DialTimeout > 0 {
			dialCtx, cancel = context.WithTimeout(ctx, cfg.DialTimeout)
		}
		conn, err := dialWebSocket(dialCtx, listen, cfg, env, cwd)
		if cancel != nil {
			cancel()
		}
		if err == nil {
			return conn, nil
		}
		backoff := min(time.Duration(25*(attempt+1))*time.Millisecond, 250*time.Millisecond)
		if attempt >= attemptLimit-1 {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		case waitErr := <-procDone:
			return nil, appServerExitedBeforeReadyError(mode, socketPath, waitErr)
		}
	}

	return nil, appServerNotReadyError(mode, socketPath, attemptLimit)
}

// appServerExitedBeforeReadyError describes a server child that exited before
// its websocket/unix endpoint became reachable, folding in the optional socket
// path and the optional Wait error (wrapped when present).
func appServerExitedBeforeReadyError(mode, socketPath string, waitErr error) error {
	switch {
	case waitErr != nil && socketPath != "":
		return fmt.Errorf("app-server exited before %s readiness (socket=%s): %w", mode, socketPath, waitErr)
	case waitErr != nil:
		return fmt.Errorf("app-server exited before %s readiness (%w)", mode, waitErr)
	case socketPath != "":
		return fmt.Errorf("app-server exited before %s readiness (socket=%s)", mode, socketPath)
	default:
		return fmt.Errorf("app-server exited before %s readiness", mode)
	}
}

// appServerNotReadyError describes an endpoint that never became reachable
// within the attempt budget.
func appServerNotReadyError(mode, socketPath string, attemptLimit int) error {
	if socketPath != "" {
		return fmt.Errorf("app-server %s not ready after %d attempts (socket=%s)", mode, attemptLimit, socketPath)
	}
	return fmt.Errorf("app-server %s not ready after %d attempts", mode, attemptLimit)
}

// dialUnixWebSocket dials the app-server websocket over a unix domain socket.
func dialUnixWebSocket(ctx context.Context, socketPath string) (*websocketTransport, error) {
	var netDialer net.Dialer
	dialer := gows.Dialer{
		NetDial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return netDialer.DialContext(ctx, "unix", socketPath)
		},
	}
	raw, hs, err := dialer.Dial(ctx, "ws://localhost/")
	if err != nil {
		return nil, websocketDialError(fmt.Sprintf("dial unix websocket %q", socketPath), err, transportRedactor{})
	}
	return newWebsocketTransport(raw, &hs), nil
}

func dialWebSocket(ctx context.Context, listen string, cfg *WebSocketConfig, env map[string]string, cwd string) (*websocketTransport, error) {
	mode, socketPath, err := websocketListenMode(listen, env, cwd)
	if err != nil {
		return nil, err
	}
	if mode == "unix websocket" {
		return dialUnixWebSocket(ctx, socketPath)
	}
	u, err := url.Parse(listen)
	if err != nil {
		return nil, err
	}
	token, err := websocketBearerToken(cfg)
	if err != nil {
		return nil, err
	}
	return dialWebSocketURL(ctx, u, token, http.ProxyFromEnvironment, nil)
}

func dialWebSocketURL(ctx context.Context, u *url.URL, token string, proxy func(*http.Request) (*url.URL, error), tlsConfig *tls.Config) (*websocketTransport, error) {
	redactor := newTransportRedactor(u, token)
	wrappedProxy := proxy
	if proxy != nil {
		wrappedProxy = func(req *http.Request) (*url.URL, error) {
			proxyURL, err := proxy(req)
			redactor.addURL(proxyURL)
			return proxyURL, err
		}
	}
	dialer := gows.Dialer{
		TLSConfig:     tlsConfig,
		Proxy:         wrappedProxy,
		CheckRedirect: func(*http.Request, []*http.Request) error { return nil },
	}
	if token != "" {
		dialer.HTTPHeader = http.Header{
			"Authorization": {fmt.Sprintf("Bearer %s", token)},
		}
	}
	raw, hs, err := dialer.Dial(ctx, u.String())
	if err != nil {
		return nil, websocketDialError("websocket dial failed", err, redactor)
	}
	return newWebsocketTransport(raw, &hs, redactor), nil
}

func websocketDialError(prefix string, err error, redactor transportRedactor) error {
	if statusErr, ok := errors.AsType[*gows.UnexpectedStatusError](err); ok {
		sanitized := &gows.UnexpectedStatusError{StatusCode: statusErr.StatusCode, Reason: redactor.sanitize(statusErr.Reason)}
		if errors.Is(err, gows.ErrProxyConnectFailed) {
			return fmt.Errorf("%s: HTTP status %d %s: %w: %w", prefix, statusErr.StatusCode, http.StatusText(statusErr.StatusCode), gows.ErrProxyConnectFailed, sanitized)
		}
		return fmt.Errorf("%s: HTTP status %d %s: %w", prefix, statusErr.StatusCode, http.StatusText(statusErr.StatusCode), sanitized)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s: %w", prefix, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", prefix, context.DeadlineExceeded)
	}
	for _, sentinel := range []error{
		gows.ErrProxyConnectFailed,
		gows.ErrProxyUnsupportedScheme,
		gows.ErrTooManyRedirects,
		gows.ErrMalformedLocation,
		gows.ErrReservedHeader,
		gows.ErrMalformedHeader,
	} {
		if errors.Is(err, sentinel) {
			return fmt.Errorf("%s: %w", prefix, sentinel)
		}
	}
	return fmt.Errorf("%s: %s", prefix, redactor.sanitize(err.Error()))
}

func websocketBearerToken(cfg *WebSocketConfig) (string, error) {
	if cfg == nil {
		return "", nil
	}
	if cfg.ClientBearerToken != "" {
		return strings.TrimSpace(cfg.ClientBearerToken), nil
	}
	if cfg.ClientBearerTokenFile != "" {
		raw, err := os.ReadFile(cfg.ClientBearerTokenFile)
		if err != nil {
			return "", fmt.Errorf("cannot read websocket client bearer token file: %w", err)
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return "", fmt.Errorf("websocket client bearer token is empty")
		}
		return token, nil
	}
	if cfg.AuthMode == WebSocketAuthCapabilityToken && cfg.TokenFile != "" {
		raw, err := os.ReadFile(cfg.TokenFile)
		if err != nil {
			return "", fmt.Errorf("cannot read websocket token file: %w", err)
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return "", fmt.Errorf("websocket capability token is empty")
		}
		return token, nil
	}
	if cfg.AuthMode == WebSocketAuthSignedBearerToken {
		return "", fmt.Errorf("websocket signed-bearer-token mode requires a client bearer source")
	}
	return "", nil
}

func validateTokenFileHasContent(path, what string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", what, err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return fmt.Errorf("%s is empty", what)
	}
	return nil
}
