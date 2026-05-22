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
	"bufio"
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// ── parseMessage unit tests ──────────────────────────────────────────────────

func TestParseMessage_TypeDiscrimination(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		line     string
		wantType string // Go type name of expected Message
	}{
		"success: system message": {
			line:     `{"type":"system","subtype":"init","session_id":"s1"}`,
			wantType: "claude.SystemMessage",
		},
		"success: result message": {
			line:     `{"type":"result","subtype":"success","duration_ms":100,"is_error":false,"num_turns":1,"session_id":"s1","total_cost_usd":0.001}`,
			wantType: "claude.ResultMessage",
		},
		"success: unknown type becomes rawMessage": {
			line:     `{"type":"future_kind","data":"something"}`,
			wantType: "claude.rawMessage",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			msg, err := parseMessage([]byte(tt.line + "\n"))
			if err != nil {
				t.Fatalf("parseMessage() error = %v", err)
			}
			if msg == nil {
				t.Fatal("parseMessage() = nil, want non-nil")
			}
			got := typeName(msg)
			if got != tt.wantType {
				t.Errorf("type = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestParseMessage_BlankLine(t *testing.T) {
	t.Parallel()

	for _, line := range [][]byte{{'\n'}, {}, {'\r', '\n'}} {
		msg, err := parseMessage(line)
		if err != nil {
			t.Errorf("parseMessage(blank) error = %v", err)
		}
		if msg != nil {
			t.Errorf("parseMessage(blank) = %v, want nil", msg)
		}
	}
}

func TestParseMessage_InvalidJSON_ReturnsCLIJSONDecodeError(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		line string
	}{
		"error: empty object no type": {
			line: `not json at all`,
		},
		"error: truncated json": {
			line: `{"type":"assistant","message":{`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := parseMessage([]byte(tt.line))
			if err == nil {
				t.Fatal("parseMessage() = nil error, want CLIJSONDecodeError")
			}
			var decErr *CLIJSONDecodeError
			if !errors.As(err, &decErr) {
				t.Fatalf("error type = %T, want *CLIJSONDecodeError", err)
			}
			if len(decErr.Line) == 0 {
				t.Error("CLIJSONDecodeError.Line is empty")
			}
		})
	}
}

// TestParseMessage_MissingType_ReturnsMessageParseError verifies that a valid
// JSON object lacking a "type" discriminator yields a *MessageParseError (not a
// rawMessage and not a decode error), mirroring upstream's "Message missing
// 'type' field". An unknown-but-present type still becomes a rawMessage (covered
// by TestParseMessage_TypeDiscrimination).
func TestParseMessage_MissingType_ReturnsMessageParseError(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"error: object with no type field":     `{"session_id":"s1","data":"x"}`,
		"error: object with empty type string": `{"type":"","session_id":"s1"}`,
	}

	for name, line := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := parseMessage([]byte(line + "\n"))
			if err == nil {
				t.Fatal("parseMessage() = nil error, want *MessageParseError")
			}
			var parseErr *MessageParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("error type = %T, want *MessageParseError", err)
			}
			if len(parseErr.Data) == 0 {
				t.Error("MessageParseError.Data is empty, want the offending payload")
			}
		})
	}
}

// ── AssistantMessage parsing ─────────────────────────────────────────────────

