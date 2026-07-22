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
	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

func rawObjectExcluding(raw jsontext.Value, names ...string) (jsontext.Value, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var object map[string]jsontext.Value
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	for _, name := range names {
		delete(object, name)
	}
	if len(object) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	return jsontext.Value(data), nil
}

func marshalRawObjectTo(enc *jsontext.Encoder, raw jsontext.Value, fields map[string]jsontext.Value) error {
	merged, err := mergeRawObject(raw, fields)
	if err != nil {
		return err
	}
	return enc.WriteValue(merged)
}

func mergeRawObject(raw jsontext.Value, fields map[string]jsontext.Value) (jsontext.Value, error) {
	object := make(map[string]jsontext.Value)
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &object); err != nil {
			return nil, err
		}
		if object == nil {
			object = make(map[string]jsontext.Value)
		}
	}
	for name, value := range fields {
		if len(value) == 0 || string(value) == "null" {
			continue
		}
		object[name] = value
	}
	data, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	return jsontext.Value(data), nil
}

func setStringJSONField(fields map[string]jsontext.Value, name, value string) error {
	if value == "" {
		return nil
	}
	return setJSONField(fields, name, value)
}

func setIntJSONField(fields map[string]jsontext.Value, name string, value int) error {
	if value == 0 {
		return nil
	}
	return setJSONField(fields, name, value)
}

func setBoolJSONField(fields map[string]jsontext.Value, name string, value bool) error {
	if !value {
		return nil
	}
	return setJSONField(fields, name, value)
}

func setFloatJSONField(fields map[string]jsontext.Value, name string, value float64) error {
	if value == 0 {
		return nil
	}
	return setJSONField(fields, name, value)
}

func setRawJSONField(fields map[string]jsontext.Value, name string, value jsontext.Value) {
	if len(value) == 0 || string(value) == "null" {
		return
	}
	fields[name] = value
}

func setJSONField(fields map[string]jsontext.Value, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	fields[name] = jsontext.Value(data)
	return nil
}

func rawValueFromDecoder(dec *jsontext.Decoder) (jsontext.Value, error) {
	var raw jsontext.Value
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

type systemMessageJSON struct {
	Subtype string `json:"subtype,omitzero"`
}

// MarshalJSONTo implements [json.MarshalerTo], merging the typed fields with the preserved unknown fields in Raw.
func (m SystemMessage) MarshalJSONTo(enc *jsontext.Encoder) error {
	fields := make(map[string]jsontext.Value)
	if err := setStringJSONField(fields, "subtype", m.Subtype); err != nil {
		return err
	}
	return marshalRawObjectTo(enc, m.Raw, fields)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom], decoding the typed fields and preserving unknown fields in Raw.
func (m *SystemMessage) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	raw, err := rawValueFromDecoder(dec)
	if err != nil {
		return err
	}
	var typed systemMessageJSON
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	preserved, err := rawObjectExcluding(raw, "subtype")
	if err != nil {
		return err
	}
	m.Subtype = typed.Subtype
	m.Raw = preserved
	return nil
}

type resultMessageJSON struct {
	Subtype           string           `json:"subtype,omitzero"`
	DurationMs        int              `json:"duration_ms,omitzero"`
	DurationAPIMs     int              `json:"duration_api_ms,omitzero"`
	IsError           bool             `json:"is_error,omitzero"`
	NumTurns          int              `json:"num_turns,omitzero"`
	SessionID         string           `json:"session_id,omitzero"`
	TotalCostUSD      float64          `json:"total_cost_usd,omitzero"`
	Usage             jsontext.Value   `json:"usage,omitzero"`
	DeferredToolUse   *DeferredToolUse `json:"deferred_tool_use,omitzero"`
	StopReason        string           `json:"stop_reason,omitzero"`
	Result            jsontext.Value   `json:"result,omitzero"`
	StructuredOutput  jsontext.Value   `json:"structured_output,omitzero"`
	ModelUsage        jsontext.Value   `json:"modelUsage,omitzero"`
	PermissionDenials jsontext.Value   `json:"permission_denials,omitzero"`
	Errors            jsontext.Value   `json:"errors,omitzero"`
	APIErrorStatus    string           `json:"api_error_status,omitzero"`
	UUID              string           `json:"uuid,omitzero"`
}

// MarshalJSONTo implements [json.MarshalerTo], merging the typed fields with the preserved unknown fields in Raw.
func (m ResultMessage) MarshalJSONTo(enc *jsontext.Encoder) error {
	fields, err := resultMessageFields(&m)
	if err != nil {
		return err
	}
	return marshalRawObjectTo(enc, m.Raw, fields)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom], decoding the typed fields and preserving unknown fields in Raw.
