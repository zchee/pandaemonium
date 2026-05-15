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

// Command plugin_example demonstrates loading a claude CLI plugin via
// [claude.Options].Plugins. Each [claude.Plugin] points to a plugin directory
// that the CLI loads at session start. This example uses the
// CLAUDE_PLUGIN_PATH environment variable to specify the plugin directory.
//
// Port of examples/plugin_example.py from claude-agent-sdk-python.
//
// Usage:
//
//	CLAUDE_PLUGIN_PATH=/path/to/plugin RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/plugin_example
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
		fmt.Fprintln(os.Stderr, "plugin_example: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	pluginPath := os.Getenv("CLAUDE_PLUGIN_PATH")
	if pluginPath == "" {
		fmt.Fprintln(os.Stderr, "plugin_example: set CLAUDE_PLUGIN_PATH to the plugin directory.")
		fmt.Fprintln(os.Stderr, "Running without any plugin loaded.")
	}

	ctx := context.Background()

	var plugins []claude.Plugin
	if pluginPath != "" {
		plugins = []claude.Plugin{
			{Name: "example-plugin", Path: pluginPath},
		}
	}

	opts := &claude.Options{
		Plugins:  plugins,
		MaxTurns: 2,
	}

	for msg, err := range claude.Query(ctx, "What tools and capabilities do you have available?", opts) {
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
