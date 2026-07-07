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
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// This file mirrors the codex-rs hook command input schemas:
//
//	https://github.com/openai/codex/tree/rust-v0.140.0-alpha.11/codex-rs/hooks/schema/generated/*.command.input.schema.json
//
// Hook command handlers receive one of these payloads on stdin as JSON. The
// hook_event_name member uses the PascalCase wire form from those schemas,
// which is distinct from the camelCase [HookEventName] values used by the
// hooks/list configuration surface in protocol_gen.go.

// HookInputEventName identifies the hook event carried in a hook command
// input payload via the hook_event_name discriminator member.
type HookInputEventName string

const (
	// HookInputEventNamePermissionRequest is the "PermissionRequest" HookInputEventName value.
	HookInputEventNamePermissionRequest HookInputEventName = "PermissionRequest"
	// HookInputEventNamePostCompact is the "PostCompact" HookInputEventName value.
	HookInputEventNamePostCompact HookInputEventName = "PostCompact"
	// HookInputEventNamePostToolUse is the "PostToolUse" HookInputEventName value.
	HookInputEventNamePostToolUse HookInputEventName = "PostToolUse"
	// HookInputEventNamePreCompact is the "PreCompact" HookInputEventName value.
	HookInputEventNamePreCompact HookInputEventName = "PreCompact"
	// HookInputEventNamePreToolUse is the "PreToolUse" HookInputEventName value.
	HookInputEventNamePreToolUse HookInputEventName = "PreToolUse"
	// HookInputEventNameSessionStart is the "SessionStart" HookInputEventName value.
	HookInputEventNameSessionStart HookInputEventName = "SessionStart"
	// HookInputEventNameStop is the "Stop" HookInputEventName value.
	HookInputEventNameStop HookInputEventName = "Stop"
	// HookInputEventNameSubagentStart is the "SubagentStart" HookInputEventName value.
	HookInputEventNameSubagentStart HookInputEventName = "SubagentStart"
	// HookInputEventNameSubagentStop is the "SubagentStop" HookInputEventName value.
	HookInputEventNameSubagentStop HookInputEventName = "SubagentStop"
	// HookInputEventNameUserPromptSubmit is the "UserPromptSubmit" HookInputEventName value.
	HookInputEventNameUserPromptSubmit HookInputEventName = "UserPromptSubmit"
)

// HookPermissionMode is the permission mode active when a hook fires.
type HookPermissionMode string

const (
	// HookPermissionModeDefault is the "default" HookPermissionMode value.
	HookPermissionModeDefault HookPermissionMode = "default"
	// HookPermissionModeAcceptEdits is the "acceptEdits" HookPermissionMode value.
	HookPermissionModeAcceptEdits HookPermissionMode = "acceptEdits"
	// HookPermissionModePlan is the "plan" HookPermissionMode value.
	HookPermissionModePlan HookPermissionMode = "plan"
	// HookPermissionModeDontAsk is the "dontAsk" HookPermissionMode value.
	HookPermissionModeDontAsk HookPermissionMode = "dontAsk"
	// HookPermissionModeBypassPermissions is the "bypassPermissions" HookPermissionMode value.
	HookPermissionModeBypassPermissions HookPermissionMode = "bypassPermissions"
)

// HookCompactTrigger reports what initiated a compaction hook event.
type HookCompactTrigger string

const (
	// HookCompactTriggerManual is the "manual" HookCompactTrigger value.
	HookCompactTriggerManual HookCompactTrigger = "manual"
	// HookCompactTriggerAuto is the "auto" HookCompactTrigger value.
	HookCompactTriggerAuto HookCompactTrigger = "auto"
)

// HookSessionStartSource reports what started the session for a SessionStart hook.
type HookSessionStartSource string

const (
	// HookSessionStartSourceStartup is the "startup" HookSessionStartSource value.
	HookSessionStartSourceStartup HookSessionStartSource = "startup"
	// HookSessionStartSourceResume is the "resume" HookSessionStartSource value.
	HookSessionStartSourceResume HookSessionStartSource = "resume"
	// HookSessionStartSourceClear is the "clear" HookSessionStartSource value.
	HookSessionStartSourceClear HookSessionStartSource = "clear"
	// HookSessionStartSourceCompact is the "compact" HookSessionStartSource value.
	HookSessionStartSourceCompact HookSessionStartSource = "compact"
)

// HookInput is the sealed set of hook command input payloads.
type HookInput interface {
	// EventName reports the hook_event_name discriminator for the payload type.
	EventName() HookInputEventName

	isHookInput()
}

