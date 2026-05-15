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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Transport reperesents a bidirectional JSON message Transport between the client and the app-server.
type Transport interface {
	io.Closer
	WriteJSON(context.Context, []byte) error
	ReadJSON(context.Context) ([]byte, error)
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
	if t.stdin == nil {
		return &TransportClosedError{Message: "app-server is not running"}
	}

	data = append(slices.Clone(data), '\n')
	_, err := t.stdin.Write(data)
	if err != nil {
		return &TransportClosedError{Message: err.Error()}
	}
	return nil
}

// ReadJSON implements [Transport].
func (t *stdioTransport) ReadJSON(ctx context.Context) ([]byte, error) {
	if t.stdout == nil {
		return nil, &TransportClosedError{Message: "app-server is not running"}
	}
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := t.stdout.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				done <- result{err: io.EOF}
				return
			}
			done <- result{err: err}
			return
		}
		done <- result{data: line}
	}()

	// The goroutine above is orphaned when ctx is cancelled; it exits naturally
	// when [stdioTransport.Close] closes stdoutCloser (the raw stdout pipe), which causes
	// ReadBytes to return an error.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		return r.data, r.err
	}
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
		if cfg.TokenFile != "" || cfg.TokenSHA256 != "" || cfg.SharedSecretFile != "" || cfg.Issuer != "" || cfg.Audience != "" || cfg.ClientBearerToken != "" || cfg.ClientBearerTokenFile != "" || cfg.MaxClockSkewSeconds != nil {
			return fmt.Errorf("ws-auth none cannot include websocket auth fields")
		}
		return nil
	}
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
	if cfg.AuthMode == WebSocketAuthCapabilityToken && cfg.TokenSHA256 != "" && !tokenSource {
		return fmt.Errorf("capability-token auth with --ws-token-sha256 requires a client bearer token source")
	}
	if cfg.AuthMode == WebSocketAuthCapabilityToken && cfg.TokenFile == "" && cfg.TokenSHA256 == "" {
		return fmt.Errorf("capability-token auth requires either ws token file or ws-token-sha256")
	}
	if cfg.AuthMode == WebSocketAuthCapabilityToken && cfg.TokenFile != "" {
		if err := validateTokenFileHasContent(cfg.TokenFile, "websocket token file"); err != nil {
			return err
		}
	}
	if cfg.AuthMode == WebSocketAuthSignedBearerToken && cfg.SharedSecretFile == "" {
		return fmt.Errorf("signed-bearer-token auth requires --ws-shared-secret-file")
	}
	if cfg.AuthMode == WebSocketAuthSignedBearerToken && !tokenSource {
		return fmt.Errorf("signed-bearer-token auth requires a client bearer token source")
	}
	if cfg.AuthMode == WebSocketAuthSignedBearerToken && cfg.SharedSecretFile != "" {
		if err := validateTokenFileHasContent(cfg.SharedSecretFile, "websocket shared secret file"); err != nil {
			return err
		}
	}
	if cfg.AuthMode == WebSocketAuthSignedBearerToken && cfg.Issuer == "" {
		// issuer is optional in app-server CLI; keep behavior permissive.
	}
	if cfg.Issuer != "" && strings.TrimSpace(cfg.Issuer) == "" {
		return fmt.Errorf("ws-issuer cannot be empty if set")
	}
	if cfg.Audience != "" && strings.TrimSpace(cfg.Audience) == "" {
		return fmt.Errorf("ws-audience cannot be empty if set")
	}
	return nil
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

func dialWebSocketWithWait(ctx context.Context, procDone <-chan error, listen string, cfg *WebSocketConfig) (*websocket.Conn, error) {
	attemptLimit := 50
	for attempt := range attemptLimit {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-procDone:
			if err != nil {
				return nil, fmt.Errorf("app-server exited before websocket readiness (%w)", err)
			}
			return nil, errors.New("app-server exited before websocket readiness")
		default:
		}
		dialCtx := ctx
		var cancel context.CancelFunc
		if cfg != nil && cfg.DialTimeout > 0 {
			dialCtx, cancel = context.WithTimeout(ctx, cfg.DialTimeout)
		}
		conn, err := dialWebSocket(dialCtx, listen, cfg)
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
		case err := <-procDone:
			if err != nil {
				return nil, fmt.Errorf("app-server exited before websocket readiness (%w)", err)
			}
			return nil, errors.New("app-server exited before websocket readiness")
		}
	}
	return nil, fmt.Errorf("app-server websocket not ready after %d attempts", attemptLimit)
}

func dialWebSocket(ctx context.Context, listen string, cfg *WebSocketConfig) (*websocket.Conn, error) {
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
		return nil, err
	}
	if resp == nil {
		return conn, nil
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close(websocket.StatusProtocolError, resp.Status)
		return nil, fmt.Errorf("websocket dial failed: %s", resp.Status)
	}
	return conn, nil
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
