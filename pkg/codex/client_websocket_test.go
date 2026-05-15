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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

func TestClientWebSocketTransportRoundTripAndRouting(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	tests := []struct {
		name         string
		wsConfig     *WebSocketConfig
		expectedAuth string
	}{
		{
			name: "capability token file",
			wsConfig: &WebSocketConfig{
				AuthMode:  WebSocketAuthCapabilityToken,
				TokenFile: writeTempFile(t, "capability-token\n"),
			},
			expectedAuth: "Bearer capability-token",
		},
		{
			name: "signed bearer token",
			wsConfig: &WebSocketConfig{
				AuthMode:          WebSocketAuthSignedBearerToken,
				SharedSecretFile:  writeTempFile(t, "shared-secret\n"),
				ClientBearerToken: "signed-token",
				Issuer:            "issuer-a",
				Audience:          "audience-a",
			},
			expectedAuth: "Bearer signed-token",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, wsURL := newWebSocketRoundTripServer(t, tc.expectedAuth)
			t.Cleanup(srv.Close)

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)

			client := NewClient(&Config{
				CodexBin: exe,
				Listen: ListenConfig{
					URL:       wsURL,
					WebSocket: tc.wsConfig,
				},
			}, nil)

			conn, err := dialWebSocket(ctx, wsURL, tc.wsConfig)
			if err != nil {
				t.Fatalf("dialWebSocket() error = %v", err)
			}
			client.storeTransport(&websocketTransport{conn: conn})
			client.responses = map[string]chan responseWait{}
			client.turnRouter = newTurnNotificationRouter()
			client.stderrDone = make(chan struct{})
			close(client.stderrDone)
			client.readDone = make(chan struct{})
			go client.readLoop(ctx, client.loadTransport(), client.readDone)
			t.Cleanup(func() {
				if err := client.Close(); err != nil {
					t.Fatalf("Client.Close() error = %v", err)
				}
			})

			metadata, err := client.Initialize(ctx)
			if err != nil {
				t.Fatalf("Client.Initialize() error = %v", err)
			}
			if metadata.ServerInfo == nil || metadata.ServerInfo.Name != "ws-test" || metadata.ServerInfo.Version != "1.2.3" {
				t.Fatalf("Initialize() metadata = %#v, want ws-test 1.2.3", metadata)
			}

			raw, err := client.RequestRaw(ctx, "test/echo", Object{"message": "hello"})
			if err != nil {
				t.Fatalf("RequestRaw() error = %v", err)
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
				t.Fatalf("NextNotification() error = %v", err)
			}
			if notification.Method != "unknown/global" {
				t.Fatalf("NextNotification().Method = %q, want unknown/global", notification.Method)
			}
		})
	}
}

func newWebSocketRoundTripServer(t *testing.T, expectedAuth string) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != expectedAuth {
			t.Errorf("Authorization header = %q, want %q", got, expectedAuth)
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket.Accept() error = %v", err)
			return
		}
		go handleWebSocketRoundTrip(t, conn)
	}))
	return srv, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func handleWebSocketRoundTrip(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := context.Background()
	sentGlobalNotification := false
	for {
		typ, payload, err := conn.Read(ctx)
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
		if msg.Method == "" && msg.ID != "" {
			if msg.ID != "srv-approval" {
				continue
			}
			if got := string(msg.Result); !strings.Contains(got, `"decision":"accept"`) {
				t.Errorf("approval response = %s, want accept decision", got)
			}
			continue
		}
		switch msg.Method {
		case RequestMethodInitialize:
			if err := writeServerRPC(ctx, conn, rpcMessage{
				ID: msg.ID,
				Result: mustJSONValue(t, InitializeResponse{
					UserAgent:  "ws-test/1.2.3",
					ServerInfo: &ServerInfo{Name: "ws-test", Version: "1.2.3"},
				}),
			}); err != nil {
				t.Errorf("write initialize response error = %v", err)
				return
			}
		case "initialized":
			if sentGlobalNotification {
				continue
			}
			sentGlobalNotification = true
			if err := writeServerRPC(ctx, conn, rpcMessage{
				ID:     "srv-approval",
				Method: "item/commandExecution/requestApproval",
				Params: mustJSONValue(t, Object{"reason": "approve"}),
			}); err != nil {
				t.Errorf("write server request error = %v", err)
				return
			}
			if err := writeServerRPC(ctx, conn, rpcMessage{
				Method: "unknown/global",
				Params: mustJSONValue(t, Object{"scope": "global"}),
			}); err != nil {
				t.Errorf("write notification error = %v", err)
				return
			}
		case "test/echo":
			if err := writeServerRPC(ctx, conn, rpcMessage{
				ID:     msg.ID,
				Result: mustJSONValue(t, Object{"echo": "hello"}),
			}); err != nil {
				t.Errorf("write echo response error = %v", err)
				return
			}
		default:
			t.Errorf("unexpected websocket method %q", msg.Method)
			return
		}
	}
}

func writeServerRPC(ctx context.Context, conn *websocket.Conn, msg rpcMessage) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, payload)
}

func mustJSONValue(t *testing.T, value any) jsontext.Value {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%T) error = %v", value, err)
	}
	return jsontext.Value(payload)
}

func writeTempFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "value.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", path, err)
	}
	return path
}
