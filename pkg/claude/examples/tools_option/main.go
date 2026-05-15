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

// Command tools_option demonstrates restricting the tools available to the
// claude CLI via [claude.Options].AllowedTools. Only the listed tool names are
// permitted; any tool not in the list is unavailable for the session.
//
// Port of examples/tools_option.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/tools_option
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
		fmt.Fprintln(os.Stderr, "tools_option: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	ctx := context.Background()

	// Only permit the Read and Bash tools; all others are unavailable.
	opts := &claude.Options{
		AllowedTools: []string{"Read", "Bash"},
		MaxTurns:     3,
	}

	for msg, err := range claude.Query(ctx, "List the files in the current directory.", opts) {
		if err != nil {
			log.Fatal(err)
		}
		switch m := msg.(type) {
		case claude.AssistantMessage:
			for _, b := range m.Content {
				if tb, ok := b.(claude.TextBlock); ok {
					fmt.Print(tb.Text)
				}
			}
		case claude.ResultMessage:
			fmt.Printf("\nTurns: %d\n", m.NumTurns)
		}
	}
}
