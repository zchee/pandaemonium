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
	"strings"
	"testing"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPServerConnectionStatus_Literals pins the five wire literals against
// upstream McpServerConnectionStatus (types.py:707-710).
func TestMCPServerConnectionStatus_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		s    MCPServerConnectionStatus
		want string
	}{
		"connected":  {MCPServerConnectionStatusConnected, "connected"},
		"failed":     {MCPServerConnectionStatusFailed, "failed"},
		"needs-auth": {MCPServerConnectionStatusNeedsAuth, "needs-auth"},
		"pending":    {MCPServerConnectionStatusPending, "pending"},
		"disabled":   {MCPServerConnectionStatusDisabled, "disabled"},
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

// TestMCPStatusResponse_JSONTagsParity hand-marshals a fully-populated
// MCPStatusResponse and asserts every wire field name matches upstream
// (types.py:680-747). Catches every JSON-tag typo at once.
func TestMCPStatusResponse_JSONTagsParity(t *testing.T) {
	t.Parallel()
	in := MCPStatusResponse{
		MCPServers: []MCPServerStatus{
			{
				Name:       "calc",
				Status:     MCPServerConnectionStatusConnected,
				ServerInfo: MCPServerInfo{Name: "calc", Version: "1.0.0"},
				Config:     jsontext.Value(`{"type":"sdk","name":"calc"}`),
				Scope:      "project",
				Tools: []MCPToolInfo{
					{
						Name:        "add",
						Description: "Adds two numbers",
						Annotations: MCPToolAnnotations{
							ReadOnly:    true,
							Destructive: false,
							OpenWorld:   false,
						},
					},
				},
			},
			{
				Name:   "broken",
				Status: MCPServerConnectionStatusFailed,
				Error:  "connection refused",
			},
		},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	servers, ok := got["mcpServers"].([]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("mcpServers = %v, want 2 entries (key is mcpServers, NOT mcp_servers)", got)
	}

	connected, _ := servers[0].(map[string]any)
	if connected["name"] != "calc" {
		t.Errorf("connected.name = %v, want calc", connected["name"])
	}
	if connected["status"] != "connected" {
		t.Errorf("connected.status = %v, want connected", connected["status"])
	}
	si, _ := connected["serverInfo"].(map[string]any)
	if si == nil || si["name"] != "calc" || si["version"] != "1.0.0" {
		t.Errorf("connected.serverInfo = %v, want {name=calc, version=1.0.0} (key is serverInfo, NOT server_info)", connected["serverInfo"])
	}
	if connected["scope"] != "project" {
		t.Errorf("connected.scope = %v, want project", connected["scope"])
	}
	tools, _ := connected["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("connected.tools = %v, want 1 entry", connected["tools"])
	}
	tool0, _ := tools[0].(map[string]any)
	ann, _ := tool0["annotations"].(map[string]any)
	if ann["readOnly"] != true {
		t.Errorf("tool.annotations.readOnly = %v, want true (key is readOnly, NOT readOnlyHint)", ann["readOnly"])
	}

	failed, _ := servers[1].(map[string]any)
	if failed["status"] != "failed" {
		t.Errorf("failed.status = %v, want failed", failed["status"])
	}
	if failed["error"] != "connection refused" {
		t.Errorf("failed.error = %v, want connection refused", failed["error"])
	}
}

// TestMCPStatusResponse_RoundTrip is the inverse parity test: decode a wire
// fixture into the typed struct and confirm the fields land correctly.
func TestMCPStatusResponse_RoundTrip(t *testing.T) {
	t.Parallel()
	wire := `{"mcpServers":[{"name":"docs","status":"needs-auth"},{"name":"calc","status":"connected","serverInfo":{"name":"calc","version":"2"},"tools":[{"name":"add","description":"adds","annotations":{"readOnly":true,"openWorld":true}}],"scope":"user"}]}`
	var resp MCPStatusResponse
	if err := json.Unmarshal([]byte(wire), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.MCPServers) != 2 {
		t.Fatalf("MCPServers len = %d, want 2", len(resp.MCPServers))
	}
	if resp.MCPServers[0].Status != MCPServerConnectionStatusNeedsAuth {
		t.Errorf("[0].Status = %q, want needs-auth", resp.MCPServers[0].Status)
	}
	if resp.MCPServers[1].ServerInfo.Version != "2" {
		t.Errorf("[1].ServerInfo.Version = %q, want 2", resp.MCPServers[1].ServerInfo.Version)
	}
	if len(resp.MCPServers[1].Tools) != 1 {
		t.Fatalf("[1].Tools len = %d, want 1", len(resp.MCPServers[1].Tools))
	}
	tool := resp.MCPServers[1].Tools[0]
	if !tool.Annotations.ReadOnly {
		t.Errorf("tool.Annotations.ReadOnly = false, want true")
	}
	if !tool.Annotations.OpenWorld {
		t.Errorf("tool.Annotations.OpenWorld = false, want true")
	}
	if tool.Annotations.Destructive {
		t.Errorf("tool.Annotations.Destructive = true, want false (unset)")
	}
}

// TestClient_GetMCPStatusTyped_Success exercises the typed accessor over the
// fake CLI: it sends a wire payload through mcp_status and confirms the
// decoded MCPStatusResponse matches.
func TestClient_GetMCPStatusTyped_Success(t *testing.T) {
	t.Parallel()
	c, cli := startConnectedClient(t, &Options{})
	defer c.Close()
	autoAnswer(t, cli, "mcp_status", `{"mcpServers":[{"name":"calc","status":"connected","serverInfo":{"name":"calc","version":"1.0.0"}}]}`)

	resp, err := c.GetMCPStatusTyped(t.Context())
	if err != nil {
		t.Fatalf("GetMCPStatusTyped() error = %v", err)
	}
	if len(resp.MCPServers) != 1 {
		t.Fatalf("MCPServers len = %d, want 1", len(resp.MCPServers))
	}
	s := resp.MCPServers[0]
	if s.Name != "calc" {
		t.Errorf("Name = %q, want calc", s.Name)
	}
	if s.Status != MCPServerConnectionStatusConnected {
		t.Errorf("Status = %q, want connected", s.Status)
	}
	if s.ServerInfo.Version != "1.0.0" {
		t.Errorf("ServerInfo.Version = %q, want 1.0.0", s.ServerInfo.Version)
	}
}

// TestToolWithAnnotations_ListToolsSurfaces verifies a tool registered with
// annotations exposes them through the in-process server's tools/list
// response, which is what the CLI uses to populate MCPServerStatus.Tools.
func TestToolWithAnnotations_ListToolsSurfaces(t *testing.T) {
	t.Parallel()

	readOnly := true
	openWorld := false
	tool := ToolWithAnnotations[map[string]any]("greet", "greets",
		nil,
		&gomcp.ToolAnnotations{ReadOnlyHint: readOnly, OpenWorldHint: &openWorld, Title: "Greet"},
		func(ctx context.Context, in map[string]any) (ToolResult, error) {
			return ToolResult{Content: "ok"}, nil
		},
	)
	srv := NewSDKMCPServer("test", "1.0.0", tool).(*inProcessMCPServer)
	listed, err := srv.listTools()
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}
	tools, _ := listed["tools"].([]map[string]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v, want 1 entry", listed["tools"])
	}
	ann, ok := tools[0]["annotations"]
	if !ok {
		t.Fatalf("annotations key absent: %v", tools[0])
	}
	ga, ok := ann.(*gomcp.ToolAnnotations)
	if !ok {
		t.Fatalf("annotations type = %T, want *gomcp.ToolAnnotations", ann)
	}
	if !ga.ReadOnlyHint {
		t.Errorf("annotations.ReadOnlyHint = false, want true")
	}
	if ga.Title != "Greet" {
		t.Errorf("annotations.Title = %q, want Greet", ga.Title)
	}
}

// TestTool_NoAnnotationsListToolsOmits verifies the additive design: a tool
// constructed via the original [Tool] (no annotations) MUST NOT carry an
// annotations key in tools/list, so the wire payload stays minimal.
func TestTool_NoAnnotationsListToolsOmits(t *testing.T) {
	t.Parallel()
	tool := Tool[map[string]any]("plain", "no annotations",
		nil,
		func(ctx context.Context, in map[string]any) (ToolResult, error) {
			return ToolResult{Content: "x"}, nil
		},
	)
	srv := NewSDKMCPServer("test", "1.0.0", tool).(*inProcessMCPServer)
	listed, err := srv.listTools()
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}
	tools, _ := listed["tools"].([]map[string]any)
	if _, has := tools[0]["annotations"]; has {
		t.Errorf("Tool without annotations must omit the key: %v", tools[0])
	}
}

// TestMCPToolAnnotations_DistinctFromGomcp pins that the status-side type
// uses different field names than the registration-side gomcp.ToolAnnotations.
// Conflating the two is the bug class this test guards against.
func TestMCPToolAnnotations_DistinctFromGomcp(t *testing.T) {
	t.Parallel()
	in := MCPToolAnnotations{ReadOnly: true, Destructive: false, OpenWorld: true}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"readOnly":true`) {
		t.Errorf("status-side MCPToolAnnotations must emit readOnly (not readOnlyHint): %s", s)
	}
	if !strings.Contains(s, `"openWorld":true`) {
		t.Errorf("status-side MCPToolAnnotations must emit openWorld (not openWorldHint): %s", s)
	}
	if strings.Contains(s, "Hint") {
		t.Errorf("status-side MCPToolAnnotations must NOT use the gomcp Hint suffix: %s", s)
	}
}
