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
	"fmt"
	"iter"
	"strings"
)

// Codex is the high-level synchronous Go SDK surface for app-server v2.
type Codex struct {
	client   *Client
	metadata InitializeResponse
}

// NewCodex starts and initializes a [Codex] app-server client.
func NewCodex(ctx context.Context, config *Config) (*Codex, error) {
	client := NewClient(config, nil)
	if err := client.Start(ctx); err != nil {
		return nil, err
	}

	metadata, err := client.Initialize(ctx)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Codex{
		client:   client,
		metadata: metadata,
	}, nil
}

// Metadata returns initialize metadata validated during construction.
func (c *Codex) Metadata() InitializeResponse {
	return c.metadata
}

// Close terminates the app-server process.
func (c *Codex) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

// ThreadStart starts a new Codex thread.
func (c *Codex) ThreadStart(ctx context.Context, params *ThreadStartParams) (*Thread, error) {
	started, err := c.client.ThreadStart(ctx, params)
	if err != nil {
		return nil, err
	}
	return &Thread{
		client: c.client,
		id:     started.Thread.ID,
	}, nil
}

// ThreadList lists threads.
func (c *Codex) ThreadList(ctx context.Context, params *ThreadListParams) (ThreadListResponse, error) {
	return c.client.ThreadList(ctx, params)
}

// ThreadResume resumes an existing thread.
func (c *Codex) ThreadResume(ctx context.Context, threadID string, params *ThreadResumeParams) (*Thread, error) {
	resumed, err := c.client.ThreadResume(ctx, threadID, params)
	if err != nil {
		return nil, err
	}
	return &Thread{
		client: c.client,
		id:     resumed.Thread.ID,
	}, nil
}

// ThreadFork forks an existing thread.
func (c *Codex) ThreadFork(ctx context.Context, threadID string, params *ThreadForkParams) (*Thread, error) {
	forked, err := c.client.ThreadFork(ctx, threadID, params)
	if err != nil {
		return nil, err
	}
	return &Thread{
		client: c.client,
		id:     forked.Thread.ID,
	}, nil
}

// ThreadArchive archives a thread.
func (c *Codex) ThreadArchive(ctx context.Context, threadID string) (ThreadArchiveResponse, error) {
	return c.client.ThreadArchive(ctx, threadID)
}

