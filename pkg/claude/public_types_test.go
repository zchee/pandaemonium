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
	"strings"
	"testing"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/google/go-cmp/cmp"
)

// ── compile-time interface assertions ────────────────────────────────────────

var (
	_ Message = AssistantMessage{}
	_ Message = UserMessage{}
	_ Message = SystemMessage{}
	_ Message = ResultMessage{}
	_ Message = rawMessage{}

	_ ContentBlock = TextBlock{}
	_ ContentBlock = ToolUseBlock{}
	_ ContentBlock = ToolResultBlock{}
	_ ContentBlock = rawContentBlock{}

	_ Error = &CLINotFoundError{}
	_ Error = &CLIConnectionError{}
	_ Error = &ProcessError{}
	_ Error = &CLIJSONDecodeError{}
)

// ── SystemMessage round-trip ─────────────────────────────────────────────────

func TestSystemMessage_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		wantSub string
		wantRaw []string // substrings that must appear in Raw
	}{
		"success: init subtype": {
			input:   `{"type":"system","subtype":"init","session_id":"abc"}`,
			wantSub: "init",
			wantRaw: []string{`"type":"system"`, `"session_id":"abc"`},
		},
		"success: unknown subtype preserved": {
			input:   `{"type":"system","subtype":"future_kind","extra_field":42}`,
			wantSub: "future_kind",
			wantRaw: []string{`"extra_field":42`},
		},
		"success: empty subtype": {
			input:   `{"type":"system"}`,
			wantSub: "",
			wantRaw: []string{`"type":"system"`},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var m SystemMessage
			if err := json.Unmarshal([]byte(tt.input), &m); err != nil {
				t.Fatalf("json.Unmarshal() = %v", err)
			}
			if m.Subtype != tt.wantSub {
				t.Errorf("Subtype = %q, want %q", m.Subtype, tt.wantSub)
			}
			rawStr := string(m.Raw)
			for _, want := range tt.wantRaw {
				if !strings.Contains(rawStr, want) {
					t.Errorf("Raw = %q, want to contain %q", rawStr, want)
				}
			}

			// Re-marshal and verify the output contains the expected substrings.
			out, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("json.Marshal() = %v", err)
			}
			if tt.wantSub != "" && !strings.Contains(string(out), `"subtype":"`+tt.wantSub+`"`) {
				t.Errorf("marshaled %q, want subtype %q", out, tt.wantSub)
			}
		})
	}
}

// ── ResultMessage round-trip ─────────────────────────────────────────────────

func TestResultMessage_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input        string
		wantSubtype  string
		wantDuration int
		wantCost     float64
		wantIsError  bool
	}{
		"success: success subtype": {
			input:        `{"type":"result","subtype":"success","duration_ms":1500,"duration_api_ms":1200,"is_error":false,"num_turns":1,"session_id":"s1","total_cost_usd":0.001,"usage":{}}`,
			wantSubtype:  "success",
			wantDuration: 1500,
			wantCost:     0.001,
		},
		"success: error subtype": {
			input:        `{"type":"result","subtype":"error_max_turns","duration_ms":500,"is_error":true,"num_turns":3,"session_id":"s2","total_cost_usd":0.0}`,
			wantSubtype:  "error_max_turns",
			wantDuration: 500,
			wantIsError:  true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var m ResultMessage
			if err := json.Unmarshal([]byte(tt.input), &m); err != nil {
				t.Fatalf("json.Unmarshal() = %v", err)
			}
			if m.Subtype != tt.wantSubtype {
				t.Errorf("Subtype = %q, want %q", m.Subtype, tt.wantSubtype)
			}
			if m.DurationMs != tt.wantDuration {
				t.Errorf("DurationMs = %d, want %d", m.DurationMs, tt.wantDuration)
			}
			if m.TotalCostUSD != tt.wantCost {
				t.Errorf("TotalCostUSD = %f, want %f", m.TotalCostUSD, tt.wantCost)
			}
			if m.IsError != tt.wantIsError {
				t.Errorf("IsError = %v, want %v", m.IsError, tt.wantIsError)
			}

			// Re-marshal — no loss of known fields.
			out, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("json.Marshal() = %v", err)
			}
			if !strings.Contains(string(out), `"subtype":"`+tt.wantSubtype+`"`) {
				t.Errorf("marshaled %q missing subtype %q", out, tt.wantSubtype)
			}
		})
	}
}

