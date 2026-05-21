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

// ── compile-time interface satisfaction ──────────────────────────────────────

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
