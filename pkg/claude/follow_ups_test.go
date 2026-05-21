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
	"context"
	stdjson "encoding/json"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

var _ Message = HookEventMessage{}

// ── HookEventMessage typed routing ─────────────────────────────────────────

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

// ── MCP tool-result content conversion ─────────────────────────────────────

// TestCallTool_TextOnlyResultStillWorks is the regression guard: the
// pre-F2 text-only path must keep producing the exact same wire output.
// A tool returning ToolResult{Content: "..."} (no RawContent) yields one
// {"type":"text","text":...} entry.
func TestCallTool_TextOnlyResultStillWorks(t *testing.T) {
	t.Parallel()

	tool := Tool[map[string]any]("say", "says something",
		nil,
		func(_ context.Context, _ map[string]any) (ToolResult, error) {
			return ToolResult{Content: "hello world"}, nil
		},
	)
	srv := NewSDKMCPServer("t", "1.0.0", tool).(*inProcessMCPServer)
	resp, err := srv.callTool(t.Context(), "say", stdjson.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("callTool: %v", err)
	}
	content, _ := resp["content"].([]map[string]any)
	if len(content) != 1 {
		t.Fatalf("content len = %d, want 1; resp=%v", len(content), resp)
	}
	if content[0]["type"] != "text" || content[0]["text"] != "hello world" {
		t.Errorf("content[0] = %v, want {type=text, text=\"hello world\"}", content[0])
	}
}

// TestCallTool_RawContentImage exercises the F2 escape hatch: returning
// ToolResult{RawContent: [...]} with a gomcp.ImageContent surfaces as an
// {"type":"image","mimeType":..., "data":...} block on the wire.
func TestCallTool_RawContentImage(t *testing.T) {
	t.Parallel()

	tool := Tool[map[string]any]("chart", "returns an image",
		nil,
		func(_ context.Context, _ map[string]any) (ToolResult, error) {
			return ToolResult{
				RawContent: []gomcp.Content{
					&gomcp.ImageContent{Data: []byte("PNGDATA"), MIMEType: "image/png"},
				},
			}, nil
		},
	)
	srv := NewSDKMCPServer("t", "1.0.0", tool).(*inProcessMCPServer)
	resp, err := srv.callTool(t.Context(), "chart", stdjson.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("callTool: %v", err)
	}
	content, _ := resp["content"].([]map[string]any)
	if len(content) != 1 {
		t.Fatalf("content len = %d, want 1; resp=%v", len(content), resp)
	}
	if content[0]["type"] != "image" {
		t.Errorf("content[0].type = %v, want image", content[0]["type"])
	}
	if content[0]["mimeType"] != "image/png" {
		t.Errorf("content[0].mimeType = %v, want image/png", content[0]["mimeType"])
	}
	if content[0]["data"] == nil {
		t.Errorf("content[0].data missing: %v", content[0])
	}
}

// TestCallTool_RawContentResourceLink verifies the resource_link variant.
func TestCallTool_RawContentResourceLink(t *testing.T) {
	t.Parallel()

	tool := Tool[map[string]any]("link", "returns a link",
		nil,
		func(_ context.Context, _ map[string]any) (ToolResult, error) {
			return ToolResult{
				RawContent: []gomcp.Content{
					&gomcp.ResourceLink{
						URI:         "file:///tmp/x.txt",
						Name:        "notes",
						Description: "scratch notes",
						MIMEType:    "text/plain",
					},
				},
			}, nil
		},
	)
	srv := NewSDKMCPServer("t", "1.0.0", tool).(*inProcessMCPServer)
	resp, err := srv.callTool(t.Context(), "link", stdjson.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("callTool: %v", err)
	}
	content, _ := resp["content"].([]map[string]any)
	if len(content) != 1 {
		t.Fatalf("content len = %d, want 1; resp=%v", len(content), resp)
	}
	if content[0]["type"] != "resource_link" {
		t.Errorf("content[0].type = %v, want resource_link", content[0]["type"])
	}
	if content[0]["uri"] != "file:///tmp/x.txt" {
		t.Errorf("content[0].uri = %v, want file:///tmp/x.txt", content[0]["uri"])
	}
	if content[0]["name"] != "notes" {
		t.Errorf("content[0].name = %v, want notes", content[0]["name"])
	}
}

// TestCallTool_RawContentMixed verifies multiple typed content entries can
// be returned from a single tool call, each preserving its own wire type.
func TestCallTool_RawContentMixed(t *testing.T) {
	t.Parallel()

	tool := Tool[map[string]any]("mixed", "returns text + image",
		nil,
		func(_ context.Context, _ map[string]any) (ToolResult, error) {
			return ToolResult{
				RawContent: []gomcp.Content{
					&gomcp.TextContent{Text: "see chart:"},
					&gomcp.ImageContent{Data: []byte("X"), MIMEType: "image/png"},
				},
			}, nil
		},
	)
	srv := NewSDKMCPServer("t", "1.0.0", tool).(*inProcessMCPServer)
	resp, err := srv.callTool(t.Context(), "mixed", stdjson.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("callTool: %v", err)
	}
	content, _ := resp["content"].([]map[string]any)
	if len(content) != 2 {
		t.Fatalf("content len = %d, want 2", len(content))
	}
	if content[0]["type"] != "text" || content[0]["text"] != "see chart:" {
		t.Errorf("content[0] = %v, want text entry", content[0])
	}
	if content[1]["type"] != "image" {
		t.Errorf("content[1].type = %v, want image", content[1]["type"])
	}
}

// TestCallTool_RawContentTakesPrecedence verifies the documented contract:
// when RawContent is non-nil, the Content string is ignored.
func TestCallTool_RawContentTakesPrecedence(t *testing.T) {
	t.Parallel()

	tool := Tool[map[string]any]("precedence", "Raw wins",
		nil,
		func(_ context.Context, _ map[string]any) (ToolResult, error) {
			return ToolResult{
				Content: "this should be ignored",
				RawContent: []gomcp.Content{
					&gomcp.TextContent{Text: "from RawContent"},
				},
			}, nil
		},
	)
	srv := NewSDKMCPServer("t", "1.0.0", tool).(*inProcessMCPServer)
	resp, err := srv.callTool(t.Context(), "precedence", stdjson.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("callTool: %v", err)
	}
	content, _ := resp["content"].([]map[string]any)
	if len(content) != 1 {
		t.Fatalf("content len = %d, want 1", len(content))
	}
	if content[0]["text"] != "from RawContent" {
		t.Errorf("content[0].text = %v, want \"from RawContent\" (Content string must be ignored when RawContent is set)", content[0]["text"])
	}
}

// TestCallTool_RawContentEmptySlicePreservesEmpty verifies an empty
// RawContent slice is honored (no fallback to Content string), so tool
// authors can deliberately emit a zero-block result.
func TestCallTool_RawContentEmptySlicePreservesEmpty(t *testing.T) {
	t.Parallel()

	tool := Tool[map[string]any]("empty", "empty",
		nil,
		func(_ context.Context, _ map[string]any) (ToolResult, error) {
			return ToolResult{
				Content:    "would-fall-back-but-shouldnt",
				RawContent: []gomcp.Content{},
			}, nil
		},
	)
	srv := NewSDKMCPServer("t", "1.0.0", tool).(*inProcessMCPServer)
	resp, err := srv.callTool(t.Context(), "empty", stdjson.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("callTool: %v", err)
	}
	content, _ := resp["content"].([]map[string]any)
	if len(content) != 0 {
		t.Errorf("content len = %d, want 0 (empty RawContent must NOT fall back to Content string)", len(content))
	}
}
