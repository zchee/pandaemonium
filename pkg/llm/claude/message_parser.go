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
	"bytes"
	"errors"
	"slices"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// ── unexported forward-compat types ─────────────────────────────────────────

// rawMessage wraps an unknown stream-JSON message type. The raw bytes of the
// original line are preserved so callers can decode future message kinds
// without requiring a library update.
type rawMessage struct {
	raw jsontext.Value
	id  string
}

func (rawMessage) isMessage() {}

func (m rawMessage) jsonRaw() jsontext.Value { return m.raw }

func (m rawMessage) messageID() string { return m.id }

// rawContentBlock wraps an unknown content block type. Unknown block kinds are
// preserved so callers can round-trip them without data loss.
type rawContentBlock struct{ raw jsontext.Value }

func (rawContentBlock) isContentBlock() {}

func (b rawContentBlock) blockRaw() jsontext.Value { return b.raw }

// ── parser ───────────────────────────────────────────────────────────────────

// parseMessage decodes one stream-JSON line emitted by the claude CLI into a
// typed Message value.
//
// Blank lines (after stripping the trailing newline) return (nil, nil) and
// should be skipped by the caller. Unknown top-level type values return a
// [rawMessage] that preserves the original bytes for forward compatibility.
// Malformed JSON returns a [CLIJSONDecodeError] with the offending line and
// byte offset; a decodable payload missing the "type" field returns a
// [MessageParseError].
//
// The claude CLI streams assistant and user messages with a nested "message"
// field that contains the Anthropic Messages API object; parseMessage unwraps
// that envelope and flattens the relevant fields (content, model) into the
// corresponding Go struct fields, mirroring the Python SDK's AssistantMessage
// and UserMessage types (_internal/types.py).
func parseMessage(line []byte) (Message, error) {
	data := bytes.TrimRight(line, "\n\r")
	if len(data) == 0 {
		//nolint:nilnil // (nil, nil) is the documented "blank line, skip" sentinel consumed by ReceiveResponse; not an error.
		return nil, nil
	}

	// Peek the type discriminator without a full decode.
	var env struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
	}

	// A decodable payload with no "type" discriminator cannot be routed to a
	// message kind. Upstream raises MessageParseError("Message missing 'type'
	// field") here; mirror that rather than silently wrapping it as a rawMessage
	// (an unknown but present type still becomes rawMessage below, preserving
	// forward compatibility).
	if env.Type == "" {
		return nil, &MessageParseError{Message: "message missing 'type' field", Data: slices.Clone(data)}
	}

	switch env.Type {
	case "assistant":
		return parseAssistantMessage(data, line)
	case "user":
		return parseUserMessage(data, line)
	case "system":
		return parseSystemMessage(data, line)
	case "result":
		var m ResultMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return m, nil
	case "stream_event":
		var m StreamEvent
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return m, nil
	case "rate_limit_event":
		var m RateLimitEvent
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return m, nil
	default:
		// Unknown type — preserve raw bytes for forward compatibility.
		return rawMessage{raw: jsontext.Value(data), id: env.ID}, nil
	}
}

