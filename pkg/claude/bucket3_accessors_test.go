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
)

// ── Content blocks (Thinking, ServerToolUse, ServerToolResult) ─────────────

// ── compile-time interface satisfaction ──────────────────────────────────────

var (
	_ ContentBlock = ThinkingBlock{}
	_ ContentBlock = ServerToolUseBlock{}
	_ ContentBlock = ServerToolResultBlock{}
)

// TestServerToolName_Literals pins the 7 wire literals against upstream
// ServerToolName (types.py:954-962).
func TestServerToolName_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		n    ServerToolName
		want string
	}{
		"advisor":                    {ServerToolNameAdvisor, "advisor"},
		"web_search":                 {ServerToolNameWebSearch, "web_search"},
		"web_fetch":                  {ServerToolNameWebFetch, "web_fetch"},
		"bash_code_execution":        {ServerToolNameBashCodeExecution, "bash_code_execution"},
		"text_editor_code_execution": {ServerToolNameTextEditorCodeExecution, "text_editor_code_execution"},
		"tool_search_tool_regex":     {ServerToolNameToolSearchToolRegex, "tool_search_tool_regex"},
		"tool_search_tool_bm25":      {ServerToolNameToolSearchToolBM25, "tool_search_tool_bm25"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.n) != tt.want {
				t.Errorf("ServerToolName = %q, want %q", string(tt.n), tt.want)
			}
		})
	}
}

// TestParseMessage_AssistantMessage_ThinkingBlock pins that a content block
// with type=thinking dispatches to ThinkingBlock (not rawContentBlock).
func TestParseMessage_AssistantMessage_ThinkingBlock(t *testing.T) {
	t.Parallel()
	line := `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"let me consider","signature":"sig_abc"}],"model":"m"}}` + "\n"
	msg, err := parseMessage([]byte(line))
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	am := msg.(AssistantMessage)
	if len(am.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(am.Content))
	}
	tb, ok := am.Content[0].(ThinkingBlock)
	if !ok {
		t.Fatalf("Content[0] type = %T, want ThinkingBlock", am.Content[0])
	}
	if tb.Thinking != "let me consider" {
		t.Errorf("Thinking = %q, want %q", tb.Thinking, "let me consider")
	}
	if tb.Signature != "sig_abc" {
		t.Errorf("Signature = %q, want sig_abc", tb.Signature)
	}
}

// TestParseMessage_AssistantMessage_ServerToolUseBlock pins that a content
// block with type=server_tool_use dispatches to ServerToolUseBlock (not
// rawContentBlock), with the Name field populated as ServerToolName.
func TestParseMessage_AssistantMessage_ServerToolUseBlock(t *testing.T) {
	t.Parallel()
	line := `{"type":"assistant","message":{"content":[{"type":"server_tool_use","id":"sut_1","name":"web_search","input":{"query":"go json"}}],"model":"m"}}` + "\n"
	msg, err := parseMessage([]byte(line))
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	am := msg.(AssistantMessage)
	stub, ok := am.Content[0].(ServerToolUseBlock)
	if !ok {
		t.Fatalf("Content[0] type = %T, want ServerToolUseBlock", am.Content[0])
	}
	if stub.ID != "sut_1" {
		t.Errorf("ID = %q, want sut_1", stub.ID)
	}
	if stub.Name != ServerToolNameWebSearch {
		t.Errorf("Name = %q, want web_search", stub.Name)
	}
	if !strings.Contains(string(stub.Input), `"query":"go json"`) {
		t.Errorf("Input = %q, want to contain query", stub.Input)
	}
}

