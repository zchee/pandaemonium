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

	"github.com/go-json-experiment/json/jsontext"
)

// Hook is a callback invoked by the SDK at specific lifecycle events during a
// claude CLI session. The event Kind identifies which lifecycle point fired.
//
// Return a zero [HookDecision] and nil error to let the CLI proceed unchanged.
// Return a non-zero [HookDecision] to inject system messages, override
// permission decisions, or supply additional context.
//
// The hook dispatcher invokes registered hooks in registration order and stops
// at the first [PermissionDeny] decision for tool-use events.
//
// Hooks must not block indefinitely; respect ctx.Done() for cancellation.
type Hook func(ctx context.Context, event HookEvent) (HookDecision, error)

// HookRegistration pairs a [Hook] function with an event kind filter and an
// optional tool-name glob matcher. Registered via [Options].Hooks.
//
// ToolGlob is matched against [HookEvent].ToolName using filepath.Match
// semantics. An empty ToolGlob matches all tool names for PreToolUse and
// PostToolUse events.
type HookRegistration struct {
	// Kind is the event kind this registration responds to.
	Kind HookEventKind

	// ToolGlob is an optional glob matched against the tool name for PreToolUse
	// and PostToolUse events. Empty means "match all tools".
	ToolGlob string

	// Fn is the hook callback to invoke when Kind and ToolGlob match.
	Fn Hook
}

// CanUseTool is a permission callback invoked before every tool call. It
// receives the tool name and raw JSON-encoded input, and returns a
// [PermissionDecision] with an optional error.
//
// Return [PermissionAllow] to permit the call, [PermissionDeny] to block it,
// or [PermissionAsk] (the zero value) to fall through to the CLI's configured
// permission_mode.
//
// The callback must not block indefinitely; respect ctx.Done().
type CanUseTool func(ctx context.Context, toolName string, input jsontext.Value) (PermissionDecision, error)