// ── ContentBlock marshal tests ────────────────────────────────────────────────

func TestTextBlock_Marshal(t *testing.T) {
	t.Parallel()

	b := TextBlock{Text: "Hello, world!"}
	out, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("json.Marshal() = %v", err)
	}
	if !strings.Contains(string(out), `"text":"Hello, world!"`) {
		t.Errorf("marshaled %q, want to contain text field", out)
	}
}

func TestTextBlock_RoundTrip(t *testing.T) {
	t.Parallel()

	input := `{"type":"text","text":"Sample","unknown_future":true}`
	var b TextBlock
	if err := json.Unmarshal([]byte(input), &b); err != nil {
		t.Fatalf("json.Unmarshal() = %v", err)
	}
	if b.Text != "Sample" {
		t.Errorf("Text = %q, want Sample", b.Text)
	}
	if !strings.Contains(string(b.Raw), `"unknown_future":true`) {
		t.Errorf("Raw = %q, want unknown_future preserved", b.Raw)
	}
}

func TestToolUseBlock_Marshal(t *testing.T) {
	t.Parallel()

	b := ToolUseBlock{
		ID:    "call_1",
		Name:  "Bash",
		Input: jsontext.Value(`{"command":"echo hello"}`),
	}
	out, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("json.Marshal() = %v", err)
	}
	for _, want := range []string{`"id":"call_1"`, `"name":"Bash"`, `"command":"echo hello"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("marshaled %q, want to contain %q", out, want)
		}
	}
}

func TestToolUseBlock_RoundTrip(t *testing.T) {
	t.Parallel()

	input := `{"type":"tool_use","id":"tu_1","name":"Write","input":{"path":"/tmp/x"},"future_key":"y"}`
	var b ToolUseBlock
	if err := json.Unmarshal([]byte(input), &b); err != nil {
		t.Fatalf("json.Unmarshal() = %v", err)
	}
	if b.ID != "tu_1" {
		t.Errorf("ID = %q, want tu_1", b.ID)
	}
	if b.Name != "Write" {
		t.Errorf("Name = %q, want Write", b.Name)
	}
	if !strings.Contains(string(b.Raw), `"future_key":"y"`) {
		t.Errorf("Raw = %q, want future_key preserved", b.Raw)
	}
}

// ── AssistantMessage marshal test ─────────────────────────────────────────────

func TestAssistantMessage_Marshal(t *testing.T) {
	t.Parallel()

	m := AssistantMessage{
		Content: []ContentBlock{
			TextBlock{Text: "Hi there"},
			ToolUseBlock{ID: "tu_1", Name: "Bash", Input: jsontext.Value(`{"cmd":"ls"}`)},
		},
		Model: "claude-opus-4-5",
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal() = %v", err)
	}
	for _, want := range []string{`"Hi there"`, `"Bash"`, `"model":"claude-opus-4-5"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("marshaled %q, want to contain %q", out, want)
		}
	}
}

// ── HookEvent round-trip ─────────────────────────────────────────────────────

func TestHookEvent_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input     string
		wantKind  HookEventKind
		wantTool  string
		wantExtra string // substring expected in Raw
	}{
		"success: PreToolUse event": {
			input:     `{"hook_event_name":"PreToolUse","session_id":"s1","tool_name":"Bash","tool_input":{"command":"rm -rf /"},"future_field":"keep"}`,
			wantKind:  HookEventPreToolUse,
			wantTool:  "Bash",
			wantExtra: `"future_field":"keep"`,
		},
		"success: Stop event": {
			input:     `{"hook_event_name":"Stop","session_id":"s2","extra":99}`,
			wantKind:  HookEventStop,
			wantExtra: `"extra":99`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var e HookEvent
			if err := json.Unmarshal([]byte(tt.input), &e); err != nil {
				t.Fatalf("json.Unmarshal() = %v", err)
			}
			if e.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", e.Kind, tt.wantKind)
			}
			if tt.wantTool != "" && e.ToolName != tt.wantTool {
				t.Errorf("ToolName = %q, want %q", e.ToolName, tt.wantTool)
			}
			if !strings.Contains(string(e.Raw), tt.wantExtra) {
				t.Errorf("Raw = %q, want to contain %q", e.Raw, tt.wantExtra)
			}

			// Re-marshal should preserve the unknown field.
			out, err := json.Marshal(e)
			if err != nil {
				t.Fatalf("json.Marshal() = %v", err)
			}
			if !strings.Contains(string(out), tt.wantExtra) {
				t.Errorf("re-marshaled %q, want to contain %q", out, tt.wantExtra)
			}
		})
	}
}

