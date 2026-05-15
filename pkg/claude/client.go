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

package claude

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// rawMessageQueueSize is the capacity of the rawMessages channel. Sized to
// accommodate a typical burst of stream-JSON lines without blocking readLoop.
const rawMessageQueueSize = 256

// ClaudeSDKClient is a bidirectional interactive client for the claude CLI
// subprocess. It supports multi-turn conversation, hook dispatch, in-process
// MCP servers, and session forking.
//
// Create a client with [NewClient]; use [Query] for one-shot iteration.
//
// # Race-safety
//
// The transport field is a plain field (not atomic.Pointer), following the
// snapshot-as-arg + writeMu-symmetry pattern from pkg/codex commit 8c16376:
//
//   - start acquires closeMu, assigns c.transport = t, then launches
//     go c.readLoop(ctx, c.transport, c.readDone) so the read goroutine
//     captures the transport as a goroutine argument and never touches
//     c.transport again. (pkg/codex/client.go:244)
//   - writeMessage acquires writeMu and reads c.transport under that lock;
//     returns &CLIConnectionError if nil.
//   - Close acquires closeMu, snapshots local copies, then acquires writeMu
//     and sets c.transport = nil inside the critical section, symmetric with
//     writeMessage. (pkg/codex/client.go:265-271)
//
// This pattern was validated across pkg/codex commits 7145a93, b56b072, and
// 8c16376 and MUST NOT be replaced with atomic.Pointer.
type ClaudeSDKClient struct {
	opts      *Options
	sessionID string

	// transport is the live subprocess transport. Accessed only under writeMu
	// for writes, and snapshot-captured under closeMu for the readLoop goroutine.
	// See race-safety documentation above.
	transport transport

	writeMu sync.Mutex
	closeMu sync.Mutex

	// cmd is the live subprocess. Nil before start and after Close.
	cmd *exec.Cmd

	// cmdDone receives the subprocess exit error once cmd.Wait returns.
	cmdDone chan error

	// rawMessages receives raw stream-JSON lines from readLoop.
	// Consumed by Phase C's ReceiveResponse.
	rawMessages chan []byte

	// readErr is set by readLoop before it closes readDone.
	readErr   error
	readErrMu sync.Mutex

	// readDone is closed by readLoop when it exits.
	readDone chan struct{}

	// stderrLines is the bounded stderr ring buffer. Protected by stderrMu.
	// Capacity mirrors pkg/codex/client.go:737 (400-line ring).
	stderrMu    sync.Mutex
	stderrLines []string

	// stderrDone is closed by drainStderr when it exits.
	stderrDone chan struct{}
}

// Query sends prompt to the claude CLI and returns when the prompt has been
// delivered. Call [ClaudeSDKClient.ReceiveResponse] to iterate the resulting
// messages.
//
// On the first call the subprocess is launched in interactive stdin mode
// (without --print); subsequent calls write the next prompt to its stdin for
// multi-turn conversation. The subprocess remains alive between calls.
//
// This is the interactive counterpart to the package-level [Query] function.
func (c *ClaudeSDKClient) Query(ctx context.Context, prompt string) error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.transport == nil {
		// First call: start the subprocess in interactive stdin mode.
		// Empty prompt omits --print so the CLI reads input from stdin.
		if err := c.launchSubprocess(ctx, ""); err != nil {
			return err
		}
	}
	// Write the prompt to stdin. writeMessage acquires writeMu internally.
	// Calling writeMessage while holding closeMu is safe: both Close and
	// writeMessage acquire closeMu→writeMu in the same order.
	return c.writeMessage(ctx, []byte(prompt))
}

