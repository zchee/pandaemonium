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
)

// ThreadStart calls thread/start.
func (c *Client) ThreadStart(ctx context.Context, params *ThreadStartParams) (ThreadStartResponse, error) {
	return c.Request[ThreadStartResponse](ctx, RequestMethodThreadStart, paramsOrEmpty(params))
}

// ThreadResume calls thread/resume.
func (c *Client) ThreadResume(ctx context.Context, threadID string, params *ThreadResumeParams) (ThreadResumeResponse, error) {
	payload := mergeParams(params, Object{"threadId": threadID})
	return c.Request[ThreadResumeResponse](ctx, RequestMethodThreadResume, payload)
}

// ThreadFork calls thread/fork.
func (c *Client) ThreadFork(ctx context.Context, threadID string, params *ThreadForkParams) (ThreadForkResponse, error) {
	payload := mergeParams(params, Object{"threadId": threadID})
	return c.Request[ThreadForkResponse](ctx, RequestMethodThreadFork, payload)
}

// ThreadList calls thread/list.
func (c *Client) ThreadList(ctx context.Context, params *ThreadListParams) (ThreadListResponse, error) {
	return c.Request[ThreadListResponse](ctx, RequestMethodThreadList, paramsOrEmpty(params))
}

// ThreadRead calls thread/read.
func (c *Client) ThreadRead(ctx context.Context, threadID string, includeTurns bool) (ThreadReadResponse, error) {
	return c.Request[ThreadReadResponse](ctx, RequestMethodThreadRead, ThreadReadParams{ThreadID: threadID, IncludeTurns: &includeTurns})
}

// ThreadArchive calls thread/archive.
func (c *Client) ThreadArchive(ctx context.Context, threadID string) (ThreadArchiveResponse, error) {
	return c.Request[ThreadArchiveResponse](ctx, RequestMethodThreadArchive, ThreadArchiveParams{ThreadID: threadID})
}

// ThreadUnarchive calls thread/unarchive.
func (c *Client) ThreadUnarchive(ctx context.Context, threadID string) (ThreadUnarchiveResponse, error) {
	return c.Request[ThreadUnarchiveResponse](ctx, RequestMethodThreadUnarchive, ThreadUnarchiveParams{ThreadID: threadID})
}

// ThreadSetName calls thread/name/set.
func (c *Client) ThreadSetName(ctx context.Context, threadID, name string) (ThreadSetNameResponse, error) {
	return c.Request[ThreadSetNameResponse](ctx, RequestMethodThreadNameSet, ThreadSetNameParams{ThreadID: threadID, Name: name})
}

// ThreadCompact calls thread/compact/start.
func (c *Client) ThreadCompact(ctx context.Context, threadID string) (ThreadCompactStartResponse, error) {
	return c.Request[ThreadCompactStartResponse](ctx, RequestMethodThreadCompactStart, ThreadCompactStartParams{ThreadID: threadID})
}

// TurnStart calls turn/start.
func (c *Client) TurnStart(ctx context.Context, threadID string, input any, params *TurnStartParams) (TurnStartResponse, error) {
	items, err := normalizeInput(input)
	if err != nil {
		return TurnStartResponse{}, err
	}
	payload := mergeParams(params, Object{"threadId": threadID, "input": items})
	return c.Request[TurnStartResponse](ctx, RequestMethodTurnStart, payload)
}

// TurnInterrupt calls turn/interrupt.
func (c *Client) TurnInterrupt(ctx context.Context, threadID, turnID string) (TurnInterruptResponse, error) {
	return c.Request[TurnInterruptResponse](ctx, RequestMethodTurnInterrupt, Object{"threadId": threadID, "turnId": turnID})
}

// TurnSteer calls turn/steer.
func (c *Client) TurnSteer(ctx context.Context, threadID, expectedTurnID string, input any) (TurnSteerResponse, error) {
	items, err := normalizeInput(input)
	if err != nil {
		return TurnSteerResponse{}, err
	}
	return c.Request[TurnSteerResponse](ctx, RequestMethodTurnSteer, Object{"threadId": threadID, "expectedTurnId": expectedTurnID, "input": items})
}

// ModelList calls model/list.
func (c *Client) ModelList(ctx context.Context, includeHidden bool) (ModelListResponse, error) {
	return c.Request[ModelListResponse](ctx, RequestMethodModelList, ModelListParams{IncludeHidden: &includeHidden})
}