func (m *ResultMessage) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	raw, err := rawValueFromDecoder(dec)
	if err != nil {
		return err
	}
	var typed resultMessageJSON
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	preserved, err := rawObjectExcluding(
		raw,
		"subtype",
		"duration_ms",
		"duration_api_ms",
		"is_error",
		"num_turns",
		"session_id",
		"total_cost_usd",
		"usage",
		"deferred_tool_use",
		"stop_reason",
		"result",
		"structured_output",
		"modelUsage",
		"permission_denials",
		"errors",
		"api_error_status",
		"uuid",
	)
	if err != nil {
		return err
	}
	*m = ResultMessage{
		Subtype:           typed.Subtype,
		DurationMs:        typed.DurationMs,
		DurationAPIMs:     typed.DurationAPIMs,
		IsError:           typed.IsError,
		NumTurns:          typed.NumTurns,
		SessionID:         typed.SessionID,
		TotalCostUSD:      typed.TotalCostUSD,
		Usage:             typed.Usage,
		DeferredToolUse:   typed.DeferredToolUse,
		StopReason:        typed.StopReason,
		Result:            typed.Result,
		StructuredOutput:  typed.StructuredOutput,
		ModelUsage:        typed.ModelUsage,
		PermissionDenials: typed.PermissionDenials,
		Errors:            typed.Errors,
		APIErrorStatus:    typed.APIErrorStatus,
		UUID:              typed.UUID,
		Raw:               preserved,
	}
	return nil
}

func resultMessageFields(m *ResultMessage) (map[string]jsontext.Value, error) {
	fields := make(map[string]jsontext.Value)
	if err := setStringJSONField(fields, "subtype", m.Subtype); err != nil {
		return nil, err
	}
	if err := setIntJSONField(fields, "duration_ms", m.DurationMs); err != nil {
		return nil, err
	}
	if err := setIntJSONField(fields, "duration_api_ms", m.DurationAPIMs); err != nil {
		return nil, err
	}
	if err := setBoolJSONField(fields, "is_error", m.IsError); err != nil {
		return nil, err
	}
	if err := setIntJSONField(fields, "num_turns", m.NumTurns); err != nil {
		return nil, err
	}
	if err := setStringJSONField(fields, "session_id", m.SessionID); err != nil {
		return nil, err
	}
	if err := setFloatJSONField(fields, "total_cost_usd", m.TotalCostUSD); err != nil {
		return nil, err
	}
	if err := setStringJSONField(fields, "stop_reason", m.StopReason); err != nil {
		return nil, err
	}
	if err := setStringJSONField(fields, "api_error_status", m.APIErrorStatus); err != nil {
		return nil, err
	}
	if err := setStringJSONField(fields, "uuid", m.UUID); err != nil {
		return nil, err
	}
	setRawJSONField(fields, "usage", m.Usage)
	if m.DeferredToolUse != nil {
		if err := setJSONField(fields, "deferred_tool_use", m.DeferredToolUse); err != nil {
			return nil, err
		}
	}
	setRawJSONField(fields, "result", m.Result)
	setRawJSONField(fields, "structured_output", m.StructuredOutput)
	setRawJSONField(fields, "modelUsage", m.ModelUsage)
	setRawJSONField(fields, "permission_denials", m.PermissionDenials)
	setRawJSONField(fields, "errors", m.Errors)
	return fields, nil
}

type textBlockJSON struct {
	Text string `json:"text,omitzero"`
}

// MarshalJSONTo implements [json.MarshalerTo], merging the typed fields with the preserved unknown fields in Raw.
func (b TextBlock) MarshalJSONTo(enc *jsontext.Encoder) error {
	fields := make(map[string]jsontext.Value)
	if err := setStringJSONField(fields, "text", b.Text); err != nil {
		return err
	}
	return marshalRawObjectTo(enc, b.Raw, fields)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom], decoding the typed fields and preserving unknown fields in Raw.
func (b *TextBlock) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	raw, err := rawValueFromDecoder(dec)
	if err != nil {
		return err
	}
	var typed textBlockJSON
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	preserved, err := rawObjectExcluding(raw, "text")
	if err != nil {
		return err
	}
	b.Text = typed.Text
	b.Raw = preserved
	return nil
}

type toolUseBlockJSON struct {
	ID    string         `json:"id,omitzero"`
	Name  string         `json:"name,omitzero"`
	Input jsontext.Value `json:"input,omitzero"`
}

