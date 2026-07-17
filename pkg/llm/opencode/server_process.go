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

package opencode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/zchee/pandaemonium/pkg/llm"
)

const (
	// serverOutputMaxLines bounds the retained child stdout/stderr ring used
	// for diagnostics; both pipes are always drained in full regardless.
	serverOutputMaxLines = 200
	// serverStopTimeout bounds the graceful-termination window before the
	// child is killed.
	serverStopTimeout = 5 * time.Second
	// portFallbackAttempts bounds the explicit-port respawn path taken when
	// the announce line never appears (bind-retry window, plan R2).
	portFallbackAttempts = 3
	// passwordEnvVar hands the basic-auth password to the child. There is no
	// password flag: env only, never argv (AC8).
	passwordEnvVar = "OPENCODE_SERVER_PASSWORD"
)

// announceLineRE matches the opencode serve startup announce line, the only
// reliable source of the bound port: a config file's server.port overrides
// the --port flag default (verified against opencode 1.18.3). The unsecured
// warning line may precede it, so the parser scans lines, never one line.
var announceLineRE = regexp.MustCompile(`opencode server listening on (https?://\S+)`)

// errAnnounceTimeout reports that the spawned server never printed a
// parseable announce line within the deadline.
var errAnnounceTimeout = errors.New("opencode serve announce line not observed")

// serverProcess owns one spawned `opencode serve` child: lifecycle, pipe
// draining (from the moment of spawn — the announce parser is a consumer of
// the drain, not a one-shot read), and bounded output retention.
type serverProcess struct {
	cmd *exec.Cmd

	mu          sync.Mutex
	stdoutLines []string
	stderrLines []string

	drained  sync.WaitGroup
	waitOnce sync.Once
	waitErr  error
}

// startServerProcess spawns `opencode serve` and returns the process and its
// base URL. With Config.Port == 0 the URL comes from the announce line; if
// the line never appears within Config.DialTimeout the child is killed and
// respawned on a self-reserved explicit port (health polling in
// Client.Start validates either path).
func startServerProcess(ctx context.Context, cfg *Config) (*serverProcess, string, error) {
	proc, baseURL, err := spawnServer(ctx, cfg, cfg.Port)
	if err == nil {
		return proc, baseURL, nil
	}
	if cfg.Port != 0 || !errors.Is(err, errAnnounceTimeout) {
		return nil, "", err
	}

	// Fallback: the announce line never parsed (format churn or a silent
	// binary). Reserve a free port ourselves and respawn with it explicitly;
	// the reserve→bind window is racy, hence bounded retries.
	lastErr := err
	for range portFallbackAttempts {
		port, err := reserveLoopbackPort(ctx, cfg.Hostname)
		if err != nil {
			return nil, "", fmt.Errorf("opencode: reserve fallback port: %w", err)
		}
		proc, baseURL, err = spawnServer(ctx, cfg, port)
		if err == nil {
			return proc, baseURL, nil
		}
		lastErr = err
	}
	return nil, "", fmt.Errorf("opencode: explicit-port fallback failed: %w", lastErr)
}

// spawnServer starts one `opencode serve` child. port == 0 announces an
// OS-assigned port that must be parsed back; port != 0 skips announce
// parsing (readiness is confirmed by health polling).
func spawnServer(ctx context.Context, cfg *Config, port int) (*serverProcess, string, error) {
	// The child is a long-lived server owned by serverProcess.Close, not by
	// the Start ctx: exec.CommandContext would kill it as soon as the
	// startup context ends.
	//nolint:gosec,noctx // G204: OpencodeBin/Hostname come from the caller's Config by design (mirrors codex CodexBin); noctx: lifecycle owned by Close, see above.
	cmd := exec.Command(cfg.OpencodeBin, "serve", "--hostname", cfg.Hostname, "--port", strconv.Itoa(port))
	cmd.Dir = cfg.Cwd
	cmd.Env = os.Environ()
	for key, value := range cfg.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if cfg.Password != "" {
		cmd.Env = append(cmd.Env, passwordEnvVar+"="+cfg.Password)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", fmt.Errorf("opencode: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, "", fmt.Errorf("opencode: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("opencode: start %s serve: %w", cfg.OpencodeBin, err)
	}

	proc := &serverProcess{cmd: cmd}
	announceCh := make(chan string, 1)
	proc.drained.Add(2)
	go func() {
		defer proc.drained.Done()
		llm.DrainLines(stdout, func(line string) {
			proc.mu.Lock()
			proc.stdoutLines = llm.AppendBoundedLine(proc.stdoutLines, line, serverOutputMaxLines)
			proc.mu.Unlock()
			if match := announceLineRE.FindStringSubmatch(line); match != nil {
				select {
				case announceCh <- match[1]:
				default:
				}
			}
		})
	}()
	go func() {
		defer proc.drained.Done()
		llm.DrainLines(stderr, func(line string) {
			proc.mu.Lock()
			proc.stderrLines = llm.AppendBoundedLine(proc.stderrLines, line, serverOutputMaxLines)
			proc.mu.Unlock()
		})
	}()

	if port != 0 {
		return proc, "http://" + net.JoinHostPort(cfg.Hostname, strconv.Itoa(port)), nil
	}

	timer := time.NewTimer(cfg.DialTimeout)
	defer timer.Stop()
	select {
	case baseURL := <-announceCh:
		return proc, baseURL, nil
	case <-timer.C:
		tail := proc.outputTail()
		_ = proc.Close()
		return nil, "", fmt.Errorf("%w within %s; server output tail:\n%s", errAnnounceTimeout, cfg.DialTimeout, tail)
	case <-ctx.Done():
		_ = proc.Close()
		return nil, "", fmt.Errorf("opencode: canceled waiting for serve announce line: %w", ctx.Err())
	}
}

// outputTail returns recent child output for diagnostics. The password never
// appears here: the child receives it via env and does not echo it.
func (p *serverProcess) outputTail() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return llm.Tail(p.stdoutLines, 20) + "\n" + llm.Tail(p.stderrLines, 20)
}

// wait reaps the child exactly once, after both pipe drains complete (the
// pipes reach EOF when the process exits).
func (p *serverProcess) wait() error {
	p.waitOnce.Do(func() {
		p.drained.Wait()
		p.waitErr = p.cmd.Wait()
	})
	return p.waitErr
}

// Close terminates the child: graceful SIGTERM, bounded wait, then SIGKILL.
// Exit errors caused by our own signals are not propagated.
func (p *serverProcess) Close() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)

	waitCh := make(chan error, 1)
	go func() { waitCh <- p.wait() }()

	timer := time.NewTimer(serverStopTimeout)
	defer timer.Stop()
	select {
	case <-waitCh:
		return nil
	case <-timer.C:
		_ = p.cmd.Process.Kill()
		<-waitCh
		return nil
	}
}

// reserveLoopbackPort binds host:0, records the assigned port, and releases
// it for the child to bind (a small race window, bounded by the caller's
// retry loop).
func reserveLoopbackPort(ctx context.Context, host string) (int, error) {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return addr.Port, nil
}
