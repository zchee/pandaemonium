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

// Command streaming_mode demonstrates real-time streaming of messages from the
// claude CLI. Each [claude.Message] is printed as it arrives, showing the
// assistant's text and the final result metadata.
//
// Port of examples/streaming_mode.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/streaming_mode
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
		fmt.Fprintln(os.Stderr, "streaming_mode: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	ctx := context.Background()

	fmt.Println("Streaming response:")
	fmt.Println("---")

	for msg, err := range claude.Query(ctx, "Explain what a goroutine is in two sentences.", nil) {
		if err != nil {
			log.Fatal(err)
		}
		switch m := msg.(type) {
		case claude.SystemMessage:
			// init message — session is starting
			_ = m
		case claude.AssistantMessage:
			for _, b := range m.Content {
				if tb, ok := b.(claude.TextBlock); ok {
					fmt.Print(tb.Text)
				}
			}
		case claude.UserMessage:
			// tool results etc. — not printed in this example
		case claude.ResultMessage:
			fmt.Printf("\n---\nTurns: %d | Cost: $%.6f | Duration: %dms\n",
				m.NumTurns, m.TotalCostUSD, m.DurationMs)
		}
	}
}
