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
)

var _ Message = HookEventMessage{}

// TestParseMessage_HookStartedMessage verifies the system+subtype=hook_started
// dispatch routes to HookEventMessage (not SystemMessage) and that the
// canonical hook_event wire key populates HookEventName.
func TestParseMessage_HookStartedMessage(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"system","subtype":"hook_started","hook_event":"PreToolUse","session_id":"s_1","uuid":"u_1","tool_name":"Bash","tool_input":{"command":"ls"}}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	hem, ok := msg.(HookEventMessage)
	if !ok {
		t.Fatalf("type = %T, want HookEventMessage", msg)
	}
	if hem.Subtype != "hook_started" {
		t.Errorf("Subtype = %q, want hook_started", hem.Subtype)
	}
	if hem.HookEventName != "PreToolUse" {
		t.Errorf("HookEventName = %q, want PreToolUse (from hook_event wire key)", hem.HookEventName)
	}
	if hem.SessionID != "s_1" {
		t.Errorf("SessionID = %q, want s_1", hem.SessionID)
	}
	if hem.UUID != "u_1" {
		t.Errorf("UUID = %q, want u_1", hem.UUID)
	}
	if !strings.Contains(string(hem.Raw), `"tool_name":"Bash"`) {
		t.Errorf("Raw = %q, want to contain tool_name", hem.Raw)
	}
	if !strings.Contains(string(hem.Raw), `"command":"ls"`) {
		t.Errorf("Raw = %q, want to contain tool_input.command", hem.Raw)
	}
}

// TestParseMessage_HookResponseMessage verifies the hook_response subtype with
// the response-specific fields (output, exit_code, outcome) preserved in Raw.
func TestParseMessage_HookResponseMessage(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"system","subtype":"hook_response","hook_event":"PostToolUse","session_id":"s_2","uuid":"u_2","output":"ok","exit_code":0,"outcome":"success"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	hem, ok := msg.(HookEventMessage)
	if !ok {
		t.Fatalf("type = %T, want HookEventMessage", msg)
	}
	if hem.Subtype != "hook_response" {
		t.Errorf("Subtype = %q, want hook_response", hem.Subtype)
	}
	if hem.HookEventName != "PostToolUse" {
		t.Errorf("HookEventName = %q, want PostToolUse", hem.HookEventName)
	}
	for _, key := range []string{`"output":"ok"`, `"exit_code":0`, `"outcome":"success"`} {
		if !strings.Contains(string(hem.Raw), key) {
			t.Errorf("Raw = %q, want to contain %s", hem.Raw, key)
		}
	}
}

// TestParseMessage_HookEventName_AlternateKeys pins the wire-key
// normalization: upstream tolerates "hook_event" (current), "hook_name"
// (legacy variant), and "hook_event_name" (legacy variant). All three must
// surface as the same HookEventName field.
func TestParseMessage_HookEventName_AlternateKeys(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		key  string
		name string
	}{
		"hook_event (canonical)":   {"hook_event", "PreToolUse"},
		"hook_name (legacy)":       {"hook_name", "Stop"},
		"hook_event_name (legacy)": {"hook_event_name", "SessionStart"},
	}
	for label, tt := range tests {
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			line := []byte(`{"type":"system","subtype":"hook_started","` + tt.key + `":"` + tt.name + `","session_id":"s","uuid":"u"}` + "\n")
			msg, err := parseMessage(line)
			if err != nil {
				t.Fatalf("parseMessage: %v", err)
			}
			hem := msg.(HookEventMessage)
			if hem.HookEventName != tt.name {
				t.Errorf("HookEventName = %q, want %q (wire key %q)", hem.HookEventName, tt.name, tt.key)
			}
		})
	}
}

// TestParseMessage_HookEventName_PrecedenceOrder verifies that when multiple
// wire-key spellings are present, hook_event wins over hook_name, which wins
// over hook_event_name -- the same precedence upstream uses
// (message_parser.py:62-66).
func TestParseMessage_HookEventName_PrecedenceOrder(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"system","subtype":"hook_started","hook_event":"canonical","hook_name":"legacy1","hook_event_name":"legacy2","session_id":"s","uuid":"u"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	hem := msg.(HookEventMessage)
	if hem.HookEventName != "canonical" {
		t.Errorf("HookEventName = %q, want canonical (hook_event must win)", hem.HookEventName)
	}
}

// TestParseMessage_HookEventName_AllAbsentIsEmpty verifies that when none of
// the three wire-key spellings is present, HookEventName is the empty string
// (matches upstream's `or ""` fallback at message_parser.py:65).
func TestParseMessage_HookEventName_AllAbsentIsEmpty(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"system","subtype":"hook_started","session_id":"s","uuid":"u"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	hem := msg.(HookEventMessage)
	if hem.HookEventName != "" {
		t.Errorf("HookEventName = %q, want empty (no wire-key present)", hem.HookEventName)
	}
}

// TestParseMessage_HookEventVsTaskDispatch is the regression guard: the new
// hook_started/hook_response dispatch must not steal task_started or other
// system subtypes.
func TestParseMessage_HookEventVsTaskDispatch(t *testing.T) {
	t.Parallel()
	taskLine := []byte(`{"type":"system","subtype":"task_started","task_id":"t","description":"d","uuid":"u","session_id":"s"}` + "\n")
	taskMsg, err := parseMessage(taskLine)
	if err != nil {
		t.Fatalf("parseMessage(task_started): %v", err)
	}
	if _, ok := taskMsg.(TaskStartedMessage); !ok {
		t.Errorf("task_started dispatched to %T, want TaskStartedMessage (hook dispatch must not steal it)", taskMsg)
	}
	hookLine := []byte(`{"type":"system","subtype":"hook_started","hook_event":"PreToolUse","session_id":"s","uuid":"u"}` + "\n")
	hookMsg, err := parseMessage(hookLine)
	if err != nil {
		t.Fatalf("parseMessage(hook_started): %v", err)
	}
	if _, ok := hookMsg.(HookEventMessage); !ok {
		t.Errorf("hook_started dispatched to %T, want HookEventMessage", hookMsg)
	}
}
