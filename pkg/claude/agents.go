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

// AgentDefinition defines a programmatic subagent for use with the claude CLI.
//
// Subagents are registered via [Options].Agents and delivered to the CLI via
// the streaming initialize request (NOT as CLI flags). Mirrors upstream
// AgentDefinition (types.py:83). Two field-name divergences from upstream are
// intentional: Go's [AgentDefinition.SystemPrompt] maps to upstream's
// `prompt`, and Go's [AgentDefinition.AllowedTools] maps to upstream's
// `tools` (matching the M1-era choice that aligns the agent surface with
// [Options]).
//
// Upstream's `mcpServers` field is `list[str | dict]`, mixing server names
// with inline {name: config} dicts. Go splits this into two fields:
// [AgentDefinition.MCPServers] []string for the name-only form (references
// a server already registered in [Options.MCPServers]), and
// [AgentDefinition.MCPServerConfigs] map[string]MCPServer for the
// inline-config form. Both fields may be set; their entries are merged
// on the wire under one `mcpServers` array.
type AgentDefinition struct {
	// Name is the unique identifier for this subagent.
	Name string `json:"name,omitzero"`

	// Description describes the subagent's role and capabilities.
	Description string `json:"description,omitzero"`

	// SystemPrompt is the system prompt for this subagent. Maps to upstream's
	// `prompt` field (types.py:87).
	SystemPrompt string `json:"systemPrompt,omitzero"`

	// AllowedTools is the list of tools the subagent is permitted to use.
	// Maps to upstream's `tools` field (types.py:89; passing "Skill" there is
	// deprecated — use [AgentDefinition.Skills] instead).
	AllowedTools []string `json:"allowedTools,omitzero"`

	// DisallowedTools is the list of tools forbidden to this subagent.
	// Mirrors upstream `disallowedTools` (types.py:90).
	DisallowedTools []string `json:"disallowedTools,omitzero"`

	// Model overrides the model for this subagent. Empty uses the parent
	// model. Accepts a model alias ("sonnet", "opus", "haiku", "inherit") or
	// a full model ID.
	Model string `json:"model,omitzero"`

	// Skills selects agent skills to enable for this subagent (mirrors
	// upstream `skills` at types.py:93). Each entry "name" expands at the CLI
	// to a Skill(name) tool; this replaces the deprecated practice of
	// passing "Skill" through AllowedTools.
	Skills []string `json:"skills,omitzero"`

	// MCPServers lists MCP server names this subagent may access. Entries
	// are server names that resolve against the parent [Options.MCPServers].
	// Mirrors upstream `mcpServers` (types.py:96) string-variant entries.
	// See [AgentDefinition.MCPServerConfigs] for the inline-config variant.
	//
	// Custom-marshaled (see [AgentDefinition.MarshalJSON]) — MCPServers and
	// MCPServerConfigs entries are merged into a single `mcpServers` array
	// on the wire matching upstream's list[str | dict].
	MCPServers []string `json:"-"`

	// MCPServerConfigs carries inline MCP server configurations keyed by
	// server name. Each value's configForCLI() output is emitted as a
	// {name: <config>} dict entry in the wire `mcpServers` array
	// (alongside any string-variant entries from [AgentDefinition.MCPServers]).
	// Iteration order is sorted by name so the wire payload is
	// deterministic.
	//
	// Use this for subagents that need an MCP server distinct from the
	// parent's [Options.MCPServers]; otherwise reference the parent's by
	// name through MCPServers.
	MCPServerConfigs map[string]MCPServer `json:"-"`

	// InitialPrompt is the prompt the CLI sends to the subagent on
	// activation. Mirrors upstream `initialPrompt` (types.py:97).
	InitialPrompt string `json:"initialPrompt,omitzero"`

	// MaxTurns limits the number of agentic turns for this subagent. Zero
	// means the CLI default. Mirrors upstream `maxTurns` (types.py:98).
	MaxTurns int `json:"maxTurns,omitzero"`

	// Background, when true, runs this subagent in the background. Mirrors
	// upstream `background` (types.py:99).
	Background bool `json:"background,omitzero"`

	// Memory selects the memory layer this subagent reads/writes. The zero
	// value (empty string) omits the wire key and uses the CLI default.
	// Mirrors upstream `memory` (types.py:94).
	Memory MemoryScope `json:"memory,omitzero"`

	// PermissionMode sets this subagent's permission mode independently of
	// the parent [Options.PermissionMode]. The zero value omits the wire
	// key and inherits from the parent. Mirrors upstream `permissionMode`
	// (types.py:101).
	PermissionMode PermissionMode `json:"permissionMode,omitzero"`

	// Effort controls how much effort this subagent puts into responses,
	// alongside adaptive thinking. The zero value omits the wire key.
	// Mirrors upstream `effort` (types.py:100).
	Effort EffortLevel `json:"effort,omitzero"`

	// Raw preserves unknown fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}
