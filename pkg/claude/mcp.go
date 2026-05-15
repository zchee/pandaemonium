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
	"io"

	"github.com/google/jsonschema-go/jsonschema"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServerMode identifies how a [MCPServer] communicates with the CLI.
type MCPServerMode string

const (
	// MCPServerModeInProcess is an in-process goroutine bridged via two
	// io.Pipe pairs (one per direction). This is the mode produced by
	// [NewSDKMCPServer].
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
	fn          any               // original typed func stored for inspection
	mcpHandler  gomcp.ToolHandler // pre-built go-sdk handler adapter
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

	// Close shuts the server down and releases any associated resources.
	// Called deterministically by the owning [ClaudeSDKClient.Close].
	Close() error
}

// inProcessMCPServer is the in-process MCPServer implementation backed by
// github.com/modelcontextprotocol/go-sdk. The server is started via the
// unexported start method and bridges to the claude CLI subprocess through a
// pair of io.Pipe connections (one per direction).
type inProcessMCPServer struct {
	name    string
	version string
	tools   []ToolDefinition

	// cancel stops the serve goroutine; nil before start is called.
	cancel context.CancelCauseFunc

	// done is closed when the serve goroutine exits; nil before start.
	done chan struct{}
}

// NewSDKMCPServer creates an in-process [MCPServer] that exposes the given
// tools to the claude CLI via a goroutine bridge over two io.Pipe pairs.
//
// The server is backed by github.com/modelcontextprotocol/go-sdk (v1.6.0).
// It is connected to the CLI subprocess by [ClaudeSDKClient] at session start
// and closed deterministically when the client closes (AC-i4).
//
// Register the returned MCPServer via [Options].MCPServers:
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

// start builds the go-sdk MCP server, registers all tools, creates two io.Pipe
// pairs for bidirectional communication, and launches the serve goroutine.
//
// It returns the CLI-side pipe ends: cliR for the CLI to read server messages
// from, and cliW for the CLI to write client messages to. The goroutine owns
// the server-side ends and closes them when done.
//
// Calling start more than once on the same instance is not supported.
func (s *inProcessMCPServer) start(ctx context.Context) (cliR io.ReadCloser, cliW io.WriteCloser, err error) {
	srv := gomcp.NewServer(
		&gomcp.Implementation{Name: s.name, Version: s.version},
		nil,
	)

	for _, def := range s.tools {
		tool := &gomcp.Tool{
			Name:        def.name,
			Description: def.description,
		}
		if def.schema != nil {
			raw, merr := stdjson.Marshal(def.schema)
			if merr != nil {
				return nil, nil, &CLIConnectionError{
					Message: fmt.Sprintf("marshal input schema for MCP tool %q: %v", def.name, merr),
				}
			}
			tool.InputSchema = stdjson.RawMessage(raw)
		} else {
			tool.InputSchema = stdjson.RawMessage(`{"type":"object"}`)
		}
		if def.mcpHandler != nil {
			srv.AddTool(tool, def.mcpHandler)
		}
	}

	// Create two io.Pipe pairs for the bidirectional bridge:
	//   r1, w1: CLI writes to w1 → server reads from r1
	//   r2, w2: server writes to w2 → CLI reads from r2
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	transport := &gomcp.IOTransport{
		Reader: r1,
		Writer: w2,
	}

	serveCtx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer r1.Close()
		defer w2.Close()
		_ = srv.Run(serveCtx, transport)
	}()

	s.cancel = cancel
	s.done = done

	return r2, w1, nil
}

// Close stops the serve goroutine and waits for it to exit.
// Idempotent: subsequent calls return nil immediately.
func (s *inProcessMCPServer) Close() error {
	if s.cancel != nil {
		s.cancel(nil)
		if s.done != nil {
			<-s.done
		}
		s.cancel = nil
	}
	return nil
}