// EncodeHook encodes a hook command input payload into the JSON wire form
// accepted by [DecodeHookInput]. The concrete type's marshaler pins an empty
// hook_event_name to the type's event, and a payload carrying a
// hook_event_name that names a different hook event is rejected so the output
// always round-trips through [DecodeHookInput].
func EncodeHookInput(hook HookInput) ([]byte, error) {
	if value := reflect.ValueOf(hook); !value.IsValid() || (value.Kind() == reflect.Pointer && value.IsNil()) {
		return nil, fmt.Errorf("encode hook input: nil Hook")
	}
	data, err := json.Marshal(hook)
	if err != nil {
		return nil, fmt.Errorf("encode %s hook input: %w", hook.EventName(), err)
	}
	var probe struct {
		HookEventName HookInputEventName `json:"hook_event_name"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("encode %s hook input: probe event name: %w", hook.EventName(), err)
	}
	if probe.HookEventName != hook.EventName() {
		return nil, fmt.Errorf("unexpected hook_event_name %q for %s hook input", probe.HookEventName, hook.EventName())
	}
	return data, nil
}

// DecodeHookInput decodes a hook command input payload into the concrete
// [HookInput] type selected by its hook_event_name member.
func DecodeHookInput(data []byte) (HookInput, error) {
	var probe struct {
		HookEventName HookInputEventName `json:"hook_event_name"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("decode hook input event name: %w", err)
	}
	switch probe.HookEventName {
	case HookInputEventNamePermissionRequest:
		return decodeHookInputPayload[PermissionRequestHookInput](data)
	case HookInputEventNamePostCompact:
		return decodeHookInputPayload[PostCompactHookInput](data)
	case HookInputEventNamePostToolUse:
		return decodeHookInputPayload[PostToolUseHookInput](data)
	case HookInputEventNamePreCompact:
		return decodeHookInputPayload[PreCompactHookInput](data)
	case HookInputEventNamePreToolUse:
		return decodeHookInputPayload[PreToolUseHookInput](data)
	case HookInputEventNameSessionStart:
		return decodeHookInputPayload[SessionStartHookInput](data)
	case HookInputEventNameStop:
		return decodeHookInputPayload[StopHookInput](data)
	case HookInputEventNameSubagentStart:
		return decodeHookInputPayload[SubagentStartHookInput](data)
	case HookInputEventNameSubagentStop:
		return decodeHookInputPayload[SubagentStopHookInput](data)
	case HookInputEventNameUserPromptSubmit:
		return decodeHookInputPayload[UserPromptSubmitHookInput](data)
	case "":
		return nil, fmt.Errorf("hook input is missing hook_event_name")
	default:
		return nil, fmt.Errorf("unsupported hook input event name %q", probe.HookEventName)
	}
}

// DecodeHookInputs decodes a stream of hook command input payloads into their
// concrete [HookInput] types. The stream is a sequence of JSON values, one per
// JSON Lines record, decoded in order via a single streaming decoder so a
// payload may span multiple physical lines. Blank lines and whitespace between
// records are tolerated.
//
// Decoding stops at the first malformed record and returns the hooks decoded
// so far alongside a record-indexed error, so partial progress is never
// discarded.
func DecodeHookInputs(data []byte) ([]HookInput, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	var hooks []HookInput
	for {
		hook, err := decodeHookInputValue(dec)
		if errors.Is(err, io.EOF) {
			return hooks, nil
		}
		if err != nil {
			return hooks, fmt.Errorf("decode hook input at record %d: %w", len(hooks), err)
		}
		hooks = append(hooks, hook)
	}
}

// decodeHookInputValue reads the next top-level JSON value from dec and decodes
// it into its concrete [HookInput] type. It reports [io.EOF] once the stream is
// exhausted.
func decodeHookInputValue(dec *jsontext.Decoder) (HookInput, error) {
	raw, err := dec.ReadValue()
	if err != nil {
		return nil, err
	}
	return DecodeHookInput(raw)
}

func decodeHookInputPayload[T HookInput](data []byte) (HookInput, error) {
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("decode %s hook input: %w", value.EventName(), err)
	}
	return value, nil
}

// PermissionRequestHookInput is the command input payload delivered to a tool permission request hook,
// mirroring the permission-request.command.input schema definition.
type PermissionRequestHookInput struct {
	AgentID        string             `json:"agent_id,omitzero"`
	AgentType      string             `json:"agent_type,omitzero"`
	Cwd            string             `json:"cwd"`
	HookEventName  HookInputEventName `json:"hook_event_name"`
	Model          string             `json:"model"`
	PermissionMode HookPermissionMode `json:"permission_mode"`
	SessionID      string             `json:"session_id"`
	ToolInput      jsontext.Value     `json:"tool_input"`
	ToolName       string             `json:"tool_name"`
	TranscriptPath *string            `json:"transcript_path"`
	// TurnID Codex extension: expose the active turn id to internal turn-scoped hooks.
	TurnID string `json:"turn_id"`
}

var (
	_ HookInput            = PermissionRequestHookInput{}
	_ json.MarshalerTo     = PermissionRequestHookInput{}
	_ json.UnmarshalerFrom = (*PermissionRequestHookInput)(nil)
)

func (PermissionRequestHookInput) isHookInput() {}

// EventName reports the hook_event_name discriminator for the payload type.
func (PermissionRequestHookInput) EventName() HookInputEventName {
	return HookInputEventNamePermissionRequest
}

// MarshalJSONTo implements [json.MarshalerTo]. An empty HookEventName is
// normalized to [HookInputEventNamePermissionRequest] so hand-built payloads stay valid.
func (value PermissionRequestHookInput) MarshalJSONTo(enc *jsontext.Encoder) error {
	raw := struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		PermissionMode HookPermissionMode `json:"permission_mode"`
		SessionID      string             `json:"session_id"`
		ToolInput      jsontext.Value     `json:"tool_input"`
		ToolName       string             `json:"tool_name"`
		TranscriptPath *string            `json:"transcript_path"`
		TurnID         string             `json:"turn_id"`
	}{
		AgentID:        value.AgentID,
		AgentType:      value.AgentType,
		Cwd:            value.Cwd,
		HookEventName:  value.HookEventName,
		Model:          value.Model,
		PermissionMode: value.PermissionMode,
		SessionID:      value.SessionID,
		ToolInput:      value.ToolInput,
		ToolName:       value.ToolName,
		TranscriptPath: value.TranscriptPath,
		TurnID:         value.TurnID,
	}
	if raw.HookEventName == "" {
		raw.HookEventName = HookInputEventNamePermissionRequest
	}
	return json.MarshalEncode(enc, &raw)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom]. It rejects payloads
