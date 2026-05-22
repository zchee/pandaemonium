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
	"maps"

	"github.com/google/jsonschema-go/jsonschema"
)

// skillsAll is the sentinel [Options].Skills value (returned by [AllSkills])
// that enables every installed skill, mirroring upstream skills == "all".
const skillsAll = "all"

// AllSkills returns the [Options].Skills value that enables every installed
// skill (injecting the bare "Skill" tool), mirroring upstream skills="all".
// Use it instead of a named-skill list:
//
//	Options{Skills: AllSkills()}
func AllSkills() []string { return []string{skillsAll} }

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
	if o.Tools != nil {
		c.Tools = append([]string(nil), o.Tools...)
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
	if o.DisallowedTools != nil {
		c.DisallowedTools = append([]string(nil), o.DisallowedTools...)
	}
	if o.Betas != nil {
		c.Betas = append([]string(nil), o.Betas...)
	}
	if o.AddDirs != nil {
		c.AddDirs = append([]string(nil), o.AddDirs...)
	}
	if o.Skills != nil {
		c.Skills = append([]string(nil), o.Skills...)
	}
	if o.Env != nil {
		c.Env = make(map[string]string, len(o.Env))
		maps.Copy(c.Env, o.Env)
	}
	if o.ExtraArgs != nil {
		c.ExtraArgs = make(map[string]*string, len(o.ExtraArgs))
		maps.Copy(c.ExtraArgs, o.ExtraArgs)
	}
	if o.TaskBudget != nil {
		tb := *o.TaskBudget
		c.TaskBudget = &tb
	}
	if o.Sandbox != nil {
		sb := *o.Sandbox
		if o.Sandbox.ExcludedCommands != nil {
			sb.ExcludedCommands = append([]string(nil), o.Sandbox.ExcludedCommands...)
		}
		if o.Sandbox.Network.AllowedDomains != nil {
			sb.Network.AllowedDomains = append([]string(nil), o.Sandbox.Network.AllowedDomains...)
		}
		if o.Sandbox.Network.DeniedDomains != nil {
			sb.Network.DeniedDomains = append([]string(nil), o.Sandbox.Network.DeniedDomains...)
		}
		if o.Sandbox.Network.AllowUnixSockets != nil {
			sb.Network.AllowUnixSockets = append([]string(nil), o.Sandbox.Network.AllowUnixSockets...)
		}
		if o.Sandbox.Network.AllowMachLookup != nil {
			sb.Network.AllowMachLookup = append([]string(nil), o.Sandbox.Network.AllowMachLookup...)
		}
		if o.Sandbox.IgnoreViolations.File != nil {
			sb.IgnoreViolations.File = append([]string(nil), o.Sandbox.IgnoreViolations.File...)
		}
		if o.Sandbox.IgnoreViolations.Network != nil {
			sb.IgnoreViolations.Network = append([]string(nil), o.Sandbox.IgnoreViolations.Network...)
		}
		c.Sandbox = &sb
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
	// SystemPrompt configures the system prompt injected at the start of every
	// session. It is a sum type: nil emits --system-prompt ""; a
	// [SystemPromptText] emits --system-prompt <text>; a [SystemPromptFile]
	// emits --system-prompt-file <path>; a [SystemPromptPreset] emits
	// --append-system-prompt <append>. See [SystemPromptSource].
	SystemPrompt SystemPromptSource

	// AllowedTools is the list of tool names the CLI is permitted to invoke.
	// An empty slice allows the default tool set.
	// Corresponds to --allowedTools in the CLI.
	AllowedTools []string

	// Tools is the base set of tools the CLI loads, DISTINCT from AllowedTools
	// (which gates permission). It mirrors upstream's separate `tools` option
	// (subprocess_cli.py:241-247): a nil slice omits the flag entirely; a
	// non-nil empty slice emits --tools "" (an explicit empty set); a non-empty
	// slice emits --tools a,b,c. ToolsPreset takes precedence over Tools.
	Tools []string

	// ToolsPreset selects a named base-tool preset (e.g. "default", upstream's
	// 'claude_code' preset → "default"). When non-empty it emits --tools
	// <preset> and Tools is ignored (subprocess_cli.py:248-250).
	ToolsPreset string

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

	// PermissionMode sets the CLI's permission mode. Use one of the
	// [PermissionMode] constants (e.g. [PermissionModeAcceptEdits],
	// [PermissionModePlan], [PermissionModeBypassPermissions]); the zero
	// value lets the CLI pick its configured default and emits no flag.
	// Corresponds to --permission-mode in the CLI.
	PermissionMode PermissionMode

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

	// SessionStore is a Go-side message-history store consumed only by
	// [ClaudeSDKClient.Fork] to snapshot the parent session's history into a
	// new branch. It is NOT wired to the CLI: no `--session-store` flag, no
	// initialize-payload field, no control-protocol traffic. CLI-side session
	// management uses [Options.SessionID] and [Options.Resume] (which drive
	// `--session-id` and `--resume`) and is independent of this store.
	//
	// A nil SessionStore disables Fork but does not affect any other client
	// behavior. See [SessionStore]'s godoc for the architectural rationale.
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

	// JSONSchema requests structured output constrained to this schema. When
	// non-nil it emits --json-schema <marshaled schema>, mirroring upstream's
	// output_format={"type":"json_schema","schema":{...}} union member
	// (subprocess_cli.py:395-404). It is independent of OutputFormat, which
	// continues to carry the plain string form. The schema is treated as
	// immutable after construction (clone shares the pointer).
	JSONSchema *jsonschema.Schema

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

	// DisallowedTools is the list of tool names the CLI is forbidden from
	// invoking. Corresponds to --disallowedTools (comma-joined).
	DisallowedTools []string

	// ContinueConversation resumes the most recent conversation in Cwd.
	// Corresponds to --continue.
	ContinueConversation bool

	// Resume is the session ID to resume. Corresponds to --resume. When this
	// client was produced by [ClaudeSDKClient.Fork], the forked session ID
	// takes precedence over this field.
	Resume string

	// SessionID sets an explicit session ID for a new session.
	// Corresponds to --session-id.
	SessionID string

	// ForkSession, when resuming, forks into a new session ID instead of
	// continuing the resumed one. Corresponds to --fork-session.
	ForkSession bool

	// FallbackModel is the model to fall back to if the primary Model is
	// unavailable. Corresponds to --fallback-model.
	FallbackModel string

	// Betas is the list of beta feature flags to enable.
	// Corresponds to --betas (comma-joined).
	Betas []string

	// PermissionPromptToolName is the name of the MCP tool the CLI calls to
	// prompt for tool permissions. Corresponds to --permission-prompt-tool.
	PermissionPromptToolName string

	// Settings is a settings JSON string or a path to a settings file. When
	// [Options.Sandbox] is also set, Settings is parsed (or read from disk
	// when it is a path) and [Options.Sandbox] is merged into the resulting
	// JSON object under the "sandbox" key before being passed to the CLI as
	// --settings. With no Sandbox, the value is passed through verbatim.
	Settings string

	// Sandbox configures bash-command sandboxing. When set it is merged into
	// --settings as the "sandbox" key (see [Options.Settings]); a nil pointer
	// leaves the CLI to its configured default. Mirrors upstream
	// _build_settings_value (subprocess_cli.py:129-181).
	Sandbox *SandboxSettings

	// TaskBudget caps the model's tool-use task in tokens. A nil pointer
	// omits the --task-budget flag; an explicit &TaskBudget{Total: 0} is
	// forwarded as --task-budget 0 (parity with upstream's `is not None` gate
	// at subprocess_cli.py:268). Mirrors upstream task_budget.
	TaskBudget *TaskBudget

	// AddDirs is the list of additional directories the CLI may access.
	// Corresponds to one --add-dir flag per entry.
	AddDirs []string

	// IncludeHookEvents enables streaming of hook lifecycle events as messages.
	// Corresponds to --include-hook-events.
	IncludeHookEvents bool

	// ExtraArgs passes arbitrary additional CLI flags. A nil value emits a
	// bare boolean flag (--key); a non-nil value emits --key <value>. Use
	// [ExtraFlag] to build a value pointer. Mirrors upstream extra_args
	// (dict[str, str | None]).
	ExtraArgs map[string]*string

	// Thinking configures extended thinking. A nil value leaves the CLI
	// default in effect; use [ThinkingConfigAdaptive], [ThinkingConfigEnabled],
	// or [ThinkingConfigDisabled]. When set, Thinking takes precedence over
	// [Options.MaxThinkingTokens] (subprocess_cli.py:372-387).
	Thinking ThinkingConfig

	// MaxThinkingTokens caps the model's thinking budget. Deprecated in favor
	// of [Options.Thinking] (set [ThinkingConfigEnabled] with a budget
	// instead); the field is honored only when [Options.Thinking] is nil. Zero
	// means the CLI default. Corresponds to --max-thinking-tokens.
	MaxThinkingTokens int

	// Effort controls how much effort Claude puts into its response, working
	// alongside adaptive thinking. The zero value (empty string) emits no
	// --effort flag. Corresponds to --effort in the CLI.
	Effort EffortLevel

	// Skills selects agent skills to enable. The sentinel value returned by
	// [AllSkills] enables every installed skill (injecting the bare "Skill"
	// tool); otherwise each entry "name" injects a "Skill(name)" tool into
	// AllowedTools. When Skills is non-empty and SettingSources is unset, the
	// CLI's settings sources default to user and project so installed skills
	// are discovered. A nil/empty Skills is a no-op. Mirrors upstream
	// skills (Literal["all"] | list[str]).
	Skills []string
}