func TestParseMessage_AssistantMessage_TextContent(t *testing.T) {
	t.Parallel()

	line := `{"type":"assistant","message":{"id":"msg_1","content":[{"type":"text","text":"Hello!"}],"model":"claude-opus-4-5"},"session_id":"s1"}` + "\n"
	msg, err := parseMessage([]byte(line))
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	am, ok := msg.(AssistantMessage)
	if !ok {
		t.Fatalf("type = %T, want AssistantMessage", msg)
	}
	if len(am.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(am.Content))
	}
	tb, ok := am.Content[0].(TextBlock)
	if !ok {
		t.Fatalf("Content[0] type = %T, want TextBlock", am.Content[0])
	}
	if tb.Text != "Hello!" {
		t.Errorf("TextBlock.Text = %q, want Hello!", tb.Text)
	}
	if am.Model != "claude-opus-4-5" {
		t.Errorf("Model = %q, want claude-opus-4-5", am.Model)
	}
	// session_id is now a promoted typed field, no longer in Raw.
	if am.SessionID != "s1" {
		t.Errorf("SessionID = %q, want s1", am.SessionID)
	}
	if am.MessageID != "msg_1" {
		t.Errorf("MessageID = %q, want msg_1", am.MessageID)
	}
}

// TestParseMessage_AssistantMessage_PromotedFields verifies the two-level field
// promotion: message.{id,stop_reason,usage} and outer
// {parent_tool_use_id,error,session_id,uuid} parse into typed fields, and the
// promoted outer keys are excluded from Raw while unknown keys remain.
func TestParseMessage_AssistantMessage_PromotedFields(t *testing.T) {
	t.Parallel()

	line := `{"type":"assistant","message":{"id":"msg_9","model":"m","content":[{"type":"text","text":"hi"}],` +
		`"stop_reason":"end_turn","usage":{"input_tokens":3}},` +
		`"parent_tool_use_id":"tu_parent","error":{"code":"e1"},"session_id":"s9","uuid":"u-9",` +
		`"future_outer":"keep"}` + "\n"
	msg, err := parseMessage([]byte(line))
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	am, ok := msg.(AssistantMessage)
	if !ok {
		t.Fatalf("type = %T, want AssistantMessage", msg)
	}

	// message.* level
	if am.MessageID != "msg_9" {
		t.Errorf("MessageID = %q, want msg_9", am.MessageID)
	}
	if am.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", am.StopReason)
	}
	if !strings.Contains(string(am.Usage), "input_tokens") {
		t.Errorf("Usage = %q, want input_tokens", am.Usage)
	}
	// outer level
	if am.ParentToolUseID != "tu_parent" {
		t.Errorf("ParentToolUseID = %q, want tu_parent", am.ParentToolUseID)
	}
	if !strings.Contains(string(am.Error), "e1") {
		t.Errorf("Error = %q, want e1", am.Error)
	}
	if am.SessionID != "s9" {
		t.Errorf("SessionID = %q, want s9", am.SessionID)
	}
	if am.UUID != "u-9" {
		t.Errorf("UUID = %q, want u-9", am.UUID)
	}

	// Raw-exclusion: promoted outer keys gone; the nested message object and an
	// unknown outer key remain.
	var rawKeys map[string]jsontext.Value
	if err := json.Unmarshal(am.Raw, &rawKeys); err != nil {
		t.Fatalf("unmarshal Raw: %v (raw=%s)", err, am.Raw)
	}
	for _, gone := range []string{"parent_tool_use_id", "error", "session_id", "uuid"} {
		if _, present := rawKeys[gone]; present {
			t.Errorf("promoted outer key %q still in Raw: %s", gone, am.Raw)
		}
	}
	if _, present := rawKeys["future_outer"]; !present {
		t.Errorf("unknown outer key dropped from Raw: %s", am.Raw)
	}
}

func TestParseMessage_AssistantMessage_ToolUseContent(t *testing.T) {
	t.Parallel()

	line := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu_1","name":"Bash","input":{"command":"ls"}}],"model":"claude-3-5-sonnet"},"session_id":"s2"}` + "\n"
	msg, err := parseMessage([]byte(line))
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	am := msg.(AssistantMessage)
	if len(am.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(am.Content))
	}
	tub, ok := am.Content[0].(ToolUseBlock)
	if !ok {
		t.Fatalf("Content[0] type = %T, want ToolUseBlock", am.Content[0])
	}
	if tub.ID != "tu_1" {
		t.Errorf("ToolUseBlock.ID = %q, want tu_1", tub.ID)
	}
	if tub.Name != "Bash" {
		t.Errorf("ToolUseBlock.Name = %q, want Bash", tub.Name)
	}
	if !strings.Contains(string(tub.Input), `"command":"ls"`) {
		t.Errorf("ToolUseBlock.Input = %q, want command field", tub.Input)
	}
}

