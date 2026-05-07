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
)

// ThreadStart calls thread/start.
func (c *Client) ThreadStart(ctx context.Context, params *ThreadStartParams) (ThreadStartResponse, error) {
	return Request[ThreadStartResponse](ctx, c, "thread/start", paramsOrEmpty(params))
}

// ThreadResume calls thread/resume.
func (c *Client) ThreadResume(ctx context.Context, threadID string, params *ThreadResumeParams) (ThreadResumeResponse, error) {
	payload := mergeParams(params, Object{"threadId": threadID})
	return Request[ThreadResumeResponse](ctx, c, "thread/resume", payload)
}

// ThreadFork calls thread/fork.
func (c *Client) ThreadFork(ctx context.Context, threadID string, params *ThreadForkParams) (ThreadForkResponse, error) {
	payload := mergeParams(params, Object{"threadId": threadID})
	return Request[ThreadForkResponse](ctx, c, "thread/fork", payload)
}

// ThreadList calls thread/list.
func (c *Client) ThreadList(ctx context.Context, params *ThreadListParams) (ThreadListResponse, error) {
	return Request[ThreadListResponse](ctx, c, "thread/list", paramsOrEmpty(params))
}

// ThreadRead calls thread/read.
func (c *Client) ThreadRead(ctx context.Context, threadID string, includeTurns bool) (ThreadReadResponse, error) {
	return Request[ThreadReadResponse](ctx, c, "thread/read", Object{"threadId": threadID, "includeTurns": includeTurns})
}

// ThreadArchive calls thread/archive.
func (c *Client) ThreadArchive(ctx context.Context, threadID string) (ThreadArchiveResponse, error) {
	return Request[ThreadArchiveResponse](ctx, c, "thread/archive", Object{"threadId": threadID})
}

// ThreadUnarchive calls thread/unarchive.
func (c *Client) ThreadUnarchive(ctx context.Context, threadID string) (ThreadUnarchiveResponse, error) {
	return Request[ThreadUnarchiveResponse](ctx, c, "thread/unarchive", Object{"threadId": threadID})
}

// ThreadSetName calls thread/name/set.
func (c *Client) ThreadSetName(ctx context.Context, threadID, name string) (ThreadSetNameResponse, error) {
	return Request[ThreadSetNameResponse](ctx, c, "thread/name/set", Object{"threadId": threadID, "name": name})
}

// ThreadCompact calls thread/compact/start.
func (c *Client) ThreadCompact(ctx context.Context, threadID string) (ThreadCompactStartResponse, error) {
	return Request[ThreadCompactStartResponse](ctx, c, "thread/compact/start", Object{"threadId": threadID})
}

// TurnStart calls turn/start.
func (c *Client) TurnStart(ctx context.Context, threadID string, input any, params *TurnStartParams) (TurnStartResponse, error) {
	items, err := normalizeInput(input)
	if err != nil {
		return TurnStartResponse{}, err
	}
	payload := mergeParams(params, Object{"threadId": threadID, "input": items})
	return Request[TurnStartResponse](ctx, c, "turn/start", payload)
}

// TurnInterrupt calls turn/interrupt.
func (c *Client) TurnInterrupt(ctx context.Context, threadID, turnID string) (TurnInterruptResponse, error) {
	return Request[TurnInterruptResponse](ctx, c, "turn/interrupt", Object{"threadId": threadID, "turnId": turnID})
}

// TurnSteer calls turn/steer.
func (c *Client) TurnSteer(ctx context.Context, threadID, expectedTurnID string, input any) (TurnSteerResponse, error) {
	items, err := normalizeInput(input)
	if err != nil {
		return TurnSteerResponse{}, err
	}
	return Request[TurnSteerResponse](ctx, c, "turn/steer", Object{"threadId": threadID, "expectedTurnId": expectedTurnID, "input": items})
}

// ModelList calls model/list.
func (c *Client) ModelList(ctx context.Context, includeHidden bool) (ModelListResponse, error) {
	return Request[ModelListResponse](ctx, c, "model/list", Object{"includeHidden": includeHidden})
}
