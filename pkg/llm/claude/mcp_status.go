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
	"github.com/go-json-experiment/json/jsontext"
)

// ─── MCP server status types ─────────────────────────────────────────────────
//
// Typed decoders for the JSON payload returned by [ClaudeSDKClient.GetMCPStatus].
// The raw [GetMCPStatus] accessor is preserved for forward compatibility; the
// typed [ClaudeSDKClient.GetMCPStatusTyped] layer below decodes it into these
// structs. Field names mirror upstream wire format (camelCase, since the CLI
// emits camelCase directly).
//
// Mirrors upstream's MCP server status TypedDicts (types.py:680-747).

// MCPServerConnectionStatus is one of the five connection states the CLI
// reports for each MCP server. Mirrors upstream McpServerConnectionStatus
// (types.py:707-710).
type MCPServerConnectionStatus string

const (
	// MCPServerConnectionStatusConnected indicates the server is healthy and
	// its tools are usable.
	MCPServerConnectionStatusConnected MCPServerConnectionStatus = "connected"

	// MCPServerConnectionStatusFailed indicates the server connect attempt
	// failed; the [MCPServerStatus.Error] field carries the cause.
	MCPServerConnectionStatusFailed MCPServerConnectionStatus = "failed"

	// MCPServerConnectionStatusNeedsAuth indicates the server requires
	// authentication that has not been provided.
	MCPServerConnectionStatusNeedsAuth MCPServerConnectionStatus = "needs-auth"

	// MCPServerConnectionStatusPending indicates the server is still in the
	// initial connection handshake.
	MCPServerConnectionStatusPending MCPServerConnectionStatus = "pending"

	// MCPServerConnectionStatusDisabled indicates the server is configured
	// but disabled by user/admin settings.
	MCPServerConnectionStatusDisabled MCPServerConnectionStatus = "disabled"
)

// MCPToolAnnotations are hints the CLI returns for each MCP-provided tool in
// the status response. Mirrors upstream McpToolAnnotations (types.py:680-688).
//
// This is the *status-side* annotation type (decoded from CLI output). It is
// distinct from the registration-side annotations a Go-side in-process server
// attaches via [ToolWithAnnotations] (which uses gomcp.ToolAnnotations with a
// different field set — DestructiveHint, ReadOnlyHint, OpenWorldHint,
// IdempotentHint). The two flow in opposite directions and carry different
// fields; conflating them would be a bug.
type MCPToolAnnotations struct {
	// ReadOnly hints that the tool does not modify state.
	ReadOnly bool `json:"readOnly,omitzero"`

	// Destructive hints that the tool may make destructive changes.
	Destructive bool `json:"destructive,omitzero"`

	// OpenWorld hints that the tool interacts with an open world (e.g.
	// network, filesystem) rather than a closed sandbox.
	OpenWorld bool `json:"openWorld,omitzero"`
}

// MCPToolInfo describes a single tool exposed by an MCP server, as reported
// in the status response. Mirrors upstream McpToolInfo (types.py:691).
type MCPToolInfo struct {
	// Name is the tool's name.
	Name string `json:"name,omitzero"`

	// Description is the tool's human-readable description; may be empty.
	Description string `json:"description,omitzero"`

	// Annotations are the CLI-reported hints about the tool. The zero value
	// (all bools false) is wire-equivalent to no annotations being reported.
	Annotations MCPToolAnnotations `json:"annotations,omitzero"`
}

// MCPServerInfo is the serverInfo block from the MCP initialize handshake,
// included in the status response when the server is connected. Mirrors
// upstream McpServerInfo (types.py:699).
type MCPServerInfo struct {
	// Name is the server's self-reported name.
	Name string `json:"name,omitzero"`

	// Version is the server's self-reported version.
	Version string `json:"version,omitzero"`
}

// MCPServerStatus is the per-server status entry inside [MCPStatusResponse].
// Mirrors upstream McpServerStatus (types.py:712).
//
// Config is kept as a raw [jsontext.Value] because upstream's
// McpServerStatusConfig is a union of five variants
// (stdio/sse/http/sdk-status/claudeai-proxy) discriminated by the "type"
// field; decoding into a typed union would expand the type surface
// significantly for a field most callers will pass through. Callers who need
// the concrete config can json.Unmarshal it themselves.
type MCPServerStatus struct {
	// Name is the server name as configured in [Options.MCPServers].
	Name string `json:"name,omitzero"`

	// Status is the current connection state.
	Status MCPServerConnectionStatus `json:"status,omitzero"`

	// ServerInfo is the server's self-reported info from the MCP handshake;
	// populated only when [MCPServerStatus.Status] is
	// [MCPServerConnectionStatusConnected].
	ServerInfo MCPServerInfo `json:"serverInfo,omitzero"`

	// Error is the failure message; populated only when
	// [MCPServerStatus.Status] is [MCPServerConnectionStatusFailed].
	Error string `json:"error,omitzero"`

	// Config is the wire-format server configuration as the CLI sees it. The
	// shape varies by server type (stdio/sse/http/sdk/claudeai-proxy);
	// callers decode it themselves if they need the typed view.
	Config jsontext.Value `json:"config,omitzero"`

	// Scope is the configuration source (e.g. "project", "user", "local",
	// "claudeai", "managed"). May be empty.
	Scope string `json:"scope,omitzero"`

	// Tools is the list of tools the server provides; populated only when
	// the server is connected.
	Tools []MCPToolInfo `json:"tools,omitzero"`
}

// MCPStatusResponse is the typed shape of the response from
// [ClaudeSDKClient.GetMCPStatusTyped]. Mirrors upstream McpStatusResponse
// (types.py:740).
type MCPStatusResponse struct {
	// MCPServers is the list of per-server status entries.
	MCPServers []MCPServerStatus `json:"mcpServers,omitzero"`
}