func TestParseMessage_AssistantMessage_UnknownBlockType(t *testing.T) {
	t.Parallel()

	line := `{"type":"assistant","message":{"content":[{"type":"future_block","data":123}],"model":"m"}}` + "\n"
	msg, err := parseMessage([]byte(line))
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	am := msg.(AssistantMessage)
	if len(am.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(am.Content))
	}
	_, ok := am.Content[0].(rawContentBlock)
	if !ok {
		t.Fatalf("Content[0] type = %T, want rawContentBlock", am.Content[0])
	}
}

// ── UserMessage parsing ──────────────────────────────────────────────────────

func TestParseMessage_UserMessage_ToolResult(t *testing.T) {
	t.Parallel()

	line := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_1","content":[{"type":"text","text":"done"}],"is_error":false}]},"session_id":"s3"}` + "\n"
	msg, err := parseMessage([]byte(line))
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	um, ok := msg.(UserMessage)
	if !ok {
		t.Fatalf("type = %T, want UserMessage", msg)
	}
	if len(um.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(um.Content))
	}
	trb, ok := um.Content[0].(ToolResultBlock)
	if !ok {
		t.Fatalf("Content[0] type = %T, want ToolResultBlock", um.Content[0])
	}
	if trb.ToolUseID != "tu_1" {
		t.Errorf("ToolUseID = %q, want tu_1", trb.ToolUseID)
	}
	if trb.IsError {
		t.Error("IsError = true, want false")
	}
	if len(trb.Content) != 1 {
		t.Fatalf("len(ToolResult.Content) = %d, want 1", len(trb.Content))
	}
	inner, ok := trb.Content[0].(TextBlock)
	if !ok {
		t.Fatalf("ToolResult.Content[0] type = %T, want TextBlock", trb.Content[0])
	}
	if inner.Text != "done" {
		t.Errorf("inner.Text = %q, want done", inner.Text)
	}
}

// ── rawMessage preservation ──────────────────────────────────────────────────

func TestParseMessage_UnknownType_PreservesRaw(t *testing.T) {
	t.Parallel()

	line := `{"type":"custom_event","payload":{"key":"value"},"ts":1234567890}` + "\n"
	msg, err := parseMessage([]byte(line))
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	rm, ok := msg.(rawMessage)
	if !ok {
		t.Fatalf("type = %T, want rawMessage", msg)
	}
	rawStr := string(rm.raw)
	for _, want := range []string{`"custom_event"`, `"key":"value"`, `"ts":1234567890`} {
		if !strings.Contains(rawStr, want) {
			t.Errorf("rawMessage.raw = %q, want to contain %q", rawStr, want)
		}
	}
}

// ── fixture file tests ───────────────────────────────────────────────────────

// parseFixtureFile reads a .jsonl fixture file and parses every non-blank line
// into a Message. It is used by the fixture-based tests below.
func parseFixtureFile(t *testing.T, path string) []Message {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = %v", path, err)
	}
	var msgs []Message
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Bytes()
		msg, err := parseMessage(append(line, '\n'))
		if err != nil {
			t.Fatalf("parseMessage() error = %v (line: %q)", err, line)
		}
		if msg != nil {
			msgs = append(msgs, msg)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	return msgs
}