// whose hook_event_name names a different hook event.
func (value *PermissionRequestHookInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		PermissionMode HookPermissionMode `json:"permission_mode"`
		SessionID      string             `json:"session_id"`
		ToolInput      jsontext.Value     `json:"tool_input"`
		ToolName       string             `json:"tool_name"`
		TranscriptPath *string            `json:"transcript_path"`
		TurnID         string             `json:"turn_id"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw.HookEventName != "" && raw.HookEventName != HookInputEventNamePermissionRequest {
		return fmt.Errorf("unexpected hook_event_name %q for %s hook input", raw.HookEventName, HookInputEventNamePermissionRequest)
	}
	value.AgentID = raw.AgentID
	value.AgentType = raw.AgentType
	value.Cwd = raw.Cwd
	value.Model = raw.Model
	value.PermissionMode = raw.PermissionMode
	value.SessionID = raw.SessionID
	value.ToolInput = raw.ToolInput
	value.ToolName = raw.ToolName
	value.TranscriptPath = raw.TranscriptPath
	value.TurnID = raw.TurnID
	value.HookEventName = HookInputEventNamePermissionRequest
	return nil
}

// PostCompactHookInput is the command input payload delivered to a post-compaction hook,
// mirroring the post-compact.command.input schema definition.
type PostCompactHookInput struct {
	AgentID        string             `json:"agent_id,omitzero"`
	AgentType      string             `json:"agent_type,omitzero"`
	Cwd            string             `json:"cwd"`
	HookEventName  HookInputEventName `json:"hook_event_name"`
	Model          string             `json:"model"`
	SessionID      string             `json:"session_id"`
	TranscriptPath *string            `json:"transcript_path"`
	Trigger        HookCompactTrigger `json:"trigger"`
	// TurnID Codex extension: expose the active turn id to internal turn-scoped hooks.
	TurnID string `json:"turn_id"`
}

var (
	_ HookInput            = PostCompactHookInput{}
	_ json.MarshalerTo     = PostCompactHookInput{}
	_ json.UnmarshalerFrom = (*PostCompactHookInput)(nil)
)

func (PostCompactHookInput) isHookInput() {}

// EventName reports the hook_event_name discriminator for the payload type.
func (PostCompactHookInput) EventName() HookInputEventName { return HookInputEventNamePostCompact }

// MarshalJSONTo implements [json.MarshalerTo]. An empty HookEventName is
// normalized to [HookInputEventNamePostCompact] so hand-built payloads stay valid.
func (value PostCompactHookInput) MarshalJSONTo(enc *jsontext.Encoder) error {
	raw := struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		SessionID      string             `json:"session_id"`
		TranscriptPath *string            `json:"transcript_path"`
		Trigger        HookCompactTrigger `json:"trigger"`
		TurnID         string             `json:"turn_id"`
	}{
		AgentID:        value.AgentID,
		AgentType:      value.AgentType,
		Cwd:            value.Cwd,
		HookEventName:  value.HookEventName,
		Model:          value.Model,
		SessionID:      value.SessionID,
		TranscriptPath: value.TranscriptPath,
		Trigger:        value.Trigger,
		TurnID:         value.TurnID,
	}
	if raw.HookEventName == "" {
		raw.HookEventName = HookInputEventNamePostCompact
	}
	return json.MarshalEncode(enc, &raw)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom]. It rejects payloads
// whose hook_event_name names a different hook event.
func (value *PostCompactHookInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		SessionID      string             `json:"session_id"`
		TranscriptPath *string            `json:"transcript_path"`
		Trigger        HookCompactTrigger `json:"trigger"`
		TurnID         string             `json:"turn_id"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw.HookEventName != "" && raw.HookEventName != HookInputEventNamePostCompact {
		return fmt.Errorf("unexpected hook_event_name %q for %s hook input", raw.HookEventName, HookInputEventNamePostCompact)
	}
	value.AgentID = raw.AgentID
	value.AgentType = raw.AgentType
	value.Cwd = raw.Cwd
	value.Model = raw.Model
	value.SessionID = raw.SessionID
	value.TranscriptPath = raw.TranscriptPath
	value.Trigger = raw.Trigger
	value.TurnID = raw.TurnID
	value.HookEventName = HookInputEventNamePostCompact
	return nil
}

// PostToolUseHookInput is the command input payload delivered to a post-tool-use hook,
// mirroring the post-tool-use.command.input schema definition.
type PostToolUseHookInput struct {
	AgentID        string             `json:"agent_id,omitzero"`
	AgentType      string             `json:"agent_type,omitzero"`
	Cwd            string             `json:"cwd"`
	HookEventName  HookInputEventName `json:"hook_event_name"`
	Model          string             `json:"model"`
	PermissionMode HookPermissionMode `json:"permission_mode"`
	SessionID      string             `json:"session_id"`
	ToolInput      jsontext.Value     `json:"tool_input"`
	ToolName       string             `json:"tool_name"`
	ToolResponse   jsontext.Value     `json:"tool_response"`
	ToolUseID      string             `json:"tool_use_id"`
	TranscriptPath *string            `json:"transcript_path"`
	// TurnID Codex extension: expose the active turn id to internal turn-scoped hooks.
	TurnID string `json:"turn_id"`
}

var (
	_ HookInput            = PostToolUseHookInput{}
	_ json.MarshalerTo     = PostToolUseHookInput{}
	_ json.UnmarshalerFrom = (*PostToolUseHookInput)(nil)
)

func (PostToolUseHookInput) isHookInput() {}

// EventName reports the hook_event_name discriminator for the payload type.
func (PostToolUseHookInput) EventName() HookInputEventName { return HookInputEventNamePostToolUse }

