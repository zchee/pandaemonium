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
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// launchHelperRemoteAppServer launches the in-test helper shim as a fake
// codex-app-server for the given scenario. Pointing AppServerBin at the shim
// drives the standalone launch path; CodexBin is set to the same shim so the
// two-process attachment flow reuses it. The helper reads its scenario from the
// propagated environment, so the injected --listen/--remote-control args are
// inert.
func launchHelperRemoteAppServer(t *testing.T, cfg *RemoteAppServerConfig, scenarioEnv map[string]string) *RemoteAppServer {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	shim := writeHelperCodexShim(t, exe)

	if cfg.AppServerBin == "" {
		cfg.AppServerBin = shim
	}
	if cfg.CodexBin == "" {
		cfg.CodexBin = shim
	}
	env := map[string]string{transportHelperEnv: "1"}
	maps.Copy(env, scenarioEnv)
	maps.Copy(env, cfg.Env)
	cfg.Env = env

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)

	server, err := LaunchRemoteAppServer(ctx, cfg)
	if err != nil {
		t.Fatalf("LaunchRemoteAppServer() error = %v", err)
	}
	return server
}

func TestLaunchRemoteAppServerWebSocketReadyAndClose(t *testing.T) {
	port := reserveLoopbackPort(t)
	endpoint := "ws://127.0.0.1:" + port

	server := launchHelperRemoteAppServer(
		t,
		&RemoteAppServerConfig{
			Listen:       ListenConfig{URL: endpoint},
			ReadyTimeout: 8 * time.Second,
		},
		map[string]string{
			"CODEX_PORT_HELPER_SCENARIO":  "websocket_roundtrip",
			"CODEX_WEBSOCKET_LISTEN_PORT": port,
		},
	)

	if server.Endpoint() != endpoint {
		t.Fatalf("Endpoint() = %q, want %q", server.Endpoint(), endpoint)
	}
	if server.SocketPath() != "" {
		t.Fatalf("SocketPath() = %q, want empty for websocket endpoint", server.SocketPath())
	}

	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, time.Second)
	if err != nil {
		t.Fatalf("net.DialTimeout(tcp, %s) error = %v", endpoint, err)
	}
	_ = conn.Close()

	closeWithinDeadline(t, server, 5*time.Second)
}

func TestLaunchRemoteAppServerUnixReadyDialAndSocketRemoval(t *testing.T) {
	skipIfUnixSocketsUnsupported(t)

	socketPath := filepath.Join(shortTempDir(t), "remote-launch.sock")
	endpoint := "unix://" + socketPath

	server := launchHelperRemoteAppServer(
		t,
		&RemoteAppServerConfig{
			Listen:       ListenConfig{URL: endpoint},
			ReadyTimeout: 8 * time.Second,
		},
		map[string]string{
			"CODEX_PORT_HELPER_SCENARIO":       "unix_websocket_roundtrip",
			"CODEX_UNIX_WEBSOCKET_LISTEN_PATH": socketPath,
		},
	)

	if server.SocketPath() != socketPath {
		t.Fatalf("SocketPath() = %q, want %q", server.SocketPath(), socketPath)
	}
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("os.Stat(%s) after launch error = %v, want socket present", socketPath, err)
	}

	conn, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		t.Fatalf("net.DialTimeout(unix, %s) error = %v", socketPath, err)
	}
	_ = conn.Close()

	closeWithinDeadline(t, server, 5*time.Second)

	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("os.Stat(%s) after Close() error = %v, want socket removed", socketPath, err)
	}
}

func TestLaunchRemoteAppServerEarlyExitSurfacesStderrTail(t *testing.T) {
	port := reserveLoopbackPort(t)

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	shim := writeHelperCodexShim(t, exe)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	result := make(chan error, 1)
	go func() {
		_, err := LaunchRemoteAppServer(ctx, &RemoteAppServerConfig{
			AppServerBin: shim,
			CodexBin:     shim,
			Listen:       ListenConfig{URL: "ws://127.0.0.1:" + port},
			ReadyTimeout: 3 * time.Second,
			Env: map[string]string{
				transportHelperEnv:           "1",
				"CODEX_PORT_HELPER_SCENARIO": "exit_without_websocket",
			},
		})
		result <- err
	}()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("LaunchRemoteAppServer() error = nil, want early-exit failure")
		}
		if !strings.Contains(err.Error(), "exited before") {
			t.Fatalf("LaunchRemoteAppServer() error = %v, want 'exited before' readiness diagnostic", err)
		}
		if !strings.Contains(err.Error(), "stderr_tail=") {
			t.Fatalf("LaunchRemoteAppServer() error = %v, want embedded stderr tail", err)
		}
		if !strings.Contains(err.Error(), "helper exited before websocket readiness") {
			t.Fatalf("LaunchRemoteAppServer() error = %v, want helper stderr line in tail", err)
		}
	case <-ctx.Done():
		t.Fatalf("LaunchRemoteAppServer() did not return before deadline: %v", ctx.Err())
	}
}