// TestParseMessage_AssistantMessage_ServerToolResultBlock_AllVariants verifies
// that all five server-tool result wire types collapse into
// ServerToolResultBlock with Content preserved.
func TestParseMessage_AssistantMessage_ServerToolResultBlock_AllVariants(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{
		"server_tool_result",
		"web_search_tool_result",
		"web_fetch_tool_result",
		"bash_code_execution_tool_result",
		"text_editor_code_execution_tool_result",
	} {
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			line := `{"type":"assistant","message":{"content":[{"type":"` + typ + `","tool_use_id":"sut_1","content":{"type":"ok","results":[1,2,3]}}],"model":"m"}}` + "\n"
			msg, err := parseMessage([]byte(line))
			if err != nil {
				t.Fatalf("parseMessage: %v", err)
			}
			am := msg.(AssistantMessage)
			strb, ok := am.Content[0].(ServerToolResultBlock)
			if !ok {
				t.Fatalf("type=%q: Content[0] = %T, want ServerToolResultBlock", typ, am.Content[0])
			}
			if strb.ToolUseID != "sut_1" {
				t.Errorf("ToolUseID = %q, want sut_1", strb.ToolUseID)
			}
			if !strings.Contains(string(strb.Content), `"results"`) {
				t.Errorf("Content = %q, want to contain results", strb.Content)
			}
		})
	}
}

// TestParseMessage_ContentBlock_UnknownStillRaw is the regression guard: a
// genuinely unknown block type must STILL fall through to rawContentBlock —
// adding the new dispatch cases must not regress forward compatibility.
func TestParseMessage_ContentBlock_UnknownStillRaw(t *testing.T) {
	t.Parallel()
	line := `{"type":"assistant","message":{"content":[{"type":"future_block_kind","data":42}],"model":"m"}}` + "\n"
	msg, err := parseMessage([]byte(line))
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	am := msg.(AssistantMessage)
	if _, ok := am.Content[0].(rawContentBlock); !ok {
		t.Errorf("future_block_kind dispatched to %T, want rawContentBlock", am.Content[0])
	}
}

// ── Task system messages + DeferredToolUse ────────────────────────────────

var (
	_ Message = TaskStartedMessage{}
	_ Message = TaskProgressMessage{}
	_ Message = TaskNotificationMessage{}
)

// TestTaskNotificationStatus_Literals pins the 3 wire literals upstream uses
// (types.py:1056).
func TestTaskNotificationStatus_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		s    TaskNotificationStatus
		want string
	}{
		"completed": {TaskNotificationStatusCompleted, "completed"},
		"failed":    {TaskNotificationStatusFailed, "failed"},
		"stopped":   {TaskNotificationStatusStopped, "stopped"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.s) != tt.want {
				t.Errorf("status = %q, want %q", string(tt.s), tt.want)
			}
		})
	}
}

// TestParseMessage_TaskStartedMessage verifies the system+subtype=task_started
// dispatch routes to TaskStartedMessage (not SystemMessage).
func TestParseMessage_TaskStartedMessage(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"system","subtype":"task_started","task_id":"t_1","description":"build the thing","uuid":"u_1","session_id":"s_1","tool_use_id":"tu_1","task_type":"build"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	tsm, ok := msg.(TaskStartedMessage)
	if !ok {
		t.Fatalf("type = %T, want TaskStartedMessage", msg)
	}
	if tsm.TaskID != "t_1" {
		t.Errorf("TaskID = %q, want t_1", tsm.TaskID)
	}
	if tsm.Description != "build the thing" {
		t.Errorf("Description = %q, want build the thing", tsm.Description)
	}
	if tsm.UUID != "u_1" {
		t.Errorf("UUID = %q, want u_1", tsm.UUID)
	}
	if tsm.SessionID != "s_1" {
		t.Errorf("SessionID = %q, want s_1", tsm.SessionID)
	}
	if tsm.ToolUseID != "tu_1" {
		t.Errorf("ToolUseID = %q, want tu_1", tsm.ToolUseID)
	}
	if tsm.TaskType != "build" {
		t.Errorf("TaskType = %q, want build", tsm.TaskType)
	}
}