// MarshalJSONTo implements [json.MarshalerTo]. An empty HookEventName is
// normalized to [HookInputEventNamePostToolUse] so hand-built payloads stay valid.
func (value PostToolUseHookInput) MarshalJSONTo(enc *jsontext.Encoder) error {
	raw := struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		PermissionMode HookPermissionMode `json:"permission_mode"`
		SessionID      string             `json:"session_id"`
		ToolInput      jsontext.Value     `json:"tool_input"`
		ToolName       string             `json:"tool_name"`
		ToolResponse   jsontext.Value     `json:"tool_response"`
		ToolUseID      string             `json:"tool_use_id"`
		TranscriptPath *string            `json:"transcript_path"`
		TurnID         string             `json:"turn_id"`
	}{
		AgentID:        value.AgentID,
		AgentType:      value.AgentType,
		Cwd:            value.Cwd,
		HookEventName:  value.HookEventName,
		Model:          value.Model,
		PermissionMode: value.PermissionMode,
		SessionID:      value.SessionID,
		ToolInput:      value.ToolInput,
		ToolName:       value.ToolName,
		ToolResponse:   value.ToolResponse,
		ToolUseID:      value.ToolUseID,
		TranscriptPath: value.TranscriptPath,
		TurnID:         value.TurnID,
	}
	if raw.HookEventName == "" {
		raw.HookEventName = HookInputEventNamePostToolUse
	}
	return json.MarshalEncode(enc, &raw)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom]. It rejects payloads
// whose hook_event_name names a different hook event.
func (value *PostToolUseHookInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		PermissionMode HookPermissionMode `json:"permission_mode"`
		SessionID      string             `json:"session_id"`
		ToolInput      jsontext.Value     `json:"tool_input"`
		ToolName       string             `json:"tool_name"`
		ToolResponse   jsontext.Value     `json:"tool_response"`
		ToolUseID      string             `json:"tool_use_id"`
		TranscriptPath *string            `json:"transcript_path"`
		TurnID         string             `json:"turn_id"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw.HookEventName != "" && raw.HookEventName != HookInputEventNamePostToolUse {
		return fmt.Errorf("unexpected hook_event_name %q for %s hook input", raw.HookEventName, HookInputEventNamePostToolUse)
	}
	value.AgentID = raw.AgentID
	value.AgentType = raw.AgentType
	value.Cwd = raw.Cwd
	value.Model = raw.Model
	value.PermissionMode = raw.PermissionMode
	value.SessionID = raw.SessionID
	value.ToolInput = raw.ToolInput
	value.ToolName = raw.ToolName
	value.ToolResponse = raw.ToolResponse
	value.ToolUseID = raw.ToolUseID
	value.TranscriptPath = raw.TranscriptPath
	value.TurnID = raw.TurnID
	value.HookEventName = HookInputEventNamePostToolUse
	return nil
}

// PreCompactHookInput is the command input payload delivered to a pre-compaction hook,
// mirroring the pre-compact.command.input schema definition.
type PreCompactHookInput struct {
	AgentID        string             `json:"agent_id,omitzero"`
	AgentType      string             `json:"agent_type,omitzero"`
	Cwd            string             `json:"cwd"`
	HookEventName  HookInputEventName `json:"hook_event_name"`
	Model          string             `json:"model"`
	SessionID      string             `json:"session_id"`
	TranscriptPath *string            `json:"transcript_path"`
	Trigger        HookCompactTrigger `json:"trigger"`
	// TurnID Codex extension: expose the active turn id to internal turn-scoped hooks.
	TurnID string `json:"turn_id"`
}

var (
	_ HookInput            = PreCompactHookInput{}
	_ json.MarshalerTo     = PreCompactHookInput{}
	_ json.UnmarshalerFrom = (*PreCompactHookInput)(nil)
)

func (PreCompactHookInput) isHookInput() {}

// EventName reports the hook_event_name discriminator for the payload type.
func (PreCompactHookInput) EventName() HookInputEventName { return HookInputEventNamePreCompact }

// MarshalJSONTo implements [json.MarshalerTo]. An empty HookEventName is
// normalized to [HookInputEventNamePreCompact] so hand-built payloads stay valid.
func (value PreCompactHookInput) MarshalJSONTo(enc *jsontext.Encoder) error {
	raw := struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		SessionID      string             `json:"session_id"`
		TranscriptPath *string            `json:"transcript_path"`
		Trigger        HookCompactTrigger `json:"trigger"`
		TurnID         string             `json:"turn_id"`
	}{
		AgentID:        value.AgentID,
		AgentType:      value.AgentType,
		Cwd:            value.Cwd,
		HookEventName:  value.HookEventName,
		Model:          value.Model,
		SessionID:      value.SessionID,
		TranscriptPath: value.TranscriptPath,
		Trigger:        value.Trigger,
		TurnID:         value.TurnID,
	}
	if raw.HookEventName == "" {
		raw.HookEventName = HookInputEventNamePreCompact
	}
	return json.MarshalEncode(enc, &raw)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom]. It rejects payloads
// whose hook_event_name names a different hook event.
func (value *PreCompactHookInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		SessionID      string             `json:"session_id"`
		TranscriptPath *string            `json:"transcript_path"`
		Trigger        HookCompactTrigger `json:"trigger"`
		TurnID         string             `json:"turn_id"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw.HookEventName != "" && raw.HookEventName != HookInputEventNamePreCompact {
		return fmt.Errorf("unexpected hook_event_name %q for %s hook input", raw.HookEventName, HookInputEventNamePreCompact)
	}
	value.AgentID = raw.AgentID
	value.AgentType = raw.AgentType
	value.Cwd = raw.Cwd
	value.Model = raw.Model
	value.SessionID = raw.SessionID
	value.TranscriptPath = raw.TranscriptPath
	value.Trigger = raw.Trigger
	value.TurnID = raw.TurnID
	value.HookEventName = HookInputEventNamePreCompact
	return nil
}

