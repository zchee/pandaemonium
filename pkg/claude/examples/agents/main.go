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

// Command agents demonstrates registering programmatic subagent definitions via
// [claude.Options].Agents. Each [claude.AgentDefinition] specifies a named
// subagent with its own system prompt and permitted tools. The parent model can
// delegate subtasks to these agents during a session.
//
// Port of examples/agents.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/agents
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/zchee/pandaemonium/pkg/claude"
)

func main() {
	if os.Getenv("RUN_REAL_CLAUDE_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "agents: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	ctx := context.Background()

	opts := &claude.Options{
		Agents: []claude.AgentDefinition{
			{
				Name:         "researcher",
				Description:  "Searches for and summarises information on a given topic.",
				SystemPrompt: "You are a concise research assistant. Return bullet-point summaries.",
				AllowedTools: []string{"WebSearch", "Read"},
			},
			{
				Name:         "coder",
				Description:  "Writes and reviews Go code.",
				SystemPrompt: "You are an expert Go programmer. Follow the Effective Go guidelines.",
				AllowedTools: []string{"Read", "Write", "Bash"},
			},
		},
		MaxTurns: 5,
	}

	for msg, err := range claude.Query(ctx, "Use the researcher to find out what a goroutine is, then have the coder write a simple goroutine example.", opts) {
		if err != nil {
			log.Fatal(err)
		}
		if am, ok := msg.(claude.AssistantMessage); ok {
			for _, b := range am.Content {
				if tb, ok := b.(claude.TextBlock); ok {
					fmt.Print(tb.Text)
				}
			}
		}
	}
	fmt.Println()
}
