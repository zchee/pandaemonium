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
	"strings"
	"testing"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	gocmp "github.com/google/go-cmp/cmp"
)

// hookInputSamples returns one schema-complete JSON payload per hook event
// alongside the concrete value DecodeHookInput must produce.
func hookInputSamples() map[string]struct {
	payload string
	want    HookInput
} {
	return map[string]struct {
		payload string
		want    HookInput
	}{
		"success: PermissionRequest with agent fields": {
			payload: `{"agent_id":"agent-1","agent_type":"reviewer","cwd":"/work","hook_event_name":"PermissionRequest","model":"gpt-5.1-codex","permission_mode":"default","session_id":"sess-1","tool_input":{"command":["git","diff"]},"tool_name":"shell","transcript_path":"/tmp/transcript.jsonl","turn_id":"turn-1"}`,
			want: PermissionRequestHookInput{
				AgentID:        "agent-1",
				AgentType:      "reviewer",
				Cwd:            "/work",
				HookEventName:  HookInputEventNamePermissionRequest,
				Model:          "gpt-5.1-codex",
				PermissionMode: HookPermissionModeDefault,
				SessionID:      "sess-1",
				ToolInput:      jsontext.Value(`{"command":["git","diff"]}`),
				ToolName:       "shell",
				TranscriptPath: new("/tmp/transcript.jsonl"),
				TurnID:         "turn-1",
			},
		},
		"success: PostCompact with null transcript_path": {
			payload: `{"cwd":"/work","hook_event_name":"PostCompact","model":"gpt-5.1-codex","session_id":"sess-1","transcript_path":null,"trigger":"auto","turn_id":"turn-2"}`,
			want: PostCompactHookInput{
				Cwd:           "/work",
				HookEventName: HookInputEventNamePostCompact,
				Model:         "gpt-5.1-codex",
				SessionID:     "sess-1",
				Trigger:       HookCompactTriggerAuto,
				TurnID:        "turn-2",
			},
		},
		"success: PostToolUse with tool_response": {
			payload: `{"cwd":"/work","hook_event_name":"PostToolUse","model":"gpt-5.1-codex","permission_mode":"acceptEdits","session_id":"sess-1","tool_input":{"path":"main.go"},"tool_name":"read_file","tool_response":{"ok":true},"tool_use_id":"call-7","transcript_path":"/tmp/t.jsonl","turn_id":"turn-3"}`,
			want: PostToolUseHookInput{
				Cwd:            "/work",
				HookEventName:  HookInputEventNamePostToolUse,
				Model:          "gpt-5.1-codex",
				PermissionMode: HookPermissionModeAcceptEdits,
				SessionID:      "sess-1",
				ToolInput:      jsontext.Value(`{"path":"main.go"}`),
				ToolName:       "read_file",
				ToolResponse:   jsontext.Value(`{"ok":true}`),
				ToolUseID:      "call-7",
				TranscriptPath: new("/tmp/t.jsonl"),
				TurnID:         "turn-3",
			},
		},
		"success: PreCompact manual trigger": {
			payload: `{"cwd":"/work","hook_event_name":"PreCompact","model":"gpt-5.1-codex","session_id":"sess-1","transcript_path":"/tmp/t.jsonl","trigger":"manual","turn_id":"turn-4"}`,
			want: PreCompactHookInput{
				Cwd:            "/work",
				HookEventName:  HookInputEventNamePreCompact,
				Model:          "gpt-5.1-codex",
				SessionID:      "sess-1",
				TranscriptPath: new("/tmp/t.jsonl"),
				Trigger:        HookCompactTriggerManual,
				TurnID:         "turn-4",
			},
		},
		"success: PreToolUse with scalar tool_input": {
			payload: `{"agent_id":"agent-2","agent_type":"explorer","cwd":"/work","hook_event_name":"PreToolUse","model":"gpt-5.1-codex","permission_mode":"plan","session_id":"sess-1","tool_input":"ls -la","tool_name":"shell","tool_use_id":"call-9","transcript_path":null,"turn_id":"turn-5"}`,
			want: PreToolUseHookInput{
				AgentID:        "agent-2",
				AgentType:      "explorer",
				Cwd:            "/work",
				HookEventName:  HookInputEventNamePreToolUse,
				Model:          "gpt-5.1-codex",
				PermissionMode: HookPermissionModePlan,
				SessionID:      "sess-1",
				ToolInput:      jsontext.Value(`"ls -la"`),
				ToolName:       "shell",
				ToolUseID:      "call-9",
				TurnID:         "turn-5",
			},
		},
		"success: SessionStart startup source": {
			payload: `{"cwd":"/work","hook_event_name":"SessionStart","model":"gpt-5.1-codex","permission_mode":"default","session_id":"sess-2","source":"startup","transcript_path":null}`,
			want: SessionStartHookInput{
				Cwd:            "/work",
				HookEventName:  HookInputEventNameSessionStart,
				Model:          "gpt-5.1-codex",
				PermissionMode: HookPermissionModeDefault,
				SessionID:      "sess-2",
				Source:         HookSessionStartSourceStartup,
			},
		},
		"success: Stop with last assistant message": {
			payload: `{"cwd":"/work","hook_event_name":"Stop","last_assistant_message":"done","model":"gpt-5.1-codex","permission_mode":"default","session_id":"sess-2","stop_hook_active":false,"transcript_path":"/tmp/t.jsonl","turn_id":"turn-6"}`,
			want: StopHookInput{
				Cwd:                  "/work",
				HookEventName:        HookInputEventNameStop,
				LastAssistantMessage: new("done"),
				Model:                "gpt-5.1-codex",
				PermissionMode:       HookPermissionModeDefault,
				SessionID:            "sess-2",
				TranscriptPath:       new("/tmp/t.jsonl"),
				TurnID:               "turn-6",
			},
		},
		"success: SubagentStart with required agent fields": {
			payload: `{"agent_id":"agent-3","agent_type":"worker","cwd":"/work","hook_event_name":"SubagentStart","model":"gpt-5.1-codex","permission_mode":"bypassPermissions","session_id":"sess-2","transcript_path":"/tmp/t.jsonl","turn_id":"turn-7"}`,
			want: SubagentStartHookInput{
				AgentID:        "agent-3",
				AgentType:      "worker",
				Cwd:            "/work",
				HookEventName:  HookInputEventNameSubagentStart,
				Model:          "gpt-5.1-codex",
				PermissionMode: HookPermissionModeBypassPermissions,
				SessionID:      "sess-2",
				TranscriptPath: new("/tmp/t.jsonl"),
				TurnID:         "turn-7",
			},
		},
		"success: SubagentStop with null nullable fields": {
			payload: `{"agent_id":"agent-3","agent_transcript_path":null,"agent_type":"worker","cwd":"/work","hook_event_name":"SubagentStop","last_assistant_message":null,"model":"gpt-5.1-codex","permission_mode":"default","session_id":"sess-2","stop_hook_active":true,"transcript_path":"/tmp/t.jsonl","turn_id":"turn-7"}`,
			want: SubagentStopHookInput{
				AgentID:        "agent-3",
				AgentType:      "worker",
				Cwd:            "/work",
				HookEventName:  HookInputEventNameSubagentStop,
				Model:          "gpt-5.1-codex",
				PermissionMode: HookPermissionModeDefault,
				SessionID:      "sess-2",
				StopHookActive: true,
				TranscriptPath: new("/tmp/t.jsonl"),
				TurnID:         "turn-7",
			},
		},
		"success: UserPromptSubmit dontAsk mode": {
			payload: `{"cwd":"/work","hook_event_name":"UserPromptSubmit","model":"gpt-5.1-codex","permission_mode":"dontAsk","prompt":"fix the bug","session_id":"sess-3","transcript_path":null,"turn_id":"turn-8"}`,
			want: UserPromptSubmitHookInput{
				Cwd:            "/work",
				HookEventName:  HookInputEventNameUserPromptSubmit,
				Model:          "gpt-5.1-codex",
				PermissionMode: HookPermissionModeDontAsk,
				Prompt:         "fix the bug",
				SessionID:      "sess-3",
				TurnID:         "turn-8",
			},
		},
	}
}

