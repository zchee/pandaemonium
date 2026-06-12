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
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zchee/pandaemonium/pkg/llm"
)

const (
	// remoteControlFlag is the hidden value-less boolean flag that enables the
	// codex-app-server remote-control subsystem (client tracking, auth,
	// enrollment) for external `codex --remote` attachment. Verified accepted by
	// both the standalone `codex-app-server` binary and the `codex app-server`
	// subcommand on codex-cli 0.140.0-alpha.13.
	remoteControlFlag = "--remote-control"

	// appServerBinName is the default standalone server binary resolved via
	// exec.LookPath when RemoteAppServerConfig.AppServerBin is empty.
	appServerBinName = "codex-app-server"

	// codexBinName is the default attached CLI and subcommand-fallback binary
	// resolved via exec.LookPath when RemoteAppServerConfig.CodexBin is empty.
	codexBinName = "codex"

	// appServerSubcommand is appended to the codex binary when falling back to
	// the `codex app-server` subcommand surface.
	appServerSubcommand = "app-server"

	// remoteAuthTokenEnv is the environment variable carrying the websocket
	// bearer token into the attached codex child. The token value never appears
	// in argv, error strings, or the stderr tail.
	remoteAuthTokenEnv = "CODEX_REMOTE_AUTH_TOKEN"

	// remoteAuthTokenEnvFlag wires the attached codex child to read its bearer
	// token from remoteAuthTokenEnv instead of an argv value.
	remoteAuthTokenEnvFlag = "--remote-auth-token-env"

	// defaultRemoteReadyTimeout bounds the readiness probe when
	// RemoteAppServerConfig.ReadyTimeout is zero.
	defaultRemoteReadyTimeout = 10 * time.Second

	// remoteServerWaitDelay is the grace period exec.Cmd allows between the
	// interrupt sent by cmd.Cancel and a forced kill.
	remoteServerWaitDelay = 5 * time.Second

	// remoteStderrTailLines bounds the retained server stderr ring used for
	// diagnostics.
	remoteStderrTailLines = 400

	// remoteReadyAttemptLimit caps readiness dial attempts, mirroring
	// dialWebSocketWithWait.
	remoteReadyAttemptLimit = 50

	// remoteReadyBackoffStep and remoteReadyBackoffCap shape the readiness
	// retry backoff, mirroring dialWebSocketWithWait.
	remoteReadyBackoffStep = 25 * time.Millisecond
	remoteReadyBackoffCap  = 250 * time.Millisecond

	// unixSocketPathLimit is the maximum portable unix socket path length. The
	// darwin sun_path field holds 104 bytes (linux holds 108); rejecting paths
	// longer than 103 bytes keeps room for the NUL terminator on every target.
	unixSocketPathLimit = 103

	// remoteAttachedKillGrace bounds the wait for a tracked codex child to exit
	// after interruption before it is force-killed.
	remoteAttachedKillGrace = 5 * time.Second
)

// errRemoteListenRequired is returned when no remote endpoint is configured. A
// stdio (or "off") transport cannot accept external `codex --remote`
// attachments, so a ws:// or unix:// listen URL is mandatory.
var errRemoteListenRequired = errors.New("remote app-server listen URL is required: configure ws://127.0.0.1:PORT, unix://, or unix://PATH (stdio:// and off cannot accept --remote attachments)")

// errRemoteFlagInArgs is returned when caller-supplied codex arguments already
// contain a --remote flag, which would make endpoint ownership ambiguous.
var errRemoteFlagInArgs = errors.New("codex arguments must not contain --remote; the endpoint is injected by the launcher")

