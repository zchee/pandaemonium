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

package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zchee/pandaemonium/pkg/llm/codex"
)

// TestRealAppServerIntegrationRemoteLaunchUnixAttach launches a real
// `codex-app-server --remote-control --listen unix://<path>` child through
// LaunchRemoteAppServer and attaches the Go SDK over the returned RemoteConfig.
// It is gated behind RUN_REAL_CODEX_TESTS=1 and skips cleanly when the
// installed binaries lack remote-control or unix listen support.
func TestRealAppServerIntegrationRemoteLaunchUnixAttach(t *testing.T) {
	codexBin := requireRealCodexBinary(t)
	if runtime.GOOS == "windows" {
		t.Skip("Unix-domain sockets are not supported on windows")
	}

	socketPath := shortRemoteLaunchSocketPath(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	t.Cleanup(cancel)

	// The sibling standalone binary shares release provenance with the
	// verified codex binary. When it is absent the launcher falls back to the
	// `codex app-server` subcommand, which keeps provenance consistent instead
	// of resolving an arbitrary codex-app-server from PATH.
	server, err := codex.LaunchRemoteAppServer(ctx, &codex.RemoteAppServerConfig{
		AppServerBin: filepath.Join(filepath.Dir(codexBin), "codex-app-server"),
		CodexBin:     codexBin,
		Listen:       codex.ListenConfig{URL: "unix://" + socketPath},
		Cwd:          t.TempDir(),
	})
	if err != nil {
		lowered := strings.ToLower(err.Error())
		for _, marker := range []string{"unexpected argument", "remote-control", "unix", "listen"} {
			if strings.Contains(lowered, marker) {
				t.Skipf("real codex binaries do not support remote-control unix listen: %v", err)
			}
		}
		t.Fatalf("LaunchRemoteAppServer() real error = %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Fatalf("RemoteAppServer.Close() real error = %v", err)
		}
	})

	if got, want := server.Endpoint(), "unix://"+socketPath; got != want {
		t.Fatalf("RemoteAppServer.Endpoint() = %q, want %q", got, want)
	}
	if got := server.SocketPath(); got != socketPath {
		t.Fatalf("RemoteAppServer.SocketPath() = %q, want %q", got, socketPath)
	}
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("os.Stat(%s) after launch error = %v, want listening socket", socketPath, err)
	}

	client, err := codex.NewRemoteCodex(ctx, server.RemoteConfig())
	if err != nil {
		t.Fatalf("NewRemoteCodex(RemoteConfig()) real error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Codex.Close() real remote-launch error = %v", err)
		}
	})

	assertRealInitializedMetadata(t, client.Metadata())

	// The app-server rejects model/list requests without a params object, so
	// always send explicit (possibly empty) params.
	includeHidden := true
	models, err := client.Models(ctx, &codex.ModelListParams{IncludeHidden: includeHidden})
	if err != nil {
		t.Fatalf("Models(includeHidden=true) real remote-launch error = %v", err)
	}
	if models.Data == nil {
		t.Fatalf("Models().Data = nil, want list shape")
	}
	for index, model := range models.Data {
		if strings.TrimSpace(model.ID) == "" {
			t.Fatalf("Models().Data[%d].ID is empty: %#v", index, model)
		}
	}
}

// shortRemoteLaunchSocketPath returns a unix socket path within the darwin
// sun_path budget (104 bytes including NUL; 103 usable). Deep go test temp
// directories can exceed it, so the helper falls back to a short
// os.MkdirTemp directory before skipping.
func shortRemoteLaunchSocketPath(t *testing.T) string {
	t.Helper()

	const sunPathLimit = 103
	socketPath := filepath.Join(t.TempDir(), "codex.sock")
	if len(socketPath) <= sunPathLimit {
		return socketPath
	}
	dir, err := os.MkdirTemp("", "cdx-*")
	if err != nil {
		t.Fatalf("os.MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath = filepath.Join(dir, "codex.sock")
	if len(socketPath) > sunPathLimit {
		t.Skipf("temp directories produce unix socket paths longer than the %d-byte sun_path limit: %s", sunPathLimit, socketPath)
	}
	return socketPath
}
