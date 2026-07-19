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
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

const transportHelperEnv = "PANDAEMONIUM_CODEX_TEST_HELPER_PROCESS"

func TestClientStartWebSocketHelperProcessRoundTrip(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	codexBin := writeHelperCodexShim(t, exe)
	port := reserveLoopbackPort(t)
	tokenFile := writeTempFile(t, "capability-token\n")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	client := NewClient(&Config{
		CodexBin: codexBin,
		Env: map[string]string{
			transportHelperEnv:              "1",
			"CODEX_PORT_HELPER_SCENARIO":    "websocket_roundtrip",
			"CODEX_WEBSOCKET_LISTEN_PORT":   port,
			"CODEX_WEBSOCKET_EXPECT_BEARER": "capability-token",
		},
		Listen: ListenConfig{
			URL: "ws://127.0.0.1:" + port,
			WebSocket: &WebSocketConfig{
				AuthMode:    WebSocketAuthCapabilityToken,
				TokenFile:   tokenFile,
				DialTimeout: 100 * time.Millisecond,
			},
		},
	}, nil)
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Client.Start() websocket error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Client.Close() websocket error = %v", err)
		}
	})

	if _, err := client.Initialize(ctx); err != nil {
		t.Fatalf("Client.Initialize() websocket error = %v", err)
	}
	raw, err := client.RequestRaw(ctx, "helper/echo", Object{"hello": "world"})
	if err != nil {
		t.Fatalf("RequestRaw(helper/echo) websocket error = %v", err)
	}
	if !strings.Contains(string(raw), `"ok":true`) {
		t.Fatalf("RequestRaw(helper/echo) = %s, want ok true", raw)
	}
	notification, err := client.NextNotification(ctx)
	if err != nil {
		t.Fatalf("NextNotification() websocket error = %v", err)
	}
	if notification.Method != "custom/global" {
		t.Fatalf("NextNotification().Method = %q, want custom/global", notification.Method)
	}
}

func TestClientStartWebSocketHelperProcessExitDoesNotHang(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	codexBin := writeHelperCodexShim(t, exe)
	port := reserveLoopbackPort(t)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	t.Cleanup(cancel)
	client := NewClient(&Config{
		CodexBin: codexBin,
		Env: map[string]string{
			transportHelperEnv:           "1",
			"CODEX_PORT_HELPER_SCENARIO": "exit_without_websocket",
		},
		Listen: ListenConfig{
			URL: "ws://127.0.0.1:" + port,
			WebSocket: &WebSocketConfig{
				AuthMode: WebSocketAuthNone,
			},
		},
	}, nil)

	result := make(chan error, 1)
	go func() {
		result <- client.Start(ctx)
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Client.Start() error = nil, want websocket readiness failure")
		}
		if !strings.Contains(err.Error(), "app-server exited before websocket readiness") {
			t.Fatalf("Client.Start() error = %v, want websocket readiness context", err)
		}
	case <-ctx.Done():
		t.Fatalf("Client.Start() did not return before context deadline: %v", ctx.Err())
	}
}

func TestCodexTransportHelperProcess(t *testing.T) {
	if os.Getenv(transportHelperEnv) != "1" {
		return
	}
	// A `--remote=<endpoint>` argument is only ever injected for a codex
	// attachment, never for the app-server child, so it selects the attachment
	// probe regardless of the shared CODEX_PORT_HELPER_SCENARIO that configures
	// the app-server role of the same shim.
	if remote, ok := helperRemoteFlag(); ok {
		runTransportHelperRemoteAttachProbe(remote)
	}
	switch os.Getenv("CODEX_PORT_HELPER_SCENARIO") {
	case "websocket_roundtrip":
		runTransportHelperWebSocket(t)
	case "unix_websocket_roundtrip":
		runTransportHelperUnixWebSocket(t)
	case "exit_without_websocket":
		fmt.Fprintln(os.Stderr, "helper exited before websocket readiness")
		os.Exit(7)
	default:
		runTransportHelperStdio()
	}
}

// helperRemoteFlag returns the --remote=<endpoint> argument the launcher
// injected for a codex attachment, if present.
func helperRemoteFlag() (string, bool) {
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "--remote=") {
			return arg, true
		}
	}
	return "", false
}