// RemoteAppServerConfig controls launching a codex-app-server child that listens
// on a ws:// or unix:// endpoint for external --remote attachment.
type RemoteAppServerConfig struct {
	// AppServerBin is the standalone server binary. Empty resolves
	// "codex-app-server" via exec.LookPath; if that lookup fails, the launcher
	// falls back to `<CodexBin> app-server` (same --listen surface).
	AppServerBin string

	// CodexBin is used for the attached CLI and the subcommand fallback. Empty
	// resolves "codex" via exec.LookPath.
	CodexBin string

	// Listen reuses ListenConfig. URL must be ws://127.0.0.1:PORT (loopback
	// unless AllowInsecureRemoteWebSocket), unix://, or unix://PATH. stdio:// and
	// "off" are rejected: a remote endpoint is required.
	Listen ListenConfig

	// RemoteControl toggles the hidden value-less --remote-control flag that
	// enables the listener's remote-control subsystem. A nil pointer means
	// enabled (the purpose of this launcher), following the ExperimentalAPI
	// *bool nil-defaults-true convention. A pointer to false omits the flag.
	RemoteControl *bool

	// ConfigOverrides are emitted as ("--config", v) pairs, mirroring
	// Config.ConfigOverrides.
	ConfigOverrides []string

	// ExtraArgs are appended verbatim after the listen and ws-auth arguments
	// (an escape hatch for other hidden or experimental flags).
	ExtraArgs []string

	// Cwd is the working directory for the server child and the resolution base
	// for relative unix socket paths.
	Cwd string

	// Env is merged over os.Environ for the server child and the attached codex
	// command. CODEX_HOME stays in sync so the default unix:// control socket
	// resolves identically on both sides.
	Env map[string]string

	// ReadyTimeout bounds the readiness probe. A zero value defaults to 10s.
	ReadyTimeout time.Duration

	// Stderr optionally mirrors server stderr lines live. A bounded tail is
	// always retained for diagnostics regardless of this field.
	Stderr io.Writer
}

// RemoteAppServer is an SDK-owned codex-app-server child listening for external
// `codex --remote` attachments. It owns the server process lifecycle, tracks
// codex children launched through it, and exposes the listen endpoint for both
// the Go SDK ([RemoteAppServer.Connect], [RemoteAppServer.RemoteConfig]) and the
// codex CLI ([RemoteAppServer.CodexCommand], [RemoteAppServer.StartCodex]).
type RemoteAppServer struct {
	endpoint   string
	socketPath string
	codexBin   string
	// removeSocketOnClose is true only for explicit custom unix://PATH
	// endpoints; the default control socket is shared with daemon tooling and
	// is never removed.
	removeSocketOnClose bool
	// bearerToken is the resolved websocket bearer token for the attached codex
	// child, or empty when no bearer auth is configured. It is wired through the
	// child environment only and never placed in argv or diagnostics.
	bearerToken string
	env         map[string]string

	proc *trackedCommand

	stderrMu    sync.Mutex
	stderrLines []string

	mu        sync.Mutex
	attached  []*trackedCommand
	closeOnce sync.Once
	closed    bool
}

// trackedCommand owns the single Cmd.Wait call for a started child process and
// broadcasts completion by closing done, so any number of observers
// ([RemoteAppServer.Wait], [AttachedCodex.Wait], terminateTracked, reapAttached)
// can block on exit without a second concurrent Wait on the same process.
type trackedCommand struct {
	cmd  *exec.Cmd
	done chan struct{}
	// err is the Cmd.Wait result, readable only after done is closed.
	err error
}

// trackCommand spawns the single Wait owner goroutine for a started command.
func trackCommand(cmd *exec.Cmd) *trackedCommand {
	t := &trackedCommand{cmd: cmd, done: make(chan struct{})}
	go func() {
		t.err = cmd.Wait()
		close(t.done)
	}()
	return t
}

// wait blocks until the child exits and returns the Cmd.Wait result. It is
// safe for any number of concurrent callers: the err write happens before the
// done close.
func (t *trackedCommand) wait() error {
	<-t.done
	return t.err
}

// AttachedCodex is a running `codex --remote` child started through
// [RemoteAppServer.StartCodex]. The server retains the single Cmd.Wait owner
// for the child, so exits are observed through [AttachedCodex.Wait] rather
// than the underlying command.
type AttachedCodex struct {
	proc *trackedCommand
}

