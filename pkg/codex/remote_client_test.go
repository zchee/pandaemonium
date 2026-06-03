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
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-json-experiment/json"
)

func TestNewRemoteClientWebSocketRoundTripAndClose(t *testing.T) {
	srv, wsURL := newWebSocketRoundTripServer(t, "Bearer remote-token")
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	client, err := NewRemoteClient(ctx, &RemoteConfig{
		URL:         wsURL,
		BearerToken: "remote-token",
	}, nil)
	if err != nil {
		t.Fatalf("NewRemoteClient() error = %v", err)
	}
	if client.cmd != nil || client.cmdDone != nil {
		t.Fatalf("NewRemoteClient() launched a process: cmd=%#v cmdDone=%#v", client.cmd, client.cmdDone)
	}

	metadata, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Client.Initialize() remote websocket error = %v", err)
	}
	assertRemoteAppServerMetadata(t, metadata)

	raw, err := client.RequestRaw(ctx, "test/echo", Object{"message": "hello"})
	if err != nil {
		t.Fatalf("RequestRaw(test/echo) remote websocket error = %v", err)
	}
	var gotEcho map[string]any
	if err := json.Unmarshal(raw, &gotEcho); err != nil {
		t.Fatalf("json.Unmarshal(RequestRaw result) error = %v", err)
	}
	if gotEcho["echo"] != "hello" {
		t.Fatalf("RequestRaw() echo = %#v, want hello", gotEcho)
	}

	notification, err := client.NextNotification(ctx)
	if err != nil {
		t.Fatalf("NextNotification() remote websocket error = %v", err)
	}
	if notification.Method != "unknown/global" {
		t.Fatalf("NextNotification().Method = %q, want unknown/global", notification.Method)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Client.Close() remote websocket error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Client.Close() remote websocket error = %v", err)
	}
}

func TestNewRemoteCodexInitializesWithBearerTokenFile(t *testing.T) {
	tokenFile := writeTempFile(t, "file-token\n")
	srv, wsURL := newWebSocketRoundTripServer(t, "Bearer file-token")
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	sdk, err := NewRemoteCodex(ctx, &RemoteConfig{
		URL:             wsURL,
		BearerTokenFile: tokenFile,
	})
	if err != nil {
		t.Fatalf("NewRemoteCodex() error = %v", err)
	}
	t.Cleanup(func() {
		if err := sdk.Close(); err != nil {
			t.Fatalf("Codex.Close() remote websocket error = %v", err)
		}
	})

	assertRemoteAppServerMetadata(t, sdk.Metadata())
	if sdk.Client().cmd != nil || sdk.Client().cmdDone != nil {
		t.Fatalf("NewRemoteCodex() launched a process: cmd=%#v cmdDone=%#v", sdk.Client().cmd, sdk.Client().cmdDone)
	}

	notification, err := sdk.Client().NextNotification(ctx)
	if err != nil {
		t.Fatalf("NextNotification() remote websocket error = %v", err)
	}
	if notification.Method != "unknown/global" {
		t.Fatalf("NextNotification().Method = %q, want unknown/global", notification.Method)
	}
}

func TestNewRemoteClientUnixWebSocketOmitsAuthorization(t *testing.T) {
	skipIfUnixSocketsUnsupported(t)

	listenURL := newUnixWebSocketRoundTripServer(t, "")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	client, err := NewRemoteClient(ctx, &RemoteConfig{URL: listenURL}, nil)
	if err != nil {
		t.Fatalf("NewRemoteClient(%q) error = %v", listenURL, err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Client.Close() unix remote websocket error = %v", err)
		}
	})
	if client.cmd != nil || client.cmdDone != nil {
		t.Fatalf("NewRemoteClient(unix) launched a process: cmd=%#v cmdDone=%#v", client.cmd, client.cmdDone)
	}

	metadata, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Client.Initialize() unix remote websocket error = %v", err)
	}
	assertRemoteAppServerMetadata(t, metadata)

	notification, err := client.NextNotification(ctx)
	if err != nil {
		t.Fatalf("NextNotification() unix remote websocket error = %v", err)
	}
	if notification.Method != "unknown/global" {
		t.Fatalf("NextNotification().Method = %q, want unknown/global", notification.Method)
	}
}