func TestLaunchRemoteAppServerConcurrentCloseIsRaceFree(t *testing.T) {
	port := reserveLoopbackPort(t)

	server := launchHelperRemoteAppServer(
		t,
		&RemoteAppServerConfig{
			Listen:       ListenConfig{URL: "ws://127.0.0.1:" + port},
			ReadyTimeout: 8 * time.Second,
		},
		map[string]string{
			"CODEX_PORT_HELPER_SCENARIO":  "websocket_roundtrip",
			"CODEX_WEBSOCKET_LISTEN_PORT": port,
		},
	)

	const closers = 4
	var wg sync.WaitGroup
	errs := make([]error, closers)
	wg.Add(closers)
	for i := range closers {
		go func() {
			defer wg.Done()
			errs[i] = server.Close()
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Close() goroutine %d error = %v, want nil", i, err)
		}
	}
}

func TestLaunchRemoteAppServerStderrTailAndMirror(t *testing.T) {
	port := reserveLoopbackPort(t)

	var mirror syncBuffer
	server := launchHelperRemoteAppServer(
		t,
		&RemoteAppServerConfig{
			Listen:       ListenConfig{URL: "ws://127.0.0.1:" + port},
			ReadyTimeout: 8 * time.Second,
			Stderr:       &mirror,
		},
		map[string]string{
			"CODEX_PORT_HELPER_SCENARIO":  "websocket_roundtrip",
			"CODEX_WEBSOCKET_LISTEN_PORT": port,
			// The websocket helper logs the line below to stderr at startup,
			// before it begins serving, so it is captured by the time the
			// readiness probe succeeds.
			"CODEX_WEBSOCKET_STARTUP_LOG": "remote-app-server-online",
		},
	)
	t.Cleanup(func() { _ = server.Close() })

	if !waitForCondition(2*time.Second, func() bool {
		return strings.Contains(server.StderrTail(40), "remote-app-server-online")
	}) {
		t.Fatalf("StderrTail() = %q, want startup log line", server.StderrTail(40))
	}
	if !waitForCondition(2*time.Second, func() bool {
		return strings.Contains(mirror.String(), "remote-app-server-online")
	}) {
		t.Fatalf("cfg.Stderr mirror = %q, want startup log line", mirror.String())
	}
}