// PreToolUseHookInput is the command input payload delivered to a pre-tool-use hook,
// mirroring the pre-tool-use.command.input schema definition.
type PreToolUseHookInput struct {
	AgentID        string             `json:"agent_id,omitzero"`
	AgentType      string             `json:"agent_type,omitzero"`
	Cwd            string             `json:"cwd"`
	HookEventName  HookInputEventName `json:"hook_event_name"`
	Model          string             `json:"model"`
	PermissionMode HookPermissionMode `json:"permission_mode"`
	SessionID      string             `json:"session_id"`
	ToolInput      jsontext.Value     `json:"tool_input"`
	ToolName       string             `json:"tool_name"`
	ToolUseID      string             `json:"tool_use_id"`
	TranscriptPath *string            `json:"transcript_path"`
	// TurnID Codex extension: expose the active turn id to internal turn-scoped hooks.
	TurnID string `json:"turn_id"`
}

var (
	_ HookInput            = PreToolUseHookInput{}
	_ json.MarshalerTo     = PreToolUseHookInput{}
	_ json.UnmarshalerFrom = (*PreToolUseHookInput)(nil)
)

func (PreToolUseHookInput) isHookInput() {}

// EventName reports the hook_event_name discriminator for the payload type.
func (PreToolUseHookInput) EventName() HookInputEventName { return HookInputEventNamePreToolUse }

// MarshalJSONTo implements [json.MarshalerTo]. An empty HookEventName is
// normalized to [HookInputEventNamePreToolUse] so hand-built payloads stay valid.
func (value PreToolUseHookInput) MarshalJSONTo(enc *jsontext.Encoder) error {
	raw := struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		PermissionMode HookPermissionMode `json:"permission_mode"`
		SessionID      string             `json:"session_id"`
		ToolInput      jsontext.Value     `json:"tool_input"`
		ToolName       string             `json:"tool_name"`
		ToolUseID      string             `json:"tool_use_id"`
		TranscriptPath *string            `json:"transcript_path"`
		TurnID         string             `json:"turn_id"`
	}{
		AgentID:        value.AgentID,
		AgentType:      value.AgentType,
		Cwd:            value.Cwd,
		HookEventName:  value.HookEventName,
		Model:          value.Model,
		PermissionMode: value.PermissionMode,
		SessionID:      value.SessionID,
		ToolInput:      value.ToolInput,
		ToolName:       value.ToolName,
		ToolUseID:      value.ToolUseID,
		TranscriptPath: value.TranscriptPath,
		TurnID:         value.TurnID,
	}
	if raw.HookEventName == "" {
		raw.HookEventName = HookInputEventNamePreToolUse
	}
	return json.MarshalEncode(enc, &raw)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom]. It rejects payloads
// whose hook_event_name names a different hook event.
func (value *PreToolUseHookInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		PermissionMode HookPermissionMode `json:"permission_mode"`
		SessionID      string             `json:"session_id"`
		ToolInput      jsontext.Value     `json:"tool_input"`
		ToolName       string             `json:"tool_name"`
		ToolUseID      string             `json:"tool_use_id"`
		TranscriptPath *string            `json:"transcript_path"`
		TurnID         string             `json:"turn_id"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw.HookEventName != "" && raw.HookEventName != HookInputEventNamePreToolUse {
		return fmt.Errorf("unexpected hook_event_name %q for %s hook input", raw.HookEventName, HookInputEventNamePreToolUse)
	}
	value.AgentID = raw.AgentID
	value.AgentType = raw.AgentType
	value.Cwd = raw.Cwd
	value.Model = raw.Model
	value.PermissionMode = raw.PermissionMode
	value.SessionID = raw.SessionID
	value.ToolInput = raw.ToolInput
	value.ToolName = raw.ToolName
	value.ToolUseID = raw.ToolUseID
	value.TranscriptPath = raw.TranscriptPath
	value.TurnID = raw.TurnID
	value.HookEventName = HookInputEventNamePreToolUse
	return nil
}

// SessionStartHookInput is the command input payload delivered to a session start hook,
// mirroring the session-start.command.input schema definition.
type SessionStartHookInput struct {
	Cwd            string                 `json:"cwd"`
	HookEventName  HookInputEventName     `json:"hook_event_name"`
	Model          string                 `json:"model"`
	PermissionMode HookPermissionMode     `json:"permission_mode"`
	SessionID      string                 `json:"session_id"`
	Source         HookSessionStartSource `json:"source"`
	TranscriptPath *string                `json:"transcript_path"`
}

var (
	_ HookInput            = SessionStartHookInput{}
	_ json.MarshalerTo     = SessionStartHookInput{}
	_ json.UnmarshalerFrom = (*SessionStartHookInput)(nil)
)

func (SessionStartHookInput) isHookInput() {}

// EventName reports the hook_event_name discriminator for the payload type.
func (SessionStartHookInput) EventName() HookInputEventName { return HookInputEventNameSessionStart }

// MarshalJSONTo implements [json.MarshalerTo]. An empty HookEventName is
// normalized to [HookInputEventNameSessionStart] so hand-built payloads stay valid.
func (value SessionStartHookInput) MarshalJSONTo(enc *jsontext.Encoder) error {
	raw := struct {
		Cwd            string                 `json:"cwd"`
		HookEventName  HookInputEventName     `json:"hook_event_name"`
		Model          string                 `json:"model"`
		PermissionMode HookPermissionMode     `json:"permission_mode"`
		SessionID      string                 `json:"session_id"`
		Source         HookSessionStartSource `json:"source"`
		TranscriptPath *string                `json:"transcript_path"`
	}{
		Cwd:            value.Cwd,
		HookEventName:  value.HookEventName,
		Model:          value.Model,
		PermissionMode: value.PermissionMode,
		SessionID:      value.SessionID,
		Source:         value.Source,
		TranscriptPath: value.TranscriptPath,
	}
	if raw.HookEventName == "" {
		raw.HookEventName = HookInputEventNameSessionStart
	}
	return json.MarshalEncode(enc, &raw)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom]. It rejects payloads
// whose hook_event_name names a different hook event.
func (value *SessionStartHookInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		Cwd            string                 `json:"cwd"`
		HookEventName  HookInputEventName     `json:"hook_event_name"`
		Model          string                 `json:"model"`
		PermissionMode HookPermissionMode     `json:"permission_mode"`
		SessionID      string                 `json:"session_id"`
		Source         HookSessionStartSource `json:"source"`
		TranscriptPath *string                `json:"transcript_path"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw.HookEventName != "" && raw.HookEventName != HookInputEventNameSessionStart {
		return fmt.Errorf("unexpected hook_event_name %q for %s hook input", raw.HookEventName, HookInputEventNameSessionStart)
	}
	value.Cwd = raw.Cwd
	value.Model = raw.Model
	value.PermissionMode = raw.PermissionMode
	value.SessionID = raw.SessionID
	value.Source = raw.Source
	value.TranscriptPath = raw.TranscriptPath
	value.HookEventName = HookInputEventNameSessionStart
	return nil
}

