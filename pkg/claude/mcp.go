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
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServerMode identifies how a [MCPServer] communicates with the CLI.
type MCPServerMode string

const (
	// MCPServerModeInProcess is an in-process server whose tools are invoked
	// directly from the control-protocol mcp_message handler (no external
	// transport). This is the mode produced by [NewSDKMCPServer].
	MCPServerModeInProcess MCPServerMode = "in_process"

	// MCPServerModeStdio is an external MCP server launched as a subprocess
	// over stdin/stdout.
	MCPServerModeStdio MCPServerMode = "stdio"

	// MCPServerModeSSE is an external MCP server reached over Server-Sent
	// Events (HTTP).
	MCPServerModeSSE MCPServerMode = "sse"
)

// ToolResult is the return value of a [Tool] function invoked by the CLI.
type ToolResult struct {
	// Content is the text content of the tool result.
	Content string

	// IsError indicates that the tool invocation failed.
	IsError bool
}

// ToolDefinition holds the metadata and handler function for a single MCP tool.
// Create instances with the [Tool] generic constructor.
type ToolDefinition struct {
	name        string
	description string
	schema      *jsonschema.Schema
	annotations *gomcp.ToolAnnotations // registration-side hints; surfaced through tools/list
	fn          any                    // original typed func stored for inspection
	mcpHandler  gomcp.ToolHandler      // pre-built go-sdk handler adapter
}

// Name returns the tool's registered name.
func (d ToolDefinition) Name() string { return d.name }

// Description returns the tool's human-readable description.
func (d ToolDefinition) Description() string { return d.description }

// Schema returns the JSON schema describing the tool's input type.
func (d ToolDefinition) Schema() *jsonschema.Schema { return d.schema }

// Tool constructs a [ToolDefinition] from a typed handler function.
//
// The type parameter I must be JSON-unmarshalable from the tool input payload
// delivered by the CLI. The schema parameter should describe the JSON schema
// for I; pass nil to omit schema validation (not recommended for production).
//
//	greetTool := claude.Tool("greet", "Greets the named person",
//	    mySchema,
//	    func(ctx context.Context, in GreetInput) (claude.ToolResult, error) {
//	        return claude.ToolResult{Content: "Hello, " + in.Name}, nil
//	    },
//	)
func Tool[I any](name, description string, schema *jsonschema.Schema, fn func(context.Context, I) (ToolResult, error)) ToolDefinition {
	return ToolWithAnnotations(name, description, schema, nil, fn)
}

// ToolWithAnnotations is like [Tool] but also attaches MCP tool annotations
// (registration-side hints such as readOnlyHint / destructiveHint /
// openWorldHint / idempotentHint, defined by gomcp.ToolAnnotations). The
// annotations are surfaced through this server's tools/list response so the
// CLI — and downstream callers reading [MCPServerStatus.Tools] — see them.
//
// Note the distinction between two annotation types in this package:
//   - [gomcp.ToolAnnotations] is the registration-side type used here; it
//     uses the official MCP wire-format field names (destructiveHint,
//     openWorldHint, etc.).
//   - [MCPToolAnnotations] is the status-side type decoded from the CLI's
//     mcp_status response; it uses upstream's status-shape names (readOnly,
//     destructive, openWorld).
//
// The two flow in opposite directions and carry different fields; do not
// conflate them.
//
//	greetTool := claude.ToolWithAnnotations("greet", "Greets the named person",
//	    mySchema,
//	    &gomcp.ToolAnnotations{ReadOnlyHint: true},
//	    func(ctx context.Context, in GreetInput) (claude.ToolResult, error) {
//	        return claude.ToolResult{Content: "Hello, " + in.Name}, nil
//	    },
//	)
func ToolWithAnnotations[I any](name, description string, schema *jsonschema.Schema, annotations *gomcp.ToolAnnotations, fn func(context.Context, I) (ToolResult, error)) ToolDefinition {
	// Pre-build the go-sdk ToolHandler that unmarshals arguments into I.
	handler := gomcp.ToolHandler(func(ctx context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		var input I
		if len(req.Params.Arguments) > 0 {
			if err := stdjson.Unmarshal(req.Params.Arguments, &input); err != nil {
				return &gomcp.CallToolResult{
					IsError: true,
					Content: []gomcp.Content{&gomcp.TextContent{Text: "invalid arguments: " + err.Error()}},
				}, nil
			}
		}
		result, err := fn(ctx, input)
		if err != nil {
			return &gomcp.CallToolResult{
				IsError: true,
				Content: []gomcp.Content{&gomcp.TextContent{Text: err.Error()}},
			}, nil
		}
		return &gomcp.CallToolResult{
			IsError: result.IsError,
			Content: []gomcp.Content{&gomcp.TextContent{Text: result.Content}},
		}, nil
	})
	return ToolDefinition{
		name:        name,
		description: description,
		schema:      schema,
		annotations: annotations,
		fn:          fn,
		mcpHandler:  handler,
	}
}

// MCPServer is the interface implemented by all MCP server variants registered
// via [Options].MCPServers.
type MCPServer interface {
	// Name returns the server's registered name.
	Name() string

	// Version returns the server's version string.
	Version() string

	// Mode returns how the server communicates with the CLI.
	Mode() MCPServerMode

	// configForCLI returns the per-server object that goes under the
	// "mcpServers" key of the --mcp-config flag. In-process servers return
	// {"type":"sdk","name":<name>} so the CLI knows to route their tool calls
	// back over the control protocol; external servers return their stdio/sse
	// configuration. Mirrors upstream subprocess_cli.py's servers_for_cli, which
	// strips the in-process "instance" and passes the rest through.
	configForCLI() map[string]any

	// Close shuts the server down and releases any associated resources.
	// Called deterministically by the owning [ClaudeSDKClient.Close].
	Close() error
}

