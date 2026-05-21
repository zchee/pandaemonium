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

	// DeferredToolUse, when non-nil, carries the tool call a PreToolUse hook
	// returned "defer" on. The run stops with this populated so the caller
	// can inspect the deferred call and decide whether to resume.
	DeferredToolUse *DeferredToolUse `json:"deferred_tool_use,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (ResultMessage) isMessage()                {}
func (m ResultMessage) jsonRaw() jsontext.Value { return m.Raw }

// DeferredToolUse describes a tool call that was deferred by a PreToolUse
// hook returning permissionDecision="defer". Carried as
// [ResultMessage.DeferredToolUse]. Mirrors upstream DeferredToolUse
// (types.py:1131).
type DeferredToolUse struct {
	// ID is the unique tool-use identifier of the deferred call, correlated
	// with the originating [ToolUseBlock.ID].
	ID string `json:"id,omitzero"`

	// Name is the name of the tool that was deferred.
	Name string `json:"name,omitzero"`

	// Input is the raw JSON input payload that would have been passed to
	// the tool had the call gone through.
	Input jsontext.Value `json:"input,omitzero"`
}

// ─── Task system messages ────────────────────────────────────────────────────

// TaskUsage is the usage breakdown reported in [TaskProgressMessage] and
// [TaskNotificationMessage]. Mirrors upstream TaskUsage (types.py:1045-1052).
type TaskUsage struct {
	// TotalTokens is the cumulative number of tokens consumed by the task.
	TotalTokens int `json:"total_tokens,omitzero"`

	// ToolUses is the count of tool invocations the task has issued.
	ToolUses int `json:"tool_uses,omitzero"`

	// DurationMs is the wall-clock duration of the task in milliseconds.
	DurationMs int `json:"duration_ms,omitzero"`
}

// TaskNotificationStatus is the terminal status carried by a
// [TaskNotificationMessage]. Mirrors upstream TaskNotificationStatus
// (types.py:1056).
type TaskNotificationStatus string

const (
	// TaskNotificationStatusCompleted indicates the task finished
	// successfully.
	TaskNotificationStatusCompleted TaskNotificationStatus = "completed"

	// TaskNotificationStatusFailed indicates the task errored out.
	TaskNotificationStatusFailed TaskNotificationStatus = "failed"

	// TaskNotificationStatusStopped indicates the task was cancelled or
	// stopped before completing.
	TaskNotificationStatusStopped TaskNotificationStatus = "stopped"
)

// TaskStartedMessage is the system message emitted when a Task begins.
//
// Upstream Python defines TaskStartedMessage as a subclass of SystemMessage so
// existing isinstance(msg, SystemMessage) checks continue to match. Go has no
// inheritance, so this is a sibling [Message] type: a parser-side dispatch on
// (type="system", subtype="task_started") returns [TaskStartedMessage] instead
// of [SystemMessage]. Callers branch on the specific type in a type switch.
//
// Mirrors upstream TaskStartedMessage (types.py:1060).
type TaskStartedMessage struct {
	// Subtype is always "task_started" but is carried for symmetry with the
	// SystemMessage envelope.
	Subtype string `json:"subtype,omitzero"`

	// TaskID is the unique identifier for this task.
	TaskID string `json:"task_id,omitzero"`

	// Description is the task's human-readable description.
	Description string `json:"description,omitzero"`

	// UUID is the message UUID.
	UUID string `json:"uuid,omitzero"`

	// SessionID identifies the CLI session.
	SessionID string `json:"session_id,omitzero"`

	// ToolUseID is the originating tool-use ID, when the task was spawned
	// by a tool call. Empty when the task was started by some other means.
	ToolUseID string `json:"tool_use_id,omitzero"`

	// TaskType is the task's type label, when set by the CLI.
	TaskType string `json:"task_type,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (TaskStartedMessage) isMessage()                {}
func (m TaskStartedMessage) jsonRaw() jsontext.Value { return m.Raw }

