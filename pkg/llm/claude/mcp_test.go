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
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ── NewSDKMCPServer / ToolDefinition accessors ────────────────────────────────

func TestNewSDKMCPServer_Accessors(t *testing.T) {
	t.Parallel()

	srv := NewSDKMCPServer("my-tools", "1.2.3")

	if got := srv.Name(); got != "my-tools" {
		t.Errorf("Name() = %q, want %q", got, "my-tools")
	}
	if got := srv.Version(); got != "1.2.3" {
		t.Errorf("Version() = %q, want %q", got, "1.2.3")
	}
	if got := srv.Mode(); got != MCPServerModeInProcess {
		t.Errorf("Mode() = %q, want %q", got, MCPServerModeInProcess)
	}
}

// ── ToolDefinition accessors ─────────────────────────────────────────────────

func TestToolDefinition_Accessors(t *testing.T) {
	t.Parallel()

	type in struct{ N int }
	def := Tool("add-one", "increments N", nil, func(_ context.Context, i in) (ToolResult, error) {
		return ToolResult{Content: "ok"}, nil
	})

	if def.Name() != "add-one" {
		t.Errorf("Name() = %q, want %q", def.Name(), "add-one")
	}
	if def.Description() != "increments N" {
		t.Errorf("Description() = %q, want %q", def.Description(), "increments N")
	}
	if def.Schema() != nil {
		t.Errorf("Schema() = %v, want nil", def.Schema())
	}
}

// ── Close ─────────────────────────────────────────────────────────────────────

func TestInProcessMCPServer_CloseNoop(t *testing.T) {
	t.Parallel()

	// The in-process server holds no resources; Close must be a no-op and
	// idempotent.
	srv := NewSDKMCPServer("test-srv", "0.1.0")
	if err := srv.Close(); err != nil {
		t.Errorf("Close() #1 error = %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Errorf("Close() #2 error = %v", err)
	}
}

// ── configForCLI ──────────────────────────────────────────────────────────────

func TestInProcessMCPServer_ConfigForCLI(t *testing.T) {
	t.Parallel()

	srv := NewSDKMCPServer("my-tools", "1.0.0").(*inProcessMCPServer)
	cfg := srv.configForCLI()
	if cfg["type"] != "sdk" {
		t.Errorf("configForCLI type = %v, want sdk", cfg["type"])
	}
	if cfg["name"] != "my-tools" {
		t.Errorf("configForCLI name = %v, want my-tools", cfg["name"])
	}
}

// ── --mcp-config launch arg ────────────────────────────────────────────────────

func TestBuildLaunchArgs_MCPConfig(t *testing.T) {
	t.Parallel()

	opts := &Options{
		MCPServers:      []MCPServer{NewSDKMCPServer("calc", "1.0.0")},
		StrictMCPConfig: true,
	}
	args := mustLaunchArgs(t, "/usr/local/bin/claude", opts, "")

	var cfg string
	var strict bool
	for i, a := range args {
		if a == "--mcp-config" && i+1 < len(args) {
			cfg = args[i+1]
		}
		if a == "--strict-mcp-config" {
			strict = true
		}
	}
	if cfg == "" {
		t.Fatalf("--mcp-config not emitted; args = %v", args)
	}
	if !strict {
		t.Errorf("--strict-mcp-config not emitted; args = %v", args)
	}

	var parsed struct {
		MCPServers map[string]struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(cfg), &parsed); err != nil {
		t.Fatalf("unmarshal --mcp-config %q: %v", cfg, err)
	}
	calc, ok := parsed.MCPServers["calc"]
	if !ok {
		t.Fatalf("mcpServers missing 'calc': %s", cfg)
	}
	if calc.Type != "sdk" || calc.Name != "calc" {
		t.Errorf("calc config = %+v, want {type:sdk name:calc}", calc)
	}
}

func TestBuildLaunchArgs_NoMCPServers(t *testing.T) {
	t.Parallel()

	args := mustLaunchArgs(t, "/usr/local/bin/claude", &Options{}, "")
	for _, a := range args {
		if a == "--mcp-config" {
			t.Fatalf("--mcp-config emitted with no MCPServers; args = %v", args)
		}
	}
}

// TestBuildLaunchArgs_StrictWithoutServers verifies that --strict-mcp-config is
// emitted independent of server presence (matching upstream) and that no
// --mcp-config accompanies it when there are no servers.
func TestBuildLaunchArgs_StrictWithoutServers(t *testing.T) {
	t.Parallel()

	args := mustLaunchArgs(t, "/usr/local/bin/claude", &Options{StrictMCPConfig: true}, "")
	var strict, hasConfig bool
	for _, a := range args {
		switch a {
		case "--strict-mcp-config":
			strict = true
		case "--mcp-config":
			hasConfig = true
		}
	}
	if !strict {
		t.Errorf("--strict-mcp-config not emitted; args = %v", args)
	}
	if hasConfig {
		t.Errorf("--mcp-config emitted with no MCPServers; args = %v", args)
	}
}

// ── mcp_message dispatch ────────────────────────────────────────────────────

// echoInput is the JSON-unmarshalable input for the echo tool used in tests.
type echoInput struct {
	Msg string `json:"msg"`
}

// newMCPProtocol builds a controlProtocol with the given in-process servers
// registered, exactly as ClaudeSDKClient.start would, plus a writeFn that
// captures responses on the returned channel.
func newMCPProtocol(t *testing.T, servers ...MCPServer) (*controlProtocol, <-chan []byte) {
	t.Helper()
	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{MCPServers: servers}, writeFn)
	cp.registerMCPServers()
	return cp, out
}

