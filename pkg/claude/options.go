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

// validate checks that o is a consistent, usable configuration. The zero value
// is always valid per AC-i1. Callers (NewClient, Query) invoke this before
// launching a subprocess so errors surface early without transport side effects.
func (o *Options) validate() error {
	if o == nil {
		return nil
	}
	if o.MaxTurns < 0 {
		return &CLIConnectionError{Message: "Options.MaxTurns must be >= 0"}
	}
	if o.MaxBudgetUSD < 0 {
		return &CLIConnectionError{Message: "Options.MaxBudgetUSD must be >= 0"}
	}
	return nil
}

// clone returns a copy of o whose reference-typed fields (slices and maps) are
// independent of the receiver's, so a caller can mutate the copy — including
// appending to its slices or writing to its Env map — without affecting the
// original. It is used by [ClaudeSDKClient.Fork] to give the child client its
// own configuration.
//
// Only the containers are copied, not their elements: [Message] values, hook
// functions, [MCPServer] instances, and the [SessionStore] are shared by
// reference because they are immutable or intentionally shared (a forked child
// branches from the same store). This is what prevents append-aliasing while
// keeping fork cheap. A nil receiver returns nil.
func (o *Options) clone() *Options {
	if o == nil {
		return nil
	}
	c := *o
	if o.AllowedTools != nil {
		c.AllowedTools = append([]string(nil), o.AllowedTools...)
	}
	if o.MCPServers != nil {
		c.MCPServers = append([]MCPServer(nil), o.MCPServers...)
	}
	if o.Hooks != nil {
		c.Hooks = append([]HookRegistration(nil), o.Hooks...)
	}
	if o.Agents != nil {
		c.Agents = append([]AgentDefinition(nil), o.Agents...)
	}
	if o.Plugins != nil {
		c.Plugins = append([]Plugin(nil), o.Plugins...)
	}
	if o.SettingSources != nil {
		c.SettingSources = append([]SettingSource(nil), o.SettingSources...)
	}
	if o.Env != nil {
		c.Env = make(map[string]string, len(o.Env))
		for k, v := range o.Env {
			c.Env[k] = v
		}
	}
	return &c
}

// Options configures a [Query] or [ClaudeSDKClient] session.
//
// The zero value is usable: it exercises stdio defaults and performs CLI
// discovery without any filesystem side effects beyond binary lookup (AC-i1).
// All fields are set once at construction time; modifying Options after passing
// it to NewClient or Query has no effect.
type Options struct {
	// SystemPrompt is the system prompt injected at the start of every session.
	// Corresponds to --system-prompt in the CLI.
	SystemPrompt string

	// AllowedTools is the list of tool names the CLI is permitted to invoke.
	// An empty slice allows the default tool set.
	// Corresponds to --allowedTools in the CLI.
	AllowedTools []string

	// MaxTurns limits the number of agentic turns per session.
	// Zero means the CLI default (no explicit limit passed).
	// Corresponds to --max-turns in the CLI.
	MaxTurns int

	// CLIPath is an explicit path to the claude binary. When non-empty it
	// bypasses exec.LookPath and the well-known install directories.
	// Corresponds to the first element of CLI discovery (AC6).
	CLIPath string

	// Cwd is the working directory for the claude CLI subprocess.
	// Empty uses the current process working directory.
	Cwd string

	// PermissionMode sets the CLI's permission mode (e.g. "default",
	// "acceptEdits", "bypassPermissions").
	// Corresponds to --permission-mode in the CLI.
	PermissionMode string

	// MCPServers is the list of MCP servers to register with the CLI session.
	// Create in-process servers with [NewSDKMCPServer]. Each server is encoded
	// into the --mcp-config flag at launch; in-process servers additionally
	// have their tool calls routed back over the control protocol.
	MCPServers []MCPServer

	// StrictMCPConfig restricts the CLI to only the MCP servers passed via
	// --mcp-config, ignoring any from filesystem settings.
	// Corresponds to --strict-mcp-config in the CLI.
	StrictMCPConfig bool

	// Hooks is the ordered list of hook registrations. The dispatcher invokes
	// matching hooks in registration order and stops at the first
	// [PermissionDeny] for tool-use events.
	Hooks []HookRegistration

	// CanUseTool is a permission callback invoked before every tool call.
	// It supplements Hooks and is invoked after the hook dispatcher. A nil
	// CanUseTool falls through to the CLI's configured permission_mode.
	CanUseTool CanUseTool

	// Agents is the list of programmatic subagent definitions passed to the
	// CLI at session start.
	Agents []AgentDefinition

	// Plugins is the list of claude CLI plugins to load at session start.
	Plugins []Plugin

	// SettingSources is the list of external settings sources for the CLI.
	SettingSources []SettingSource

	// SessionStore is the persistent store for conversation sessions.
	// A nil SessionStore disables session persistence.
	SessionStore SessionStore

	// Model overrides the default model. Empty uses the CLI's configured
	// default.
	// Corresponds to --model in the CLI.
	Model string

	// MaxBudgetUSD is the maximum spend budget in US dollars for this session.
	// Zero means no budget limit.
	// Corresponds to --max-budget in the CLI.
	MaxBudgetUSD float64

	// OutputFormat overrides the CLI output format (e.g. "json", "text",
	// "stream-json"). Empty uses the CLI default ("stream-json").
	// Corresponds to --output-format in the CLI.
	OutputFormat string

	// InputFormat overrides the CLI input format. Empty uses the CLI default.
	// Corresponds to --input-format in the CLI.
	InputFormat string

	// APIKeyHelper is the path to a helper binary that produces an API key on
	// stdout. Empty uses the ANTHROPIC_API_KEY environment variable.
	// Corresponds to --api-key-helper in the CLI.
	APIKeyHelper string

	// Env is a map of additional environment variables injected into the CLI
	// subprocess. These are merged with (and can override) the parent process
	// environment.
	Env map[string]string

	// Verbose enables verbose output from the CLI subprocess.
	// Corresponds to --verbose in the CLI.
	Verbose bool

	// IncludePartialMessages enables streaming of partial/incomplete messages
	// from the CLI subprocess.
	// Corresponds to --include-partial-messages in the CLI (if supported).
	IncludePartialMessages bool
}
