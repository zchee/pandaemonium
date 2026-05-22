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
// that every server-tool result wire type collapses into ServerToolResultBlock
// with Content preserved: the current-upstream advisor_tool_result discriminator
// plus the legacy per-tool discriminators the port originally targeted.
func TestParseMessage_AssistantMessage_ServerToolResultBlock_AllVariants(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{
		"advisor_tool_result",
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
