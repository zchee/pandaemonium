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
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-json-experiment/json"
)

func TestClientBuildAppServerArgsUnixWebSocket(t *testing.T) {
	customSocket := filepath.Join(shortTempDir(t), "custom.sock")

	tests := map[string]struct {
		listen ListenConfig
		want   []string
	}{
		"success: unix default preserves listen URL": {
			listen: ListenConfig{URL: "unix://"},
			want:   []string{os.Args[0], "app-server", "--listen", "unix://"},
		},
		"success: unix absolute path preserves listen URL and dial timeout only": {
			listen: ListenConfig{
				URL: "unix://" + customSocket,
				WebSocket: &WebSocketConfig{
					AuthMode:    WebSocketAuthNone,
					DialTimeout: 100 * time.Millisecond,
				},
			},
			want: []string{os.Args[0], "app-server", "--listen", "unix://" + customSocket},
		},
		"success: unix host-looking suffix remains literal": {
			listen: ListenConfig{URL: "unix://localhost/tmp/codex.sock"},
			want:   []string{os.Args[0], "app-server", "--listen", "unix://localhost/tmp/codex.sock"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client := &Client{config: Config{CodexBin: os.Args[0]}}
			args, err := client.buildAppServerArgs(tt.listen)
			if err != nil {
				t.Fatalf("buildAppServerArgs() error = %v", err)
			}
			if diff := compareStringSlice(args, tt.want); diff != "" {
				t.Fatalf("buildAppServerArgs() mismatch: %s\ngot:  %q\nwant: %q", diff, args, tt.want)
			}
		})
	}
}

func TestClientBuildAppServerArgsRejectsInvalidUnixWebSocketListenURL(t *testing.T) {
	tests := map[string]struct {
		listenURL string
		wantErr   string
	}{
		"error: opaque relative URL": {
			listenURL: "unix:relative.sock",
			wantErr:   "unix listen endpoints must use unix:// prefix",
		},
		"error: percent-encoded path": {
			listenURL: "unix://%2Ftmp%2Fcodex.sock",
			wantErr:   "percent-encoded unix socket paths are not supported",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client := &Client{config: Config{CodexBin: os.Args[0]}}
			_, err := client.buildAppServerArgs(ListenConfig{
				URL: tt.listenURL,
				WebSocket: &WebSocketConfig{
					AuthMode:          WebSocketAuthCapabilityToken,
					TokenSHA256:       strings.Repeat("a", 64),
					ClientBearerToken: "jwt-token-should-not-leak",
				},
			})
			if err == nil {
				t.Fatal("buildAppServerArgs() error = nil, want invalid unix listen URL rejection")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("buildAppServerArgs() error = %v, want contain %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "jwt-token-should-not-leak") {
				t.Fatal("error leaks client bearer token")
			}
		})
	}
}

func TestClientStartRejectsUnixWebSocketAuthWithLaunchArgsOverride(t *testing.T) {
	skipIfUnixSocketsUnsupported(t)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	client := NewClient(&Config{
		LaunchArgsOverride: []string{os.Args[0], "-test.run=TestCodexTransportHelperProcess"},
		Listen: ListenConfig{
			URL: "unix://" + filepath.Join(shortTempDir(t), "override.sock"),
			WebSocket: &WebSocketConfig{
				ClientBearerToken: "secret-token-should-not-leak",
			},
		},
	}, nil)

	err := client.Start(ctx)
	if err == nil {
		t.Fatal("Client.Start() error = nil, want unix websocket auth rejection")
	}
	if !strings.Contains(err.Error(), "unix websocket listen does not support websocket auth fields") {
		t.Fatalf("Client.Start() error = %v, want unix websocket auth rejection", err)
	}
	if strings.Contains(err.Error(), "secret-token-should-not-leak") {
		t.Fatal("Client.Start() error leaks bearer token")
	}
}

func TestDialWebSocketUnixRoundTripOmitsAuthorization(t *testing.T) {
	skipIfUnixSocketsUnsupported(t)

	listenURL := newUnixWebSocketRoundTripServer(t, "")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	client := NewClient(&Config{
		CodexBin: os.Args[0],
		Listen: ListenConfig{
			URL: listenURL,
			WebSocket: &WebSocketConfig{
				AuthMode: WebSocketAuthNone,
			},
		},
	}, nil)

	conn, err := dialWebSocket(ctx, listenURL, client.config.Listen.WebSocket, client.config.Env, client.config.Cwd)
	if err != nil {
		t.Fatalf("dialWebSocket(%q) error = %v", listenURL, err)
	}
	client.storeTransport(&websocketTransport{conn: conn})
	client.rpcState = newJSONRPCClientState()
	client.turnRouter = newTurnNotificationRouter()
	client.stderrDone = make(chan struct{})
	close(client.stderrDone)
	client.readDone = make(chan struct{})
	go client.readLoop(ctx, client.loadTransport(), client.readDone)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Client.Close() unix websocket error = %v", err)
		}
	})

	metadata, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Client.Initialize() unix websocket error = %v", err)
	}
	if metadata.ServerInfo == nil || metadata.ServerInfo.Name != "ws-test" || metadata.ServerInfo.Version != "1.2.3" {
		t.Fatalf("Initialize() metadata = %#v, want ws-test 1.2.3", metadata)
	}

	raw, err := client.RequestRaw(ctx, "test/echo", Object{"message": "hello"})
	if err != nil {
		t.Fatalf("RequestRaw(test/echo) unix websocket error = %v", err)
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
		t.Fatalf("NextNotification() unix websocket error = %v", err)
	}
	if notification.Method != "unknown/global" {
		t.Fatalf("NextNotification().Method = %q, want unknown/global", notification.Method)
	}
}