// runTransportHelperRemoteAttachProbe emulates a `codex --remote=<endpoint>`
// attachment: it asserts the launcher injected exactly the expected --remote
// flag, dials the endpoint to prove it is reachable, then exits 0 (or idles on
// stdin when asked to stay alive). It backs the two-process hermetic flow for
// RemoteAppServer.StartCodex.
func runTransportHelperRemoteAttachProbe(remoteFlag string) {
	if expected := os.Getenv("CODEX_REMOTE_ATTACH_EXPECT_FLAG"); expected != "" && remoteFlag != expected {
		fmt.Fprintf(os.Stderr, "remote flag %q does not match expected %q\n", remoteFlag, expected)
		os.Exit(2)
	}
	endpoint := strings.TrimPrefix(remoteFlag, "--remote=")
	network, address := remoteAttachDialTarget(endpoint)
	conn, err := net.Dial(network, address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial %s %s: %v\n", network, address, err)
		os.Exit(3)
	}
	_ = conn.Close()

	if sentinel := os.Getenv("CODEX_REMOTE_ATTACH_SENTINEL"); sentinel != "" {
		if err := os.WriteFile(sentinel, []byte("ok\n"), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write sentinel %s: %v\n", sentinel, err)
			os.Exit(4)
		}
	}

	if os.Getenv("CODEX_REMOTE_ATTACH_STAY_ALIVE") == "1" {
		// Block on stdin so the parent controls our lifetime through Close().
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
	os.Exit(0)
}

// remoteAttachDialTarget maps a launcher endpoint to a net.Dial target, mirroring
// the production probe so the attachment proves reachability the same way.
func remoteAttachDialTarget(endpoint string) (string, string) {
	if path, ok := strings.CutPrefix(endpoint, "unix://"); ok {
		return "unix", path
	}
	rest := strings.TrimPrefix(endpoint, "ws://")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return "tcp", rest
}

func runTransportHelperStdio() {
	scanner := bufio.NewScanner(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	defer func() { _ = w.Flush() }()
	for scanner.Scan() {
		var req rpcMessage
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		switch req.Method {
		case RequestMethodInitialize:
			enc := jsontext.NewEncoder(w)
			if os.Getenv("CODEX_EXEC_SERVER_HANDSHAKE") == "1" {
				json.MarshalEncode(enc, rpcMessage{
					ID: req.ID,
					Result: mustJSONValueForHelper(ExecServerInitializeResponse{
						SessionID: "session-1",
					}),
				})
			} else {
				json.MarshalEncode(enc, rpcMessage{
					ID: req.ID,
					Result: mustJSONValueForHelper(InitializeResponse{
						UserAgent:  "codex-bench/1.0",
						ServerInfo: &ServerInfo{Name: "codex-bench", Version: "1.0"},
					}),
				})
			}
			_ = w.Flush()
		case "initialized":
		case "helper/echo":
			enc := jsontext.NewEncoder(w)
			json.MarshalEncode(enc, rpcMessage{ID: req.ID, Result: mustJSONValueForHelper(Object{"ok": true})})
			_ = w.Flush()
		default:
			enc := jsontext.NewEncoder(w)
			json.MarshalEncode(enc, rpcMessage{ID: req.ID, Error: &rpcErrorBody{Code: -32601, Message: "unexpected method"}})
			_ = w.Flush()
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func runTransportHelperWebSocket(t *testing.T) {
	t.Helper()
	port := os.Getenv("CODEX_WEBSOCKET_LISTEN_PORT")
	if port == "" {
		port = "49815"
	}
	expectedBearer := os.Getenv("CODEX_WEBSOCKET_EXPECT_BEARER")
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if startupLog := os.Getenv("CODEX_WEBSOCKET_STARTUP_LOG"); startupLog != "" {
		fmt.Fprintln(os.Stderr, startupLog)
	}
	serveTrackedTransportHelper(t, ln, expectedBearer)
}

func mustJSONValueForHelper(value any) jsontext.Value {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return jsontext.Value(data)
}

func reserveLoopbackPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen(127.0.0.1:0) error = %v", err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("net.SplitHostPort(%q) error = %v", ln.Addr().String(), err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatalf("reserved port %q is not numeric: %v", port, err)
	}
	return port
}

func writeHelperCodexShim(t *testing.T, exe string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex-helper")
	body := "#!/bin/sh\nexec " + strconv.Quote(exe) + " -test.run=TestCodexTransportHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", path, err)
	}
	return path
}