// Cmd exposes the underlying started command for argv and process
// introspection. Callers must not call Wait on it: the server owns the single
// Cmd.Wait, and a second concurrent Wait on the same process blocks until the
// launch context ends. Use [AttachedCodex.Wait] instead.
func (a *AttachedCodex) Cmd() *exec.Cmd { return a.proc.cmd }

// Wait blocks until the attached codex child exits and returns its Cmd.Wait
// result. It is safe to call from any number of goroutines and never races
// the server's internal tracking or [RemoteAppServer.Close].
func (a *AttachedCodex) Wait() error { return a.proc.wait() }

// ReserveLoopbackPort binds 127.0.0.1:0, frees it, and returns the chosen port.
//
// Upstream rejects websocket listen port 0 ([validateListenConfig]), so callers
// must pick a concrete port. The reserve-then-bind window is racy; a launch that
// fails because the port was taken surfaces the server stderr tail, and retrying
// with a fresh port is the caller's loop.
func ReserveLoopbackPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve loopback port: %w", err)
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("reserve loopback port: unexpected address type %T", ln.Addr())
	}
	return addr.Port, nil
}

// LaunchRemoteAppServer starts the server child and blocks until the endpoint
// accepts connections or the child exits or ReadyTimeout elapses.
//
// On success the returned [RemoteAppServer] owns the child process; the caller
// must call [RemoteAppServer.Close] to release it. On any error the child (if
// started) is terminated before returning.
func LaunchRemoteAppServer(ctx context.Context, cfg *RemoteAppServerConfig) (*RemoteAppServer, error) {
	if cfg == nil {
		cfg = &RemoteAppServerConfig{}
	}

	endpoint, kind, socketPath, err := resolveRemoteEndpoint(cfg)
	if err != nil {
		return nil, err
	}

	codexBin, err := resolveCodexBinary(cfg.CodexBin)
	if err != nil {
		return nil, err
	}

	args, err := buildRemoteAppServerLaunch(cfg, endpoint, codexBin)
	if err != nil {
		return nil, err
	}

	bearerToken, err := remoteAttachBearerToken(cfg.Listen.WebSocket)
	if err != nil {
		return nil, err
	}

	env := remoteEffectiveEnv(cfg.Env)

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = remoteServerWaitDelay
	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}
	cmd.Env = remoteEnvSlice(env)

	server := &RemoteAppServer{
		endpoint:            endpoint,
		socketPath:          socketPath,
		codexBin:            codexBin,
		removeSocketOnClose: kind == listenTransportUnixWebSocket && socketPath != "" && remoteEndpointHasExplicitPath(cfg.Listen.URL),
		bearerToken:         bearerToken,
		env:                 env,
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create %s stderr: %w", appServerBinName, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create %s stdout: %w", appServerBinName, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", appServerBinName, err)
	}
	server.proc = trackCommand(cmd)
	stderrDone := make(chan struct{})
	go server.drainStderr(stderr, cfg.Stderr, stderrDone)
	go drainAndDiscard(stdout)

	timeout := cfg.ReadyTimeout
	if timeout <= 0 {
		timeout = defaultRemoteReadyTimeout
	}
	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := waitRemoteAppServerReady(readyCtx, server, kind); err != nil {
		terminateTracked(server.proc, remoteServerWaitDelay)
		<-stderrDone
		return nil, err
	}
	return server, nil
}

// Endpoint returns the configured canonical listen URL
// (ws://127.0.0.1:PORT or unix://[PATH]).
func (s *RemoteAppServer) Endpoint() string { return s.endpoint }

// SocketPath returns the resolved filesystem path for unix endpoints, or "" for
// ws:// endpoints.
func (s *RemoteAppServer) SocketPath() string { return s.socketPath }

// RemoteConfig returns a [RemoteConfig] addressing this server, ready for
// [NewRemoteClient] or [NewRemoteCodex].
func (s *RemoteAppServer) RemoteConfig() *RemoteConfig {
	cfg := &RemoteConfig{
		URL:         s.endpoint,
		DialTimeout: 5 * time.Second,
	}
	if s.socketPath == "" && s.bearerToken != "" {
		cfg.BearerToken = s.bearerToken
	}
	return cfg
}

