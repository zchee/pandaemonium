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

// Command filesystem_agents demonstrates configuring subagents that specialise
// in filesystem operations. The orchestrator agent coordinates two specialists:
// a reader (read-only) and a writer (write-capable), each constrained to their
// respective tool sets.
//
// Port of examples/filesystem_agents.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/filesystem_agents
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
		fmt.Fprintln(os.Stderr, "filesystem_agents: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	ctx := context.Background()

	opts := &claude.Options{
		Agents: []claude.AgentDefinition{
			{
				Name:         "fs-reader",
				Description:  "Reads files and directories; never modifies anything.",
				SystemPrompt: "You are a read-only filesystem inspector. Use Read and Bash(ls/cat/find) only.",
				AllowedTools: []string{"Read", "Bash"},
			},
			{
				Name:         "fs-writer",
				Description:  "Creates and updates files as instructed.",
				SystemPrompt: "You are a filesystem writer. Use Write and Edit tools.",
				AllowedTools: []string{"Write", "Edit"},
			},
		},
		AllowedTools: []string{"Read", "Write", "Edit", "Bash"},
		MaxTurns:     6,
	}

	for msg, err := range claude.Query(ctx, "Use fs-reader to list Go files in the current directory, then use fs-writer to create a summary.txt with the list.", opts) {
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