func TestNewRemoteClientRejectsUnsafeBearerPlaintextWebSocket(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)

	secret := "secret-token-should-not-leak"
	_, err := NewRemoteClient(ctx, &RemoteConfig{
		URL:         "ws://codex.example.test:49815",
		BearerToken: secret,
	}, nil)
	if err == nil {
		t.Fatal("NewRemoteClient() error = nil, want unsafe bearer rejection")
	}
	if !strings.Contains(err.Error(), "refusing bearer auth over non-loopback ws://") {
		t.Fatalf("NewRemoteClient() error = %v, want non-loopback bearer rejection", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("NewRemoteClient() error leaks bearer token")
	}

	for name, cfg := range map[string]RemoteConfig{
		"success: loopback ws bearer": {
			URL:         "ws://127.0.0.1:49815",
			BearerToken: secret,
		},
		"success: wss bearer": {
			URL:         "wss://codex.example.test/app-server",
			BearerToken: secret,
		},
		"success: explicit unsafe opt-in": {
			URL:                          "ws://codex.example.test:49815",
			BearerToken:                  secret,
			AllowInsecureRemoteWebSocket: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateRemoteConfig(cfg); err != nil {
				t.Fatalf("validateRemoteConfig() error = %v", err)
			}
		})
	}
}

func TestNewRemoteClientRejectsUnixBearerAuth(t *testing.T) {
	skipIfUnixSocketsUnsupported(t)

	secret := "secret-token-should-not-leak"
	_, err := NewRemoteClient(t.Context(), &RemoteConfig{
		URL:         "unix:///tmp/codex.sock",
		BearerToken: secret,
	}, nil)
	if err == nil {
		t.Fatal("NewRemoteClient() error = nil, want unix bearer rejection")
	}
	if !strings.Contains(err.Error(), "do not support bearer auth") {
		t.Fatalf("NewRemoteClient() error = %v, want unix bearer rejection", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("NewRemoteClient() error leaks bearer token")
	}
}

func TestRemoteClientCloseUnblocksPendingRequest(t *testing.T) {
	wsURL, requestReceived := newRemotePendingWebSocketServer(t, "Bearer close-token", "never/respond")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	client, err := NewRemoteClient(ctx, &RemoteConfig{
		URL:         wsURL,
		BearerToken: "close-token",
	}, nil)
	if err != nil {
		t.Fatalf("NewRemoteClient() error = %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := client.RequestRaw(ctx, "never/respond", nil)
		result <- err
	}()

	select {
	case <-requestReceived:
	case <-ctx.Done():
		t.Fatalf("server did not receive pending request before context deadline: %v", ctx.Err())
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Client.Close() remote pending request error = %v", err)
	}

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("RequestRaw() error = nil, want transport closed")
		}
		var closedErr *TransportClosedError
		if !errors.As(err, &closedErr) {
			t.Fatalf("RequestRaw() error = %T %v, want *TransportClosedError", err, err)
		}
	case <-ctx.Done():
		t.Fatalf("RequestRaw() did not unblock before context deadline: %v", ctx.Err())
	}
}

func assertRemoteAppServerMetadata(t *testing.T, metadata InitializeResponse) {
	t.Helper()
	if metadata.ServerInfo == nil || metadata.ServerInfo.Name != "ws-test" || metadata.ServerInfo.Version != "1.2.3" {
		t.Fatalf("Initialize() metadata = %#v, want ws-test 1.2.3", metadata)
	}
}

func newRemotePendingWebSocketServer(t *testing.T, expectedAuth, pendingMethod string) (string, <-chan struct{}) {
	t.Helper()

	requestReceived := make(chan struct{})
	var closeRequestReceived sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != expectedAuth {
			t.Errorf("Authorization header = %q, want %q", got, expectedAuth)
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket.Accept() error = %v", err)
			return
		}
		go func() {
			defer conn.Close(websocket.StatusNormalClosure, "")
			for {
				typ, payload, err := conn.Read(context.Background())
				if err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) || websocket.CloseStatus(err) == websocket.StatusNormalClosure {
						return
					}
					t.Errorf("websocket Read() error = %v", err)
					return
				}
				if typ != websocket.MessageText {
					continue
				}
				var msg rpcMessage
				if err := json.Unmarshal(payload, &msg); err != nil {
					t.Errorf("json.Unmarshal websocket payload error = %v", err)
					return
				}
				if msg.Method == pendingMethod {
					closeRequestReceived.Do(func() { close(requestReceived) })
					continue
				}
				t.Errorf("unexpected websocket method %q", msg.Method)
				return
			}
		}()
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), requestReceived
}
