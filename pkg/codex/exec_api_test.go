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
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewExecServerStartsExecServerAndPerformsInitializeHandshake(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	codexBin := writeHelperCodexShim(t, exe)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	// Exercise real command start semantics with script-backed helper and the exec-server
	// initialization path that uses initialize/initialized for the exec protocol.
	client, err := NewExecServer(ctx, &Config{
		CodexBin: codexBin,
		Env: map[string]string{
			transportHelperEnv:            "1",
			"CODEX_EXEC_SERVER_HANDSHAKE": "1",
		},
		ServerMode: ServerModeExecServer,
	})
	if err != nil {
		t.Fatalf("NewExecServer() error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("ExecServer.Close() error = %v", err)
		}
	})

	if client == nil || client.Client() == nil {
		t.Fatal("NewExecServer() returned nil client")
	}
	if got := filepath.Base(client.Client().config.CodexBin); got != filepath.Base(codexBin) {
		t.Fatalf("unexpected codex binary = %q, want %q", got, filepath.Base(codexBin))
	}
}

func TestNewExecServerStartsExecServerOverWebSocketAndPerformsInitializeHandshake(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	codexBin := writeHelperCodexShim(t, exe)
	port := reserveLoopbackPort(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	client, err := NewExecServer(ctx, &Config{
		CodexBin: codexBin,
		Env: map[string]string{
			transportHelperEnv:            "1",
			"CODEX_PORT_HELPER_SCENARIO":  "websocket_roundtrip",
			"CODEX_WEBSOCKET_LISTEN_PORT": port,
			"CODEX_EXEC_SERVER_HANDSHAKE": "1",
		},
		Listen: ListenConfig{
			URL: "ws://127.0.0.1:" + port,
		},
		ServerMode: ServerModeExecServer,
	})
	if err != nil {
		t.Fatalf("NewExecServer() error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("ExecServer.Close() error = %v", err)
		}
	})

	if got := filepath.Base(client.Client().config.CodexBin); got != filepath.Base(codexBin) {
		t.Fatalf("unexpected codex binary = %q, want %q", got, filepath.Base(codexBin))
	}
	if client.Client().config.ServerMode != ServerModeExecServer {
		t.Fatalf("unexpected server mode = %q, want %q", client.Client().config.ServerMode, ServerModeExecServer)
	}
}

// TestExecServerCommandMethodsRejectNilReceiver verifies command wrappers fail
// loudly without a client.
func TestExecServerCommandMethodsRejectNilReceiver(t *testing.T) {
	t.Parallel()

	var nilServer *ExecServer
	emptyServer := &ExecServer{}
	tests := map[string]struct {
		server *ExecServer
		call   func(context.Context, *ExecServer) error
	}{
		"error: nil command exec": {
			server: nilServer,
			call: func(ctx context.Context, server *ExecServer) error {
				_, err := server.CommandExec(ctx, &CommandExecParams{})
				return err
			},
		},
		"error: nil command exec write": {
			server: nilServer,
			call: func(ctx context.Context, server *ExecServer) error {
				_, err := server.CommandExecWrite(ctx, &CommandExecWriteParams{})
				return err
			},
		},
		"error: nil command exec terminate": {
			server: nilServer,
			call: func(ctx context.Context, server *ExecServer) error {
				_, err := server.CommandExecTerminate(ctx, &CommandExecTerminateParams{})
				return err
			},
		},
		"error: nil command exec resize": {
			server: nilServer,
			call: func(ctx context.Context, server *ExecServer) error {
				_, err := server.CommandExecResize(ctx, &CommandExecResizeParams{})
				return err
			},
		},
		"error: nil underlying client": {
			server: emptyServer,
			call: func(ctx context.Context, server *ExecServer) error {
				_, err := server.CommandExec(ctx, &CommandExecParams{})
				return err
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tt.call(t.Context(), tt.server)
			if !errors.Is(err, errExecServerNil) {
				t.Fatalf("command wrapper error = %v, want %v", err, errExecServerNil)
			}
		})
	}
}