// TestParseMessage_TaskProgressMessage verifies dispatch + nested TaskUsage
// decode.
func TestParseMessage_TaskProgressMessage(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"system","subtype":"task_progress","task_id":"t_2","description":"halfway","usage":{"total_tokens":1234,"tool_uses":3,"duration_ms":5000},"uuid":"u_2","session_id":"s_2","last_tool_name":"Bash"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	tpm, ok := msg.(TaskProgressMessage)
	if !ok {
		t.Fatalf("type = %T, want TaskProgressMessage", msg)
	}
	if tpm.TaskID != "t_2" {
		t.Errorf("TaskID = %q, want t_2", tpm.TaskID)
	}
	if tpm.Usage.TotalTokens != 1234 {
		t.Errorf("Usage.TotalTokens = %d, want 1234", tpm.Usage.TotalTokens)
	}
	if tpm.Usage.ToolUses != 3 {
		t.Errorf("Usage.ToolUses = %d, want 3", tpm.Usage.ToolUses)
	}
	if tpm.Usage.DurationMs != 5000 {
		t.Errorf("Usage.DurationMs = %d, want 5000", tpm.Usage.DurationMs)
	}
	if tpm.LastToolName != "Bash" {
		t.Errorf("LastToolName = %q, want Bash", tpm.LastToolName)
	}
}

// TestParseMessage_TaskNotificationMessage verifies dispatch + TaskNotificationStatus
// enum decode + nested optional Usage pointer.
func TestParseMessage_TaskNotificationMessage(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"system","subtype":"task_notification","task_id":"t_3","status":"completed","output_file":"/tmp/out","summary":"all good","uuid":"u_3","session_id":"s_3","usage":{"total_tokens":99,"tool_uses":1,"duration_ms":100}}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	tnm, ok := msg.(TaskNotificationMessage)
	if !ok {
		t.Fatalf("type = %T, want TaskNotificationMessage", msg)
	}
	if tnm.Status != TaskNotificationStatusCompleted {
		t.Errorf("Status = %q, want completed", tnm.Status)
	}
	if tnm.OutputFile != "/tmp/out" {
		t.Errorf("OutputFile = %q, want /tmp/out", tnm.OutputFile)
	}
	if tnm.Summary != "all good" {
		t.Errorf("Summary = %q, want all good", tnm.Summary)
	}
	if tnm.Usage == nil {
		t.Fatalf("Usage = nil, want non-nil pointer")
	}
	if tnm.Usage.TotalTokens != 99 {
		t.Errorf("Usage.TotalTokens = %d, want 99", tnm.Usage.TotalTokens)
	}
}

// TestParseMessage_TaskNotification_FailedStatus verifies the other two
// status wire literals.
func TestParseMessage_TaskNotification_FailedStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"failed", "stopped"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			line := []byte(`{"type":"system","subtype":"task_notification","task_id":"t","status":"` + status + `","summary":"x","uuid":"u","session_id":"s"}` + "\n")
			msg, err := parseMessage(line)
			if err != nil {
				t.Fatalf("parseMessage: %v", err)
			}
			tnm := msg.(TaskNotificationMessage)
			if string(tnm.Status) != status {
				t.Errorf("Status = %q, want %q", tnm.Status, status)
			}
		})
	}
}

// TestParseMessage_SystemMessage_FallbackRegression is the regression guard:
// system messages with a NON-task subtype must still return generic
// SystemMessage so the parser stays forward-compatible.
func TestParseMessage_SystemMessage_FallbackRegression(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"system","subtype":"compaction","data":{"reason":"context_full"}}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	sm, ok := msg.(SystemMessage)
	if !ok {
		t.Fatalf("type = %T, want SystemMessage for unknown subtype", msg)
	}
	if sm.Subtype != "compaction" {
		t.Errorf("Subtype = %q, want compaction", sm.Subtype)
	}
	if !strings.Contains(string(sm.Raw), "context_full") {
		t.Errorf("Raw = %q, want context_full preserved", sm.Raw)
	}
}

// TestParseMessage_SystemMessage_NoSubtypeFallback verifies a system message
// with NO subtype field still routes to SystemMessage (not a parse error).
func TestParseMessage_SystemMessage_NoSubtypeFallback(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"system","session_id":"s"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if _, ok := msg.(SystemMessage); !ok {
		t.Errorf("type = %T, want SystemMessage", msg)
	}
}

