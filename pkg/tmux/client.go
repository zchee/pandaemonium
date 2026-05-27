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

package tmux

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Client is a persistent tmux control-mode client.
type Client struct {
	options Options

	transport    transport
	stdoutCloser io.Closer
	cmd          *exec.Cmd

	events     chan Notification
	readDone   chan struct{}
	stderrDone chan struct{}

	writeMu     sync.Mutex
	closeMu     sync.Mutex
	stateMu     sync.Mutex
	closed      bool
	closeErr    error
	cleanupDone bool
	cleanupErr  error

	pendingMu sync.Mutex
	pending   *pendingCommand

	stderrMu    sync.Mutex
	stderrLines []string

	droppedNotifications atomic.Uint64
}

type pendingCommand struct {
	line string
	ch   chan responseResult
}

type responseResult struct {
	response Response
	err      error
}

// New starts a new persistent `tmux -C` control-mode client.
//
// ctx bounds executable lookup/startup and the initial tmux response handshake;
// after New returns, [Client.Close] owns subprocess shutdown.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := applyOptions(opts)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := cfg.Path
	if path == "" {
		path, err = exec.LookPath("tmux")
		if err != nil {
			return nil, fmt.Errorf("tmux: find executable: %w", err)
		}
	}
	args := cfg.launchArgs()
	// Do not bind the subprocess lifetime to ctx. The startup context bounds
	// New through the initial response handshake; after New returns, Close owns
	// shutdown so callers may cancel their startup context without killing tmux.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), path, args...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), cfg.cloneEnv()...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("tmux: create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("tmux: create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("tmux: create stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("tmux: start %q: %w", path, err)
	}

	tr := &stdioTransport{
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}
	client := newClient(cfg, tr, stdout, cmd)
	startup := &pendingCommand{line: cfg.initialCommandLine(), ch: make(chan responseResult, 1)}
	if err := client.registerPending(startup); err != nil {
		_ = client.Close(ctx)
		return nil, err
	}
	client.stderrDone = make(chan struct{})
	go client.drainStderr(stderr, client.stderrDone)
	client.readDone = make(chan struct{})
	go client.readLoop(context.Background(), client.transport, client.readDone)
	select {
	case result := <-startup.ch:
		if result.err != nil {
			_ = client.Close(ctx)
			return nil, fmt.Errorf("tmux: initial command: %w", result.err)
		}
	case <-ctx.Done():
		client.clearPending(startup)
		_ = client.Close(ctx)
		return nil, ctx.Err()
	}
	return client, nil
}

func newClient(cfg Options, tr transport, stdoutCloser io.Closer, cmd *exec.Cmd) *Client {
	return &Client{
		options:      cfg,
		transport:    tr,
		stdoutCloser: stdoutCloser,
		cmd:          cmd,
		events:       make(chan Notification, cfg.EventBuffer),
	}
}

// Exec sends a rendered tmux command and waits for its response block.
//
// Commands are serialized. If ctx is canceled while waiting for this command's
// response, the client is closed because a late response could otherwise be
// misassociated with a later command.
func (c *Client) Exec(ctx context.Context, command Command, args ...Arg) (Response, error) {
	return c.ExecLine(ctx, NewCommandLine(command, args...))
}

// ExecLine sends a rendered tmux command line and waits for its response block.
//
// Commands are serialized; see [Client.Exec] for cancellation semantics.
func (c *Client) ExecLine(ctx context.Context, line CommandLine) (Response, error) {
	rendered, err := line.String()
	if err != nil {
		return Response{}, err
	}
	return c.ExecRaw(ctx, rendered)
}