// parseSystemMessage routes a {"type":"system",...} envelope to the right
// concrete [Message] type based on its subtype. Mirrors upstream's
// match-on-subtype dispatch (message_parser.py:188-240): task_started /
// task_progress / task_notification get their own typed structs; every other
// subtype (or no subtype at all) falls back to generic [SystemMessage].
//
// Note on parity: upstream's TaskStartedMessage / TaskProgressMessage /
// TaskNotificationMessage are Python subclasses of SystemMessage, so existing
// isinstance(msg, SystemMessage) checks continue to match. Go has no
// inheritance, so the typed variants are siblings: a callers' type switch
// must list each kind explicitly. The SystemMessage fallback case is
// preserved for unknown subtypes so forward compatibility is unaffected.
func parseSystemMessage(data, line []byte) (Message, error) {
	var env struct {
		Subtype string `json:"subtype"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
	}
	switch env.Subtype {
	case "task_started":
		var m TaskStartedMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return m, nil
	case "task_progress":
		var m TaskProgressMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return m, nil
	case "task_notification":
		var m TaskNotificationMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return m, nil
	case "hook_started", "hook_response":
		// Upstream tolerates three wire-key spellings for the hook event name:
		// "hook_event" (current CLI) / "hook_name" / "hook_event_name"
		// (legacy). Decode the looser shape and copy whichever one is present
		// into HookEventName so callers see a single normalized field.
		var raw struct {
			Subtype       string         `json:"subtype"`
			HookEvent     string         `json:"hook_event"`
			HookName      string         `json:"hook_name"`
			HookEventName string         `json:"hook_event_name"`
			SessionID     string         `json:"session_id"`
			UUID          string         `json:"uuid"`
			Raw           jsontext.Value `json:",inline"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		name := raw.HookEvent
		if name == "" {
			name = raw.HookName
		}
		if name == "" {
			name = raw.HookEventName
		}
		return HookEventMessage{
			Subtype:       raw.Subtype,
			HookEventName: name,
			SessionID:     raw.SessionID,
			UUID:          raw.UUID,
			Raw:           raw.Raw,
		}, nil
	default:
		var m SystemMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return m, nil
	}
}

// parseAssistantMessage extracts a flattened [AssistantMessage] from the
// nested {"type":"assistant","message":{...},...} envelope used by the CLI.
// The outer unknown fields (type, session_id, future keys) are captured in
// AssistantMessage.Raw via the json:",inline" catchall.
func parseAssistantMessage(data, line []byte) (AssistantMessage, error) {
	var raw struct {
		Message struct {
			Content    []jsontext.Value `json:"content,omitzero"`
			Model      string           `json:"model,omitzero"`
			ID         string           `json:"id,omitzero"`
			StopReason string           `json:"stop_reason,omitzero"`
			Usage      jsontext.Value   `json:"usage,omitzero"`
		} `json:"message,omitzero"`
		// Outer-envelope fields are named here so json/v2 ,inline excludes them
		// from Raw (the Raw-exclusion policy); they are promoted to typed fields.
		ParentToolUseID string         `json:"parent_tool_use_id,omitzero"`
		Error           jsontext.Value `json:"error,omitzero"`
		SessionID       string         `json:"session_id,omitzero"`
		UUID            string         `json:"uuid,omitzero"`
		Raw             jsontext.Value `json:",inline"` // captures type and unknown keys
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return AssistantMessage{}, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
	}
	blocks, err := parseContentBlocks(raw.Message.Content, line)
	if err != nil {
		return AssistantMessage{}, err
	}
	return AssistantMessage{
		Content:         blocks,
		Model:           raw.Message.Model,
		MessageID:       raw.Message.ID,
		StopReason:      raw.Message.StopReason,
		Usage:           raw.Message.Usage,
		ParentToolUseID: raw.ParentToolUseID,
		Error:           raw.Error,
		SessionID:       raw.SessionID,
		UUID:            raw.UUID,
		Raw:             raw.Raw,
	}, nil
}

// parseUserMessage extracts a flattened [UserMessage] from the
// nested {"type":"user","message":{...},...} envelope used by the CLI.
func parseUserMessage(data, line []byte) (UserMessage, error) {
	var raw struct {
		Message struct {
			Content []jsontext.Value `json:"content,omitzero"`
		} `json:"message,omitzero"`
		// Outer-envelope fields named here so json/v2 ,inline excludes them from
		// Raw (the Raw-exclusion policy); promoted to typed fields below.
		ParentToolUseID string         `json:"parent_tool_use_id,omitzero"`
		ToolUseResult   jsontext.Value `json:"tool_use_result,omitzero"`
		UUID            string         `json:"uuid,omitzero"`
		Raw             jsontext.Value `json:",inline"` // captures type, session_id, unknown keys
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return UserMessage{}, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
	}
	blocks, err := parseContentBlocks(raw.Message.Content, line)
	if err != nil {
		return UserMessage{}, err
	}
	return UserMessage{
		Content:         blocks,
		ParentToolUseID: raw.ParentToolUseID,
		ToolUseResult:   raw.ToolUseResult,
		UUID:            raw.UUID,
		Raw:             raw.Raw,
	}, nil
}

