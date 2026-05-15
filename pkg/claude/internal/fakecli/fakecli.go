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

	// lines is a buffered channel of raw lines queued from script frames.
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
	copy := append([]byte(nil), p...)
	f.written = append(f.written, copy)
	idx := f.frameIdx
	if idx < len(f.script) {
		f.frameIdx++
	}
	f.mu.Unlock()

	// Enqueue this frame's lines for ReadJSON. Each line gets a trailing
	// newline so ReadJSON callers see well-formed stream-JSON lines.
	if idx < len(f.script) {
		for _, line := range f.script[idx].Lines {
			select {
			case f.lines <- []byte(line + "\n"):
			case <-f.closed:
				return io.ErrClosedPipe
			}
		}
	}
	return nil
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
		out[i] = append([]byte(nil), b...)
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
