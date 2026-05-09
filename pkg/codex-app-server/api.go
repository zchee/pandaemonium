// Copyright 2026 The omxx Authors.
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
	"fmt"
	"strings"

	"github.com/zchee/omxx/pkg/codex-app-server/protocol"
)

// Codex is the high-level synchronous Go SDK surface for app-server v2.
type Codex struct {
	client   *Client
	metadata InitializeResponse
}

// NewCodex starts and initializes a Codex app-server client.
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
	return &Codex{client: client, metadata: metadata}, nil
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
	return &Thread{client: c.client, id: started.Thread.ID}, nil
}

// ThreadList lists threads.
func (c *Codex) ThreadList(ctx context.Context, params *ThreadListParams) (protocol.ThreadListResponse, error) {
	return c.client.ThreadList(ctx, params)
}

// ThreadResume resumes an existing thread.
func (c *Codex) ThreadResume(ctx context.Context, threadID string, params *ThreadResumeParams) (*Thread, error) {
	resumed, err := c.client.ThreadResume(ctx, threadID, params)
	if err != nil {
		return nil, err
	}
	return &Thread{client: c.client, id: resumed.Thread.ID}, nil
}

// ThreadFork forks an existing thread.
func (c *Codex) ThreadFork(ctx context.Context, threadID string, params *ThreadForkParams) (*Thread, error) {
	forked, err := c.client.ThreadFork(ctx, threadID, params)
	if err != nil {
		return nil, err
	}
	return &Thread{client: c.client, id: forked.Thread.ID}, nil
}

// ThreadArchive archives a thread.
func (c *Codex) ThreadArchive(ctx context.Context, threadID string) (protocol.ThreadArchiveResponse, error) {
	return c.client.ThreadArchive(ctx, threadID)
}

// ThreadUnarchive unarchives a thread.
func (c *Codex) ThreadUnarchive(ctx context.Context, threadID string) (*Thread, error) {
	unarchived, err := c.client.ThreadUnarchive(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return &Thread{client: c.client, id: unarchived.Thread.ID}, nil
}

// Models lists available models.
func (c *Codex) Models(ctx context.Context, includeHidden bool) (protocol.ModelListResponse, error) {
	return c.client.ModelList(ctx, includeHidden)
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
	return &TurnHandle{client: t.client, threadID: t.id, turnID: started.Turn.ID}, nil
}

// Read reads thread state.
func (t *Thread) Read(ctx context.Context, includeTurns bool) (protocol.ThreadReadResponse, error) {
	return t.client.ThreadRead(ctx, t.id, includeTurns)
}

// SetName sets the thread name.
func (t *Thread) SetName(ctx context.Context, name string) (protocol.ThreadSetNameResponse, error) {
	return t.client.ThreadSetName(ctx, t.id, name)
}

// Compact starts compaction for the thread.
func (t *Thread) Compact(ctx context.Context) (protocol.ThreadCompactStartResponse, error) {
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
func (h *TurnHandle) Steer(ctx context.Context, input any) (protocol.TurnSteerResponse, error) {
	return h.client.TurnSteer(ctx, h.threadID, h.turnID, input)
}

// Interrupt interrupts the active turn.
func (h *TurnHandle) Interrupt(ctx context.Context) (protocol.TurnInterruptResponse, error) {
	return h.client.TurnInterrupt(ctx, h.threadID, h.turnID)
}

// Stream returns a channel of notifications until this turn completes.
func (h *TurnHandle) Stream(ctx context.Context) (<-chan Notification, <-chan error, error) {
	if err := h.client.acquireTurnConsumer(h.turnID); err != nil {
		return nil, nil, err
	}
	notifications := make(chan Notification)
	errs := make(chan error, 1)
	go func() {
		defer h.client.releaseTurnConsumer(h.turnID)
		defer close(notifications)
		defer close(errs)
		for {
			notification, err := h.client.NextNotification(ctx)
			if err != nil {
				errs <- err
				return
			}
			select {
			case notifications <- notification:
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}
			completed, ok, err := notification.TurnCompleted()
			if err != nil {
				errs <- err
				return
			}
			if ok && completed.Turn.ID == h.turnID {
				return
			}
		}
	}()
	return notifications, errs, nil
}

// Run consumes notifications until this turn completes and returns the final turn.
func (h *TurnHandle) Run(ctx context.Context) (RunResult, error) {
	if err := h.client.acquireTurnConsumer(h.turnID); err != nil {
		return RunResult{}, err
	}
	defer h.client.releaseTurnConsumer(h.turnID)
	return collectRunResult(ctx, h.client, h.turnID)
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
	payload.ServerInfo = &ServerInfo{Name: serverName, Version: serverVersion}
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