// ExecRaw sends one newline-framed tmux command line and waits for its response block.
//
// Commands are serialized; see [Client.Exec] for cancellation semantics.
func (c *Client) ExecRaw(ctx context.Context, line string) (Response, error) {
	if c == nil {
		return Response{}, ErrClosed
	}
	if err := validateRawLine(line); err != nil {
		return Response{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.closedError(); err != nil {
		return Response{}, err
	}
	pending := &pendingCommand{line: line, ch: make(chan responseResult, 1)}
	if err := c.registerPending(pending); err != nil {
		return Response{}, err
	}
	registered := true
	defer func() {
		if registered {
			c.clearPending(pending)
		}
	}()

	if err := c.transport.WriteLine(ctx, line); err != nil {
		c.clearPending(pending)
		registered = false
		return Response{}, fmt.Errorf("tmux: write command %q: %w", line, err)
	}

	select {
	case result := <-pending.ch:
		registered = false
		if result.err != nil {
			return Response{}, result.err
		}
		return result.response, nil
	case <-ctx.Done():
		registered = false
		c.clearPending(pending)
		c.abort(ctx.Err())
		return Response{}, ctx.Err()
	}
}

// Events returns an iterator over asynchronous tmux notifications.
//
// The underlying notification queue is bounded by [Options.EventBuffer]. When it
// is full, the client drops the oldest buffered notification it can observe so
// the stdout reader can continue draining tmux output. Iteration stops when the
// client closes, when the caller breaks from the range loop, or when ctx is
// canceled. A nil ctx is treated as [context.Background].
func (c *Client) Events(ctx context.Context) iter.Seq[Notification] {
	return func(yield func(Notification) bool) {
		if c == nil {
			return
		}
		if ctx == nil {
			ctx = context.Background()
		}
		for {
			select {
			case notification, ok := <-c.events:
				if !ok || !yield(notification) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
}

// DroppedNotifications returns the notification backpressure counter.
//
// The counter is incremented when the reader observes a full event buffer and
// cannot deliver without applying the drop policy. Concurrent consumers can make
// this an approximate pressure signal rather than an exact discard count.
func (c *Client) DroppedNotifications() uint64 {
	if c == nil {
		return 0
	}
	return c.droppedNotifications.Load()
}

// StderrTail returns the bounded stderr tail retained for diagnostics.
func (c *Client) StderrTail() []string {
	if c == nil {
		return nil
	}
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	return slices.Clone(c.stderrLines)
}

// Close detaches and releases the tmux control-mode subprocess.
func (c *Client) Close(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.closeMu.Lock()
	if c.cleanupDone {
		err := c.cleanupErr
		c.closeMu.Unlock()
		return err
	}
	c.cleanupDone = true
	c.closeMu.Unlock()

	alreadyClosed := c.closedError() != nil
	c.markClosed(ErrClosed)
	c.failPending(ErrClosed)

	var errs []error
	if c.transport != nil {
		if !alreadyClosed {
			c.writeMu.Lock()
			if err := c.transport.WriteLine(ctx, string(DetachClient)); err != nil && !errors.Is(err, ErrClosed) {
				errs = append(errs, fmt.Errorf("write detach-client: %w", err))
			}
			c.writeMu.Unlock()
		}
		if err := c.transport.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			errs = append(errs, fmt.Errorf("close stdin: %w", err))
		}
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, c.options.ShutdownTimeout)
	defer cancel()
	if err := c.waitProcess(shutdownCtx); err != nil {
		errs = append(errs, err)
	}
	if err := waitOptional(shutdownCtx, c.readDone, "stdout read loop"); err != nil {
		if c.stdoutCloser != nil {
			_ = c.stdoutCloser.Close()
		}
		errs = append(errs, err)
	}
	if err := waitOptional(shutdownCtx, c.stderrDone, "stderr drain"); err != nil {
		errs = append(errs, err)
	}
	err := errors.Join(errs...)
	c.closeMu.Lock()
	c.cleanupErr = err
	c.closeMu.Unlock()
	return err
}

func waitForCmd(cmd *exec.Cmd) <-chan error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return done
}

func (c *Client) readLoop(ctx context.Context, tr transport, done chan<- struct{}) {
	defer close(done)
	defer close(c.events)
	parser := &protocolParser{}
	for {
		line, err := tr.ReadLine(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if parserErr := parser.eof(); parserErr != nil {
					c.abort(parserErr)
					return
				}
				c.abort(io.EOF)
				return
			}
			c.abort(err)
			return
		}
		message, err := parser.feed(line)
		if err != nil {
			c.deliverEvent(Notification{Kind: "%protocol-error", Raw: line, Args: []string{err.Error()}})
			c.abort(err)
			return
		}
		switch message.kind {
		case protocolMessageResponse:
			c.deliverResponse(message.response)
		case protocolMessageNotification:
			c.deliverEvent(message.notification)
			if exit, ok := message.notification.Exit(); ok {
				c.abort(&ExitError{Reason: exit.Reason})
			}
		}
	}
}

func (c *Client) deliverResponse(response Response) {
	c.pendingMu.Lock()
	pending := c.pending
	if pending != nil {
		c.pending = nil
	}
	c.pendingMu.Unlock()
	if pending == nil {
		return
	}
	result := responseResult{response: response}
	if response.Error {
		result.err = &CommandError{Line: pending.line, Response: response}
	}
	pending.ch <- result
}

func (c *Client) deliverEvent(notification Notification) {
	select {
	case c.events <- notification:
		return
	default:
		// Buffer is full; drop the oldest notification to make room.
		select {
		case <-c.events:
			c.droppedNotifications.Add(1)
		default:
			// A consumer drained the buffer in the meantime.
		}
		// This write is now guaranteed to succeed as we are the sole writer.
		c.events <- notification
	}
}

func (c *Client) registerPending(p *pendingCommand) error {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if c.pending != nil {
		return fmt.Errorf("tmux: another command is already pending")
	}
	c.pending = p
	return nil
}

func (c *Client) clearPending(p *pendingCommand) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if c.pending == p {
		c.pending = nil
	}
}

func (c *Client) failPending(err error) {
	c.pendingMu.Lock()
	pending := c.pending
	if pending != nil {
		c.pending = nil
	}
	c.pendingMu.Unlock()
	if pending != nil {
		pending.ch <- responseResult{err: err}
	}
}

func (c *Client) abort(err error) {
	if err == nil {
		err = ErrClosed
	}
	c.markClosed(err)
	c.failPending(err)
}

func (c *Client) markClosed(err error) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed {
		return false
	}
	c.closed = true
	c.closeErr = err
	return true
}