// inProcessMCPServer is the in-process MCPServer implementation. Its tools are
// invoked directly from the control-protocol mcp_message handler: the CLI sends
// JSONRPC requests (initialize / tools/list / tools/call) as control requests,
// and the handler routes them against this server's tools without any external
// transport. This mirrors upstream Query._handle_sdk_mcp_request, which routes
// JSONRPC methods manually rather than running a streaming MCP server.
type inProcessMCPServer struct {
	name    string
	version string
	tools   []ToolDefinition
}

// NewSDKMCPServer creates an in-process [MCPServer] that exposes the given
// tools to the claude CLI over the control protocol.
//
// Register the returned MCPServer via [Options].MCPServers; the owning
// [ClaudeSDKClient] advertises it to the CLI via --mcp-config at launch and
// routes the CLI's tool calls back to it through the control protocol. The
// server holds no OS resources, so Close is a no-op kept for interface
// symmetry.
//
//	opts := &claude.Options{
//	    MCPServers: []claude.MCPServer{
//	        claude.NewSDKMCPServer("my-tools", "1.0.0", myTool),
//	    },
//	}
func NewSDKMCPServer(name, version string, tools ...ToolDefinition) MCPServer {
	return &inProcessMCPServer{
		name:    name,
		version: version,
		tools:   tools,
	}
}

func (s *inProcessMCPServer) Name() string        { return s.name }
func (s *inProcessMCPServer) Version() string     { return s.version }
func (s *inProcessMCPServer) Mode() MCPServerMode { return MCPServerModeInProcess }

// configForCLI returns {"type":"sdk","name":<name>}: the wire shape an SDK
// (in-process) server takes under --mcp-config's mcpServers map, matching
// upstream's sdk_config (everything except the in-process "instance").
func (s *inProcessMCPServer) configForCLI() map[string]any {
	return map[string]any{"type": "sdk", "name": s.name}
}

// Close releases the server's resources. The in-process server holds none, so
// this is a no-op; it exists to satisfy the [MCPServer] interface and is called
// deterministically by [ClaudeSDKClient.Close].
func (s *inProcessMCPServer) Close() error { return nil }

// serverInfo returns the MCP serverInfo object for an initialize response,
// defaulting an empty version to "1.0.0" as upstream does.
func (s *inProcessMCPServer) serverInfo() map[string]any {
	version := s.version
	if version == "" {
		version = "1.0.0"
	}
	return map[string]any{"name": s.name, "version": version}
}

// listTools returns the tools/list JSONRPC result object for this server: a
// {"tools":[...]} map whose entries carry name, description, and inputSchema
// (defaulting to {"type":"object"} when a tool declares no schema), matching
// upstream's tools_data construction.
func (s *inProcessMCPServer) listTools() (map[string]any, error) {
	tools := make([]map[string]any, 0, len(s.tools))
	for _, def := range s.tools {
		var schema any = map[string]any{"type": "object"}
		if def.schema != nil {
			raw, err := stdjson.Marshal(def.schema)
			if err != nil {
				return nil, fmt.Errorf("marshal input schema for MCP tool %q: %w", def.name, err)
			}
			schema = stdjson.RawMessage(raw)
		}
		entry := map[string]any{
			"name":        def.name,
			"description": def.description,
			"inputSchema": schema,
		}
		if def.annotations != nil {
			entry["annotations"] = def.annotations
		}
		tools = append(tools, entry)
	}
	return map[string]any{"tools": tools}, nil
}

// callTool invokes the named tool with the given raw JSON arguments and returns
// the tools/call JSONRPC result object {"content":[...], "isError"?:true}.
//
// Tool results are converted following upstream: text content passes through;
// the in-process Tool constructor only ever emits TextContent, so other content
// types are not produced here (the conversion loop is shaped to match upstream
// so future image/resource support slots in). A missing tool is a JSONRPC
// method-not-found style error returned to the caller as a Go error so the
// handler can map it to the -32601/-32603 envelope.
func (s *inProcessMCPServer) callTool(ctx context.Context, name string, arguments stdjson.RawMessage) (map[string]any, error) {
	var def *ToolDefinition
	for i := range s.tools {
		if s.tools[i].name == name {
			def = &s.tools[i]
			break
		}
	}
	if def == nil || def.mcpHandler == nil {
		return nil, fmt.Errorf("tool %q not found", name)
	}

	req := &gomcp.CallToolRequest{
		Params: &gomcp.CallToolParamsRaw{
			Name:      name,
			Arguments: arguments,
		},
	}
	result, err := def.mcpHandler(ctx, req)
	if err != nil {
		return nil, err
	}

	content := make([]map[string]any, 0, len(result.Content))
	for _, item := range result.Content {
		if tc, ok := item.(*gomcp.TextContent); ok {
			content = append(content, map[string]any{"type": "text", "text": tc.Text})
		}
		// Non-text content is not emitted by the in-process Tool constructor;
		// match upstream by silently skipping unsupported content types.
	}

	out := map[string]any{"content": content}
	if result.IsError {
		out["isError"] = true
	}
	return out, nil
}
