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

	"github.com/google/jsonschema-go/jsonschema"
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
	fn          any // func(context.Context, I) (ToolResult, error) stored as any
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
	return ToolDefinition{
		name:        name,
		description: description,
		schema:      schema,
		fn:          fn,
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
// github.com/modelcontextprotocol/go-sdk. Implementation is filled in Phase E.
type inProcessMCPServer struct {
	name    string
	version string
	tools   []ToolDefinition
}

// NewSDKMCPServer creates an in-process [MCPServer] that exposes the given
// tools to the claude CLI via a goroutine bridge over two io.Pipe pairs.
//
// The server is backed by github.com/modelcontextprotocol/go-sdk (version
// pinned in internal/version/version.go). It is connected to the CLI
// subprocess by [ClaudeSDKClient] at session start and closed deterministically
// when the client closes (AC-i4).
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
func (s *inProcessMCPServer) Close() error        { return nil } // Phase E fills this