func (c *Client) closedError() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if !c.closed {
		return nil
	}
	if c.closeErr != nil {
		return c.closeErr
	}
	return ErrClosed
}

func (c *Client) drainStderr(r io.Reader, done chan<- struct{}) {
	defer close(done)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		c.appendStderrLine(scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		c.appendStderrLine("stderr read error: " + err.Error())
	}
}

func (c *Client) appendStderrLine(line string) {
	if c.options.StderrLineLimit == 0 {
		return
	}
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	c.stderrLines = append(c.stderrLines, line)
	if limit := c.options.StderrLineLimit; len(c.stderrLines) > limit {
		copy(c.stderrLines, c.stderrLines[len(c.stderrLines)-limit:])
		c.stderrLines = c.stderrLines[:limit]
	}
}

func (c *Client) waitProcess(ctx context.Context) error {
	if c.cmd == nil {
		return nil
	}
	done := waitForCmd(c.cmd)
	select {
	case err := <-done:
		if err != nil && !isExpectedClosedProcessError(err) {
			return fmt.Errorf("tmux process exited: %w", err)
		}
		return nil
	case <-ctx.Done():
		if c.cmd != nil && c.cmd.Process != nil {
			if err := c.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return errors.Join(ctx.Err(), fmt.Errorf("kill tmux process: %w", err))
			}
			select {
			case err := <-done:
				if err != nil && !isExpectedClosedProcessError(err) {
					return errors.Join(ctx.Err(), fmt.Errorf("tmux process exited after kill: %w", err))
				}
			case <-time.After(time.Second):
				return errors.Join(ctx.Err(), fmt.Errorf("tmux process did not exit after kill"))
			}
		}
		return ctx.Err()
	}
}

func waitOptional(ctx context.Context, done <-chan struct{}, name string) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("tmux: wait for %s: %w", name, ctx.Err())
	}
}

func isExpectedClosedProcessError(err error) bool {
	if err == nil {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "signal: killed")
}
