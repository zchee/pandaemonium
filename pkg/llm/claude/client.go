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
	"slices"
	"sync"
	"time"

	"github.com/go-json-experiment/json"

	llm "github.com/zchee/pandaemonium/pkg/llm"
)

// rawMessageBuffer is a single-consumer, unbounded FIFO for stream-JSON data
// messages. It deliberately decouples readLoop from ReceiveResponse: the read
// loop must keep reading so it can route control_response traffic even when the
// application is slow to consume ordinary assistant/user/result messages.
//
// Queueing is the shared [llm.NotifyQueue]; this wrapper pins the
// claude-specific contract: push clones the line and reports io.ErrClosedPipe
// after close, and next returns (nil, nil) as the drained-EOF sentinel.
type rawMessageBuffer struct {
	q *llm.NotifyQueue[[]byte]
}

func newRawMessageBuffer() *rawMessageBuffer {
	return &rawMessageBuffer{q: llm.NewNotifyQueue[[]byte]()}
}

func (b *rawMessageBuffer) push(ctx context.Context, line []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !b.q.Push(slices.Clone(line)) {
		return io.ErrClosedPipe
	}
	return nil
}

func (b *rawMessageBuffer) close() {
	b.q.Close(nil)
}

func (b *rawMessageBuffer) next(ctx context.Context) ([]byte, error) {
	line, err := b.q.Next(ctx)
	if err != nil {
		if errors.Is(err, llm.ErrQueueClosed) {
			return nil, nil
		}
		return nil, err
	}
	return line, nil
}

// ClaudeSDKClient is a bidirectional interactive client for the claude CLI
// subprocess. It supports multi-turn conversation, hook dispatch, in-process
// MCP servers, and session forking.
//
// Create a client with [NewClient]; use [Query] for one-shot iteration.
//
// # Race-safety
//
// The transport field is a plain field guarded by the snapshot-as-arg +
// writeMu-symmetry pattern below. (pkg/llm/codex uses an atomic.Pointer for the
// equivalent field; this package instead serializes all transport access
// through writeMu/closeMu — either discipline is sound, so switching to
// atomic.Pointer here would be a valid refactor, not a regression.)
//
//   - start acquires closeMu, assigns c.transport = t, then launches
//     go c.readLoop(ctx, c.transport, c.cp, c.readDone) so the read goroutine
//     captures the transport as a goroutine argument and never touches
//     c.transport again.
//   - writeMessage acquires writeMu and reads c.transport under that lock;
//     returns &CLIConnectionError if nil.
//   - Close acquires closeMu, snapshots local copies, closes the transport
//     (before taking writeMu, so a blocked write unblocks via EPIPE — see
//     Close), then acquires writeMu and sets c.transport = nil, symmetric with
//     writeMessage's read.
//
//nolint:revive // ClaudeSDKClient is the established public API type name; renaming to SDKClient would break compatibility.
type ClaudeSDKClient struct {
	opts      *Options
	sessionID string

	// transport is the live subprocess transport. Its nil→non-nil assignment
	// happens in start, which runs under closeMu before readLoop exists and
	// before writeMessage is reachable, so no concurrent access exists at that
	// point. Thereafter it is read under writeMu (writes) and snapshot-captured
	// under closeMu (the readLoop goroutine argument); its non-nil→nil clear
	// happens under writeMu in Close. See race-safety documentation above.
	transport transport

	// cp is the control-protocol layer bound to this client's transport. It is
	// constructed in start under closeMu and passed to readLoop as a goroutine
	// argument (never read from inside the goroutine), mirroring the
	// snapshot-as-arg discipline used for transport.
	cp *controlProtocol

	writeMu sync.Mutex
	closeMu sync.Mutex

	// cmd is the live subprocess. Nil before start and after Close.
	cmd *exec.Cmd

	// cmdDone receives the subprocess exit error once cmd.Wait returns.
	cmdDone chan error

	// rawMessages receives raw stream-JSON lines from readLoop through an
	// unbounded in-process queue. Keeping the read loop off the consumer
	// channel prevents data-message backpressure from delaying control
	// responses such as initialize, interrupt, or set_model.
	rawMessages *rawMessageBuffer

	// readErr is set by readLoop before it closes readDone.
	readErr   error
	readErrMu sync.Mutex

	// readDone is closed by readLoop when it exits.
	readDone chan struct{}

	// stderrBuf is the bounded stderr ring buffer. Capacity mirrors the
	// 400-line drainStderr ring in pkg/llm/codex/client.go.
	stderrBuf *llm.LineBuffer

	// stderrDone is closed by drainStderr when it exits.
	stderrDone chan struct{}
}