func TestLaunchRemoteAppServerRemoteConfigInitializeRoundTrip(t *testing.T) {
	port := reserveLoopbackPort(t)

	server := launchHelperRemoteAppServer(
		t,
		&RemoteAppServerConfig{
			Listen:       ListenConfig{URL: "ws://127.0.0.1:" + port},
			ReadyTimeout: 8 * time.Second,
		},
		map[string]string{
			"CODEX_PORT_HELPER_SCENARIO":  "websocket_roundtrip",
			"CODEX_WEBSOCKET_LISTEN_PORT": port,
		},
	)
	t.Cleanup(func() { _ = server.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	// NewRemoteCodex performs the initialize handshake against the launched
	// endpoint internally; success plus the returned metadata proves the
	// round-trip over RemoteConfig().
	codex, err := NewRemoteCodex(ctx, server.RemoteConfig())
	if err != nil {
		t.Fatalf("NewRemoteCodex(RemoteConfig()) error = %v", err)
	}
	t.Cleanup(func() {
		if err := codex.Close(); err != nil {
			t.Fatalf("Codex.Close() error = %v", err)
		}
	})

	metadata := codex.Metadata()
	if metadata.ServerInfo == nil || metadata.ServerInfo.Name != "codex-bench" {
		t.Fatalf("Metadata() = %#v, want codex-bench server info", metadata)
	}
}

func TestLaunchRemoteAppServerStartCodexInjectsRemoteFlag(t *testing.T) {
	port := reserveLoopbackPort(t)
	endpoint := "ws://127.0.0.1:" + port
	sentinel := filepath.Join(shortTempDir(t), "attach-ok")

	server := launchHelperRemoteAppServer(
		t,
		&RemoteAppServerConfig{
			Listen:       ListenConfig{URL: endpoint},
			ReadyTimeout: 8 * time.Second,
			Env: map[string]string{
				// The attachment shim (selected by the injected --remote flag)
				// asserts this exact flag, dials the endpoint, touches the
				// sentinel, then exits 0.
				"CODEX_REMOTE_ATTACH_EXPECT_FLAG": "--remote=" + endpoint,
				"CODEX_REMOTE_ATTACH_SENTINEL":    sentinel,
			},
		},
		map[string]string{
			"CODEX_PORT_HELPER_SCENARIO":  "websocket_roundtrip",
			"CODEX_WEBSOCKET_LISTEN_PORT": port,
		},
	)
	t.Cleanup(func() { _ = server.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	t.Cleanup(cancel)

	attached, err := server.StartCodex(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("StartCodex() error = %v", err)
	}
	if !slices.Contains(attached.Cmd().Args, "--remote="+endpoint) {
		t.Fatalf("StartCodex() argv = %v, want injected --remote=%s", attached.Cmd().Args, endpoint)
	}

	// StartCodex owns the single Wait on the child; the test observes the
	// attachment's clean exit through the sentinel rather than reaping it again.
	if !waitForCondition(5*time.Second, func() bool {
		_, statErr := os.Stat(sentinel)
		return statErr == nil
	}) {
		t.Fatalf("attachment sentinel %s not created; --remote dial+exit not confirmed", sentinel)
	}
}

func TestRemoteAppServerStartCodexKillsChildBeforeServer(t *testing.T) {
	port := reserveLoopbackPort(t)
	endpoint := "ws://127.0.0.1:" + port

	server := launchHelperRemoteAppServer(
		t,
		&RemoteAppServerConfig{
			Listen:       ListenConfig{URL: endpoint},
			ReadyTimeout: 8 * time.Second,
			Env: map[string]string{
				// Keep the attachment alive (blocked on stdin) until Close()
				// interrupts it, so kill-ordering is observable.
				"CODEX_REMOTE_ATTACH_STAY_ALIVE": "1",
			},
		},
		map[string]string{
			"CODEX_PORT_HELPER_SCENARIO":  "websocket_roundtrip",
			"CODEX_WEBSOCKET_LISTEN_PORT": port,
		},
	)

	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	t.Cleanup(cancel)

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	t.Cleanup(func() {
		_ = stdinR.Close()
		_ = stdinW.Close()
	})

	attached, err := server.StartCodex(ctx, stdinR, nil, nil)
	if err != nil {
		t.Fatalf("StartCodex() error = %v", err)
	}
	if attached.Cmd().Process == nil {
		t.Fatal("StartCodex() returned a command without a started process")
	}
	childPID := attached.Cmd().Process.Pid

	// StartCodex owns the only Wait on the child, so liveness is probed with
	// signal 0 (never a second Wait) to avoid a concurrent reap.
	if !waitForCondition(time.Second, func() bool { return processAlive(childPID) }) {
		t.Fatal("attached codex never became live")
	}

	closeWithinDeadline(t, server, 6*time.Second)

	if !waitForCondition(2*time.Second, func() bool { return !processAlive(childPID) }) {
		t.Fatal("attached codex still running after Close(); kill-ordering not enforced")
	}
}

// TestRemoteAppServerAttachedCodexWaitObservesNaturalExit guards the /exit
// regression: the example-shaped flow StartCodex -> Wait must return once the
// child exits on its own. A second Cmd.Wait on the same process raced the
// server's internal reaper and blocked until the launch context ended.
func TestRemoteAppServerAttachedCodexWaitObservesNaturalExit(t *testing.T) {
	port := reserveLoopbackPort(t)
	endpoint := "ws://127.0.0.1:" + port
	sentinel := filepath.Join(shortTempDir(t), "attach-ok")

	server := launchHelperRemoteAppServer(
		t,
		&RemoteAppServerConfig{
			Listen:       ListenConfig{URL: endpoint},
			ReadyTimeout: 8 * time.Second,
			Env: map[string]string{
				// Without CODEX_REMOTE_ATTACH_STAY_ALIVE the attachment shim
				// dials the endpoint, touches the sentinel, and exits 0 on its
				// own, mimicking a TUI quitting via /exit.
				"CODEX_REMOTE_ATTACH_EXPECT_FLAG": "--remote=" + endpoint,
				"CODEX_REMOTE_ATTACH_SENTINEL":    sentinel,
			},
		},
		map[string]string{
			"CODEX_PORT_HELPER_SCENARIO":  "websocket_roundtrip",
			"CODEX_WEBSOCKET_LISTEN_PORT": port,
		},
	)
	t.Cleanup(func() { _ = server.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	t.Cleanup(cancel)

	attached, err := server.StartCodex(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("StartCodex() error = %v", err)
	}

	const waiters = 3
	waitErrs := make(chan error, waiters)
	for range waiters {
		go func() { waitErrs <- attached.Wait() }()
	}
	for i := range waiters {
		select {
		case err := <-waitErrs:
			if err != nil {
				t.Fatalf("Wait() observer %d error = %v, want nil for the shim's clean exit", i, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Wait() observer %d did not return after the child exited on its own", i)
		}
	}

	// The shim touches the sentinel before exiting 0, so a nil Wait result
	// must come with the sentinel present.
	if _, statErr := os.Stat(sentinel); statErr != nil {
		t.Fatalf("attachment sentinel missing after Wait returned: %v", statErr)
	}

	// The internal reaper drops the exited child from the registry, leaving
	// Close() nothing to terminate but the server itself.
	if !waitForCondition(2*time.Second, func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()
		return len(server.attached) == 0
	}) {
		t.Fatal("attached registry not reaped after natural child exit")
	}

	closeWithinDeadline(t, server, 5*time.Second)
}

// processAlive reports whether pid names a live process, probing with signal 0
// which never reaps the process (so it does not race the owning Wait).
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func closeWithinDeadline(t *testing.T, server *RemoteAppServer, d time.Duration) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- server.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(d):
		t.Fatalf("Close() did not return within %v", d)
	}
}

func waitForCondition(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// syncBuffer is a goroutine-safe io.Writer used to capture mirrored stderr from
// the server drain goroutine without racing the test's reader.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