// MarshalJSONTo implements [json.MarshalerTo], merging the typed fields with the preserved unknown fields in Raw.
func (b ToolUseBlock) MarshalJSONTo(enc *jsontext.Encoder) error {
	fields := make(map[string]jsontext.Value)
	if err := setStringJSONField(fields, "id", b.ID); err != nil {
		return err
	}
	if err := setStringJSONField(fields, "name", b.Name); err != nil {
		return err
	}
	setRawJSONField(fields, "input", b.Input)
	return marshalRawObjectTo(enc, b.Raw, fields)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom], decoding the typed fields and preserving unknown fields in Raw.
func (b *ToolUseBlock) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	raw, err := rawValueFromDecoder(dec)
	if err != nil {
		return err
	}
	var typed toolUseBlockJSON
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	preserved, err := rawObjectExcluding(raw, "id", "name", "input")
	if err != nil {
		return err
	}
	b.ID = typed.ID
	b.Name = typed.Name
	b.Input = typed.Input
	b.Raw = preserved
	return nil
}

type toolResultBlockJSON struct {
	ToolUseID string           `json:"tool_use_id,omitzero"`
	Content   []jsontext.Value `json:"content,omitzero"`
	IsError   bool             `json:"is_error,omitzero"`
}

// MarshalJSONTo implements [json.MarshalerTo], merging the typed fields with the preserved unknown fields in Raw.
func (b ToolResultBlock) MarshalJSONTo(enc *jsontext.Encoder) error {
	fields := make(map[string]jsontext.Value)
	if err := setStringJSONField(fields, "tool_use_id", b.ToolUseID); err != nil {
		return err
	}
	if len(b.Content) > 0 {
		if err := setJSONField(fields, "content", b.Content); err != nil {
			return err
		}
	}
	if err := setBoolJSONField(fields, "is_error", b.IsError); err != nil {
		return err
	}
	return marshalRawObjectTo(enc, b.Raw, fields)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom], decoding the typed fields and preserving unknown fields in Raw.
func (b *ToolResultBlock) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	raw, err := rawValueFromDecoder(dec)
	if err != nil {
		return err
	}
	var typed toolResultBlockJSON
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	blocks, err := parseContentBlocks(typed.Content, []byte(raw))
	if err != nil {
		return err
	}
	preserved, err := rawObjectExcluding(raw, "tool_use_id", "content", "is_error")
	if err != nil {
		return err
	}
	b.ToolUseID = typed.ToolUseID
	b.Content = blocks
	b.IsError = typed.IsError
	b.Raw = preserved
	return nil
}

type hookEventJSON struct {
	Kind       HookEventKind  `json:"hook_event_name,omitzero"`
	SessionID  string         `json:"session_id,omitzero"`
	ToolName   string         `json:"tool_name,omitzero"`
	ToolInput  jsontext.Value `json:"tool_input,omitzero"`
	ToolResult jsontext.Value `json:"tool_result,omitzero"`
	Prompt     string         `json:"prompt,omitzero"`
}

// MarshalJSONTo implements [json.MarshalerTo], merging the typed fields with the preserved unknown fields in Raw.
func (e HookEvent) MarshalJSONTo(enc *jsontext.Encoder) error {
	fields := make(map[string]jsontext.Value)
	if err := setJSONField(fields, "hook_event_name", e.Kind); err != nil {
		return err
	}
	if e.Kind == "" {
		delete(fields, "hook_event_name")
	}
	if err := setStringJSONField(fields, "session_id", e.SessionID); err != nil {
		return err
	}
	if err := setStringJSONField(fields, "tool_name", e.ToolName); err != nil {
		return err
	}
	setRawJSONField(fields, "tool_input", e.ToolInput)
	setRawJSONField(fields, "tool_result", e.ToolResult)
	if err := setStringJSONField(fields, "prompt", e.Prompt); err != nil {
		return err
	}
	return marshalRawObjectTo(enc, e.Raw, fields)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom], decoding the typed fields and preserving unknown fields in Raw.
func (e *HookEvent) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	raw, err := rawValueFromDecoder(dec)
	if err != nil {
		return err
	}
	var typed hookEventJSON
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	preserved, err := rawObjectExcluding(raw, "hook_event_name", "session_id", "tool_name", "tool_input", "tool_result", "prompt")
	if err != nil {
		return err
	}
	e.Kind = typed.Kind
	e.SessionID = typed.SessionID
	e.ToolName = typed.ToolName
	e.ToolInput = typed.ToolInput
	e.ToolResult = typed.ToolResult
	e.Prompt = typed.Prompt
	e.Raw = preserved
	return nil
}

type hookDecisionJSON struct {
	HookSpecificOutput HookSpecificOutput `json:"hookSpecificOutput,omitzero"`
	SystemMessage      string             `json:"systemMessage,omitzero"`
	AdditionalContext  string             `json:"additionalContext,omitzero"`
}