// StopHookInput is the command input payload delivered to a session stop hook,
// mirroring the stop.command.input schema definition.
type StopHookInput struct {
	Cwd                  string             `json:"cwd"`
	HookEventName        HookInputEventName `json:"hook_event_name"`
	LastAssistantMessage *string            `json:"last_assistant_message"`
	Model                string             `json:"model"`
	PermissionMode       HookPermissionMode `json:"permission_mode"`
	SessionID            string             `json:"session_id"`
	StopHookActive       bool               `json:"stop_hook_active"`
	TranscriptPath       *string            `json:"transcript_path"`
	// TurnID Codex extension: expose the active turn id to internal turn-scoped hooks.
	TurnID string `json:"turn_id"`
}

var (
	_ HookInput            = StopHookInput{}
	_ json.MarshalerTo     = StopHookInput{}
	_ json.UnmarshalerFrom = (*StopHookInput)(nil)
)

func (StopHookInput) isHookInput() {}

// EventName reports the hook_event_name discriminator for the payload type.
func (StopHookInput) EventName() HookInputEventName { return HookInputEventNameStop }

// MarshalJSONTo implements [json.MarshalerTo]. An empty HookEventName is
// normalized to [HookInputEventNameStop] so hand-built payloads stay valid.
func (value StopHookInput) MarshalJSONTo(enc *jsontext.Encoder) error {
	raw := struct {
		Cwd                  string             `json:"cwd"`
		HookEventName        HookInputEventName `json:"hook_event_name"`
		LastAssistantMessage *string            `json:"last_assistant_message"`
		Model                string             `json:"model"`
		PermissionMode       HookPermissionMode `json:"permission_mode"`
		SessionID            string             `json:"session_id"`
		StopHookActive       bool               `json:"stop_hook_active"`
		TranscriptPath       *string            `json:"transcript_path"`
		TurnID               string             `json:"turn_id"`
	}{
		Cwd:                  value.Cwd,
		HookEventName:        value.HookEventName,
		LastAssistantMessage: value.LastAssistantMessage,
		Model:                value.Model,
		PermissionMode:       value.PermissionMode,
		SessionID:            value.SessionID,
		StopHookActive:       value.StopHookActive,
		TranscriptPath:       value.TranscriptPath,
		TurnID:               value.TurnID,
	}
	if raw.HookEventName == "" {
		raw.HookEventName = HookInputEventNameStop
	}
	return json.MarshalEncode(enc, &raw)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom]. It rejects payloads