// Query sends prompt to the claude CLI and returns when the prompt has been
// delivered. Call [ClaudeSDKClient.ReceiveResponse] to iterate the resulting
// messages.
//
// On the first call the subprocess is launched in streaming stdin mode and the
// initialize handshake is performed (see [ClaudeSDKClient.launchSubprocess]).
// Every prompt — including the first — is then sent as a JSON envelope on the
// subprocess stdin (never as a CLI flag). The subprocess remains alive between
// calls so subsequent calls write the next envelope for multi-turn
// conversation.
//
// This is the interactive counterpart to the package-level [Query] function.
func (c *ClaudeSDKClient) Query(ctx context.Context, prompt string) error {
	// Hold closeMu only for the launch/start phase (which mutates c.transport,
	// c.cp, c.cmd and must be serialized against Close), then RELEASE it before
	// the steady-state write. writeMessage does a blocking stdin.Write that does
	// not observe ctx; holding closeMu across it would let a hung subprocess
	// (full stdin pipe) wedge Close — which also takes closeMu — with no way to
	// run the kill sequence. Releasing closeMu first lets Close proceed and tear
	// down the transport, unblocking the stuck write. writeMessage's own writeMu
	// + nil-transport check (symmetric with Close's transport clear) preserves
	// the Close-race safety: a Query racing a Close surfaces as a
	// CLIConnectionError instead of a data race.
	//
	// NOTE: this bounds the steady-state hot path. launchSubprocess runs under
	// closeMu and itself performs the initialize handshake via a control write,
	// so a first-call hang during initialize is still possible — a much smaller
	// exposure (the initialize envelope is tiny and the pipe is not yet
	// pressured) tracked separately from this fix.
	c.closeMu.Lock()
	if c.transport == nil {
		// First call: launch the subprocess in streaming stdin mode and run
		// the initialize handshake before any prompt is sent.
		if err := c.launchSubprocess(ctx); err != nil {
			c.closeMu.Unlock()
			return err
		}
	}
	// Snapshot the session ID under closeMu, then release it before the write.
	sessionID := c.sessionID
	c.closeMu.Unlock()

	// Encode the prompt as a stream-JSON user envelope and write it to stdin.
	// writeMessage acquires writeMu internally; it reads c.transport under
	// writeMu and returns a CLIConnectionError if Close has cleared it.
	envelope, err := userEnvelope(sessionID, prompt)
	if err != nil {
		return err
	}
	return c.writeMessage(ctx, envelope)
}