func TestDialWebSocketUnixFailureErrorIsActionableAndRedacted(t *testing.T) {
	skipIfUnixSocketsUnsupported(t)

	missingSocket := filepath.Join(shortTempDir(t), "missing.sock")
	listenURL := "unix://" + missingSocket
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	t.Cleanup(cancel)

	_, err := dialWebSocket(ctx, listenURL, &WebSocketConfig{
		ClientBearerToken: "secret-token-should-not-leak",
	}, nil, "")
	if err == nil {
		t.Fatal("dialWebSocket() error = nil, want missing unix socket failure")
	}
	for _, want := range []string{"unix websocket", missingSocket} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("dialWebSocket() error = %v, want contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "secret-token-should-not-leak") {
		t.Fatal("dialWebSocket() error leaks bearer token")
	}
}

func TestClientStartUnixWebSocketHelperProcessRoundTrip(t *testing.T) {
	skipIfUnixSocketsUnsupported(t)

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	codexBin := writeHelperCodexShim(t, exe)

	tests := map[string]struct {
		listenURL func(t *testing.T) (listenURL, socketPath string)
	}{
		"success: default unix socket path uses effective CODEX_HOME": {
			listenURL: func(t *testing.T) (string, string) {
				codexHome := shortTempDir(t)
				socketPath := filepath.Join(codexHome, "app-server-control", "app-server-control.sock")
				return "unix://", socketPath
			},
		},
		"success: custom absolute unix socket path": {
			listenURL: func(t *testing.T) (string, string) {
				socketPath := filepath.Join(shortTempDir(t), "custom.sock")
				return "unix://" + socketPath, socketPath
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			listenURL, socketPath := tt.listenURL(t)
			env := map[string]string{
				transportHelperEnv:                   "1",
				"CODEX_PORT_HELPER_SCENARIO":         "unix_websocket_roundtrip",
				"CODEX_UNIX_WEBSOCKET_LISTEN_PATH":   socketPath,
				"CODEX_UNIX_WEBSOCKET_EXPECT_LISTEN": listenURL,
			}
			if listenURL == "unix://" {
				env["CODEX_HOME"] = filepath.Dir(filepath.Dir(socketPath))
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)
			client := NewClient(&Config{
				CodexBin: codexBin,
				Cwd:      shortTempDir(t),
				Env:      env,
				Listen: ListenConfig{
					URL: listenURL,
					WebSocket: &WebSocketConfig{
						AuthMode:    WebSocketAuthNone,
						DialTimeout: 100 * time.Millisecond,
					},
				},
			}, nil)
			if err := client.Start(ctx); err != nil {
				t.Fatalf("Client.Start() unix websocket error = %v", err)
			}
			t.Cleanup(func() {
				if err := client.Close(); err != nil {
					t.Fatalf("Client.Close() unix websocket error = %v", err)
				}
			})

			if _, err := client.Initialize(ctx); err != nil {
				t.Fatalf("Client.Initialize() unix websocket error = %v", err)
			}
			raw, err := client.RequestRaw(ctx, "helper/echo", Object{"hello": "world"})
			if err != nil {
				t.Fatalf("RequestRaw(helper/echo) unix websocket error = %v", err)
			}
			if !strings.Contains(string(raw), `"ok":true`) {
				t.Fatalf("RequestRaw(helper/echo) = %s, want ok true", raw)
			}
			notification, err := client.NextNotification(ctx)
			if err != nil {
				t.Fatalf("NextNotification() unix websocket error = %v", err)
			}
			if notification.Method != "custom/global" {
				t.Fatalf("NextNotification().Method = %q, want custom/global", notification.Method)
			}
		})
	}
}