// whose hook_event_name names a different hook event.
func (value *StopHookInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		Cwd                  string             `json:"cwd"`
		HookEventName        HookInputEventName `json:"hook_event_name"`
		LastAssistantMessage *string            `json:"last_assistant_message"`
		Model                string             `json:"model"`
		PermissionMode       HookPermissionMode `json:"permission_mode"`
		SessionID            string             `json:"session_id"`
		StopHookActive       bool               `json:"stop_hook_active"`
		TranscriptPath       *string            `json:"transcript_path"`
		TurnID               string             `json:"turn_id"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw.HookEventName != "" && raw.HookEventName != HookInputEventNameStop {
		return fmt.Errorf("unexpected hook_event_name %q for %s hook input", raw.HookEventName, HookInputEventNameStop)
	}
	value.Cwd = raw.Cwd
	value.LastAssistantMessage = raw.LastAssistantMessage
	value.Model = raw.Model
	value.PermissionMode = raw.PermissionMode
	value.SessionID = raw.SessionID
	value.StopHookActive = raw.StopHookActive
	value.TranscriptPath = raw.TranscriptPath
	value.TurnID = raw.TurnID
	value.HookEventName = HookInputEventNameStop
	return nil
}

// SubagentStartHookInput is the command input payload delivered to a subagent start hook,
// mirroring the subagent-start.command.input schema definition.
type SubagentStartHookInput struct {
	AgentID        string             `json:"agent_id"`
	AgentType      string             `json:"agent_type"`
	Cwd            string             `json:"cwd"`
	HookEventName  HookInputEventName `json:"hook_event_name"`
	Model          string             `json:"model"`
	PermissionMode HookPermissionMode `json:"permission_mode"`
	SessionID      string             `json:"session_id"`
	TranscriptPath *string            `json:"transcript_path"`
	// TurnID Codex extension: expose the active turn id to internal turn-scoped hooks.
	TurnID string `json:"turn_id"`
}

var (
	_ HookInput            = SubagentStartHookInput{}
	_ json.MarshalerTo     = SubagentStartHookInput{}
	_ json.UnmarshalerFrom = (*SubagentStartHookInput)(nil)
)

func (SubagentStartHookInput) isHookInput() {}

// EventName reports the hook_event_name discriminator for the payload type.
func (SubagentStartHookInput) EventName() HookInputEventName { return HookInputEventNameSubagentStart }

// MarshalJSONTo implements [json.MarshalerTo]. An empty HookEventName is
// normalized to [HookInputEventNameSubagentStart] so hand-built payloads stay valid.
func (value SubagentStartHookInput) MarshalJSONTo(enc *jsontext.Encoder) error {
	raw := struct {
		AgentID        string             `json:"agent_id"`
		AgentType      string             `json:"agent_type"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		PermissionMode HookPermissionMode `json:"permission_mode"`
		SessionID      string             `json:"session_id"`
		TranscriptPath *string            `json:"transcript_path"`
		TurnID         string             `json:"turn_id"`
	}{
		AgentID:        value.AgentID,
		AgentType:      value.AgentType,
		Cwd:            value.Cwd,
		HookEventName:  value.HookEventName,
		Model:          value.Model,
		PermissionMode: value.PermissionMode,
		SessionID:      value.SessionID,
		TranscriptPath: value.TranscriptPath,
		TurnID:         value.TurnID,
	}
	if raw.HookEventName == "" {
		raw.HookEventName = HookInputEventNameSubagentStart
	}
	return json.MarshalEncode(enc, &raw)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom]. It rejects payloads
// whose hook_event_name names a different hook event.
func (value *SubagentStartHookInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		AgentID        string             `json:"agent_id"`
		AgentType      string             `json:"agent_type"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		PermissionMode HookPermissionMode `json:"permission_mode"`
		SessionID      string             `json:"session_id"`
		TranscriptPath *string            `json:"transcript_path"`
		TurnID         string             `json:"turn_id"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw.HookEventName != "" && raw.HookEventName != HookInputEventNameSubagentStart {
		return fmt.Errorf("unexpected hook_event_name %q for %s hook input", raw.HookEventName, HookInputEventNameSubagentStart)
	}
	value.AgentID = raw.AgentID
	value.AgentType = raw.AgentType
	value.Cwd = raw.Cwd
	value.Model = raw.Model
	value.PermissionMode = raw.PermissionMode
	value.SessionID = raw.SessionID
	value.TranscriptPath = raw.TranscriptPath
	value.TurnID = raw.TurnID
	value.HookEventName = HookInputEventNameSubagentStart
	return nil
}

// SubagentStopHookInput is the command input payload delivered to a subagent stop hook,
// mirroring the subagent-stop.command.input schema definition.
type SubagentStopHookInput struct {
	AgentID              string             `json:"agent_id"`
	AgentTranscriptPath  *string            `json:"agent_transcript_path"`
	AgentType            string             `json:"agent_type"`
	Cwd                  string             `json:"cwd"`
	HookEventName        HookInputEventName `json:"hook_event_name"`
	Model                string             `json:"model"`
	LastAssistantMessage *string            `json:"last_assistant_message"`
	PermissionMode       HookPermissionMode `json:"permission_mode"`
	SessionID            string             `json:"session_id"`
	StopHookActive       bool               `json:"stop_hook_active"`
	TranscriptPath       *string            `json:"transcript_path"`
	// TurnID Codex extension: expose the active turn id to internal turn-scoped hooks.
	TurnID string `json:"turn_id"`
}

var (
	_ HookInput            = SubagentStopHookInput{}
	_ json.MarshalerTo     = SubagentStopHookInput{}
	_ json.UnmarshalerFrom = (*SubagentStopHookInput)(nil)
)

func (SubagentStopHookInput) isHookInput() {}

// EventName reports the hook_event_name discriminator for the payload type.
func (SubagentStopHookInput) EventName() HookInputEventName { return HookInputEventNameSubagentStop }

// MarshalJSONTo implements [json.MarshalerTo]. An empty HookEventName is
// normalized to [HookInputEventNameSubagentStop] so hand-built payloads stay valid.
func (value SubagentStopHookInput) MarshalJSONTo(enc *jsontext.Encoder) error {
	raw := struct {
		AgentID              string             `json:"agent_id"`
		AgentTranscriptPath  *string            `json:"agent_transcript_path"`
		AgentType            string             `json:"agent_type"`
		Cwd                  string             `json:"cwd"`
		HookEventName        HookInputEventName `json:"hook_event_name"`
		Model                string             `json:"model"`
		LastAssistantMessage *string            `json:"last_assistant_message"`
		PermissionMode       HookPermissionMode `json:"permission_mode"`
		SessionID            string             `json:"session_id"`
		StopHookActive       bool               `json:"stop_hook_active"`
		TranscriptPath       *string            `json:"transcript_path"`
		TurnID               string             `json:"turn_id"`
	}{
		AgentID:              value.AgentID,
		AgentTranscriptPath:  value.AgentTranscriptPath,
		AgentType:            value.AgentType,
		Cwd:                  value.Cwd,
		HookEventName:        value.HookEventName,
		Model:                value.Model,
		LastAssistantMessage: value.LastAssistantMessage,
		PermissionMode:       value.PermissionMode,
		SessionID:            value.SessionID,
		StopHookActive:       value.StopHookActive,
		TranscriptPath:       value.TranscriptPath,
		TurnID:               value.TurnID,
	}
	if raw.HookEventName == "" {
		raw.HookEventName = HookInputEventNameSubagentStop
	}
	return json.MarshalEncode(enc, &raw)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom]. It rejects payloads