// ThreadUnarchive unarchives a thread.
func (c *Codex) ThreadUnarchive(ctx context.Context, threadID string) (*Thread, error) {
	unarchived, err := c.client.ThreadUnarchive(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return &Thread{
		client: c.client,
		id:     unarchived.Thread.ID,
	}, nil
}

// Models lists available models.
func (c *Codex) Models(ctx context.Context, params *ModelListParams) (ModelListResponse, error) {
	return c.client.ModelList(ctx, params)
}

// Client exposes the lower-level JSON-RPC client.
func (c *Codex) Client() *Client {
	return c.client
}

// Thread is a high-level handle for an app-server thread.
type Thread struct {
	client *Client
	id     string
}

// ID returns the thread id.
func (t *Thread) ID() string { return t.id }

// Run starts a turn, consumes events until completion, and returns collected output.
func (t *Thread) Run(ctx context.Context, input any, params *TurnStartParams) (RunResult, error) {
	handle, err := t.Turn(ctx, input, params)
	if err != nil {
		return RunResult{}, err
	}
	return handle.Run(ctx)
}

// Turn starts a turn and returns a handle for stream, steer, interrupt, or run.
func (t *Thread) Turn(ctx context.Context, input any, params *TurnStartParams) (*TurnHandle, error) {
	started, err := t.client.TurnStart(ctx, t.id, input, params)
	if err != nil {
		return nil, err
	}
	return &TurnHandle{
		client:   t.client,
		threadID: t.id,
		turnID:   started.Turn.ID,
	}, nil
}

// Read reads thread state.
func (t *Thread) Read(ctx context.Context, params *ThreadReadParams) (ThreadReadResponse, error) {
	return t.client.ThreadRead(ctx, t.id, params)
}

// Unsubscribe unsubscribes from thread notifications.
func (t *Thread) Unsubscribe(ctx context.Context, params *ThreadUnsubscribeParams) (ThreadUnsubscribeResponse, error) {
	return t.client.ThreadUnsubscribe(ctx, t.id, params)
}

// MetadataUpdate updates thread metadata.
func (t *Thread) MetadataUpdate(ctx context.Context, params *ThreadMetadataUpdateParams) (ThreadMetadataUpdateResponse, error) {
	return t.client.ThreadMetadataUpdate(ctx, t.id, params)
}

// ShellCommand runs a shell command in the thread context.
func (t *Thread) ShellCommand(ctx context.Context, params *ThreadShellCommandParams) (ThreadShellCommandResponse, error) {
	return t.client.ThreadShellCommand(ctx, t.id, params)
}

// ApproveGuardianDeniedAction approves a guardian-denied action in the thread.
func (t *Thread) ApproveGuardianDeniedAction(ctx context.Context, params *ThreadApproveGuardianDeniedActionParams) (ThreadApproveGuardianDeniedActionResponse, error) {
	return t.client.ThreadApproveGuardianDeniedAction(ctx, t.id, params)
}

// Rollback rolls back the thread by the number of turns specified in params.
func (t *Thread) Rollback(ctx context.Context, params *ThreadRollbackParams) (ThreadRollbackResponse, error) {
	return t.client.ThreadRollback(ctx, t.id, params)
}

// InjectItems injects items into the thread.
func (t *Thread) InjectItems(ctx context.Context, params *ThreadInjectItemsParams) (ThreadInjectItemsResponse, error) {
	return t.client.ThreadInjectItems(ctx, t.id, params)
}

// SetName sets the thread name.
func (t *Thread) SetName(ctx context.Context, name string) (ThreadSetNameResponse, error) {
	return t.client.ThreadSetName(ctx, t.id, name)
}

// Compact starts compaction for the thread.
func (t *Thread) Compact(ctx context.Context) (ThreadCompactStartResponse, error) {
	return t.client.ThreadCompact(ctx, t.id)
}

// TurnHandle controls or consumes one app-server turn.
type TurnHandle struct {
	client   *Client
	threadID string
	turnID   string
}

// ID returns the turn id.
func (h *TurnHandle) ID() string { return h.turnID }

// Steer sends additional input to the active turn.
func (h *TurnHandle) Steer(ctx context.Context, input any) (TurnSteerResponse, error) {
	return h.client.TurnSteer(ctx, h.threadID, h.turnID, input)
}

// Interrupt interrupts the active turn.
func (h *TurnHandle) Interrupt(ctx context.Context) (TurnInterruptResponse, error) {
	return h.client.TurnInterrupt(ctx, h.threadID, h.turnID)
}

// Stream returns an iterator of notifications until this turn completes.
//
// The iterator yields (Notification{}, err) once and stops when acquiring the
// stream consumer, reading from the transport, decoding completion, or the
// context fails. It yields no error on normal turn completion. Stopping
// iteration early releases the stream consumer before Stream returns.
func (h *TurnHandle) Stream(ctx context.Context) iter.Seq2[Notification, error] {
	return func(yield func(Notification, error) bool) {
		if err := h.client.acquireTurnConsumer(h.turnID); err != nil {
			yield(Notification{}, err)
			return
		}

		defer h.client.releaseTurnConsumer(h.turnID)
		defer h.client.clearTurnPending(h.turnID)

		for {
			notification, err := h.client.nextTurnNotification(ctx, h.turnID)
			if err != nil {
				yield(Notification{}, err)
				return
			}
			if !yield(notification, nil) {
				return
			}
			completed, ok, err := notification.TurnCompleted()
			if err != nil {
				yield(Notification{}, err)
				return
			}
			if ok && completed.Turn.ID == h.turnID {
				h.client.clearTurnPending(h.turnID)
				return
			}
		}
	}
}

// Run consumes notifications until this turn completes and returns the final turn.
func (h *TurnHandle) Run(ctx context.Context) (RunResult, error) {
	if err := h.client.acquireTurnConsumer(h.turnID); err != nil {
		return RunResult{}, err
	}
	defer h.client.releaseTurnConsumer(h.turnID)

	result, err := collectRunResult(ctx, h.client, h.turnID)
	if err == nil {
		h.client.clearTurnPending(h.turnID)
	}
	return result, err
}

func validateInitialize(payload InitializeResponse) (InitializeResponse, error) {
	userAgent := strings.TrimSpace(payload.UserAgent)
	server := payload.ServerInfo

	var serverName, serverVersion string
	if server != nil {
		serverName = strings.TrimSpace(server.Name)
		serverVersion = strings.TrimSpace(server.Version)
	}
	if (serverName == "" || serverVersion == "") && userAgent != "" {
		parsedName, parsedVersion := splitUserAgent(userAgent)
		if serverName == "" {
			serverName = parsedName
		}
		if serverVersion == "" {
			serverVersion = parsedVersion
		}
	}
	if userAgent == "" || serverName == "" || serverVersion == "" {
		return InitializeResponse{}, fmt.Errorf("initialize response missing required metadata (user_agent=%q, server_name=%q, server_version=%q)", userAgent, serverName, serverVersion)
	}

	payload.ServerInfo = &ServerInfo{
		Name:    serverName,
		Version: serverVersion,
	}
	return payload, nil
}

func splitUserAgent(userAgent string) (string, string) {
	raw := strings.TrimSpace(userAgent)
	if raw == "" {
		return "", ""
	}

	if name, version, ok := strings.Cut(raw, "/"); ok {
		return strings.TrimSpace(name), strings.TrimSpace(version)
	}

	parts := strings.Fields(raw)
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}

	return raw, ""
}
