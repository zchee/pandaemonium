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

	"github.com/zchee/pandaemonium/pkg/codex-app-server/protocol"
)

// ThreadStart calls thread/start.
func (c *Client) ThreadStart(ctx context.Context, params *protocol.ThreadStartParams) (protocol.ThreadStartResponse, error) {
	return Request[protocol.ThreadStartResponse](ctx, c, "thread/start", paramsOrEmpty(params))
}

// ThreadResume calls thread/resume.
func (c *Client) ThreadResume(ctx context.Context, threadID string, params *protocol.ThreadResumeParams) (protocol.ThreadResumeResponse, error) {
	payload := mergeParams(params, Object{"threadId": threadID})
	return Request[protocol.ThreadResumeResponse](ctx, c, "thread/resume", payload)
}

// ThreadFork calls thread/fork.
func (c *Client) ThreadFork(ctx context.Context, threadID string, params *protocol.ThreadForkParams) (protocol.ThreadForkResponse, error) {
	payload := mergeParams(params, Object{"threadId": threadID})
	return Request[protocol.ThreadForkResponse](ctx, c, "thread/fork", payload)
}

// ThreadList calls thread/list.
func (c *Client) ThreadList(ctx context.Context, params *protocol.ThreadListParams) (protocol.ThreadListResponse, error) {
	return Request[protocol.ThreadListResponse](ctx, c, "thread/list", paramsOrEmpty(params))
}

// ThreadRead calls thread/read.
func (c *Client) ThreadRead(ctx context.Context, threadID string, includeTurns bool) (protocol.ThreadReadResponse, error) {
	return Request[protocol.ThreadReadResponse](ctx, c, "thread/read", protocol.ThreadReadParams{ThreadID: threadID, IncludeTurns: &includeTurns})
}

// ThreadArchive calls thread/archive.
func (c *Client) ThreadArchive(ctx context.Context, threadID string) (protocol.ThreadArchiveResponse, error) {
	return Request[protocol.ThreadArchiveResponse](ctx, c, "thread/archive", protocol.ThreadArchiveParams{ThreadID: threadID})
}

// ThreadUnarchive calls thread/unarchive.
func (c *Client) ThreadUnarchive(ctx context.Context, threadID string) (protocol.ThreadUnarchiveResponse, error) {
	return Request[protocol.ThreadUnarchiveResponse](ctx, c, "thread/unarchive", protocol.ThreadUnarchiveParams{ThreadID: threadID})
}

// ThreadSetName calls thread/name/set.
func (c *Client) ThreadSetName(ctx context.Context, threadID, name string) (protocol.ThreadSetNameResponse, error) {
	return Request[protocol.ThreadSetNameResponse](ctx, c, "thread/name/set", protocol.ThreadSetNameParams{ThreadID: threadID, Name: name})
}

// ThreadCompact calls thread/compact/start.
func (c *Client) ThreadCompact(ctx context.Context, threadID string) (protocol.ThreadCompactStartResponse, error) {
	return Request[protocol.ThreadCompactStartResponse](ctx, c, "thread/compact/start", protocol.ThreadCompactStartParams{ThreadID: threadID})
}

// TurnStart calls turn/start.
func (c *Client) TurnStart(ctx context.Context, threadID string, input any, params *protocol.TurnStartParams) (protocol.TurnStartResponse, error) {
	items, err := normalizeInput(input)
	if err != nil {
		return protocol.TurnStartResponse{}, err
	}
	payload := mergeParams(params, Object{"threadId": threadID, "input": items})
	return Request[protocol.TurnStartResponse](ctx, c, "turn/start", payload)
}

// TurnInterrupt calls turn/interrupt.
func (c *Client) TurnInterrupt(ctx context.Context, threadID, turnID string) (protocol.TurnInterruptResponse, error) {
	return Request[protocol.TurnInterruptResponse](ctx, c, "turn/interrupt", Object{"threadId": threadID, "turnId": turnID})
}

// TurnSteer calls turn/steer.
func (c *Client) TurnSteer(ctx context.Context, threadID, expectedTurnID string, input any) (protocol.TurnSteerResponse, error) {
	items, err := normalizeInput(input)
	if err != nil {
		return protocol.TurnSteerResponse{}, err
	}
	return Request[protocol.TurnSteerResponse](ctx, c, "turn/steer", Object{"threadId": threadID, "expectedTurnId": expectedTurnID, "input": items})
}

// ModelList calls model/list.
func (c *Client) ModelList(ctx context.Context, includeHidden bool) (protocol.ModelListResponse, error) {
	return Request[protocol.ModelListResponse](ctx, c, "model/list", protocol.ModelListParams{IncludeHidden: &includeHidden})
}
