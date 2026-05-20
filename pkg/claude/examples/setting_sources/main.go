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

// Command setting_sources demonstrates restricting which settings layers the
// claude CLI loads via [claude.Options].SettingSources. Each
// [claude.SettingSource] is one of the fixed layers the CLI understands
// (user, project, local); the CLI merges the selected layers in order.
//
// Port of examples/setting_sources.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/setting_sources
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
		fmt.Fprintln(os.Stderr, "setting_sources: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	// Example: load only the user-level and project-level settings layers,
	// excluding the local (gitignored) layer.
	ctx := context.Background()

	opts := &claude.Options{
		SettingSources: []claude.SettingSource{
			claude.SettingSourceUser,
			claude.SettingSourceProject,
		},
		MaxTurns: 1,
	}

	fmt.Println("Loading settings from layers: user, project")

	for msg, err := range claude.Query(ctx, "What model are you?", opts) {
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