// ReceiveResponse returns an iterator over the [Message] values streamed by the
// claude CLI in response to the last [ClaudeSDKClient.Query] call. The iterator
// stops after delivering the terminal [ResultMessage] or when ctx is cancelled.
//
// rawMessages and readDone are captured once under closeMu so they are
// consistent with the transport state at the time ReceiveResponse is called.
func (c *ClaudeSDKClient) ReceiveResponse(ctx context.Context) iter.Seq2[Message, error] {
	c.closeMu.Lock()
	rawMessages := c.rawMessages
	readDone := c.readDone
	c.closeMu.Unlock()

	return func(yield func(Message, error) bool) {
		if rawMessages == nil {
			yield(nil, &CLIConnectionError{Message: "no active query; call Query first"})
			return
		}

		for {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return

			case line := <-rawMessages:
				if line == nil {
					// nil sentinel: readLoop hit EOF or error before ResultMessage.
					c.readErrMu.Lock()
					err := c.readErr
					c.readErrMu.Unlock()
					if err != nil && !errors.Is(err, io.EOF) {
						yield(nil, err)
					}
					return
				}
				msg, parseErr := parseMessage(append(line, '\n'))
				if parseErr != nil {
					if !yield(nil, parseErr) {
						return
					}
					continue
				}
				if msg == nil {
					continue // blank line
				}
				if !yield(msg, nil) {
					return
				}
				if _, ok := msg.(ResultMessage); ok {
					return
				}

			case <-readDone:
				// readLoop exited — drain any lines already buffered in the channel.
			drain:
				for {
					select {
					case line := <-rawMessages:
						if line == nil {
							break drain
						}
						msg, parseErr := parseMessage(append(line, '\n'))
						if parseErr != nil {
							yield(nil, parseErr)
							return
						}
						if msg == nil {
							continue
						}
						if !yield(msg, nil) {
							return
						}
						if _, ok := msg.(ResultMessage); ok {
							return
						}
					default:
						break drain
					}
				}
				c.readErrMu.Lock()
				err := c.readErr
				c.readErrMu.Unlock()
				if err != nil && !errors.Is(err, io.EOF) {
					yield(nil, err)
				}
				return
			}
		}
	}
}

// Interrupt sends SIGINT to the claude CLI subprocess, requesting that it
// cancel the current operation. Returns CLIConnectionError if no subprocess
// is running.
func (c *ClaudeSDKClient) Interrupt(ctx context.Context) error {
	_ = ctx
	c.closeMu.Lock()
	cmd := c.cmd
	c.closeMu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return &CLIConnectionError{Message: "no active subprocess to interrupt"}
	}
	return cmd.Process.Signal(os.Interrupt)
}

// Fork creates a new [ClaudeSDKClient] whose conversation history is branched
// from fromMessageID in the current session. The parent client continues
// unaffected; the forked client has its own independent transport.
//
// Fork requires a non-nil [Options].SessionStore. The store's Fork method is
// called to snapshot the parent session's history up to fromMessageID; the
// resulting forked session ID is stored on the child client so its subprocess
// launches with --resume <forkedSessionID>.
//
// The parent's transport is never touched (AC-i5): the forked client starts
// its own subprocess on the first call to [ClaudeSDKClient.Query].
func (c *ClaudeSDKClient) Fork(ctx context.Context, fromMessageID string) (*ClaudeSDKClient, error) {
	if c.opts == nil || c.opts.SessionStore == nil {
		return nil, &CLIConnectionError{Message: "Fork requires Options.SessionStore to be set"}
	}

	c.closeMu.Lock()
	parentSessionID := c.sessionID
	c.closeMu.Unlock()

	if parentSessionID == "" {
		return nil, &CLIConnectionError{Message: "Fork: parent client has no active session ID"}
	}

	// Snapshot history up to fromMessageID in the store. The forked session
	// receives a new unique ID and includes only messages up to and including
	// the one matching fromMessageID (or all messages if fromMessageID is "").
	forked, err := c.opts.SessionStore.Fork(ctx, parentSessionID, fromMessageID)
	if err != nil {
		return nil, fmt.Errorf("Fork: %w", err)
	}

	// Shallow-copy the options so the forked client is independently
	// configurable without affecting the parent. Options is frozen at Phase 0;
	// no fields will be added that would require a deep copy.
	childOpts := *c.opts

	// The child's sessionID drives --resume in buildLaunchArgs so the CLI
	// subprocess loads the forked session when it starts.
	child := &ClaudeSDKClient{
		opts:      &childOpts,
		sessionID: forked.ID,
	}
	return child, nil
}