// TestResultMessage_DeferredToolUse verifies the new field decodes when
// upstream's deferred_tool_use key is present on a result message.
func TestResultMessage_DeferredToolUse(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"result","subtype":"success","duration_ms":100,"is_error":false,"num_turns":1,"session_id":"s","deferred_tool_use":{"id":"tu_1","name":"Bash","input":{"command":"rm -rf /"}}}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	rm := msg.(ResultMessage)
	if rm.DeferredToolUse == nil {
		t.Fatalf("DeferredToolUse = nil, want non-nil pointer")
	}
	if rm.DeferredToolUse.ID != "tu_1" {
		t.Errorf("ID = %q, want tu_1", rm.DeferredToolUse.ID)
	}
	if rm.DeferredToolUse.Name != "Bash" {
		t.Errorf("Name = %q, want Bash", rm.DeferredToolUse.Name)
	}
	if !strings.Contains(string(rm.DeferredToolUse.Input), `"command":"rm -rf /"`) {
		t.Errorf("Input = %q, want command preserved", rm.DeferredToolUse.Input)
	}
}

// TestResultMessage_NoDeferredToolUseOmits verifies the field is nil when the
// wire payload doesn't carry deferred_tool_use (regression against
// over-allocation).
func TestResultMessage_NoDeferredToolUseOmits(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"result","subtype":"success","duration_ms":100,"is_error":false,"num_turns":1,"session_id":"s"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	rm := msg.(ResultMessage)
	if rm.DeferredToolUse != nil {
		t.Errorf("DeferredToolUse = %+v, want nil", rm.DeferredToolUse)
	}
}

// TestTaskStartedMessage_JSONTagsParity is the wire-tag parity guard. A
// hand-marshal vs literal-map check catches any tag typo at once. Pinned
// against upstream TaskStartedMessage (types.py:1060).
func TestTaskStartedMessage_JSONTagsParity(t *testing.T) {
	t.Parallel()
	in := TaskStartedMessage{
		Subtype:     "task_started",
		TaskID:      "t",
		Description: "d",
		UUID:        "u",
		SessionID:   "s",
		ToolUseID:   "tu",
		TaskType:    "build",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]string{
		"subtype":     "task_started",
		"task_id":     "t",
		"description": "d",
		"uuid":        "u",
		"session_id":  "s",
		"tool_use_id": "tu",
		"task_type":   "build",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("key %q = %v, want %q", k, got[k], v)
		}
	}
}

// ── Top-level events (StreamEvent, RateLimitEvent) ────────────────────────

var (
	_ Message = StreamEvent{}
	_ Message = RateLimitEvent{}
)

// TestRateLimitStatus_Literals pins the 3 wire literals (types.py:1181).
func TestRateLimitStatus_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		s    RateLimitStatus
		want string
	}{
		"allowed":         {RateLimitStatusAllowed, "allowed"},
		"allowed_warning": {RateLimitStatusAllowedWarning, "allowed_warning"},
		"rejected":        {RateLimitStatusRejected, "rejected"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.s) != tt.want {
				t.Errorf("status = %q, want %q", string(tt.s), tt.want)
			}
		})
	}
}

// TestRateLimitType_Literals pins the 5 wire literals (types.py:1182-1184).
func TestRateLimitType_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		s    RateLimitType
		want string
	}{
		"five_hour":        {RateLimitTypeFiveHour, "five_hour"},
		"seven_day":        {RateLimitTypeSevenDay, "seven_day"},
		"seven_day_opus":   {RateLimitTypeSevenDayOpus, "seven_day_opus"},
		"seven_day_sonnet": {RateLimitTypeSevenDaySonnet, "seven_day_sonnet"},
		"overage":          {RateLimitTypeOverage, "overage"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.s) != tt.want {
				t.Errorf("type = %q, want %q", string(tt.s), tt.want)
			}
		})
	}
}

// TestParseMessage_StreamEvent verifies dispatch from type=stream_event and
// that the inner Event payload survives as opaque jsontext.Value.
func TestParseMessage_StreamEvent(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"stream_event","uuid":"u_1","session_id":"s_1","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"}},"parent_tool_use_id":"tu_1"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	se, ok := msg.(StreamEvent)
	if !ok {
		t.Fatalf("type = %T, want StreamEvent", msg)
	}
	if se.UUID != "u_1" {
		t.Errorf("UUID = %q, want u_1", se.UUID)
	}
	if se.SessionID != "s_1" {
		t.Errorf("SessionID = %q, want s_1", se.SessionID)
	}
	if se.ParentToolUseID != "tu_1" {
		t.Errorf("ParentToolUseID = %q, want tu_1", se.ParentToolUseID)
	}
	// Event is kept opaque; just confirm it round-trips the inner shape.
	if !strings.Contains(string(se.Event), `"stop_reason":"end_turn"`) {
		t.Errorf("Event = %q, want to contain stop_reason", se.Event)
	}
}

