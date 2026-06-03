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
	"github.com/go-json-experiment/json/jsontext"
)

// PermissionResult is the sealed return type of a [CanUseTool] callback. The
// two variants — [PermissionResultAllow] and [PermissionResultDeny] — carry
// disjoint optional fields (Allow may modify the input and add permission
// updates; Deny carries a message and an optional interrupt flag). The
// unexported sentinel method keeps the set closed to this package.
//
// Mirrors upstream PermissionResult = PermissionResultAllow |
// PermissionResultDeny (types.py:232-251).
type PermissionResult interface {
	isPermissionResult()
}

// PermissionResultAllow permits the tool call, optionally modifying the input
// the CLI passes to the tool and emitting permission updates the CLI should
// apply for the rest of the session. Mirrors upstream PermissionResultAllow
// (types.py:233).
type PermissionResultAllow struct {
	// UpdatedInput overrides the input the CLI delivers to the tool. A nil
	// (zero-length) value preserves the original input the callback received;
	// any non-empty value replaces it. Must be valid JSON.
	UpdatedInput jsontext.Value

	// UpdatedPermissions, when non-nil, is applied as additional permission
	// updates after the tool call. Each entry is serialized through
	// [PermissionUpdate.ToWire].
	UpdatedPermissions []PermissionUpdate
}

func (PermissionResultAllow) isPermissionResult() {}

// PermissionResultDeny blocks the tool call, optionally carrying a message
// and an interrupt flag that asks the CLI to abort the current turn. Mirrors
// upstream PermissionResultDeny (types.py:243).
type PermissionResultDeny struct {
	// Message is the human-readable explanation the CLI shows to the user.
	// May be empty.
	Message string

	// Interrupt, when true, asks the CLI to abort the current turn after
	// emitting the deny. Only emitted on the wire when true (parity with
	// upstream's `if response.interrupt` gate).
	Interrupt bool
}

func (PermissionResultDeny) isPermissionResult() {}

// MemoryScope identifies which memory layer a subagent uses. Mirrors upstream
// AgentDefinition.memory = Literal["user", "project", "local"] (types.py:94).
// Kept distinct from [SettingSource] because the two semantic domains may
// drift (SettingSource is likely to gain "managed"; Memory is not).
type MemoryScope string

const (
	// MemoryScopeUser uses the per-user memory layer.
	MemoryScopeUser MemoryScope = "user"

	// MemoryScopeProject uses the project memory layer.
	MemoryScopeProject MemoryScope = "project"

	// MemoryScopeLocal uses the per-machine memory layer.
	MemoryScopeLocal MemoryScope = "local"
)

// ToolPermissionContext carries the contextual information the CLI delivers
// alongside a can_use_tool request. Mirrors upstream ToolPermissionContext
// (types.py:198). All fields are optional; the zero value is the empty
// string. The Suggestions slice is the wire's `permission_suggestions` field
// decoded through [PermissionUpdateFromWire].
//
// The Signal field upstream defines (types.py:201) is omitted: it is
// documented as future abort-signal support and is not wired today.
type ToolPermissionContext struct {
	// Suggestions are the [PermissionUpdate] entries the CLI delivers as
	// recommended responses to the current tool-permission prompt. Decoded
	// from the wire's "permission_suggestions" key.
	Suggestions []PermissionUpdate

	// ToolUseID uniquely identifies this tool call within the assistant
	// message. The wire protocol guarantees a non-empty string when the
	// callback is invoked.
	ToolUseID string

	// AgentID is the sub-agent ID, when the call originates from one.
	AgentID string

	// BlockedPath is the file path that triggered the permission request
	// (for example, when a Bash command tries to access a path outside the
	// allowed directories).
	BlockedPath string

	// DecisionReason explains why this permission request was triggered. When
	// a PreToolUse hook returns permissionDecision "ask" with a
	// permissionDecisionReason, that reason is forwarded here.
	DecisionReason string

	// Title is the full permission prompt sentence (e.g. "Claude wants to
	// read foo.txt"). Use this as the primary prompt text when present
	// instead of reconstructing from tool name + input.
	Title string

	// DisplayName is a human-readable subtitle for the permission UI.
	DisplayName string

	// Description is a longer-form description of the permission request.
	Description string
}
