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

// ─── Message sealed interface ────────────────────────────────────────────────

// Message is the sealed interface implemented by every stream message emitted
// by the claude CLI subprocess. The unexported sentinel methods keep the set
// closed to this package, enabling exhaustive type-switch coverage.
//
// Concrete types: [AssistantMessage], [UserMessage], [SystemMessage],
// [ResultMessage].
//
// To inspect unknown CLI fields that have not yet been promoted to typed
// struct fields, cast the message to its concrete type and read the Raw field:
//
//	if am, ok := msg.(AssistantMessage); ok {
//	    raw := am.Raw // jsontext.Value with unknown fields
//	}
type Message interface {
	isMessage()
	jsonRaw() jsontext.Value // returns the message's Raw inline catchall
}

// AssistantMessage carries one or more content blocks produced by the model.
type AssistantMessage struct {
	// Content holds the ordered content blocks in this assistant turn.
	Content []ContentBlock `json:"content,omitzero"`

	// Model is the identifier of the model that generated this message.
	Model string `json:"model,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility.
	// Mirrors pkg/codex/public_types.go:33-37.
	Raw jsontext.Value `json:",inline"`
}

func (AssistantMessage) isMessage()                {}
func (m AssistantMessage) jsonRaw() jsontext.Value { return m.Raw }

// UserMessage carries content submitted by the user (or injected by a tool).
type UserMessage struct {
	// Content holds the ordered content blocks in this user turn.
	Content []ContentBlock `json:"content,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (UserMessage) isMessage()                {}
func (m UserMessage) jsonRaw() jsontext.Value { return m.Raw }

// SystemMessage carries a system-level notification from the CLI subprocess.
type SystemMessage struct {
	// Subtype discriminates system message variants (e.g. "init").
	Subtype string `json:"subtype,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (SystemMessage) isMessage()                {}
func (m SystemMessage) jsonRaw() jsontext.Value { return m.Raw }

// ResultMessage is the final message in a stream. It carries session and usage
// metadata. Iterators stop cleanly after delivering this message.
type ResultMessage struct {
	// Subtype discriminates result variants (e.g. "success", "error_max_turns").
	Subtype string `json:"subtype,omitzero"`

	// DurationMs is the wall-clock duration of the request in milliseconds.
	DurationMs int `json:"duration_ms,omitzero"`

	// DurationAPIMs is the API-call duration in milliseconds.
	DurationAPIMs int `json:"duration_api_ms,omitzero"`

	// IsError indicates that the result represents an error condition.
	IsError bool `json:"is_error,omitzero"`

	// NumTurns is the number of conversation turns in this session.
	NumTurns int `json:"num_turns,omitzero"`

	// SessionID identifies the CLI session.
	SessionID string `json:"session_id,omitzero"`

	// TotalCostUSD is the estimated total cost of the request in US dollars.
	TotalCostUSD float64 `json:"total_cost_usd,omitzero"`

	// Usage contains raw token-usage statistics as emitted by the CLI.
	Usage jsontext.Value `json:"usage,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (ResultMessage) isMessage()                {}
func (m ResultMessage) jsonRaw() jsontext.Value { return m.Raw }

// ─── ContentBlock sealed interface ──────────────────────────────────────────

// ContentBlock is the sealed interface implemented by every content block
// within a [Message]. The unexported sentinel keeps the set closed to this
// package.
//
// Concrete types: [TextBlock], [ToolUseBlock], [ToolResultBlock].
type ContentBlock interface {
	isContentBlock()
	blockRaw() jsontext.Value
}

// TextBlock is a plain-text content block.
type TextBlock struct {
	// Text is the plain-text content.
	Text string `json:"text,omitzero"`

	// Raw preserves unknown fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (TextBlock) isContentBlock()            {}
func (b TextBlock) blockRaw() jsontext.Value { return b.Raw }

// ToolUseBlock records a tool invocation requested by the model.
type ToolUseBlock struct {
	// ID is the unique tool-use identifier, correlated with ToolResultBlock.
	ID string `json:"id,omitzero"`

	// Name is the name of the tool being called.
	Name string `json:"name,omitzero"`

	// Input is the raw JSON-encoded tool input arguments.
	Input jsontext.Value `json:"input,omitzero"`

	// Raw preserves unknown fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (ToolUseBlock) isContentBlock()            {}
func (b ToolUseBlock) blockRaw() jsontext.Value { return b.Raw }

// ToolResultBlock carries the result of a tool invocation.
type ToolResultBlock struct {
	// ToolUseID correlates this result with the originating [ToolUseBlock.ID].
	ToolUseID string `json:"tool_use_id,omitzero"`

	// Content holds the tool result content blocks.
	Content []ContentBlock `json:"content,omitzero"`

	// IsError indicates the tool invocation returned an error.
	IsError bool `json:"is_error,omitzero"`

	// Raw preserves unknown fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (ToolResultBlock) isContentBlock()            {}
func (b ToolResultBlock) blockRaw() jsontext.Value { return b.Raw }

// ─── Hook event types ────────────────────────────────────────────────────────

// HookEventKind identifies the lifecycle event that triggered a [Hook].
type HookEventKind string

const (
	// HookEventPreToolUse fires before the CLI invokes a tool.
	HookEventPreToolUse HookEventKind = "PreToolUse"

	// HookEventPostToolUse fires after the CLI receives a tool result.
	HookEventPostToolUse HookEventKind = "PostToolUse"

	// HookEventUserPromptSubmit fires when a user prompt is submitted.
	HookEventUserPromptSubmit HookEventKind = "UserPromptSubmit"

	// HookEventNotification fires on a notification from the CLI.
	HookEventNotification HookEventKind = "Notification"

	// HookEventStop fires when the CLI session stops.
	HookEventStop HookEventKind = "Stop"

	// HookEventSubagentStop fires when a subagent session stops.
	HookEventSubagentStop HookEventKind = "SubagentStop"

	// HookEventPreCompact fires before the CLI compacts the conversation.
	HookEventPreCompact HookEventKind = "PreCompact"

	// HookEventSessionStart fires when a CLI session starts.
	HookEventSessionStart HookEventKind = "SessionStart"

	// HookEventSessionEnd fires when a CLI session ends.
	HookEventSessionEnd HookEventKind = "SessionEnd"
)

// HookEvent is the structured payload delivered to a [Hook] function.
//
// Only the fields relevant to the event Kind are populated; the rest are zero.
// Unknown CLI fields are preserved in Raw for forward compatibility, mirroring
// the inline catchall pattern in pkg/codex/public_types.go:33-37.
type HookEvent struct {
	// Kind identifies the lifecycle event.
	Kind HookEventKind `json:"hook_event_name,omitzero"`

	// SessionID is the CLI session identifier associated with the event.
	SessionID string `json:"session_id,omitzero"`

	// ToolName is the name of the tool (PreToolUse / PostToolUse only).
	ToolName string `json:"tool_name,omitzero"`

	// ToolInput is the raw JSON-encoded tool input (PreToolUse only).
	ToolInput jsontext.Value `json:"tool_input,omitzero"`

	// ToolResult is the raw JSON-encoded tool result (PostToolUse only).
	ToolResult jsontext.Value `json:"tool_result,omitzero"`

	// Prompt is the user prompt text (UserPromptSubmit only).
	Prompt string `json:"prompt,omitzero"`

	// Raw preserves unknown top-level CLI fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

// ─── Hook decision types ─────────────────────────────────────────────────────

// PermissionDecision is the allow/deny verdict returned by a [CanUseTool]
// callback. The zero value (empty string) falls through to the CLI's
// configured permission_mode.
type PermissionDecision string

const (
	// PermissionAllow permits the tool call unconditionally.
	PermissionAllow PermissionDecision = "allow"

	// PermissionDeny blocks the tool call unconditionally.
	PermissionDeny PermissionDecision = "deny"

	// PermissionAsk is the zero value; the CLI falls through to its
	// configured permission_mode for the final decision.
	PermissionAsk PermissionDecision = ""
)

// HookDecision is the structured return value of a [Hook].
//
// It mirrors the Python upstream {"hookSpecificOutput": {...}} return envelope.
// Unknown fields are preserved in Raw for forward compatibility (AC-i7).
type HookDecision struct {
	// HookSpecificOutput carries hook-kind-specific output fields.
	HookSpecificOutput HookSpecificOutput `json:"hookSpecificOutput,omitzero"`

	// SystemMessage is an optional message injected into the system prompt.
	SystemMessage string `json:"systemMessage,omitzero"`

	// AdditionalContext is optional context appended to the hook response.
	AdditionalContext string `json:"additionalContext,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility (AC-i7).
	Raw jsontext.Value `json:",inline"`
}

// HookSpecificOutput carries hook-kind-specific fields within a [HookDecision].
type HookSpecificOutput struct {
	// HookEventName identifies the event kind this output applies to.
	HookEventName HookEventKind `json:"hookEventName,omitzero"`

	// PermissionDecision is the allow/deny verdict for tool-use hooks.
	PermissionDecision PermissionDecision `json:"permissionDecision,omitzero"`

	// PermissionDecisionReason is a human-readable reason for the decision.
	PermissionDecisionReason string `json:"permissionDecisionReason,omitzero"`

	// Raw preserves unknown fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}
