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
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

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