// mcpResponseFromControl drives a control_request line through route and
// returns the inner mcp_response JSONRPC object from the success control
// response.
func mcpResponseFromControl(t *testing.T, cp *controlProtocol, out <-chan []byte, line string) map[string]any {
	t.Helper()
	if _, err := cp.route(t.Context(), []byte(line)); err != nil {
		t.Fatalf("route error = %v", err)
	}
	subtype, _, errMsg, resp := awaitControlResponse(t, out)
	if subtype != "success" {
		t.Fatalf("control response subtype = %q (err=%q), want success", subtype, errMsg)
	}
	mr, ok := resp["mcp_response"].(map[string]any)
	if !ok {
		t.Fatalf("response missing mcp_response object: %v", resp)
	}
	return mr
}

func TestControlProtocol_MCPMessage_Initialize(t *testing.T) {
	t.Parallel()

	cp, out := newMCPProtocol(t, NewSDKMCPServer("test-srv", "2.3.4"))
	mr := mcpResponseFromControl(t, cp, out,
		`{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"test-srv","message":{"id":1,"method":"initialize"}}}`)

	if mr["id"] != float64(1) {
		t.Errorf("jsonrpc id = %v, want 1", mr["id"])
	}
	result, ok := mr["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize result missing: %v", mr)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v, want 2024-11-05", result["protocolVersion"])
	}
	info, ok := result["serverInfo"].(map[string]any)
	if !ok || info["name"] != "test-srv" || info["version"] != "2.3.4" {
		t.Errorf("serverInfo = %v, want {name:test-srv version:2.3.4}", result["serverInfo"])
	}
}

func TestControlProtocol_MCPMessage_ToolsList(t *testing.T) {
	t.Parallel()

	echoTool := Tool("echo", "echoes the msg field", nil, func(_ context.Context, i echoInput) (ToolResult, error) {
		return ToolResult{Content: i.Msg}, nil
	})
	cp, out := newMCPProtocol(t, NewSDKMCPServer("test-srv", "0.1.0", echoTool))
	mr := mcpResponseFromControl(t, cp, out,
		`{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"test-srv","message":{"id":2,"method":"tools/list"}}}`)

	result, ok := mr["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list result missing: %v", mr)
	}
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v, want 1 entry", result["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "echo" {
		t.Errorf("tool name = %v, want echo", tool["name"])
	}
	if tool["description"] != "echoes the msg field" {
		t.Errorf("tool description = %v", tool["description"])
	}
	if _, has := tool["inputSchema"]; !has {
		t.Error("tool inputSchema missing")
	}
}