func TestFixture_AssistantText(t *testing.T) {
	t.Parallel()

	msgs := parseFixtureFile(t, "testdata/stream/assistant_text.jsonl")
	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3", len(msgs))
	}

	// Line 1: SystemMessage
	if _, ok := msgs[0].(SystemMessage); !ok {
		t.Errorf("msgs[0] type = %T, want SystemMessage", msgs[0])
	}

	// Line 2: AssistantMessage with one TextBlock
	am, ok := msgs[1].(AssistantMessage)
	if !ok {
		t.Fatalf("msgs[1] type = %T, want AssistantMessage", msgs[1])
	}
	if len(am.Content) != 1 {
		t.Fatalf("len(am.Content) = %d, want 1", len(am.Content))
	}
	tb, ok := am.Content[0].(TextBlock)
	if !ok {
		t.Fatalf("am.Content[0] type = %T, want TextBlock", am.Content[0])
	}
	if tb.Text == "" {
		t.Error("TextBlock.Text is empty")
	}
	if am.Model != "claude-opus-4-5" {
		t.Errorf("am.Model = %q, want claude-opus-4-5", am.Model)
	}

	// Line 3: ResultMessage
	rm, ok := msgs[2].(ResultMessage)
	if !ok {
		t.Fatalf("msgs[2] type = %T, want ResultMessage", msgs[2])
	}
	if rm.Subtype != "success" {
		t.Errorf("rm.Subtype = %q, want success", rm.Subtype)
	}
	if rm.NumTurns != 1 {
		t.Errorf("rm.NumTurns = %d, want 1", rm.NumTurns)
	}
}

func TestFixture_ToolUse(t *testing.T) {
	t.Parallel()

	msgs := parseFixtureFile(t, "testdata/stream/tool_use.jsonl")
	if len(msgs) != 4 {
		t.Fatalf("len(msgs) = %d, want 4", len(msgs))
	}

	// Line 1: AssistantMessage with ToolUseBlock
	am1, ok := msgs[0].(AssistantMessage)
	if !ok {
		t.Fatalf("msgs[0] type = %T, want AssistantMessage", msgs[0])
	}
	if len(am1.Content) != 1 {
		t.Fatalf("len(am1.Content) = %d, want 1", len(am1.Content))
	}
	tub, ok := am1.Content[0].(ToolUseBlock)
	if !ok {
		t.Fatalf("am1.Content[0] type = %T, want ToolUseBlock", am1.Content[0])
	}
	if tub.Name != "Bash" {
		t.Errorf("ToolUseBlock.Name = %q, want Bash", tub.Name)
	}

	// Line 2: UserMessage with ToolResultBlock
	um, ok := msgs[1].(UserMessage)
	if !ok {
		t.Fatalf("msgs[1] type = %T, want UserMessage", msgs[1])
	}
	if len(um.Content) != 1 {
		t.Fatalf("len(um.Content) = %d, want 1", len(um.Content))
	}
	trb, ok := um.Content[0].(ToolResultBlock)
	if !ok {
		t.Fatalf("um.Content[0] type = %T, want ToolResultBlock", um.Content[0])
	}
	if trb.ToolUseID != "toolu_001" {
		t.Errorf("ToolUseID = %q, want toolu_001", trb.ToolUseID)
	}

	// Line 3: AssistantMessage with TextBlock
	if _, ok := msgs[2].(AssistantMessage); !ok {
		t.Errorf("msgs[2] type = %T, want AssistantMessage", msgs[2])
	}

	// Line 4: ResultMessage
	rm, ok := msgs[3].(ResultMessage)
	if !ok {
		t.Fatalf("msgs[3] type = %T, want ResultMessage", msgs[3])
	}
	if rm.NumTurns != 2 {
		t.Errorf("rm.NumTurns = %d, want 2", rm.NumTurns)
	}
}

func TestFixture_Result(t *testing.T) {
	t.Parallel()

	msgs := parseFixtureFile(t, "testdata/stream/result.jsonl")
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	rm, ok := msgs[0].(ResultMessage)
	if !ok {
		t.Fatalf("msgs[0] type = %T, want ResultMessage", msgs[0])
	}
	if rm.Subtype != "success" {
		t.Errorf("Subtype = %q, want success", rm.Subtype)
	}
	if rm.SessionID != "sess-003" {
		t.Errorf("SessionID = %q, want sess-003", rm.SessionID)
	}
}

