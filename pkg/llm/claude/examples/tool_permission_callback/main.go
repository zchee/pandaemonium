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

// Command tool_permission_callback demonstrates using [claude.Options].CanUseTool
// to gate every tool call with a typed permission callback. The callback receives
// the tool name, its raw JSON input, and a [claude.ToolPermissionContext], and
// returns a [claude.PermissionResultAllow] (optionally modifying the input or
// emitting permission updates) or a [claude.PermissionResultDeny] (optionally
// with a message and an interrupt flag).
//
// Port of examples/tool_permission_callback.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/tool_permission_callback
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"

	"github.com/zchee/pandaemonium/pkg/llm/claude"
)

// permissionCallback approves Read and Bash(ls/echo) tool calls; denies everything else.
func permissionCallback(_ context.Context, toolName string, input jsontext.Value, _ claude.ToolPermissionContext) (claude.PermissionResult, error) {
	fmt.Printf("[permission] tool=%q input=%s\n", toolName, input)

	switch toolName {
	case "Read":
		return claude.PermissionResultAllow{}, nil
	case "Bash":
		// Allow only safe read-only Bash commands.
		var inp struct {
			Command string `json:"command"`
		}
		if len(input) > 0 {
			if err := json.Unmarshal(input, &inp); err != nil {
				return nil, err
			}
		}
		if strings.HasPrefix(inp.Command, "ls") || strings.HasPrefix(inp.Command, "echo") {
			return claude.PermissionResultAllow{}, nil
		}
		fmt.Printf("[permission] denying Bash command: %q\n", inp.Command)
		return claude.PermissionResultDeny{Message: "denied unsafe Bash command"}, nil
	default:
		fmt.Printf("[permission] denying unknown tool: %q\n", toolName)
		return claude.PermissionResultDeny{Message: "denied unknown tool"}, nil
	}
}

func main() {
	if os.Getenv("RUN_REAL_CLAUDE_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "tool_permission_callback: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	ctx := context.Background()

	opts := &claude.Options{
		CanUseTool:   permissionCallback,
		AllowedTools: []string{"Bash", "Read"},
		MaxTurns:     3,
	}

	for msg, err := range claude.Query(ctx, "List the files in the current directory.", opts) {
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