// ── HookDecision.Raw round-trip (AC-i7) ──────────────────────────────────────

// TestHookDecision_RawCatchall verifies that a HookDecision with an unknown
// top-level field encodes and decodes losslessly (AC-i7).
func TestHookDecision_RawCatchall(t *testing.T) {
	t.Parallel()

	const input = `{"hookSpecificOutput":{"permissionDecision":"allow"},"systemMessage":"ok","unknownFutureKey":"preserved","anotherKey":42}`

	var d HookDecision
	if err := json.Unmarshal([]byte(input), &d); err != nil {
		t.Fatalf("json.Unmarshal() = %v", err)
	}
	if d.SystemMessage != "ok" {
		t.Errorf("SystemMessage = %q, want ok", d.SystemMessage)
	}
	if d.HookSpecificOutput.PermissionDecision != PermissionAllow {
		t.Errorf("PermissionDecision = %q, want allow", d.HookSpecificOutput.PermissionDecision)
	}

	// Unknown keys must be in Raw.
	for _, want := range []string{`"unknownFutureKey":"preserved"`, `"anotherKey":42`} {
		if !strings.Contains(string(d.Raw), want) {
			t.Errorf("Raw = %q, want to contain %q", d.Raw, want)
		}
	}

	// Re-marshal must preserve the unknown keys (lossless round-trip).
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal() = %v", err)
	}
	for _, want := range []string{`"unknownFutureKey":"preserved"`, `"anotherKey":42`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("re-marshaled %q, want to contain %q", out, want)
		}
	}
}

// TestHookSpecificOutput_RawCatchall verifies that HookSpecificOutput also
// round-trips unknown fields through its Raw inline catchall.
func TestHookSpecificOutput_RawCatchall(t *testing.T) {
	t.Parallel()

	input := `{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"unsafe","futureOutputKey":"x"}`
	var o HookSpecificOutput
	if err := json.Unmarshal([]byte(input), &o); err != nil {
		t.Fatalf("json.Unmarshal() = %v", err)
	}
	if o.PermissionDecision != PermissionDeny {
		t.Errorf("PermissionDecision = %q, want deny", o.PermissionDecision)
	}
	if !strings.Contains(string(o.Raw), `"futureOutputKey":"x"`) {
		t.Errorf("Raw = %q, want futureOutputKey", o.Raw)
	}

	// Re-marshal preserves the unknown key.
	out, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("json.Marshal() = %v", err)
	}
	if !strings.Contains(string(out), `"futureOutputKey":"x"`) {
		t.Errorf("re-marshaled %q, missing futureOutputKey", out)
	}
}

// ── jsonRaw / blockRaw sentinels ─────────────────────────────────────────────

func TestMessage_SentinelMethods(t *testing.T) {
	t.Parallel()

	raw := jsontext.Value(`{"x":1}`)
	msgs := []struct {
		name string
		msg  Message
	}{
		{"AssistantMessage", AssistantMessage{Raw: raw}},
		{"UserMessage", UserMessage{Raw: raw}},
		{"SystemMessage", SystemMessage{Raw: raw}},
		{"ResultMessage", ResultMessage{Raw: raw}},
		{"rawMessage", rawMessage{raw: raw}},
	}
	for _, tc := range msgs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff([]byte(raw), []byte(tc.msg.jsonRaw())); diff != "" {
				t.Errorf("jsonRaw() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestContentBlock_SentinelMethods(t *testing.T) {
	t.Parallel()

	raw := jsontext.Value(`{"y":2}`)
	blocks := []struct {
		name  string
		block ContentBlock
	}{
		{"TextBlock", TextBlock{Raw: raw}},
		{"ToolUseBlock", ToolUseBlock{Raw: raw}},
		{"ToolResultBlock", ToolResultBlock{Raw: raw}},
		{"rawContentBlock", rawContentBlock{raw: raw}},
	}
	for _, tc := range blocks {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff([]byte(raw), []byte(tc.block.blockRaw())); diff != "" {
				t.Errorf("blockRaw() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
