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
// Subagents are registered via [Options].Agents and round-trip into CLI launch
// arguments. The exact CLI flag mapping is implemented in Phase G.
type AgentDefinition struct {
	// Name is the unique identifier for this subagent.
	Name string `json:"name,omitzero"`

	// Description describes the subagent's role and capabilities.
	Description string `json:"description,omitzero"`

	// SystemPrompt is the system prompt for this subagent.
	SystemPrompt string `json:"systemPrompt,omitzero"`

	// AllowedTools is the list of tools the subagent is permitted to use.
	AllowedTools []string `json:"allowedTools,omitzero"`

	// Model overrides the model for this subagent. Empty uses the parent model.
	Model string `json:"model,omitzero"`

	// Raw preserves unknown fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}
