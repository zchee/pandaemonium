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
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	llm "github.com/zchee/pandaemonium/pkg/llm"
)

// Transport represents a bidirectional JSON message Transport between the client and the app-server.
type Transport interface {
	io.Closer
	WriteJSON(ctx context.Context, data []byte) error
	ReadJSON(ctx context.Context) ([]byte, error)
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
	// The goroutine in ReadJSONLineContext is orphaned when ctx is cancelled; it
	// exits naturally when [stdioTransport.Close] closes stdoutCloser (the raw
	// stdout pipe), which causes ReadBytes to return an error.
	return llm.ReadJSONLineContext(
		ctx,
		t.stdout,
		func() error { return &TransportClosedError{Message: "app-server is not running"} },
	)
}

// websocketTransport represents a bidirectional JSON message transport over a websocket connection to the app-server.
type websocketTransport struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

var _ Transport = (*websocketTransport)(nil)

// Close implements [Transport].
func (t *websocketTransport) Close() error {
	if t.conn != nil {
		return t.conn.Close(websocket.StatusNormalClosure, "")
	}
	return nil
}

// WriteJSON implements [Transport].
func (t *websocketTransport) WriteJSON(ctx context.Context, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn == nil {
		return &TransportClosedError{Message: "app-server is not running"}
	}
	return t.conn.Write(ctx, websocket.MessageText, data)
}

// ReadJSON implements [Transport].
func (t *websocketTransport) ReadJSON(ctx context.Context) ([]byte, error) {
	for {
		if t.conn == nil {
			return nil, &TransportClosedError{Message: "app-server is not running"}
		}

		typ, payload, err := t.conn.Read(ctx)
		if err != nil {
			if status := websocket.CloseStatus(err); status != websocket.StatusNormalClosure {
				return nil, &TransportClosedError{Message: err.Error()}
			}
			return nil, io.EOF
		}

		switch typ {
		case websocket.MessageText:
			return append(payload, '\n'), nil
		case websocket.MessageBinary:
			return nil, &AppServerError{Message: "unexpected binary websocket message"}
		}
	}
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

func newUnixWebSocketHTTPClient(socketPath string) *http.Client {
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{Transport: transport}
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

func dialWebSocketWithWait(ctx context.Context, procDone <-chan error, listen string, cfg *WebSocketConfig, env map[string]string, cwd string) (*websocket.Conn, error) {
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
func dialUnixWebSocket(ctx context.Context, socketPath string) (*websocket.Conn, error) {
	httpClient := newUnixWebSocketHTTPClient(socketPath)
	if transport, ok := httpClient.Transport.(*http.Transport); ok {
		defer transport.CloseIdleConnections()
	}
	opts := &websocket.DialOptions{}
	opts.HTTPClient = httpClient
	conn, resp, err := websocket.Dial(ctx, "ws://localhost/", opts)
	if err != nil {
		return nil, websocketDialError(fmt.Sprintf("dial unix websocket %q", socketPath), resp, err)
	}
	if resp == nil {
		return conn, nil
	}
	// A successful websocket upgrade hijacks the connection, so resp.Body can be
	// nil; guard the close to avoid a nil dereference on the 101 handshake path.
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close(websocket.StatusProtocolError, resp.Status)
		return nil, fmt.Errorf("dial unix websocket %q failed: %s", socketPath, resp.Status)
	}
	return conn, nil
}

func dialWebSocket(ctx context.Context, listen string, cfg *WebSocketConfig, env map[string]string, cwd string) (*websocket.Conn, error) {
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
	opts := &websocket.DialOptions{}
	if token != "" {
		opts.HTTPHeader = http.Header{
			"Authorization": {fmt.Sprintf("Bearer %s", token)},
		}
	}
	conn, resp, err := websocket.Dial(ctx, u.String(), opts)
	if err != nil {
		return nil, websocketDialError("websocket dial failed", resp, err)
	}
	if resp == nil {
		return conn, nil
	}
	// A successful websocket upgrade hijacks the connection, so resp.Body can be
	// nil; guard the close to avoid a nil dereference on the 101 handshake path.
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close(websocket.StatusProtocolError, resp.Status)
		return nil, fmt.Errorf("websocket dial failed: %s", resp.Status)
	}
	return conn, nil
}

func websocketDialError(prefix string, resp *http.Response, err error) error {
	if resp == nil {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	return fmt.Errorf("%s: %s: %w", prefix, resp.Status, err)
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