// Connect attaches a Go SDK [Client] to this server without launching another
// codex process. The caller owns the returned client and must close it.
func (s *RemoteAppServer) Connect(ctx context.Context, h ApprovalHandler) (*Client, error) {
	return NewRemoteClient(ctx, s.RemoteConfig(), h)
}

// CodexCommand returns an unstarted `codex --remote=<endpoint> [args...]`
// exec.Cmd with the environment merged (CODEX_HOME synced) and, when websocket
// bearer auth is configured, --remote-auth-token-env wired so the token reaches
// the child via the environment rather than argv.
//
// The caller owns stdio wiring; an attached TUI requires a TTY. CodexCommand
// rejects args that already contain a --remote flag.
func (s *RemoteAppServer) CodexCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	if slices.ContainsFunc(args, argIsRemoteFlag) {
		return nil, errRemoteFlagInArgs
	}

	cmdArgs := make([]string, 0, len(args)+1)
	cmdArgs = append(cmdArgs, "--remote="+s.endpoint)
	if s.bearerToken != "" {
		cmdArgs = append(cmdArgs, remoteAuthTokenEnvFlag, remoteAuthTokenEnv)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, s.codexBin, cmdArgs...)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = remoteAttachedKillGrace

	childEnv := maps.Clone(s.env)
	if childEnv == nil {
		childEnv = map[string]string{}
	}
	if s.bearerToken != "" {
		childEnv[remoteAuthTokenEnv] = s.bearerToken
	}
	cmd.Env = remoteEnvSlice(childEnv)
	return cmd, nil
}

// StartCodex starts [RemoteAppServer.CodexCommand] with the given stdio and
// tracks the child so [RemoteAppServer.Close] terminates it before the server.
// The server owns the single Cmd.Wait on the child; observe its exit through
// [AttachedCodex.Wait] on the returned handle. For full control over the
// exec.Cmd, pair [RemoteAppServer.CodexCommand] with
// [RemoteAppServer.StartCodexWithCmd].
func (s *RemoteAppServer) StartCodex(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) (*AttachedCodex, error) {
	cmd, err := s.CodexCommand(ctx, args...)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	return s.StartCodexWithCmd(cmd)
}

// StartCodexWithCmd starts a caller-prepared attach command, typically built
// by [RemoteAppServer.CodexCommand] and customized (stdio, Dir, extra env)
// before start, and tracks the child so [RemoteAppServer.Close] terminates it
// before the server. It fails once the server is closed.
//
// cmd must not have been started yet. On success the server owns the single
// Cmd.Wait on the child: do not call cmd.Wait; observe the exit through
// [AttachedCodex.Wait] on the returned handle.
func (s *RemoteAppServer) StartCodexWithCmd(cmd *exec.Cmd) (*AttachedCodex, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errors.New("remote app-server is closed")
	}
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("start codex --remote: %w", err)
	}
	tracked := trackCommand(cmd)
	s.attached = append(s.attached, tracked)
	s.mu.Unlock()

	go s.reapAttached(tracked)
	return &AttachedCodex{proc: tracked}, nil
}

// StderrTail returns the most recent n server stderr lines joined by newlines.
func (s *RemoteAppServer) StderrTail(n int) string {
	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()
	return llm.Tail(s.stderrLines, n)
}

// Wait blocks until the server child exits and returns its exit error. It is
// safe for any number of concurrent callers, including alongside
// [RemoteAppServer.Close].
func (s *RemoteAppServer) Wait() error {
	if s.proc == nil {
		return nil
	}
	return s.proc.wait()
}

// Close terminates the server and any tracked codex children. It is idempotent:
// repeated calls return the same result. Termination order is attached codex
// children first (interrupt, grace, then kill), then the server child, then a
// best-effort removal of an explicit custom unix socket file after the child
// has exited.
func (s *RemoteAppServer) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		attached := slices.Clone(s.attached)
		s.attached = nil
		s.mu.Unlock()

		for _, tracked := range attached {
			terminateTracked(tracked, remoteAttachedKillGrace)
		}

		terminateTracked(s.proc, remoteServerWaitDelay)

		if s.removeSocketOnClose && s.socketPath != "" {
			_ = os.Remove(s.socketPath)
		}
	})
	return nil
}