// parseContentBlocks converts a slice of raw content block JSON values into
// typed [ContentBlock] values. Unknown block types become [rawContentBlock].
func parseContentBlocks(rawBlocks []jsontext.Value, line []byte) ([]ContentBlock, error) {
	if len(rawBlocks) == 0 {
		return nil, nil
	}
	blocks := make([]ContentBlock, 0, len(rawBlocks))
	for _, v := range rawBlocks {
		b, err := parseContentBlock(v, line)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// parseContentBlock decodes a single content block raw JSON value into a typed
// [ContentBlock]. Unknown block type values are wrapped in [rawContentBlock].
func parseContentBlock(v jsontext.Value, line []byte) (ContentBlock, error) {
	var env struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(v, &env); err != nil {
		return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
	}
	switch env.Type {
	case "text":
		var b TextBlock
		if err := json.Unmarshal(v, &b); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return b, nil
	case "tool_use":
		var b ToolUseBlock
		if err := json.Unmarshal(v, &b); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return b, nil
	case "tool_result":
		return parseToolResultBlock(v, line)
	case "thinking":
		var b ThinkingBlock
		if err := json.Unmarshal(v, &b); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return b, nil
	case "server_tool_use":
		var b ServerToolUseBlock
		if err := json.Unmarshal(v, &b); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return b, nil
	case "advisor_tool_result", "server_tool_result", "web_search_tool_result", "web_fetch_tool_result", "bash_code_execution_tool_result", "text_editor_code_execution_tool_result":
		// Server-tool result blocks ship under a single discriminator on current
		// upstream main (advisor_tool_result, message_parser.py:164) and, on the
		// revision this port originally targeted, under per-tool discriminators
		// (web_search_tool_result, web_fetch_tool_result, etc.). All of them
		// collapse into ServerToolResultBlock with the wire type preserved in Raw
		// for callers that need to discriminate.
		var b ServerToolResultBlock
		if err := json.Unmarshal(v, &b); err != nil {
			return nil, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
		}
		return b, nil
	default:
		// Unknown block type — preserve raw bytes for forward compatibility.
		return rawContentBlock{raw: v}, nil
	}
}

// parseToolResultBlock extracts a typed [ToolResultBlock] from raw JSON,
// recursively parsing its Content field (also []ContentBlock).
func parseToolResultBlock(v jsontext.Value, line []byte) (ToolResultBlock, error) {
	var raw struct {
		ToolUseID string           `json:"tool_use_id,omitzero"`
		Content   []jsontext.Value `json:"content,omitzero"`
		IsError   bool             `json:"is_error,omitzero"`
		Raw       jsontext.Value   `json:",inline"`
	}
	if err := json.Unmarshal(v, &raw); err != nil {
		return ToolResultBlock{}, &CLIJSONDecodeError{Line: line, Offset: parseOffset(err)}
	}
	blocks, err := parseContentBlocks(raw.Content, line)
	if err != nil {
		return ToolResultBlock{}, err
	}
	return ToolResultBlock{
		ToolUseID: raw.ToolUseID,
		Content:   blocks,
		IsError:   raw.IsError,
		Raw:       raw.Raw,
	}, nil
}

// parseOffset extracts the byte offset from a go-json-experiment error for use
// in [CLIJSONDecodeError.Offset]. Returns 0 if the error carries no offset.
func parseOffset(err error) int64 {
	if se, ok := errors.AsType[*json.SemanticError](err); ok {
		return se.ByteOffset
	}
	return 0
}