// TestParseMessage_ResultMessage_PromotedFields verifies the typed fields
// promoted from Raw (stop_reason, result, structured_output, modelUsage,
// permission_denials, errors, api_error_status, uuid) parse into first-class
// fields AND no longer appear in Raw (the Raw-exclusion policy).
func TestParseMessage_ResultMessage_PromotedFields(t *testing.T) {
	t.Parallel()

	line := `{"type":"result","subtype":"success","duration_ms":10,"duration_api_ms":8,` +
		`"is_error":false,"num_turns":2,"session_id":"s1","total_cost_usd":0.5,` +
		`"stop_reason":"end_turn","result":{"answer":42},"structured_output":{"k":"v"},` +
		`"modelUsage":{"opus":{"in":1}},"permission_denials":[{"tool":"Bash"}],` +
		`"errors":["boom"],"api_error_status":"overloaded","uuid":"u-1",` +
		`"some_future_field":"keep-me"}` + "\n"
	msg, err := parseMessage([]byte(line))
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	rm, ok := msg.(ResultMessage)
	if !ok {
		t.Fatalf("type = %T, want ResultMessage", msg)
	}

	if rm.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", rm.StopReason)
	}
	if rm.APIErrorStatus != "overloaded" {
		t.Errorf("APIErrorStatus = %q, want overloaded", rm.APIErrorStatus)
	}
	if rm.UUID != "u-1" {
		t.Errorf("UUID = %q, want u-1", rm.UUID)
	}
	for name, got := range map[string]jsontext.Value{
		"result":             rm.Result,
		"structured_output":  rm.StructuredOutput,
		"modelUsage":         rm.ModelUsage,
		"permission_denials": rm.PermissionDenials,
		"errors":             rm.Errors,
	} {
		if len(got) == 0 {
			t.Errorf("%s typed field is empty, want populated", name)
		}
	}

	// Raw-exclusion: promoted KEYS must NOT remain in Raw; unknown ones must.
	// Decode Raw into a key set so we match object keys precisely (a substring
	// check would false-positive, e.g. "result" inside the value "type":"result").
	var rawKeys map[string]jsontext.Value
	if err := json.Unmarshal(rm.Raw, &rawKeys); err != nil {
		t.Fatalf("unmarshal Raw: %v (raw=%s)", err, rm.Raw)
	}
	for _, promoted := range []string{"stop_reason", "result", "structured_output", "modelUsage", "permission_denials", "errors", "api_error_status", "uuid", "subtype", "session_id"} {
		if _, present := rawKeys[promoted]; present {
			t.Errorf("promoted/typed key %q still present in Raw: %s", promoted, rm.Raw)
		}
	}
	if _, present := rawKeys["some_future_field"]; !present {
		t.Errorf("unknown field dropped from Raw: %s", rm.Raw)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// typeName returns the package-qualified type name of v using fmt.Sprintf.
func typeName(v any) string {
	if v == nil {
		return "<nil>"
	}
	// Use %T which includes package path for unexported types.
	// Trim the leading module path to keep the name stable across environments.
	import_prefix := "github.com/zchee/pandaemonium/"
	name := typeNameRaw(v)
	name = strings.TrimPrefix(name, import_prefix)
	return name
}

func typeNameRaw(v any) string {
	switch v.(type) {
	case SystemMessage:
		return "claude.SystemMessage"
	case ResultMessage:
		return "claude.ResultMessage"
	case AssistantMessage:
		return "claude.AssistantMessage"
	case UserMessage:
		return "claude.UserMessage"
	case rawMessage:
		return "claude.rawMessage"
	default:
		return "unknown"
	}
}