func TestDecodeHookInput(t *testing.T) {
	t.Parallel()

	for name, tt := range hookInputSamples() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeHookInput([]byte(tt.payload))
			if err != nil {
				t.Fatalf("DecodeHookInput returned error: %v", err)
			}
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("DecodeHookInput mismatch (-want +got):\n%s", diff)
			}
			if got.EventName() != tt.want.EventName() {
				t.Fatalf("EventName() = %q, want %q", got.EventName(), tt.want.EventName())
			}
		})
	}
}

func TestDecodeHookInputErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		payload string
		wantErr string
	}{
		"error: missing hook_event_name": {
			payload: `{"cwd":"/work"}`,
			wantErr: "missing hook_event_name",
		},
		"error: unsupported event name": {
			payload: `{"hook_event_name":"Bogus"}`,
			wantErr: `unsupported hook input event name "Bogus"`,
		},
		"error: malformed JSON": {
			payload: `{"hook_event_name":`,
			wantErr: "decode hook input event name",
		},
		"error: payload type mismatch after dispatch": {
			payload: `{"hook_event_name":"Stop","stop_hook_active":"not-a-bool"}`,
			wantErr: "decode Stop hook input",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeHookInput([]byte(tt.payload))
			if err == nil {
				t.Fatalf("DecodeHookInput succeeded with %#v, want error containing %q", got, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("DecodeHookInput error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

// TestHookInputRoundTrip proves MarshalJSONTo and UnmarshalJSONFrom are
// symmetric: decode -> encode reproduces the original schema-complete JSON.
func TestHookInputRoundTrip(t *testing.T) {
	t.Parallel()

	for name, tt := range hookInputSamples() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			decoded, err := DecodeHookInput([]byte(tt.payload))
			if err != nil {
				t.Fatalf("DecodeHookInput returned error: %v", err)
			}
			encoded, err := json.Marshal(decoded)
			if err != nil {
				t.Fatalf("json.Marshal returned error: %v", err)
			}

			var wantJSON, gotJSON any
			if err := json.Unmarshal([]byte(tt.payload), &wantJSON); err != nil {
				t.Fatalf("unmarshal original payload: %v", err)
			}
			if err := json.Unmarshal(encoded, &gotJSON); err != nil {
				t.Fatalf("unmarshal re-encoded payload %s: %v", encoded, err)
			}
			if diff := gocmp.Diff(wantJSON, gotJSON); diff != "" {
				t.Fatalf("round-trip JSON mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestHookInputMarshalDefaultsEventName proves MarshalJSONTo pins the
// hook_event_name discriminator even when the caller leaves it empty.
func TestHookInputMarshalDefaultsEventName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value HookInput
		want  HookInputEventName
	}{
		"success: PermissionRequest": {value: PermissionRequestHookInput{}, want: HookInputEventNamePermissionRequest},
		"success: PostCompact":       {value: PostCompactHookInput{}, want: HookInputEventNamePostCompact},
		"success: PostToolUse":       {value: PostToolUseHookInput{}, want: HookInputEventNamePostToolUse},
		"success: PreCompact":        {value: PreCompactHookInput{}, want: HookInputEventNamePreCompact},
		"success: PreToolUse":        {value: PreToolUseHookInput{}, want: HookInputEventNamePreToolUse},
		"success: SessionStart":      {value: SessionStartHookInput{}, want: HookInputEventNameSessionStart},
		"success: Stop":              {value: StopHookInput{}, want: HookInputEventNameStop},
		"success: SubagentStart":     {value: SubagentStartHookInput{}, want: HookInputEventNameSubagentStart},
		"success: SubagentStop":      {value: SubagentStopHookInput{}, want: HookInputEventNameSubagentStop},
		"success: UserPromptSubmit":  {value: UserPromptSubmitHookInput{}, want: HookInputEventNameUserPromptSubmit},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := tt.value.EventName(); got != tt.want {
				t.Fatalf("EventName() = %q, want %q", got, tt.want)
			}

			encoded, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("json.Marshal returned error: %v", err)
			}
			var probe struct {
				HookEventName HookInputEventName `json:"hook_event_name"`
			}
			if err := json.Unmarshal(encoded, &probe); err != nil {
				t.Fatalf("unmarshal encoded payload %s: %v", encoded, err)
			}
			if probe.HookEventName != tt.want {
				t.Fatalf("marshaled hook_event_name = %q, want %q\npayload: %s", probe.HookEventName, tt.want, encoded)
			}

			roundTripped, err := DecodeHookInput(encoded)
			if err != nil {
				t.Fatalf("DecodeHookInput on marshaled zero value: %v", err)
			}
			if roundTripped.EventName() != tt.want {
				t.Fatalf("round-tripped EventName() = %q, want %q", roundTripped.EventName(), tt.want)
			}
		})
	}
}

// TestHookInputUnmarshalRejectsMismatchedEvent proves UnmarshalJSONFrom
// validates the discriminator instead of silently absorbing foreign payloads.
func TestHookInputUnmarshalRejectsMismatchedEvent(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		payload string
		into    json.UnmarshalerFrom
		wantErr string
	}{
		"error: Stop payload into PreToolUse": {
			payload: `{"cwd":"/work","hook_event_name":"Stop","last_assistant_message":null,"model":"m","permission_mode":"default","session_id":"s","stop_hook_active":false,"transcript_path":null,"turn_id":"t"}`,
			into:    &PreToolUseHookInput{},
			wantErr: `unexpected hook_event_name "Stop"`,
		},
		"error: SessionStart payload into SubagentStop": {
			payload: `{"cwd":"/work","hook_event_name":"SessionStart","model":"m","permission_mode":"default","session_id":"s","source":"startup","transcript_path":null}`,
			into:    &SubagentStopHookInput{},
			wantErr: `unexpected hook_event_name "SessionStart"`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := json.Unmarshal([]byte(tt.payload), tt.into)
			if err == nil {
				t.Fatalf("json.Unmarshal succeeded into %T, want error containing %q", tt.into, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("json.Unmarshal error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

// TestHookInputUnmarshalNormalizesEventName proves a payload without the
// discriminator still decodes into the concrete type with the pinned name.
func TestHookInputUnmarshalNormalizesEventName(t *testing.T) {
	t.Parallel()

	var got SessionStartHookInput
	payload := `{"cwd":"/work","model":"m","permission_mode":"default","session_id":"s","source":"resume","transcript_path":null}`
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	want := SessionStartHookInput{
		Cwd:            "/work",
		HookEventName:  HookInputEventNameSessionStart,
		Model:          "m",
		PermissionMode: HookPermissionModeDefault,
		SessionID:      "s",
		Source:         HookSessionStartSourceResume,
	}
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Fatalf("normalized decode mismatch (-want +got):\n%s", diff)
	}
}

// TestEncodeHookInput proves EncodeHook emits schema-complete JSON that
// DecodeHookInput accepts and maps back to the identical concrete value.
func TestEncodeHookInput(t *testing.T) {
	t.Parallel()

	for name, tt := range hookInputSamples() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			encoded, err := EncodeHookInput(tt.want)
			if err != nil {
				t.Fatalf("EncodeHookInput returned error: %v", err)
			}

			var wantJSON, gotJSON any
			if err := json.Unmarshal([]byte(tt.payload), &wantJSON); err != nil {
				t.Fatalf("unmarshal original payload: %v", err)
			}
			if err := json.Unmarshal(encoded, &gotJSON); err != nil {
				t.Fatalf("unmarshal encoded payload %s: %v", encoded, err)
			}
			if diff := gocmp.Diff(wantJSON, gotJSON); diff != "" {
				t.Fatalf("encoded JSON mismatch (-want +got):\n%s", diff)
			}

			decoded, err := DecodeHookInput(encoded)
			if err != nil {
				t.Fatalf("DecodeHookInput on encoded payload: %v", err)
			}
			if diff := gocmp.Diff(tt.want, decoded); diff != "" {
				t.Fatalf("encode/decode round-trip mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestEncodeHookInputErrors proves EncodeHook rejects payloads that could not
// round-trip through DecodeHookInput.
func TestEncodeHookInputErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		hook    HookInput
		wantErr string
	}{
		"error: nil hook": {
			hook:    nil,
			wantErr: "nil Hook",
		},
		"error: typed-nil pointer hook": {
			hook:    (*StopHookInput)(nil),
			wantErr: "nil Hook",
		},
		"error: mismatched event name": {
			hook:    PreToolUseHookInput{HookEventName: HookInputEventNameStop},
			wantErr: `unexpected hook_event_name "Stop" for PreToolUse hook input`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := EncodeHookInput(tt.hook)
			if err == nil {
				t.Fatalf("EncodeHookInput succeeded with %s, want error containing %q", got, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("EncodeHookInput error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}