// TaskProgressMessage is the system message emitted while a Task is running.
// See [TaskStartedMessage]'s godoc for the Python-subclass / Go-sibling note.
// Mirrors upstream TaskProgressMessage (types.py:1077).
type TaskProgressMessage struct {
	// Subtype is always "task_progress".
	Subtype string `json:"subtype,omitzero"`

	// TaskID is the unique identifier for this task.
	TaskID string `json:"task_id,omitzero"`

	// Description is the task's human-readable description.
	Description string `json:"description,omitzero"`

	// Usage is the running usage breakdown for the task.
	Usage TaskUsage `json:"usage,omitzero"`

	// UUID is the message UUID.
	UUID string `json:"uuid,omitzero"`

	// SessionID identifies the CLI session.
	SessionID string `json:"session_id,omitzero"`

	// ToolUseID is the originating tool-use ID, when applicable.
	ToolUseID string `json:"tool_use_id,omitzero"`

	// LastToolName is the name of the most-recently-invoked tool, when set.
	LastToolName string `json:"last_tool_name,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (TaskProgressMessage) isMessage()                {}
func (m TaskProgressMessage) jsonRaw() jsontext.Value { return m.Raw }

// TaskNotificationMessage is the system message emitted when a Task completes,
// fails, or is stopped. See [TaskStartedMessage]'s godoc for the
// Python-subclass / Go-sibling note. Mirrors upstream TaskNotificationMessage
// (types.py:1095).
type TaskNotificationMessage struct {
	// Subtype is always "task_notification".
	Subtype string `json:"subtype,omitzero"`

	// TaskID is the unique identifier for this task.
	TaskID string `json:"task_id,omitzero"`

	// Status is the terminal state — completed, failed, or stopped.
	Status TaskNotificationStatus `json:"status,omitzero"`

	// OutputFile is the path the task wrote its output to, when applicable.
	OutputFile string `json:"output_file,omitzero"`

	// Summary is the human-readable summary of the task outcome.
	Summary string `json:"summary,omitzero"`

	// UUID is the message UUID.
	UUID string `json:"uuid,omitzero"`

	// SessionID identifies the CLI session.
	SessionID string `json:"session_id,omitzero"`

	// ToolUseID is the originating tool-use ID, when applicable.
	ToolUseID string `json:"tool_use_id,omitzero"`

	// Usage is the final usage breakdown for the task, when reported.
	Usage *TaskUsage `json:"usage,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (TaskNotificationMessage) isMessage()                {}
func (m TaskNotificationMessage) jsonRaw() jsontext.Value { return m.Raw }

// ─── StreamEvent ─────────────────────────────────────────────────────────────

// StreamEvent is a partial-message update emitted during streaming when
// [Options.IncludePartialMessages] is enabled. Mirrors upstream StreamEvent
// (types.py:1171).
//
// Event is the raw Anthropic API stream event, kept opaque to this layer
// because the shape varies per Anthropic stream-event type
// (message_start, content_block_start, content_block_delta, etc.). Callers
// that need to discriminate decode it themselves.
type StreamEvent struct {
	// UUID is the event UUID.
	UUID string `json:"uuid,omitzero"`

	// SessionID identifies the CLI session.
	SessionID string `json:"session_id,omitzero"`

	// Event is the raw Anthropic API stream event payload, opaque to this
	// layer.
	Event jsontext.Value `json:"event,omitzero"`

	// ParentToolUseID, when non-empty, identifies the originating tool call.
	ParentToolUseID string `json:"parent_tool_use_id,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (StreamEvent) isMessage()                {}
func (m StreamEvent) jsonRaw() jsontext.Value { return m.Raw }

// ─── Rate limit types ────────────────────────────────────────────────────────

// RateLimitStatus is the current allow/reject state of a rate-limit window.
// Mirrors upstream RateLimitStatus (types.py:1181).
type RateLimitStatus string

const (
	// RateLimitStatusAllowed indicates the request is within the limit.
	RateLimitStatusAllowed RateLimitStatus = "allowed"

	// RateLimitStatusAllowedWarning indicates the limit is being
	// approached — useful as a "back off soon" signal.
	RateLimitStatusAllowedWarning RateLimitStatus = "allowed_warning"

	// RateLimitStatusRejected indicates the limit has been hit and the
	// request was rejected.
	RateLimitStatusRejected RateLimitStatus = "rejected"
)

// RateLimitType identifies which rate-limit window applies. Mirrors upstream
// RateLimitType (types.py:1182-1184).
type RateLimitType string

const (
	// RateLimitTypeFiveHour is the 5-hour rolling-window limit.
	RateLimitTypeFiveHour RateLimitType = "five_hour"

	// RateLimitTypeSevenDay is the 7-day rolling-window limit.
	RateLimitTypeSevenDay RateLimitType = "seven_day"

	// RateLimitTypeSevenDayOpus is the Opus-specific 7-day limit.
	RateLimitTypeSevenDayOpus RateLimitType = "seven_day_opus"

	// RateLimitTypeSevenDaySonnet is the Sonnet-specific 7-day limit.
	RateLimitTypeSevenDaySonnet RateLimitType = "seven_day_sonnet"

	// RateLimitTypeOverage is the overage / pay-as-you-go window.
	RateLimitTypeOverage RateLimitType = "overage"
)

// RateLimitInfo describes a rate-limit window's current state. Carried inside
// [RateLimitEvent.RateLimitInfo]. Mirrors upstream RateLimitInfo
// (types.py:1188).
//
// Wire-tag trap: upstream Python attrs are snake_case (resets_at,
// rate_limit_type) but the CLI sends camelCase on the wire (resetsAt,
// rateLimitType). The JSON tags below carry the camelCase wire names; the
// Go field names follow Go conventions.
type RateLimitInfo struct {
	// Status is the current rate-limit verdict.
	Status RateLimitStatus `json:"status,omitzero"`

	// ResetsAt is the Unix-seconds timestamp when the limit window resets.
	// Zero means unknown.
	ResetsAt int64 `json:"resetsAt,omitzero"`

	// RateLimitType identifies which window applies.
	RateLimitType RateLimitType `json:"rateLimitType,omitzero"`

	// Utilization is the fraction of the rate limit consumed (0.0–1.0).
	// Zero means unknown.
	Utilization float64 `json:"utilization,omitzero"`

	// OverageStatus is the overage / pay-as-you-go status when applicable.
	OverageStatus RateLimitStatus `json:"overageStatus,omitzero"`

	// OverageResetsAt is when the overage window resets, in Unix seconds.
	OverageResetsAt int64 `json:"overageResetsAt,omitzero"`

	// OverageDisabledReason explains why overage is unavailable when
	// applicable.
	OverageDisabledReason string `json:"overageDisabledReason,omitzero"`

	// Raw preserves the full wire payload, including any fields not yet
	// modeled above, for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

// RateLimitEvent is the top-level message the CLI emits when a rate-limit
// status transitions (e.g. from allowed to allowed_warning). Mirrors upstream
// RateLimitEvent (types.py:1214).
type RateLimitEvent struct {
	// RateLimitInfo is the current rate-limit state.
	RateLimitInfo RateLimitInfo `json:"rate_limit_info,omitzero"`

	// UUID is the event UUID.
	UUID string `json:"uuid,omitzero"`

	// SessionID identifies the CLI session.
	SessionID string `json:"session_id,omitzero"`

	// Raw preserves unknown top-level fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (RateLimitEvent) isMessage()                {}
func (m RateLimitEvent) jsonRaw() jsontext.Value { return m.Raw }

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

// ThinkingBlock carries the model's extended-thinking output. Emitted when
// extended thinking is enabled via [Options.Thinking] or
// [Options.MaxThinkingTokens]. Mirrors upstream ThinkingBlock (types.py:928).
type ThinkingBlock struct {
	// Thinking is the model's thinking text (omitted if
	// [Options.Thinking] is configured with
	// [ThinkingDisplayOmitted] — the signature is still emitted).
	Thinking string `json:"thinking,omitzero"`

	// Signature is the cryptographic signature the API attaches to the
	// thinking content for verification.
	Signature string `json:"signature,omitzero"`

	// Raw preserves unknown fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (ThinkingBlock) isContentBlock()            {}
func (b ThinkingBlock) blockRaw() jsontext.Value { return b.Raw }

// ServerToolName names a server-side tool the API runs on the model's behalf.
// Mirrors upstream ServerToolName (types.py:954-962); the literal set is
// closed and matches the tools the Anthropic API currently runs server-side.
type ServerToolName string

const (
	// ServerToolNameAdvisor invokes the server-side advisor tool.
	ServerToolNameAdvisor ServerToolName = "advisor"

	// ServerToolNameWebSearch invokes the server-side web-search tool.
	ServerToolNameWebSearch ServerToolName = "web_search"

	// ServerToolNameWebFetch invokes the server-side web-fetch tool.
	ServerToolNameWebFetch ServerToolName = "web_fetch"

	// ServerToolNameBashCodeExecution invokes the server-side bash
	// code-execution tool.
	ServerToolNameBashCodeExecution ServerToolName = "bash_code_execution"

	// ServerToolNameTextEditorCodeExecution invokes the server-side
	// text-editor code-execution tool.
	ServerToolNameTextEditorCodeExecution ServerToolName = "text_editor_code_execution"

	// ServerToolNameToolSearchToolRegex invokes the server-side tool-search
	// (regex variant).
	ServerToolNameToolSearchToolRegex ServerToolName = "tool_search_tool_regex"

	// ServerToolNameToolSearchToolBM25 invokes the server-side tool-search
	// (BM25 variant).
	ServerToolNameToolSearchToolBM25 ServerToolName = "tool_search_tool_bm25"
)

// ServerToolUseBlock records a server-side tool call. The API executes these
// tools (advisor, web_search, web_fetch, etc.) without the caller needing to
// return a result; they appear in the message stream alongside regular
// [ToolUseBlock] values but the caller is informational, not actionable.
// Mirrors upstream ServerToolUseBlock (types.py:966).
type ServerToolUseBlock struct {
	// ID is the unique server-tool-use identifier, correlated with
	// [ServerToolResultBlock.ToolUseID].
	ID string `json:"id,omitzero"`

	// Name discriminates which server tool was invoked. Branch on this to
	// know which result schema to expect.
	Name ServerToolName `json:"name,omitzero"`

	// Input is the tool's JSON input payload, opaque to this layer.
	Input jsontext.Value `json:"input,omitzero"`

	// Raw preserves unknown fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (ServerToolUseBlock) isContentBlock()            {}
func (b ServerToolUseBlock) blockRaw() jsontext.Value { return b.Raw }

// ServerToolResultBlock carries the result of a server-side tool call.
// Mirrors upstream ServerToolResultBlock (types.py:981); the Content field is
// kept as a raw [jsontext.Value] because the shape varies per server-tool
// (see [ServerToolUseBlock.Name]).
type ServerToolResultBlock struct {
	// ToolUseID correlates this result with [ServerToolUseBlock.ID].
	ToolUseID string `json:"tool_use_id,omitzero"`

	// Content is the raw result payload; its shape depends on the originating
	// server tool. Callers that care about a specific server-tool's result
	// schema can decode it into the corresponding type.
	Content jsontext.Value `json:"content,omitzero"`

	// Raw preserves unknown fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

func (ServerToolResultBlock) isContentBlock()            {}
func (b ServerToolResultBlock) blockRaw() jsontext.Value { return b.Raw }

// ─── Hook event types ────────────────────────────────────────────────────────

// HookEventKind identifies the lifecycle event that triggered a [Hook].
type HookEventKind string

const (
	// HookEventPreToolUse fires before the CLI invokes a tool.
	HookEventPreToolUse HookEventKind = "PreToolUse"

	// HookEventPostToolUse fires after the CLI receives a tool result.
	HookEventPostToolUse HookEventKind = "PostToolUse"

	// HookEventPostToolUseFailure fires after a tool invocation fails.
	// Mirrors upstream PostToolUseFailure (types.py:330).
	HookEventPostToolUseFailure HookEventKind = "PostToolUseFailure"

	// HookEventSubagentStart fires when a subagent session starts. Mirrors
	// upstream SubagentStart (types.py:382).
	HookEventSubagentStart HookEventKind = "SubagentStart"

	// HookEventPermissionRequest fires when the CLI raises a tool-permission
	// request (e.g., for a hookSpecificOutput permissionDecision "ask"
	// follow-up). Mirrors upstream PermissionRequest (types.py:390).
	HookEventPermissionRequest HookEventKind = "PermissionRequest"

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

// ─── Permission mode ─────────────────────────────────────────────────────────

// PermissionMode is the CLI's permission policy for tool calls. The zero value
// (empty string) lets the CLI pick its configured default and emits no
// --permission-mode flag.
//
// Mirrors upstream PermissionMode = Literal["default", "acceptEdits", "plan",
// "bypassPermissions", "dontAsk", "auto"] (types.py:23-25). Used by
// [Options.PermissionMode] at launch and [ClaudeSDKClient.SetPermissionMode]
// at runtime; values are sent on the wire verbatim, so callers must use one of
// the constants or convert with PermissionMode(s) to remain forward-compatible
// with future CLI literals.
type PermissionMode string

const (
	// PermissionModeDefault selects the CLI's standard permission behavior,
	// prompting for dangerous operations.
	PermissionModeDefault PermissionMode = "default"

	// PermissionModeAcceptEdits auto-accepts file-edit tool calls.
	PermissionModeAcceptEdits PermissionMode = "acceptEdits"

	// PermissionModePlan restricts the session to planning (no execution).
	PermissionModePlan PermissionMode = "plan"

	// PermissionModeBypassPermissions disables every permission check.
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"

	// PermissionModeDontAsk silently allows tool calls without prompting.
	PermissionModeDontAsk PermissionMode = "dontAsk"

	// PermissionModeAuto lets the CLI decide automatically based on context.
	PermissionModeAuto PermissionMode = "auto"
)

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

	// PermissionAsk is the zero value, meaning the SDK expresses no opinion.
	// The control protocol has no third "ask" behavior: a can_use_tool
	// response is either allow or deny. PermissionAsk is therefore sent on the
	// wire as allow with the original tool input unchanged, so the call
	// proceeds and the CLI's configured permission_mode still governs it. Use
	// PermissionDeny to actively block a call.
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
