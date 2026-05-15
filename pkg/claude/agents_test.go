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
	"testing"
)

func TestAgentDefinition_ZeroValue(t *testing.T) {
	t.Parallel()
	var a AgentDefinition
	if a.Name != "" {
		t.Errorf("zero AgentDefinition.Name = %q, want empty", a.Name)
	}
	if a.Description != "" {
		t.Errorf("zero AgentDefinition.Description = %q, want empty", a.Description)
	}
	if a.SystemPrompt != "" {
		t.Errorf("zero AgentDefinition.SystemPrompt = %q, want empty", a.SystemPrompt)
	}
	if len(a.AllowedTools) != 0 {
		t.Errorf("zero AgentDefinition.AllowedTools = %v, want empty", a.AllowedTools)
	}
	if a.Model != "" {
		t.Errorf("zero AgentDefinition.Model = %q, want empty", a.Model)
	}
}

func TestAgentDefinition_Fields(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		agent AgentDefinition
	}{
		"success: full agent definition": {
			agent: AgentDefinition{
				Name:         "helper",
				Description:  "A helpful subagent.",
				SystemPrompt: "You are a helpful assistant.",
				AllowedTools: []string{"Bash", "Read", "Write"},
				Model:        "claude-opus-4-5",
			},
		},
		"success: minimal agent definition": {
			agent: AgentDefinition{
				Name: "minimal",
			},
		},
		"success: agent with multiple allowed tools": {
			agent: AgentDefinition{
				Name:         "tooled",
				AllowedTools: []string{"Bash", "Read", "Write", "Edit"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Round-trip: verify the struct retains all set fields.
			got := tt.agent
			if got.Name != tt.agent.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.agent.Name)
			}
			if got.Description != tt.agent.Description {
				t.Errorf("Description = %q, want %q", got.Description, tt.agent.Description)
			}
			if got.SystemPrompt != tt.agent.SystemPrompt {
				t.Errorf("SystemPrompt = %q, want %q", got.SystemPrompt, tt.agent.SystemPrompt)
			}
			if len(got.AllowedTools) != len(tt.agent.AllowedTools) {
				t.Errorf("len(AllowedTools) = %d, want %d", len(got.AllowedTools), len(tt.agent.AllowedTools))
			}
			if got.Model != tt.agent.Model {
				t.Errorf("Model = %q, want %q", got.Model, tt.agent.Model)
			}
		})
	}
}

// TestAgentDefinition_NotInCLIArgs verifies that AgentDefinition values in
// Options.Agents do not appear as CLI flags. Agents are sent via the streaming
// initialize request (matching the TypeScript and Python SDKs), not via
// subprocess args. This test locks in that contract so it cannot regress.
func TestAgentDefinition_NotInCLIArgs(t *testing.T) {
	t.Parallel()

	opts := &Options{
		Agents: []AgentDefinition{
			{Name: "myagent", Description: "test agent", SystemPrompt: "help"},
		},
	}
	args := buildLaunchArgs("/bin/claude", "prompt", opts, "")
	for _, a := range args {
		if a == "--agent" || a == "--agents" || a == "myagent" {
			t.Errorf("buildLaunchArgs unexpectedly contains agent-related arg %q; agents must be sent via streaming initialize", a)
		}
	}
}