// reapAttached removes a tracked codex child from the registry once it exits so
// a long-lived server does not accumulate finished commands.
func (s *RemoteAppServer) reapAttached(tracked *trackedCommand) {
	<-tracked.done
	s.mu.Lock()
	if i := slices.Index(s.attached, tracked); i >= 0 {
		s.attached = slices.Delete(s.attached, i, i+1)
	}
	s.mu.Unlock()
}

func (s *RemoteAppServer) drainStderr(stderr io.Reader, mirror io.Writer, done chan<- struct{}) {
	defer close(done)
	llm.DrainLines(stderr, func(line string) {
		s.stderrMu.Lock()
		s.stderrLines = llm.AppendBoundedLine(s.stderrLines, line, remoteStderrTailLines)
		s.stderrMu.Unlock()
		if mirror != nil {
			_, _ = io.WriteString(mirror, line+"\n")
		}
	})
}

// resolveRemoteEndpoint validates the listen configuration and returns the
// canonical endpoint, its transport kind, and the resolved unix socket path (""
// for ws://). It rejects stdio/off/empty before any process is spawned.
func resolveRemoteEndpoint(cfg *RemoteAppServerConfig) (string, listenTransportKind, string, error) {
	listenURL := strings.TrimSpace(cfg.Listen.URL)
	if listenURL == "" || listenURL == defaultListenURL || listenURL == "off" || listenURL == "stdio" {
		return "", listenTransportStdio, "", errRemoteListenRequired
	}

	kind, err := parseListenTransport(listenURL)
	if err != nil {
		return "", listenTransportStdio, "", err
	}
	if kind == listenTransportStdio {
		return "", listenTransportStdio, "", errRemoteListenRequired
	}
	if err := validateListenConfig(cfg.Listen, kind, listenURL); err != nil {
		return "", kind, "", err
	}

	socketPath := ""
	if kind == listenTransportUnixWebSocket {
		socketPath, err = unixSocketPathFromListenURL(listenURL, cfg.Env, cfg.Cwd)
		if err != nil {
			return "", kind, "", err
		}
		if len(socketPath) > unixSocketPathLimit {
			return "", kind, "", fmt.Errorf("unix socket path %q has length %d, exceeding the %d-byte limit (darwin sun_path holds 104 bytes, linux 108)", socketPath, len(socketPath), unixSocketPathLimit)
		}
	}
	return listenURL, kind, socketPath, nil
}

// buildRemoteAppServerLaunch resolves the server binary (standalone preferred,
// `codex app-server` subcommand fallback) and returns the full launch argv.
func buildRemoteAppServerLaunch(cfg *RemoteAppServerConfig, endpoint, codexBin string) ([]string, error) {
	bin, err := resolveAppServerBinary(cfg.AppServerBin)
	if err == nil {
		return buildRemoteAppServerArgs(cfg, bin, endpoint, false), nil
	}
	// Standalone binary unavailable: fall back to the `codex app-server`
	// subcommand, which accepts the same --listen and --remote-control surface.
	return buildRemoteAppServerArgs(cfg, codexBin, endpoint, true), nil
}

// buildRemoteAppServerArgs is a pure argv builder. When useSubcommand is true it
// emits `<bin> [--config v]... app-server --remote-control? --listen URL ...`
// (mirroring buildServerArgsForCommand --config placement); otherwise it emits
// `<bin> --remote-control? [--config v]... --listen URL ...`.
func buildRemoteAppServerArgs(cfg *RemoteAppServerConfig, bin, endpoint string, useSubcommand bool) []string {
	args := []string{bin}
	if useSubcommand {
		for _, override := range cfg.ConfigOverrides {
			args = append(args, "--config", override)
		}
		args = append(args, appServerSubcommand)
		if remoteControlEnabled(cfg.RemoteControl) {
			args = append(args, remoteControlFlag)
		}
	} else {
		if remoteControlEnabled(cfg.RemoteControl) {
			args = append(args, remoteControlFlag)
		}
		for _, override := range cfg.ConfigOverrides {
			args = append(args, "--config", override)
		}
	}
	args = append(args, "--listen", endpoint)
	if parsedKind, err := parseListenTransport(endpoint); err == nil && parsedKind == listenTransportWebSocket {
		args = append(args, wsLaunchArgs(cfg.Listen.WebSocket)...)
	}
	args = append(args, cfg.ExtraArgs...)
	return args
}