func TestControlProtocol_MCPMessage_ToolsCall(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args        string
		wantContent string
		wantIsError bool
	}{
		"success: echo returns msg": {args: `{"msg":"hello"}`, wantContent: "hello"},
		"success: empty args":       {args: `{}`, wantContent: ""},
	}

	echoTool := Tool("echo", "echoes the msg field", nil, func(_ context.Context, i echoInput) (ToolResult, error) {
		return ToolResult{Content: i.Msg}, nil
	})

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cp, out := newMCPProtocol(t, NewSDKMCPServer("test-srv", "0.1.0", echoTool))
			line := `{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"test-srv","message":{"id":3,"method":"tools/call","params":{"name":"echo","arguments":` + tt.args + `}}}}`
			mr := mcpResponseFromControl(t, cp, out, line)

			result, ok := mr["result"].(map[string]any)
			if !ok {
				t.Fatalf("tools/call result missing: %v", mr)
			}
			content, ok := result["content"].([]any)
			if !ok || len(content) != 1 {
				t.Fatalf("content = %v, want 1 entry", result["content"])
			}
			item := content[0].(map[string]any)
			if item["type"] != "text" {
				t.Errorf("content type = %v, want text", item["type"])
			}
			if item["text"] != tt.wantContent {
				t.Errorf("content text = %v, want %q", item["text"], tt.wantContent)
			}
			if _, hasErr := result["isError"]; hasErr != tt.wantIsError {
				t.Errorf("isError present = %v, want %v", hasErr, tt.wantIsError)
			}
		})
	}
}

func TestControlProtocol_MCPMessage_ToolsCall_ToolError(t *testing.T) {
	t.Parallel()

	failTool := Tool("fail", "always fails", nil, func(_ context.Context, _ struct{}) (ToolResult, error) {
		return ToolResult{}, &CLIConnectionError{Message: "intentional tool error"}
	})
	cp, out := newMCPProtocol(t, NewSDKMCPServer("test-srv", "0.1.0", failTool))
	mr := mcpResponseFromControl(t, cp, out,
		`{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"test-srv","message":{"id":4,"method":"tools/call","params":{"name":"fail","arguments":{}}}}}`)

	// A tool returning an error is a successful JSONRPC result with isError set
	// (not a JSONRPC protocol error), matching the Tool constructor's contract.
	result, ok := mr["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/call result missing: %v", mr)
	}
	if result["isError"] != true {
		t.Errorf("isError = %v, want true", result["isError"])
	}
	content := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("content empty, want error text")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "intentional tool error") {
		t.Errorf("content text = %q, want to contain error message", text)
	}
}

func TestControlProtocol_MCPMessage_ToolsCall_UnknownTool(t *testing.T) {
	t.Parallel()

	cp, out := newMCPProtocol(t, NewSDKMCPServer("test-srv", "0.1.0"))
	mr := mcpResponseFromControl(t, cp, out,
		`{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"test-srv","message":{"id":5,"method":"tools/call","params":{"name":"nope","arguments":{}}}}}`)

	jerr, ok := mr["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected JSONRPC error for unknown tool: %v", mr)
	}
	if jerr["code"] != float64(-32603) {
		t.Errorf("error code = %v, want -32603", jerr["code"])
	}
}

func TestControlProtocol_MCPMessage_UnknownServer(t *testing.T) {
	t.Parallel()

	cp, out := newMCPProtocol(t, NewSDKMCPServer("test-srv", "0.1.0"))
	mr := mcpResponseFromControl(t, cp, out,
		`{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"ghost","message":{"id":6,"method":"tools/list"}}}`)

	jerr, ok := mr["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected JSONRPC error for unknown server: %v", mr)
	}
	if jerr["code"] != float64(-32601) {
		t.Errorf("error code = %v, want -32601", jerr["code"])
	}
	if msg, _ := jerr["message"].(string); !strings.Contains(msg, "ghost") {
		t.Errorf("error message = %q, want to mention server name", msg)
	}
}

func TestControlProtocol_MCPMessage_UnknownMethod(t *testing.T) {
	t.Parallel()

	cp, out := newMCPProtocol(t, NewSDKMCPServer("test-srv", "0.1.0"))
	mr := mcpResponseFromControl(t, cp, out,
		`{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"test-srv","message":{"id":7,"method":"resources/list"}}}`)

	jerr, ok := mr["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected JSONRPC error for unknown method: %v", mr)
	}
	if jerr["code"] != float64(-32601) {
		t.Errorf("error code = %v, want -32601", jerr["code"])
	}
}

