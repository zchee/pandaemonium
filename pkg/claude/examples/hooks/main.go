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

// Command hooks demonstrates registering a PreToolUse hook via
// [claude.Options].Hooks to inspect and gate tool calls before the claude CLI
// executes them. In this example, Bash commands containing dangerous operations
// (rm, rmdir, dd) are blocked; safe commands are allowed through.
//
// Port of examples/hooks.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/hooks
package main

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/zchee/pandaemonium/pkg/claude"
)

// dangerousPatterns lists substrings that indicate a dangerous Bash command.
var dangerousPatterns = []string{"rm ", "rmdir", " dd ", "mkfs", "> /dev"}

func bashGuard(ctx context.Context, event claude.HookEvent) (claude.HookDecision, error) {
	_ = ctx
	if event.Kind != claude.HookEventPreToolUse || event.ToolName != "Bash" {
		return claude.HookDecision{}, nil
	}

	// Extract the "command" field from the raw tool input JSON.
	var input struct {
		Command string `json:"command"`
	}
	if len(event.ToolInput) > 0 {
		_ = stdjson.Unmarshal(event.ToolInput, &input)
	}

	for _, pat := range dangerousPatterns {
		if strings.Contains(input.Command, pat) {
			fmt.Printf("[hook] BLOCKED dangerous Bash command: %q\n", input.Command)
			return claude.HookDecision{
				HookSpecificOutput: claude.HookSpecificOutput{
					PermissionDecision:       claude.PermissionDeny,
					PermissionDecisionReason: fmt.Sprintf("command contains dangerous pattern %q", pat),
				},
			}, nil
		}
	}

	fmt.Printf("[hook] allowing Bash command: %q\n", input.Command)
	return claude.HookDecision{}, nil
}

func main() {
	if os.Getenv("RUN_REAL_CLAUDE_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "hooks: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	ctx := context.Background()

	opts := &claude.Options{
		Hooks: []claude.HookRegistration{
			{
				Kind:     claude.HookEventPreToolUse,
				ToolGlob: "Bash",
				Fn:       bashGuard,
			},
		},
		AllowedTools: []string{"Bash"},
		MaxTurns:     3,
	}

	for msg, err := range claude.Query(ctx, "List the files in the current directory, then try to remove a file named /tmp/test.txt.", opts) {
		if err != nil {
			log.Fatal(err)
		}
		if am, ok := msg.(claude.AssistantMessage); ok {
			for _, b := range am.Content {
				if tb, ok := b.(claude.TextBlock); ok {
					fmt.Println(tb.Text)
				}
			}
		}
	}
}