// Close terminates the claude CLI subprocess and releases all resources
// associated with this client, including any registered in-process MCP servers.
//
// Close is idempotent; subsequent calls return nil.
//
// The transport is cleared inside the writeMu critical section, mirroring
// pkg/codex/client.go:265-271 (write-symmetric clear from commit 8c16376).
func (c *ClaudeSDKClient) Close() error {
	c.closeMu.Lock()
	if c.transport == nil {
		c.closeMu.Unlock()
		return nil
	}

	// Snapshot local references under closeMu before releasing it.
	cmd := c.cmd
	cmdDone := c.cmdDone
	tr := c.transport
	readDone := c.readDone
	stderrDone := c.stderrDone
	c.cmd = nil
	c.cmdDone = nil

	// Clear c.transport inside writeMu — write-symmetric clear (pkg/codex/client.go:265-271).
	// writeMessage also reads c.transport under writeMu, so this is the only
	// critical section where transport transitions from non-nil to nil.
	c.writeMu.Lock()
	c.transport = nil
	if tr != nil {
		_ = tr.Close()
	}
	c.writeMu.Unlock()
	c.closeMu.Unlock()

	// Signal and wait for the subprocess.
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
		done := cmdDone
		if done == nil {
			done = waitForCmd(cmd)
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}

	// Wait for readLoop and drainStderr to exit — mirrors pkg/codex/client.go:293,297.
	// Budget 500ms each, matching the codex drain timeout.
	if readDone != nil {
		select {
		case <-readDone:
		case <-time.After(500 * time.Millisecond):
		}
	}
	if stderrDone != nil {
		select {
		case <-stderrDone:
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil
}

// ── unexported infrastructure ────────────────────────────────────────────────

// start wires up the readLoop and drainStderr goroutines for an active transport.
//
// It MUST be called with c.closeMu held so that the transport snapshot
// captured by readLoop is consistent with c.transport — mirrors
// pkg/codex/client.go:244 (snapshot-as-arg discipline).
//
// cmd and cmdDone may be nil for test transports that do not back a real
// subprocess (e.g. FakeCLI). stderrR may be nil; if so, stderrDone is
// closed immediately.
func (c *ClaudeSDKClient) start(ctx context.Context, t transport, cmd *exec.Cmd, cmdDone chan error, stderrR io.Reader) {
	c.transport = t
	c.cmd = cmd
	c.cmdDone = cmdDone
	c.rawMessages = make(chan []byte, rawMessageQueueSize)
	c.readDone = make(chan struct{})
	c.stderrDone = make(chan struct{})

	if stderrR != nil {
		go c.drainStderr(stderrR, c.stderrDone)
	} else {
		// No subprocess stderr — signal done immediately.
		close(c.stderrDone)
	}

	// Launch readLoop with a snapshot of c.transport captured under closeMu.
	// The goroutine receives the transport as an argument and never reads
	// c.transport directly — this is the snapshot-as-arg discipline that
	// prevents the Close/readMessage data race fixed in pkg/codex commit 8c16376.
	go c.readLoop(ctx, c.transport, c.readDone) // snapshot under closeMu
}

// launchSubprocess resolves the CLI binary, builds launch args, starts the
// subprocess, and calls start under closeMu. Returns with closeMu still held
// so the caller can check c.transport atomically.
//
// Called by Phase C's Query implementation. Not used in Phase A tests (which
// inject a FakeCLI transport via start directly).
func (c *ClaudeSDKClient) launchSubprocess(ctx context.Context, prompt string) error {
	cliPath, err := discoverCLI(c.opts)
	if err != nil {
		return err
	}
	args := buildLaunchArgs(cliPath, prompt, c.opts, c.sessionID)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if c.opts != nil && c.opts.Cwd != "" {
		cmd.Dir = c.opts.Cwd
	}
	cmd.Env = os.Environ()
	if c.opts != nil {
		for k, v := range c.opts.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return &CLIConnectionError{Message: fmt.Sprintf("create stdin pipe: %v", err)}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &CLIConnectionError{Message: fmt.Sprintf("create stdout pipe: %v", err)}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return &CLIConnectionError{Message: fmt.Sprintf("create stderr pipe: %v", err)}
	}

	if err := cmd.Start(); err != nil {
		return &CLIConnectionError{Message: fmt.Sprintf("start claude CLI %q: %v", cliPath, err)}
	}

	cmdDone := waitForCmd(cmd)
	t := &stdioTransport{
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}
	c.start(ctx, t, cmd, cmdDone, stderr)
	return nil
}

// writeMessage encodes data and writes it to the transport under writeMu.
//
// Returns CLIConnectionError if the transport is nil (i.e. after Close).
// This is the symmetric half of the Close pattern: both writeMessage and
// Close access c.transport under writeMu, so they cannot interleave.
// (pkg/codex/client.go:637-648)
func (c *ClaudeSDKClient) writeMessage(ctx context.Context, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.transport == nil {
		return &CLIConnectionError{Message: "CLI is not running"}
	}
	return c.transport.WriteJSON(ctx, data)
}

// readLoop reads stream-JSON lines from t (a snapshot of c.transport captured
// at start time) and pushes them to c.rawMessages.
//
// The goroutine argument t MUST be the snapshot captured under closeMu — it
// never reads c.transport directly. This is the core of the race-safety
// discipline from pkg/codex commit 8c16376 (pkg/codex/client.go:244).
func (c *ClaudeSDKClient) readLoop(ctx context.Context, t transport, done chan<- struct{}) {
	defer close(done)
	for {
		line, err := t.ReadJSON(ctx)
		if err != nil {
			// Propagate the error so Phase C's ReceiveResponse can surface it.
			c.readErrMu.Lock()
			c.readErr = err
			c.readErrMu.Unlock()
			// Drain the rawMessages channel to unblock any pending ReceiveResponse.
			// A nil sentinel signals EOF/error to the consumer.
			select {
			case c.rawMessages <- nil:
			default:
			}
			return
		}
		select {
		case c.rawMessages <- line:
		case <-ctx.Done():
			c.readErrMu.Lock()
			c.readErr = ctx.Err()
			c.readErrMu.Unlock()
			return
		}
	}
}

// drainStderr reads lines from r into a bounded ring buffer so that
// ProcessError.StderrTail is populated on subprocess crash.
//
// Mirrors pkg/codex/client.go:737. The ring capacity is 400 lines;
// stderrTail(40) returns the last 40 for ProcessError, matching the codex
// pattern at pkg/codex/client.go:657.
func (c *ClaudeSDKClient) drainStderr(r io.Reader, done chan<- struct{}) {
	defer close(done)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		c.stderrMu.Lock()
		c.stderrLines = append(c.stderrLines, line)
		if len(c.stderrLines) > 400 {
			copy(c.stderrLines, c.stderrLines[len(c.stderrLines)-400:])
			c.stderrLines = c.stderrLines[:400]
		}
		c.stderrMu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		c.stderrMu.Lock()
		c.stderrLines = append(c.stderrLines, "stderr read error: "+err.Error())
		if len(c.stderrLines) > 400 {
			copy(c.stderrLines, c.stderrLines[len(c.stderrLines)-400:])
			c.stderrLines = c.stderrLines[:400]
		}
		c.stderrMu.Unlock()
	}
}

// stderrTail returns the last limit lines from the stderr ring buffer as a
// single string. Mirrors pkg/codex/client.go:761.
func (c *ClaudeSDKClient) stderrTail(limit int) string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	if limit > len(c.stderrLines) {
		limit = len(c.stderrLines)
	}
	return strings.Join(c.stderrLines[len(c.stderrLines)-limit:], "\n")
}

// waitForCmd starts a goroutine that calls cmd.Wait and returns a channel that
// receives the exit error once the process exits.
func waitForCmd(cmd *exec.Cmd) chan error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		close(done)
	}()
	return done
}
