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

// Command setting_sources demonstrates passing external settings files to the
// claude CLI via [claude.Options].SettingSources. Each [claude.SettingSource]
// may point to a local file path or a remote URL; the CLI merges settings from
// all sources in order.
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

	// Example: load a project-specific settings file, then overlay a user-level
	// settings file. Paths are illustrative; adjust for your installation.
	settingsPath := os.Getenv("CLAUDE_SETTINGS_PATH")
	if settingsPath == "" {
		settingsPath = os.ExpandEnv("$HOME/.claude/settings.json")
	}

	ctx := context.Background()

	opts := &claude.Options{
		SettingSources: []claude.SettingSource{
			{Path: settingsPath},
		},
		MaxTurns: 1,
	}

	fmt.Printf("Loading settings from: %s\n", settingsPath)

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