// remoteControlEnabled reports whether the hidden --remote-control flag should
// be emitted. A nil pointer means enabled (the launcher's purpose); a pointer to
// false opts out.
func remoteControlEnabled(p *bool) bool {
	return p == nil || *p
}

// remoteAttachBearerToken resolves the websocket bearer token for the attached
// codex child, reading a token file when configured. It returns an empty token
// when no bearer source is present. Errors never embed the token value.
func remoteAttachBearerToken(cfg *WebSocketConfig) (string, error) {
	if cfg == nil {
		return "", nil
	}
	if token := strings.TrimSpace(cfg.ClientBearerToken); token != "" {
		return token, nil
	}
	if cfg.ClientBearerTokenFile != "" {
		raw, err := os.ReadFile(cfg.ClientBearerTokenFile)
		if err != nil {
			return "", fmt.Errorf("read remote bearer token file: %w", err)
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return "", fmt.Errorf("remote bearer token file %q is empty", cfg.ClientBearerTokenFile)
		}
		return token, nil
	}
	return "", nil
}

// waitRemoteAppServerReady probes the endpoint with plain net.Dial until it
// accepts a connection, failing fast if the server child exits first. It mirrors
// dialWebSocketWithWait's loop shape (procDone fail-fast, capped backoff, attempt
// cap) but proves only that the listener is bound.
func waitRemoteAppServerReady(ctx context.Context, s *RemoteAppServer, kind listenTransportKind) error {
	network, address := remoteDialTarget(kind, s.endpoint, s.socketPath)

	for attempt := range remoteReadyAttemptLimit {
		if err := remoteProcExited(s, kind); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return remoteReadyContextError(ctx, s, kind)
		default:
		}

		conn, err := net.Dial(network, address)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		if attempt >= remoteReadyAttemptLimit-1 {
			return remoteNotReadyError(s, kind, err)
		}
		backoff := min(remoteReadyBackoffStep*time.Duration(attempt+1), remoteReadyBackoffCap)
		select {
		case <-ctx.Done():
			return remoteReadyContextError(ctx, s, kind)
		case <-s.proc.done:
			return remoteExitedBeforeReadyError(s, kind, s.proc.err)
		case <-time.After(backoff):
		}
	}
	return remoteNotReadyError(s, kind, nil)
}

// remoteProcExited returns a fail-fast error if the server child has already
// exited, polling the tracked completion without blocking.
func remoteProcExited(s *RemoteAppServer, kind listenTransportKind) error {
	select {
	case <-s.proc.done:
		return remoteExitedBeforeReadyError(s, kind, s.proc.err)
	default:
		return nil
	}
}

func remoteReadyContextError(ctx context.Context, s *RemoteAppServer, kind listenTransportKind) error {
	// An exited child takes precedence: its Wait result explains the failure
	// better than a bare deadline.
	select {
	case <-s.proc.done:
		return remoteExitedBeforeReadyError(s, kind, s.proc.err)
	default:
	}
	return fmt.Errorf("%s readiness timed out: %w; stderr_tail=%s", remoteReadyModeLabel(kind), ctx.Err(), s.StderrTail(40))
}

func remoteExitedBeforeReadyError(s *RemoteAppServer, kind listenTransportKind, waitErr error) error {
	label := remoteReadyModeLabel(kind)
	tail := s.StderrTail(40)
	if waitErr != nil {
		return fmt.Errorf("%s exited before %s readiness: %v; stderr_tail=%s", appServerBinName, label, waitErr, tail)
	}
	return fmt.Errorf("%s exited before %s readiness; stderr_tail=%s", appServerBinName, label, tail)
}

