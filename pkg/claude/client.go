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
	"context"
	"errors"
	"iter"
	"sync"
)

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
//   - Start acquires closeMu, assigns c.transport = t, then launches
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

	// readDone is closed by the readLoop goroutine when it exits.
	readDone chan struct{}

	// stderrDone is closed by the drainStderr goroutine when it exits.
	stderrDone chan struct{}
}

// Query sends prompt to the claude CLI and returns when the CLI has accepted
// it. Call [ClaudeSDKClient.ReceiveResponse] to iterate the resulting messages.
//
// This is the interactive counterpart to the package-level [Query] function.
// The body is stubbed to errors.ErrUnsupported until Phase C.
func (c *ClaudeSDKClient) Query(ctx context.Context, prompt string) error {
	_, _ = ctx, prompt
	return errors.ErrUnsupported
}

// ReceiveResponse returns an iterator over the [Message] values streamed by the
// claude CLI in response to the last [ClaudeSDKClient.Query] call. The iterator
// stops after delivering the terminal [ResultMessage] or when ctx is cancelled.
//
// The body is stubbed until Phase C.
func (c *ClaudeSDKClient) ReceiveResponse(ctx context.Context) iter.Seq2[Message, error] {
	_ = ctx
	return func(yield func(Message, error) bool) {
		yield(nil, errors.ErrUnsupported)
	}
}

// Interrupt sends an interrupt signal to the claude CLI subprocess, requesting
// that it cancel the current operation.
//
// The body is stubbed to errors.ErrUnsupported until Phase C.
func (c *ClaudeSDKClient) Interrupt(ctx context.Context) error {
	_ = ctx
	return errors.ErrUnsupported
}

// Fork creates a new [ClaudeSDKClient] whose conversation history is branched
// from fromMessageID in the current session. The parent client continues
// unaffected; the forked client has its own transport and session ID.
//
// Fork requires a non-nil [Options].SessionStore (Phase F). The body is stubbed
// to errors.ErrUnsupported until Phase G.
func (c *ClaudeSDKClient) Fork(ctx context.Context, fromMessageID string) (*ClaudeSDKClient, error) {
	_, _ = ctx, fromMessageID
	return nil, errors.ErrUnsupported
}

// Close terminates the claude CLI subprocess and releases all resources
// associated with this client, including any registered in-process MCP servers.
//
// Close is idempotent; subsequent calls return nil. The body is stubbed until
// Phase C.
func (c *ClaudeSDKClient) Close() error {
	return errors.ErrUnsupported
}