// MarshalJSONTo implements [json.MarshalerTo], merging the typed fields with the preserved unknown fields in Raw.
func (d HookDecision) MarshalJSONTo(enc *jsontext.Encoder) error {
	fields := make(map[string]jsontext.Value)
	if !d.HookSpecificOutput.isZero() {
		if err := setJSONField(fields, "hookSpecificOutput", d.HookSpecificOutput); err != nil {
			return err
		}
	}
	if err := setStringJSONField(fields, "systemMessage", d.SystemMessage); err != nil {
		return err
	}
	if err := setStringJSONField(fields, "additionalContext", d.AdditionalContext); err != nil {
		return err
	}
	return marshalRawObjectTo(enc, d.Raw, fields)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom], decoding the typed fields and preserving unknown fields in Raw.
func (d *HookDecision) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	raw, err := rawValueFromDecoder(dec)
	if err != nil {
		return err
	}
	var typed hookDecisionJSON
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	preserved, err := rawObjectExcluding(raw, "hookSpecificOutput", "systemMessage", "additionalContext")
	if err != nil {
		return err
	}
	d.HookSpecificOutput = typed.HookSpecificOutput
	d.SystemMessage = typed.SystemMessage
	d.AdditionalContext = typed.AdditionalContext
	d.Raw = preserved
	return nil
}

type hookSpecificOutputJSON struct {
	HookEventName            HookEventKind      `json:"hookEventName,omitzero"`
	PermissionDecision       PermissionDecision `json:"permissionDecision,omitzero"`
	PermissionDecisionReason string             `json:"permissionDecisionReason,omitzero"`
}

// MarshalJSONTo implements [json.MarshalerTo], merging the typed fields with the preserved unknown fields in Raw.
func (o HookSpecificOutput) MarshalJSONTo(enc *jsontext.Encoder) error {
	fields := make(map[string]jsontext.Value)
	if o.HookEventName != "" {
		if err := setJSONField(fields, "hookEventName", o.HookEventName); err != nil {
			return err
		}
	}
	if o.PermissionDecision != "" {
		if err := setJSONField(fields, "permissionDecision", o.PermissionDecision); err != nil {
			return err
		}
	}
	if err := setStringJSONField(fields, "permissionDecisionReason", o.PermissionDecisionReason); err != nil {
		return err
	}
	return marshalRawObjectTo(enc, o.Raw, fields)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom], decoding the typed fields and preserving unknown fields in Raw.
func (o *HookSpecificOutput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	raw, err := rawValueFromDecoder(dec)
	if err != nil {
		return err
	}
	var typed hookSpecificOutputJSON
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	preserved, err := rawObjectExcluding(raw, "hookEventName", "permissionDecision", "permissionDecisionReason")
	if err != nil {
		return err
	}
	o.HookEventName = typed.HookEventName
	o.PermissionDecision = typed.PermissionDecision
	o.PermissionDecisionReason = typed.PermissionDecisionReason
	o.Raw = preserved
	return nil
}

func (o HookSpecificOutput) isZero() bool {
	return o.HookEventName == "" &&
		o.PermissionDecision == "" &&
		o.PermissionDecisionReason == "" &&
		len(o.Raw) == 0
}

type hookEventMessageJSON struct {
	Subtype       string `json:"subtype,omitzero"`
	HookEventName string `json:"hook_event_name,omitzero"`
	SessionID     string `json:"session_id,omitzero"`
	UUID          string `json:"uuid,omitzero"`
}

// MarshalJSONTo implements [json.MarshalerTo], merging the typed fields with the preserved unknown fields in Raw.
func (m HookEventMessage) MarshalJSONTo(enc *jsontext.Encoder) error {
	fields := make(map[string]jsontext.Value)
	if err := setStringJSONField(fields, "subtype", m.Subtype); err != nil {
		return err
	}
	if err := setStringJSONField(fields, "hook_event_name", m.HookEventName); err != nil {
		return err
	}
	if err := setStringJSONField(fields, "session_id", m.SessionID); err != nil {
		return err
	}
	if err := setStringJSONField(fields, "uuid", m.UUID); err != nil {
		return err
	}
	return marshalRawObjectTo(enc, m.Raw, fields)
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom], decoding the typed fields and preserving unknown fields in Raw.
func (m *HookEventMessage) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	raw, err := rawValueFromDecoder(dec)
	if err != nil {
		return err
	}
	var typed hookEventMessageJSON
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	preserved, err := rawObjectExcluding(raw, "subtype", "hook_event_name", "session_id", "uuid")
	if err != nil {
		return err
	}
	m.Subtype = typed.Subtype
	m.HookEventName = typed.HookEventName
	m.SessionID = typed.SessionID
	m.UUID = typed.UUID
	m.Raw = preserved
	return nil
}