func remoteNotReadyError(s *RemoteAppServer, kind listenTransportKind, dialErr error) error {
	label := remoteReadyModeLabel(kind)
	tail := s.StderrTail(40)
	if dialErr != nil {
		return fmt.Errorf("%s %s not ready after %d attempts: %v; stderr_tail=%s", appServerBinName, label, remoteReadyAttemptLimit, dialErr, tail)
	}
	return fmt.Errorf("%s %s not ready after %d attempts; stderr_tail=%s", appServerBinName, label, remoteReadyAttemptLimit, tail)
}

func remoteReadyModeLabel(kind listenTransportKind) string {
	if kind == listenTransportUnixWebSocket {
		return "unix socket"
	}
	return "websocket"
}

func remoteDialTarget(kind listenTransportKind, endpoint, socketPath string) (string, string) {
	if kind == listenTransportUnixWebSocket {
		return "unix", socketPath
	}
	return "tcp", remoteHostPort(endpoint)
}

// remoteHostPort extracts host:port from a ws:// endpoint for net.Dial. The URL
// was validated by validateListenConfig, so it always carries an explicit host
// and port here.
func remoteHostPort(endpoint string) string {
	rest := strings.TrimPrefix(endpoint, "ws://")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// remoteEndpointHasExplicitPath reports whether a unix:// listen URL names an
// explicit socket path (so it is SDK-owned and safe to unlink on Close) versus
// the bare default control socket (shared with daemon tooling, never removed).
func remoteEndpointHasExplicitPath(listenURL string) bool {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(listenURL), unixListenPrefix)) != ""
}

// argIsRemoteFlag reports whether arg is a --remote flag in any accepted form.
func argIsRemoteFlag(arg string) bool {
	return arg == "--remote" || strings.HasPrefix(arg, "--remote=")
}

func resolveAppServerBinary(bin string) (string, error) {
	return resolveBinary(bin, appServerBinName)
}

func resolveCodexBinary(bin string) (string, error) {
	return resolveBinary(bin, codexBinName)
}

// resolveBinary mirrors buildServerArgsForCommand binary resolution: LookPath
// for the default name, LookPath for a non-absolute override, then an os.Stat
// existence check.
func resolveBinary(bin, defaultName string) (string, error) {
	if bin == "" {
		path, err := exec.LookPath(defaultName)
		if err != nil {
			return "", fmt.Errorf("locate %s binary: %w", defaultName, err)
		}
		return path, nil
	}
	if !filepath.IsAbs(bin) {
		resolved, err := exec.LookPath(bin)
		if err != nil {
			return "", fmt.Errorf("locate %s binary %q: %w", defaultName, bin, err)
		}
		bin = resolved
	}
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("%s binary not found at %s: %w", defaultName, bin, err)
	}
	return bin, nil
}

// remoteEffectiveEnv merges overrides over os.Environ, mirroring the
// Client.effectiveEnv pattern so CODEX_HOME and other inherited variables flow
// to children consistently.
func remoteEffectiveEnv(overrides map[string]string) map[string]string {
	env := make(map[string]string, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		env[key] = value
	}
	maps.Copy(env, overrides)
	return env
}

func remoteEnvSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}

// terminateTracked interrupts a tracked process and waits for its single Wait
// owner to report completion, forcing a kill after grace. The tracked done
// channel is the only exit observation point, so no second concurrent Wait is
// issued. It is safe to call with a nil tracked command or a never-started
// process.
func terminateTracked(tracked *trackedCommand, grace time.Duration) {
	if tracked == nil || tracked.cmd == nil || tracked.cmd.Process == nil {
		return
	}
	_ = tracked.cmd.Process.Signal(os.Interrupt)
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-tracked.done:
	case <-timer.C:
		_ = tracked.cmd.Process.Kill()
		<-tracked.done
	}
}

func drainAndDiscard(r io.Reader) {
	_, _ = io.Copy(io.Discard, r)
}