// whose hook_event_name names a different hook event.
func (value *SubagentStopHookInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		AgentID              string             `json:"agent_id"`
		AgentTranscriptPath  *string            `json:"agent_transcript_path"`
		AgentType            string             `json:"agent_type"`
		Cwd                  string             `json:"cwd"`
		HookEventName        HookInputEventName `json:"hook_event_name"`
		Model                string             `json:"model"`
		LastAssistantMessage *string            `json:"last_assistant_message"`
		PermissionMode       HookPermissionMode `json:"permission_mode"`
		SessionID            string             `json:"session_id"`
		StopHookActive       bool               `json:"stop_hook_active"`
		TranscriptPath       *string            `json:"transcript_path"`
		TurnID               string             `json:"turn_id"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw.HookEventName != "" && raw.HookEventName != HookInputEventNameSubagentStop {
		return fmt.Errorf("unexpected hook_event_name %q for %s hook input", raw.HookEventName, HookInputEventNameSubagentStop)
	}
	value.AgentID = raw.AgentID
	value.AgentTranscriptPath = raw.AgentTranscriptPath
	value.AgentType = raw.AgentType
	value.Cwd = raw.Cwd
	value.Model = raw.Model
	value.LastAssistantMessage = raw.LastAssistantMessage
	value.PermissionMode = raw.PermissionMode
	value.SessionID = raw.SessionID
	value.StopHookActive = raw.StopHookActive
	value.TranscriptPath = raw.TranscriptPath
	value.TurnID = raw.TurnID
	value.HookEventName = HookInputEventNameSubagentStop
	return nil
}

// UserPromptSubmitHookInput is the command input payload delivered to a user prompt submission hook,
// mirroring the user-prompt-submit.command.input schema definition.
type UserPromptSubmitHookInput struct {
	AgentID        string             `json:"agent_id,omitzero"`
	AgentType      string             `json:"agent_type,omitzero"`
	Cwd            string             `json:"cwd"`
	HookEventName  HookInputEventName `json:"hook_event_name"`
	Model          string             `json:"model"`
	PermissionMode HookPermissionMode `json:"permission_mode"`
	Prompt         string             `json:"prompt"`
	SessionID      string             `json:"session_id"`
	TranscriptPath *string            `json:"transcript_path"`
	// TurnID Codex extension: expose the active turn id to internal turn-scoped hooks.
	TurnID string `json:"turn_id"`
}

var (
	_ HookInput            = UserPromptSubmitHookInput{}
	_ json.MarshalerTo     = UserPromptSubmitHookInput{}
	_ json.UnmarshalerFrom = (*UserPromptSubmitHookInput)(nil)
)

func (UserPromptSubmitHookInput) isHookInput() {}

// EventName reports the hook_event_name discriminator for the payload type.
func (UserPromptSubmitHookInput) EventName() HookInputEventName {
	return HookInputEventNameUserPromptSubmit
}

// MarshalJSONTo implements [json.MarshalerTo]. An empty HookEventName is
// normalized to [HookInputEventNameUserPromptSubmit] so hand-built payloads stay valid.
func (value UserPromptSubmitHookInput) MarshalJSONTo(enc *jsontext.Encoder) error {
	raw := struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		PermissionMode HookPermissionMode `json:"permission_mode"`
		Prompt         string             `json:"prompt"`
		SessionID      string             `json:"session_id"`
		TranscriptPath *string            `json:"transcript_path"`
		TurnID         string             `json:"turn_id"`
	}{
		AgentID:        value.AgentID,
		AgentType:      value.AgentType,
		Cwd:            value.Cwd,
		HookEventName:  value.HookEventName,
		Model:          value.Model,
		PermissionMode: value.PermissionMode,
		Prompt:         value.Prompt,
		SessionID:      value.SessionID,
		TranscriptPath: value.TranscriptPath,
		TurnID:         value.TurnID,
	}
	if raw.HookEventName == "" {
		raw.HookEventName = HookInputEventNameUserPromptSubmit
	}
	return json.MarshalEncode(enc, &raw)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom]. It rejects payloads
// whose hook_event_name names a different hook event.
func (value *UserPromptSubmitHookInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		AgentID        string             `json:"agent_id,omitzero"`
		AgentType      string             `json:"agent_type,omitzero"`
		Cwd            string             `json:"cwd"`
		HookEventName  HookInputEventName `json:"hook_event_name"`
		Model          string             `json:"model"`
		PermissionMode HookPermissionMode `json:"permission_mode"`
		Prompt         string             `json:"prompt"`
		SessionID      string             `json:"session_id"`
		TranscriptPath *string            `json:"transcript_path"`
		TurnID         string             `json:"turn_id"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	if raw.HookEventName != "" && raw.HookEventName != HookInputEventNameUserPromptSubmit {
		return fmt.Errorf("unexpected hook_event_name %q for %s hook input", raw.HookEventName, HookInputEventNameUserPromptSubmit)
	}
	value.AgentID = raw.AgentID
	value.AgentType = raw.AgentType
	value.Cwd = raw.Cwd
	value.Model = raw.Model
	value.PermissionMode = raw.PermissionMode
	value.Prompt = raw.Prompt
	value.SessionID = raw.SessionID
	value.TranscriptPath = raw.TranscriptPath
	value.TurnID = raw.TurnID
	value.HookEventName = HookInputEventNameUserPromptSubmit
	return nil
}