func TestControlProtocol_MCPMessage_NotificationsInitialized(t *testing.T) {
	t.Parallel()

	cp, out := newMCPProtocol(t, NewSDKMCPServer("test-srv", "0.1.0"))
	mr := mcpResponseFromControl(t, cp, out,
		`{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"test-srv","message":{"method":"notifications/initialized"}}}`)

	// A notification carries no id, so the response must omit it (upstream sends
	// {"jsonrpc":"2.0","result":{}}).
	if _, has := mr["id"]; has {
		t.Errorf("notification response carried an id: %v", mr)
	}
	if _, ok := mr["result"].(map[string]any); !ok {
		t.Errorf("result = %v, want empty object", mr["result"])
	}
}

// TestControlProtocol_MCPMessage_StringID verifies that a string JSONRPC id is
// echoed verbatim (not coerced to a number).
func TestControlProtocol_MCPMessage_StringID(t *testing.T) {
	t.Parallel()

	cp, out := newMCPProtocol(t, NewSDKMCPServer("test-srv", "0.1.0"))
	mr := mcpResponseFromControl(t, cp, out,
		`{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"test-srv","message":{"id":"abc","method":"initialize"}}}`)

	if mr["id"] != "abc" {
		t.Errorf("jsonrpc id = %v, want \"abc\"", mr["id"])
	}
}

// TestControlProtocol_MCPMessage_Cancel verifies that an in-flight mcp_message
// handler blocked in a slow tool is cancelled by closeInflight and writes no
// response — the same cancel contract as hook_callback, exercised on the MCP
// path so a future special-case in handleMCPMessage cannot silently break it.
func TestControlProtocol_MCPMessage_Cancel(t *testing.T) {
	t.Parallel()

	wrote := make(chan struct{}, 1)
	writeFn := func(context.Context, []byte) error {
		select {
		case wrote <- struct{}{}:
		default:
		}
		return nil
	}

	started := make(chan struct{})
	done := make(chan struct{})
	blockTool := Tool("block", "blocks until cancelled", nil, func(ctx context.Context, _ struct{}) (ToolResult, error) {
		close(started)
		<-ctx.Done()
		close(done)
		return ToolResult{}, ctx.Err()
	})

	cp := newControlProtocol(&Options{MCPServers: []MCPServer{NewSDKMCPServer("test-srv", "0.1.0", blockTool)}}, writeFn)
	cp.registerMCPServers()

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"test-srv","message":{"id":1,"method":"tools/call","params":{"name":"block","arguments":{}}}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}

	<-started
	cp.closeInflight()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("tool did not unblock after closeInflight")
	}
	select {
	case <-wrote:
		t.Fatal("cancelled mcp_message handler wrote a response, want none")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestInProcessMCPServer_CallToolPreservesRawContentJSON(t *testing.T) {
	t.Parallel()

	tool := Tool("raw", "returns raw content", nil, func(_ context.Context, _ struct{}) (ToolResult, error) {
		return ToolResult{RawContent: []mcp.Content{
			&mcp.TextContent{Text: "hello"},
			&mcp.ImageContent{Data: []byte("png"), MIMEType: "image/png"},
		}}, nil
	})
	srv := NewSDKMCPServer("test", "1.0.0", tool).(*inProcessMCPServer)
	got, err := srv.callTool(t.Context(), "raw", jsontext.Value(`{}`))
	if err != nil {
		t.Fatalf("callTool() error = %v", err)
	}
	content, ok := got["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("content = %#v, want two raw JSON content entries", got["content"])
	}
	for i, item := range content {
		if _, ok := item.(jsontext.Value); !ok {
			t.Fatalf("content[%d] type = %T, want jsontext.Value", i, item)
		}
	}
	if string(content[0].(jsontext.Value)) != `{"type":"text","text":"hello"}` {
		t.Fatalf("content[0] = %s", content[0].(jsontext.Value))
	}
	if string(content[1].(jsontext.Value)) != `{"type":"image","mimeType":"image/png","data":"cG5n"}` {
		t.Fatalf("content[1] = %s", content[1].(jsontext.Value))
	}
}