func TestClientStartUnixWebSocketHelperProcessExitDoesNotHang(t *testing.T) {
	skipIfUnixSocketsUnsupported(t)

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	codexBin := writeHelperCodexShim(t, exe)

	tests := map[string]struct {
		listenURL string
	}{
		"error: helper exits before custom unix socket readiness": {
			listenURL: "unix://" + filepath.Join(shortTempDir(t), "never-created.sock"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			t.Cleanup(cancel)
			client := NewClient(&Config{
				CodexBin: codexBin,
				Env: map[string]string{
					transportHelperEnv:           "1",
					"CODEX_PORT_HELPER_SCENARIO": "exit_without_websocket",
				},
				Listen: ListenConfig{
					URL:       tt.listenURL,
					WebSocket: &WebSocketConfig{AuthMode: WebSocketAuthNone},
				},
			}, nil)

			result := make(chan error, 1)
			go func() {
				result <- client.Start(ctx)
			}()
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("Client.Start() error = nil, want unix websocket readiness failure")
				}
				if !strings.Contains(err.Error(), "app-server exited before unix websocket readiness") {
					t.Fatalf("Client.Start() error = %v, want unix websocket readiness context", err)
				}
			case <-ctx.Done():
				t.Fatalf("Client.Start() did not return before context deadline: %v", ctx.Err())
			}
		})
	}
}

func newUnixWebSocketRoundTripServer(t *testing.T, expectedAuth string) string {
	t.Helper()

	socketPath := filepath.Join(shortTempDir(t), "roundtrip.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("net.Listen(unix, %s) error = %v", socketPath, err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != expectedAuth {
			t.Errorf("Authorization header = %q, want %q", got, expectedAuth)
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket.Accept() unix error = %v", err)
			return
		}
		go handleWebSocketRoundTrip(t, conn)
	})}
	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("unix websocket server error = %v", err)
		}
	})
	return "unix://" + socketPath
}

func runTransportHelperUnixWebSocket() {
	if expectedListen := os.Getenv("CODEX_UNIX_WEBSOCKET_EXPECT_LISTEN"); expectedListen != "" && !helperArgsContainListen(expectedListen) {
		fmt.Fprintf(os.Stderr, "missing --listen %s in helper args\n", expectedListen)
		os.Exit(2)
	}
	socketPath := os.Getenv("CODEX_UNIX_WEBSOCKET_LISTEN_PATH")
	if socketPath == "" {
		fmt.Fprintln(os.Stderr, "missing CODEX_UNIX_WEBSOCKET_LISTEN_PATH")
		os.Exit(2)
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer os.Remove(socketPath)

	expectedBearer := os.Getenv("CODEX_WEBSOCKET_EXPECT_BEARER")
	srv := &http.Server{Handler: newTransportHelperWebSocketHandler(expectedBearer)}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func newTransportHelperWebSocketHandler(expectedBearer string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if expectedBearer != "" && r.Header.Get("Authorization") != "Bearer "+expectedBearer {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		for {
			typ, data, err := conn.Read(context.Background())
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) || websocket.CloseStatus(err) == websocket.StatusNormalClosure {
					return
				}
				fmt.Fprintln(os.Stderr, err)
				return
			}
			if typ != websocket.MessageText {
				continue
			}
			var req rpcMessage
			if err := json.Unmarshal(data, &req); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}
			switch req.Method {
			case RequestMethodInitialize:
				_ = conn.Write(context.Background(), websocket.MessageText, mustJSONValueForHelper(rpcMessage{
					ID: req.ID,
					Result: mustJSONValueForHelper(InitializeResponse{
						UserAgent:  "codex-bench/1.0",
						ServerInfo: &ServerInfo{Name: "codex-bench", Version: "1.0"},
					}),
				}))
			case "initialized":
				_ = conn.Write(context.Background(), websocket.MessageText, mustJSONValueForHelper(rpcMessage{
					Method: "custom/global",
					Params: mustJSONValueForHelper(Object{"scope": "global"}),
				}))
			case "helper/echo":
				_ = conn.Write(context.Background(), websocket.MessageText, mustJSONValueForHelper(rpcMessage{
					ID:     req.ID,
					Result: mustJSONValueForHelper(Object{"ok": true}),
				}))
			default:
				_ = conn.Write(context.Background(), websocket.MessageText, mustJSONValueForHelper(rpcMessage{
					ID:    req.ID,
					Error: &rpcErrorBody{Code: -32601, Message: "unexpected method"},
				}))
			}
		}
	})
}

func helperArgsContainListen(expectedListen string) bool {
	for i := 0; i < len(os.Args)-1; i++ {
		if os.Args[i] == "--listen" && os.Args[i+1] == expectedListen {
			return true
		}
	}
	return false
}

func skipIfUnixSocketsUnsupported(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix-domain sockets are not supported on windows")
	}
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	base := os.TempDir()
	if runtime.GOOS != "windows" {
		if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
			base = "/tmp"
		}
	}
	dir, err := os.MkdirTemp(base, "pdx-cx-")
	if err != nil {
		t.Fatalf("os.MkdirTemp(%s) error = %v", base, err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("os.RemoveAll(%s) error = %v", dir, err)
		}
	})
	return dir
}