// TestParseMessage_RateLimitEvent verifies dispatch from type=rate_limit_event
// and the full RateLimitInfo nested decode with all camelCase wire keys.
func TestParseMessage_RateLimitEvent(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning","resetsAt":1700000000,"rateLimitType":"five_hour","utilization":0.85,"overageStatus":"allowed","overageResetsAt":1700001000,"overageDisabledReason":""},"uuid":"u_42","session_id":"s_42"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	rle, ok := msg.(RateLimitEvent)
	if !ok {
		t.Fatalf("type = %T, want RateLimitEvent", msg)
	}
	if rle.UUID != "u_42" {
		t.Errorf("UUID = %q, want u_42", rle.UUID)
	}
	if rle.SessionID != "s_42" {
		t.Errorf("SessionID = %q, want s_42", rle.SessionID)
	}
	info := rle.RateLimitInfo
	if info.Status != RateLimitStatusAllowedWarning {
		t.Errorf("Status = %q, want allowed_warning", info.Status)
	}
	if info.ResetsAt != 1700000000 {
		t.Errorf("ResetsAt = %d, want 1700000000 (wire key resetsAt; snake_case resets_at would not decode)", info.ResetsAt)
	}
	if info.RateLimitType != RateLimitTypeFiveHour {
		t.Errorf("RateLimitType = %q, want five_hour (wire key rateLimitType)", info.RateLimitType)
	}
	if info.Utilization != 0.85 {
		t.Errorf("Utilization = %f, want 0.85", info.Utilization)
	}
	if info.OverageStatus != RateLimitStatusAllowed {
		t.Errorf("OverageStatus = %q, want allowed (wire key overageStatus)", info.OverageStatus)
	}
	if info.OverageResetsAt != 1700001000 {
		t.Errorf("OverageResetsAt = %d, want 1700001000 (wire key overageResetsAt)", info.OverageResetsAt)
	}
}

// TestRateLimitInfo_JSONTagsParity is the wire-tag parity guard. A
// hand-marshal vs literal-map check pins all 5 camelCase wire-key renames
// (resetsAt, rateLimitType, overageStatus, overageResetsAt,
// overageDisabledReason).
func TestRateLimitInfo_JSONTagsParity(t *testing.T) {
	t.Parallel()
	in := RateLimitInfo{
		Status:                RateLimitStatusRejected,
		ResetsAt:              1700000000,
		RateLimitType:         RateLimitTypeSevenDay,
		Utilization:           1.0,
		OverageStatus:         RateLimitStatusAllowed,
		OverageResetsAt:       1700001000,
		OverageDisabledReason: "free tier",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, want := range []string{
		`"resetsAt":1700000000`,
		`"rateLimitType":"seven_day"`,
		`"overageStatus":"allowed"`,
		`"overageResetsAt":1700001000`,
		`"overageDisabledReason":"free tier"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("marshal output missing %q (wire-tag parity broken):\n%s", want, s)
		}
	}
	// Guard against accidental snake_case leakage.
	for _, banned := range []string{"resets_at", "rate_limit_type", "overage_status"} {
		if strings.Contains(s, banned) {
			t.Errorf("marshal output contains snake_case %q, want camelCase wire keys:\n%s", banned, s)
		}
	}
}

// TestParseMessage_UnknownTopLevelTypeStillRaw is the forward-compat
// regression guard: an unknown top-level "type" must STILL fall through to
// rawMessage, so the new stream_event / rate_limit_event dispatches didn't
// shrink the envelope.
func TestParseMessage_UnknownTopLevelTypeStillRaw(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"future_message_kind","payload":{"x":1}}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if _, ok := msg.(rawMessage); !ok {
		t.Errorf("future_message_kind dispatched to %T, want rawMessage", msg)
	}
}