// userEnvelope marshals a single user-turn stream-JSON envelope for the claude
// CLI stdin, mirroring the upstream Python SDK client.py wire shape:
//
//	{"type":"user","session_id":"<id>","message":{"role":"user","content":"<prompt>"},"parent_tool_use_id":null}
//
// sessionID is emitted as-is. parent_tool_use_id always serializes as JSON null
// at the top of a user turn.
//
// Divergence from upstream client.py:212, which hard-codes "session_id": "":
// here sessionID is c.sessionID, which is empty for a normal client (so the
// envelope is byte-identical to upstream) and non-empty ONLY for a client
// produced by Fork — where it is the forked session ID and the subprocess was
// launched with --resume <forkedID>. Emitting the live ID on the fork path is
// deliberate (it keeps the envelope consistent with the resumed session) and
// must not be "corrected" to a literal "" to match upstream.
func userEnvelope(sessionID, prompt string) ([]byte, error) {
	type userMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	envelope := struct {
		Type            string      `json:"type"`
		SessionID       string      `json:"session_id"`
		Message         userMessage `json:"message"`
		ParentToolUseID *string     `json:"parent_tool_use_id"`
	}{
		Type:            "user",
		SessionID:       sessionID,
		Message:         userMessage{Role: "user", Content: prompt},
		ParentToolUseID: nil,
	}
	return json.Marshal(envelope)
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
	c.closeMu.Unlock()

	return func(yield func(Message, error) bool) {
		if rawMessages == nil {
			yield(nil, &CLIConnectionError{Message: "no active query; call Query first"})
			return
		}

		for {
			line, err := rawMessages.next(ctx)
			if err != nil {
				yield(nil, err)
				return
			}
			if line == nil {
				// readLoop hit EOF or error after all queued data was delivered.
				if rerr := c.finalReadErr(); rerr != nil {
					yield(nil, rerr)
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
		}
	}
}

// finalReadErr returns readLoop's terminal error after the buffer drained, or
// nil when the loop ended on a clean EOF. It is read under readErrMu so the
// value is consistent with the readLoop writer.
func (c *ClaudeSDKClient) finalReadErr() error {
	c.readErrMu.Lock()
	defer c.readErrMu.Unlock()
	if c.readErr != nil && !errors.Is(c.readErr, io.EOF) {
		return c.readErr
	}
	return nil
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
		return nil, &CLIConnectionError{Message: "\"Fork\" requires Options.SessionStore to be set"}
	}

	c.closeMu.Lock()
	parentSessionID := c.sessionID
	c.closeMu.Unlock()

	if parentSessionID == "" {
		return nil, &CLIConnectionError{Message: "\"Fork\" parent client has no active session ID"}
	}

	// Snapshot history up to fromMessageID in the store. The forked session
	// receives a new unique ID and includes only messages up to and including
	// the one matching fromMessageID (or all messages if fromMessageID is "").
	forked, err := c.opts.SessionStore.Fork(ctx, parentSessionID, fromMessageID)
	if err != nil {
		return nil, fmt.Errorf("\"Fork\": %w", err)
	}

	// Clone the options so the forked client is independently configurable:
	// clone gives the child its own slice/map containers, so mutating the
	// child's Hooks/MCPServers/Env/etc. never reaches the parent (a shallow
	// struct copy would alias them).
	childOpts := c.opts.clone()

	// The child's sessionID drives --resume in buildLaunchArgs so the CLI
	// subprocess loads the forked session when it starts.
	child := &ClaudeSDKClient{
		opts:      childOpts,
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
// the write-symmetric clear in pkg/llm/codex/client.go's Close (commit 8c16376).
//
//nolint:unparam // Close() error is the idiomatic io.Closer-style signature kept for API compatibility; the current teardown cannot fail.
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
	cp := c.cp
	c.cmd = nil
	c.cmdDone = nil

	// Close the transport BEFORE acquiring writeMu. A writeMessage in flight
	// (e.g. a Query whose blocking stdin.Write is stuck on a full pipe to a hung
	// subprocess) holds writeMu and does not observe ctx; if Close acquired
	// writeMu first it would deadlock waiting for that write to finish. Closing
	// the transport here closes stdin, so the stuck Write fails with EPIPE,
	// returns, and releases writeMu — letting the writeMu section below proceed.
	//
	// Closing the transport while a concurrent writeMessage may still be calling
	// transport.WriteJSON is safe: stdioTransport.WriteJSON tolerates a closed
	// stdin (the Write simply errors), and transport.Close is idempotent.
	if tr != nil {
		_ = tr.Close() // ignore: transport close errors are non-fatal during shutdown.
	}

	// Clear c.transport inside writeMu — write-symmetric clear with writeMessage,
	// which reads c.transport under writeMu. This is the only critical section
	// where transport transitions from non-nil to nil. (We diverge from
	// pkg/llm/codex/client.go's Close, which closes the transport INSIDE its write lock;
	// closing it before the lock is what bounds Close against a hung write.)
	c.writeMu.Lock()
	c.transport = nil
	c.writeMu.Unlock()
	c.closeMu.Unlock()

	// Cancel any in-flight inbound control-request handlers so they don't
	// outlive the session: the read loop that would route their responses is
	// shutting down with the transport. Done after releasing closeMu since the
	// cancel funcs are independent of the locks above.
	if cp != nil {
		cp.closeInflight()
		// Wait (bounded) for the cancelled handler goroutines to actually exit so
		// a ctx-respecting handler does not leak past Close. Budget 500ms, matching
		// the readDone/stderrDone drains below; a ctx-ignoring handler cannot stall
		// Close beyond this.
		cp.waitInflight(500 * time.Millisecond)
		// Fail any outbound control requests still awaiting a response so a
		// Close that races ahead of readLoop noticing the closed transport does
		// not leave callers blocked until their timeout. Idempotent with the
		// readLoop error-path call.
		cp.failPending(nil)
	}

	// Close registered MCP servers deterministically (MCPServer.Close contract).
	// c.opts is set once at construction and never mutated, so reading it here
	// without a lock is safe.
	if c.opts != nil {
		for _, srv := range c.opts.MCPServers {
			_ = srv.Close() // ignore: best-effort MCP server cleanup on shutdown.
		}
	}

	// Signal and wait for the subprocess.
	terminateProcess(cmd, cmdDone)

	// Wait for readLoop and drainStderr to exit — mirrors the Close sequence in pkg/llm/codex/client.go.
	// Budget 500ms each, matching the codex drain timeout.
	_ = llm.WaitUntil(readDone, time.Now().Add(500*time.Millisecond))
	_ = llm.WaitUntil(stderrDone, time.Now().Add(500*time.Millisecond))
	return nil
}

// terminateProcess signals the subprocess to stop and waits for it to exit,
// escalating to Kill after a 2s grace period. It is a no-op when cmd or its
// process is nil (e.g. a FakeCLI-backed client with no real subprocess).
func terminateProcess(cmd *exec.Cmd, cmdDone chan error) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	llm.TerminateCommand(cmd, cmdDone, os.Interrupt, time.Now().Add(2*time.Second), time.Time{})
}

// ── unexported infrastructure ────────────────────────────────────────────────

// start wires up the readLoop and drainStderr goroutines for an active transport.
//
// It MUST be called with c.closeMu held so that the transport snapshot
// captured by readLoop is consistent with c.transport — mirrors
// Start in pkg/llm/codex/client.go (snapshot-as-arg discipline).
//
// cmd and cmdDone may be nil for test transports that do not back a real
// subprocess (e.g. FakeCLI). stderrR may be nil; if so, stderrDone is
// closed immediately.
func (c *ClaudeSDKClient) start(ctx context.Context, t transport, cmd *exec.Cmd, cmdDone chan error, stderrR io.Reader) {
	// Assigning c.transport and c.cp here (rather than under writeMu) is safe:
	// start runs with closeMu held, before readLoop is launched and before
	// writeMessage can be called, so there is no concurrent reader yet. The
	// writeMu discipline guards only the steady-state reads and the Close clear.
	c.transport = t
	c.cmd = cmd
	c.cmdDone = cmdDone
	c.rawMessages = newRawMessageBuffer()
	if c.stderrBuf == nil {
		c.stderrBuf = llm.NewLineBuffer(maxStderrLines)
	}
	c.readDone = make(chan struct{})
	c.stderrDone = make(chan struct{})

	// Construct the control protocol bound to writeMessage (the writeMu-guarded
	// writer) so the control layer reuses the single write-exclusion discipline.
	c.cp = newControlProtocol(c.opts, c.writeMessage)

	// Index in-process MCP servers by name before initialize so the CLI's
	// mcp_message control requests can be routed back to them.
	c.cp.registerMCPServers()

	if stderrR != nil {
		go c.drainStderr(stderrR, c.stderrDone)
	} else {
		// No subprocess stderr — signal done immediately.
		close(c.stderrDone)
	}

	// Launch readLoop with a snapshot of c.transport and c.cp captured under
	// closeMu. The goroutine receives both as arguments and never reads
	// c.transport or c.cp directly — this is the snapshot-as-arg discipline that
	// prevents the Close/readMessage data race fixed in pkg/llm/codex commit 8c16376.
	go c.readLoop(ctx, c.transport, c.cp, c.rawMessages, c.readDone) // snapshot under closeMu
}

// launchSubprocess resolves the CLI binary, builds launch args, starts the
// subprocess, and calls start under closeMu. After start wires up the readLoop
// goroutine and control protocol, it performs the initialize handshake so the
// CLI is ready to receive prompt envelopes on stdin.
//
// Ordering is connect → start (read loop running) → initialize: the
// control_response for initialize is routed by readLoop, so readLoop MUST be
// running before initialize sends its request. initialize writes via
// cp.writeFn (= c.writeMessage, guarded by writeMu), so it interleaves safely
// with the running read goroutine.
//
// If initialize fails the subprocess is up but unusable; the error is returned
// and the caller should Close the client to release the subprocess and
// goroutines (start has already set c.transport, c.cp, and launched readLoop).
//
// Called by Query. Not used in Phase A tests (which inject a FakeCLI transport
// via start directly and never call Query, so they never hit initialize).
func (c *ClaudeSDKClient) launchSubprocess(ctx context.Context) error {
	cliPath, err := discoverCLI(c.opts)
	if err != nil {
		return err
	}
	args, err := buildLaunchArgs(cliPath, c.opts, c.sessionID)
	if err != nil {
		return err
	}
	//nolint:gosec // G204: launching the resolved claude CLI (args[0] from discoverCLI) with SDK-built flags from typed Options is the intended subprocess behavior.
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var cwd string
	if c.opts != nil {
		cwd = c.opts.Cwd
	}
	if cwd != "" {
		cmd.Dir = cwd
	}
	// Build the subprocess env: inherit the parent environment minus CLAUDECODE,
	// inject the SDK identity and PWD, and let opts.Env override. See
	// buildSubprocessEnv (client_env.go).
	cmd.Env = buildSubprocessEnv(os.Environ(), c.opts, cwd)

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

	cmdDone := llm.WaitForCommand(cmd)
	t := newStdioTransport(stdin, bufio.NewReader(stdout))
	c.start(ctx, t, cmd, cmdDone, stderr)

	// Perform the initialize handshake now that readLoop is running and c.cp
	// is set. initialize writes the control request via c.writeMessage and
	// blocks until readLoop routes the matching control_response.
	if _, err := c.cp.initialize(ctx); err != nil {
		return err
	}
	return nil
}

// writeMessage encodes data and writes it to the transport under writeMu.
//
// Returns CLIConnectionError if the transport is nil (i.e. after Close).
// This is the symmetric half of the Close pattern: both writeMessage and
// Close access c.transport under writeMu, so they cannot interleave.
// (Mirrors writeMessage in pkg/llm/codex/client.go.)
func (c *ClaudeSDKClient) writeMessage(ctx context.Context, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.transport == nil {
		return &CLIConnectionError{Message: "\"claude\" is not running"}
	}
	return c.transport.WriteJSON(ctx, data)
}

// readLoop reads stream-JSON lines from t (a snapshot of c.transport captured
// at start time) and pushes them to c.rawMessages.
//
// The goroutine argument t MUST be the snapshot captured under closeMu — it
// never reads c.transport directly. This is the core of the race-safety
// discipline from pkg/llm/codex commit 8c16376.
func (c *ClaudeSDKClient) readLoop(ctx context.Context, t transport, cp *controlProtocol, rawMessages *rawMessageBuffer, done chan<- struct{}) {
	defer close(done)
	for {
		line, err := t.ReadJSON(ctx)
		if err != nil {
			// Propagate the error so Phase C's ReceiveResponse can surface it.
			c.readErrMu.Lock()
			c.readErr = err
			c.readErrMu.Unlock()
			// Fail any in-flight outbound control requests so they return a
			// CLIConnectionError immediately instead of stalling for their full
			// timeout. This covers a clean EOF (subprocess exit) as well as a
			// transport error, mirroring upstream's read-loop reaping. cp is the
			// snapshot argument, never c.cp directly.
			if cp != nil {
				cp.failPending(err)
			}
			// Closing the unbounded buffer unblocks any pending ReceiveResponse after
			// queued data messages have been delivered.
			rawMessages.close()
			return
		}
		// Intercept control-protocol messages before the regular message path.
		// route returns consumed=true for control messages it handled; only
		// non-control lines fall through to rawMessages. cp is the goroutine
		// argument captured under closeMu, never c.cp directly.
		if cp != nil {
			if consumed, _ := cp.route(ctx, line); consumed {
				continue
			}
		}
		if err := rawMessages.push(ctx, line); err != nil {
			c.readErrMu.Lock()
			c.readErr = err
			c.readErrMu.Unlock()
			rawMessages.close()
			return
		}
	}
}

// drainStderr reads lines from r into the bounded stderr ring buffer.
//
// Mirrors drainStderr in pkg/llm/codex/client.go; the ring retains the
// newest maxStderrLines lines as the subprocess diagnostic tail.
func (c *ClaudeSDKClient) drainStderr(r io.Reader, done chan<- struct{}) {
	defer close(done)
	buf := c.stderrBuf
	if buf == nil {
		// Zero-value clients (tests) reach drainStderr without start; the
		// launched path initializes the buffer in start under closeMu.
		buf = llm.NewLineBuffer(maxStderrLines)
		c.stderrBuf = buf
	}
	llm.DrainLines(r, func(line string) {
		buf.Append(line)
	})
}

// maxStderrLines bounds the retained stderr tail ring.
const maxStderrLines = 400
