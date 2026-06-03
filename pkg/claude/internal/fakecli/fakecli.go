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

// Package fakecli provides a hermetic transport double for the claude CLI
// subprocess, used by pkg/claude tests that do not require a real claude binary.
//
// A FakeCLI serves scripted [Frame] sequences: each WriteJSON call from the
// client advances to the next frame, and the frame's Lines are queued for
// subsequent ReadJSON calls.  Written payloads are recorded and can be
// inspected after the fact with [FakeCLI.Written].
//
// Tests import this package and pass the returned *FakeCLI directly as a
// transport value inside tests in the pkg/claude package (same-package access
// to the unexported transport interface).
package fakecli

import (
	"context"
	"io"
	"slices"
	"sync"
	"testing"
)

// Frame is one scripted exchange in a [FakeCLI] session.
//
// When the FakeCLI receives a WriteJSON call corresponding to this frame, it
// enqueues Lines into its read buffer so that subsequent ReadJSON calls return
// them in order.
type Frame struct {
	// Lines is the sequence of raw stream-JSON lines (without trailing newline)
	// that the FakeCLI emits in response to the corresponding WriteJSON call.
	Lines []string
}

// writeHook is a bidirectional-scripting registration created by
// [FakeCLI.OnWrite]: when a WriteJSON payload satisfies matcher, the lines from
// respond() are injected into the read buffer.
type writeHook struct {
	matcher func([]byte) bool
	respond func() []string
}

// FakeCLI is a hermetic transport double that implements the pkg/claude
// transport interface (Close + WriteJSON + ReadJSON) without launching a
// real subprocess.
//
// Create with [New]; close automatically via t.Cleanup.
type FakeCLI struct {
	t      *testing.T
	script []Frame

	mu       sync.Mutex
	written  [][]byte // copies of payloads received via WriteJSON
	frameIdx int      // index of next frame to serve

	// onWrite holds bidirectional-scripting hooks registered via OnWrite. When
	// a WriteJSON payload matches a hook's matcher, the hook's respond lines are
	// injected into the read buffer. Guarded by mu.
	onWrite []writeHook

	// lines is a buffered channel of raw lines queued from script frames or
	// injected directly via Inject / OnWrite.
	lines chan []byte

	// closed is closed by Close to unblock ReadJSON.
	closed    chan struct{}
	closeOnce sync.Once
}

// New returns a new FakeCLI that will serve the given script.
//
// Each element in script corresponds to one WriteJSON call.  When that call
// arrives, the FakeCLI enqueues the frame's Lines so ReadJSON returns them.
// A nil or empty script is valid (FakeCLI accepts writes but emits nothing).
//
// t.Cleanup is registered to close the FakeCLI at test end.
func New(t *testing.T, script []Frame) *FakeCLI {
	t.Helper()
	f := &FakeCLI{
		t:      t,
		script: script,
		lines:  make(chan []byte, 1024),
		closed: make(chan struct{}),
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// Close signals EOF to any pending ReadJSON call and prevents further writes.
// Idempotent; t.Cleanup calls it automatically.
func (f *FakeCLI) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

// WriteJSON records p and enqueues the next frame's lines, if any.
//
// This satisfies the transport.WriteJSON signature and is safe for concurrent
// use, though only one goroutine should call WriteJSON at a time in practice
// (protected by ClaudeSDKClient.writeMu).
func (f *FakeCLI) WriteJSON(_ context.Context, p []byte) error {
	select {
	case <-f.closed:
		return io.ErrClosedPipe
	default:
	}

	f.mu.Lock()
	payload := slices.Clone(p)
	f.written = append(f.written, payload)
	idx := f.frameIdx
	if idx < len(f.script) {
		f.frameIdx++
	}
	// Snapshot the matching OnWrite hooks (by matcher) under mu so the hook
	// slice is read consistently, but invoke their respond closures AFTER
	// releasing mu. respond may call back into FakeCLI accessors such as
	// Written(), which re-acquire mu; running it under mu would self-deadlock.
	var matched []func() []string
	for _, h := range f.onWrite {
		if h.matcher(payload) {
			matched = append(matched, h.respond)
		}
	}
	f.mu.Unlock()

	// Enqueue this frame's lines for ReadJSON. Each line gets a trailing
	// newline so ReadJSON callers see well-formed stream-JSON lines.
	if idx < len(f.script) {
		if err := f.pushLines(f.script[idx].Lines); err != nil {
			return err
		}
	}
	// Invoke matched OnWrite responders (in registration order) and enqueue
	// their lines after the frame lines.
	for _, respond := range matched {
		if err := f.pushLines(respond()); err != nil {
			return err
		}
	}
	return nil
}

// pushLines enqueues raw lines into the read buffer, appending a trailing
// newline to each so ReadJSON callers see well-formed stream-JSON lines.
// It returns io.ErrClosedPipe if the FakeCLI is closed before all lines are
// enqueued.
func (f *FakeCLI) pushLines(lines []string) error {
	for _, line := range lines {
		select {
		case f.lines <- []byte(line + "\n"):
		case <-f.closed:
			return io.ErrClosedPipe
		}
	}
	return nil
}

// Inject enqueues raw stream-JSON lines into the read buffer immediately,
// without waiting for a WriteJSON call. This lets a test script the case where
// the CLI sends data (e.g. an inbound control_request) before the SDK writes.
//
// Lines are appended after anything already queued. Inject is safe to call from
// a test goroutine concurrently with the client's read loop. It is a no-op for
// the lines that cannot be enqueued because the FakeCLI is already closed.
func (f *FakeCLI) Inject(lines ...string) {
	_ = f.pushLines(lines)
}

// OnWrite registers a bidirectional-scripting hook: whenever a subsequent
// WriteJSON payload satisfies matcher, the lines returned by respond are
// injected into the read buffer. This is what lets a test auto-answer the
// initialize control_request by echoing its request_id.
//
// matcher runs synchronously inside WriteJSON while FakeCLI chooses matching
// hooks. respond runs after the mutex is released, so it may inspect FakeCLI
// state without deadlocking. Multiple hooks are evaluated in registration
// order; all matching hooks fire (frame advancement is unaffected).
func (f *FakeCLI) OnWrite(matcher func([]byte) bool, respond func() []string) {
	f.mu.Lock()
	f.onWrite = append(f.onWrite, writeHook{matcher: matcher, respond: respond})
	f.mu.Unlock()
}

// ReadJSON returns the next queued stream-JSON line.
//
// It blocks until a line is available, Close is called (returns io.EOF), or
// ctx is cancelled (returns ctx.Err).
func (f *FakeCLI) ReadJSON(ctx context.Context) ([]byte, error) {
	select {
	case line, ok := <-f.lines:
		if !ok {
			return nil, io.EOF
		}
		return line, nil
	case <-f.closed:
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Written returns a snapshot of all payloads written to this FakeCLI via
// WriteJSON, in order. Safe for concurrent use.
func (f *FakeCLI) Written() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.written))
	for i, b := range f.written {
		out[i] = slices.Clone(b)
	}
	return out
}

// FrameIdx returns the index of the next frame to be served. Safe for
// concurrent use; useful for assertions in tests.
func (f *FakeCLI) FrameIdx() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.frameIdx
}
