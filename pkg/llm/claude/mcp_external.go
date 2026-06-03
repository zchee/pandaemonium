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

// MCPServerModeHTTP is an external MCP server reached over plain HTTP.
// Mirrors upstream McpHttpServerConfig (types.py:619). Declared here because
// the other modes (in_process, stdio, sse) predate this commit.
const MCPServerModeHTTP MCPServerMode = "http"

// ─── External MCP server variants ─────────────────────────────────────────────
//
// The three variants below mirror upstream's MCP TypedDicts
// (McpStdioServerConfig / McpSSEServerConfig / McpHttpServerConfig, types.py:
// 602-625). They satisfy [MCPServer], so they can be mixed with in-process
// servers from [NewSDKMCPServer] in the same [Options.MCPServers] slice.
//
// Unlike in-process servers, these variants hold no OS resources at the SDK
// layer: the claude CLI subprocess owns the actual stdio process or HTTP
// connection. The SDK only ships the wire-config map under --mcp-config at
// launch. Consequently, [MCPServer.Version] returns "" and [MCPServer.Close]
// is a no-op for all three.
//
// Use struct literals to construct them — the types ARE the constructor, the
// same way upstream's TypedDicts are:
//
//	opts := &claude.Options{
//	    MCPServers: []claude.MCPServer{
//	        &claude.MCPStdioServerConfig{Name: "fs", Command: "mcp-fs", Args: []string{"--root", "/srv"}},
//	        &claude.MCPSSEServerConfig{Name: "events", URL: "https://example.com/sse"},
//	        &claude.MCPHTTPServerConfig{Name: "tools", URL: "https://api.example.com/mcp", Headers: map[string]string{"Authorization": "Bearer ..."}},
//	        claude.NewSDKMCPServer("my-tools", "1.0.0", greetTool),
//	    },
//	}

// MCPStdioServerConfig configures an external MCP server the CLI launches as
// a subprocess over stdio. Mirrors upstream McpStdioServerConfig
// (types.py:602). The wire "type" field is optional upstream for backwards
// compatibility; this SDK emits "stdio" explicitly so the CLI never has to
// disambiguate by absence.
type MCPStdioServerConfig struct {
	// MCPName is the server's registered name. Required.
	MCPName string

	// Command is the executable to launch. Required.
	Command string

	// Args are the command-line arguments passed to Command.
	Args []string

	// Env is an environment-variable map merged into the subprocess
	// environment. Nil leaves the parent environment intact.
	Env map[string]string
}

// Name returns the server's registered name.
func (c *MCPStdioServerConfig) Name() string { return c.MCPName }

// Version returns the empty string: stdio servers have no SDK-side version
// metadata, only in-process servers carry one.
func (c *MCPStdioServerConfig) Version() string { return "" }

// Mode returns [MCPServerModeStdio].
func (c *MCPStdioServerConfig) Mode() MCPServerMode { return MCPServerModeStdio }

// configForCLI emits {type: "stdio", command, args?, env?} matching upstream
// McpStdioServerConfig (types.py:605-609). Omits args/env when nil/empty so
// the wire payload stays minimal.
func (c *MCPStdioServerConfig) configForCLI() map[string]any {
	out := map[string]any{"type": "stdio", "command": c.Command}
	if len(c.Args) > 0 {
		out["args"] = c.Args
	}
	if len(c.Env) > 0 {
		out["env"] = c.Env
	}
	return out
}

// Close is a no-op: the claude CLI owns the subprocess lifecycle, the Go
// config struct holds no OS resources.
func (c *MCPStdioServerConfig) Close() error { return nil }

// MCPSSEServerConfig configures an external MCP server reached over
// Server-Sent Events. Mirrors upstream McpSSEServerConfig (types.py:611).
type MCPSSEServerConfig struct {
	// MCPName is the server's registered name. Required.
	MCPName string

	// URL is the SSE endpoint URL. Required.
	URL string

	// Headers are extra HTTP headers sent on the SSE request.
	Headers map[string]string
}

// Name returns the server's registered name.
func (c *MCPSSEServerConfig) Name() string { return c.MCPName }

// Version returns the empty string: SSE servers have no SDK-side version
// metadata, only in-process servers carry one.
func (c *MCPSSEServerConfig) Version() string { return "" }

// Mode returns [MCPServerModeSSE].
func (c *MCPSSEServerConfig) Mode() MCPServerMode { return MCPServerModeSSE }

// configForCLI emits {type: "sse", url, headers?} matching upstream
// McpSSEServerConfig (types.py:614-617).
func (c *MCPSSEServerConfig) configForCLI() map[string]any {
	out := map[string]any{"type": "sse", "url": c.URL}
	if len(c.Headers) > 0 {
		out["headers"] = c.Headers
	}
	return out
}

// Close is a no-op: the claude CLI owns the SSE connection lifecycle.
func (c *MCPSSEServerConfig) Close() error { return nil }

// MCPHTTPServerConfig configures an external MCP server reached over plain
// HTTP. Mirrors upstream McpHttpServerConfig (types.py:619); the Go SDK
// keeps HTTP uppercase to match the existing MCPServerMode initialism style
// (the upstream Python name is McpHttpServerConfig).
type MCPHTTPServerConfig struct {
	// MCPName is the server's registered name. Required.
	MCPName string

	// URL is the HTTP endpoint URL. Required.
	URL string

	// Headers are extra HTTP headers sent on each request.
	Headers map[string]string
}

// Name returns the server's registered name.
func (c *MCPHTTPServerConfig) Name() string { return c.MCPName }

// Version returns the empty string: HTTP servers have no SDK-side version
// metadata, only in-process servers carry one.
func (c *MCPHTTPServerConfig) Version() string { return "" }

// Mode returns [MCPServerModeHTTP].
func (c *MCPHTTPServerConfig) Mode() MCPServerMode { return MCPServerModeHTTP }

// configForCLI emits {type: "http", url, headers?} matching upstream
// McpHttpServerConfig (types.py:622-625).
func (c *MCPHTTPServerConfig) configForCLI() map[string]any {
	out := map[string]any{"type": "http", "url": c.URL}
	if len(c.Headers) > 0 {
		out["headers"] = c.Headers
	}
	return out
}

// Close is a no-op: the claude CLI owns the HTTP connection lifecycle.
func (c *MCPHTTPServerConfig) Close() error { return nil }
