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

package codexappserver

import (
	"context"
	"iter"
)

// StreamThread is a high-level thread handle for Go-native streaming flows.
//
// It is the Go parity analog for the upstream Python async thread handle, but
// it does not own a separate app-server process or transport. It shares the
// same Client as Codex and Thread, and keeps Run result-oriented while exposing
// RunStream for callers that want notification iteration directly.
type StreamThread struct {
	thread *Thread
}

// StreamTurnHandle is a high-level turn handle for Go-native streaming flows.
//
// It delegates to the same turn consumer guard used by TurnHandle, so only one
// stream/run consumer can read notifications from a Client at a time.
type StreamTurnHandle struct {
	handle *TurnHandle
}

func newStreamThread(client *Client, id string) *StreamThread {
	return &StreamThread{thread: &Thread{client: client, id: id}}
}

// StreamThreadStart starts a new Codex thread and returns a streaming handle.
func (c *Codex) StreamThreadStart(ctx context.Context, params *ThreadStartParams) (*StreamThread, error) {
	thread, err := c.ThreadStart(ctx, params)
	if err != nil {
		return nil, err
	}
	return &StreamThread{thread: thread}, nil
}

// StreamThreadResume resumes an existing thread and returns a streaming handle.
func (c *Codex) StreamThreadResume(ctx context.Context, threadID string, params *ThreadResumeParams) (*StreamThread, error) {
	thread, err := c.ThreadResume(ctx, threadID, params)
	if err != nil {
		return nil, err
	}
	return &StreamThread{thread: thread}, nil
}

// StreamThreadFork forks an existing thread and returns a streaming handle.
func (c *Codex) StreamThreadFork(ctx context.Context, threadID string, params *ThreadForkParams) (*StreamThread, error) {
	thread, err := c.ThreadFork(ctx, threadID, params)
	if err != nil {
		return nil, err
	}
	return &StreamThread{thread: thread}, nil
}

// StreamThreadUnarchive unarchives a thread and returns a streaming handle.
func (c *Codex) StreamThreadUnarchive(ctx context.Context, threadID string) (*StreamThread, error) {
	thread, err := c.ThreadUnarchive(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return &StreamThread{thread: thread}, nil
}

// ID returns the thread id.
func (t *StreamThread) ID() string {
	if t == nil || t.thread == nil {
		return ""
	}
	return t.thread.ID()
}

// Turn starts a turn and returns a streaming turn handle.
func (t *StreamThread) Turn(ctx context.Context, input any, params *TurnStartParams) (*StreamTurnHandle, error) {
	handle, err := t.thread.Turn(ctx, input, params)
	if err != nil {
		return nil, err
	}
	return &StreamTurnHandle{handle: handle}, nil
}

// Run starts a turn, consumes notifications until completion, and returns the
// collected result. Iterator output is available through RunStream.
func (t *StreamThread) Run(ctx context.Context, input any, params *TurnStartParams) (RunResult, error) {
	handle, err := t.Turn(ctx, input, params)
	if err != nil {
		return RunResult{}, err
	}
	return handle.Run(ctx)
}

// RunStream starts a turn and returns its notification stream.
func (t *StreamThread) RunStream(ctx context.Context, input any, params *TurnStartParams) iter.Seq2[Notification, error] {
	handle, err := t.Turn(ctx, input, params)
	if err != nil {
		return func(yield func(Notification, error) bool) {
			yield(Notification{}, err)
		}
	}
	return handle.Stream(ctx)
}

// Read reads thread state.
func (t *StreamThread) Read(ctx context.Context, includeTurns bool) (ThreadReadResponse, error) {
	return t.thread.Read(ctx, includeTurns)
}

// SetName sets the thread name.
func (t *StreamThread) SetName(ctx context.Context, name string) (ThreadSetNameResponse, error) {
	return t.thread.SetName(ctx, name)
}

// Compact starts compaction for the thread.
func (t *StreamThread) Compact(ctx context.Context) (ThreadCompactStartResponse, error) {
	return t.thread.Compact(ctx)
}

// ID returns the turn id.
func (h *StreamTurnHandle) ID() string {
	if h == nil || h.handle == nil {
		return ""
	}
	return h.handle.ID()
}

// Steer sends additional input to the active turn.
func (h *StreamTurnHandle) Steer(ctx context.Context, input any) (TurnSteerResponse, error) {
	return h.handle.Steer(ctx, input)
}

// Interrupt interrupts the active turn.
func (h *StreamTurnHandle) Interrupt(ctx context.Context) (TurnInterruptResponse, error) {
	return h.handle.Interrupt(ctx)
}

// Stream returns an iterator of notifications until this turn completes.
func (h *StreamTurnHandle) Stream(ctx context.Context) iter.Seq2[Notification, error] {
	return h.handle.Stream(ctx)
}

// Run consumes notifications until this turn completes and returns the final turn.
func (h *StreamTurnHandle) Run(ctx context.Context) (RunResult, error) {
	return h.handle.Run(ctx)
}
