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

// Command include_partial_messages enables streaming of partial/incomplete
// messages from the claude CLI via [claude.Options].IncludePartialMessages.
// Partial messages let callers render incremental output before the assistant
// finishes its full response.
//
// Port of examples/include_partial_messages.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/include_partial_messages
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
		fmt.Fprintln(os.Stderr, "include_partial_messages: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	ctx := context.Background()

	opts := &claude.Options{
		IncludePartialMessages: true,
		MaxTurns:               1,
	}

	fmt.Println("Streaming with partial messages enabled:")
	var totalBlocks int
	for msg, err := range claude.Query(ctx, "Count from 1 to 5, one number per line.", opts) {
		if err != nil {
			log.Fatal(err)
		}
		switch m := msg.(type) {
		case claude.AssistantMessage:
			for _, b := range m.Content {
				if tb, ok := b.(claude.TextBlock); ok {
					totalBlocks++
					fmt.Printf("[block %d] %s", totalBlocks, tb.Text)
				}
			}
		case claude.ResultMessage:
			fmt.Printf("\nDone. Turns: %d | Partial blocks seen: %d\n", m.NumTurns, totalBlocks)
		}
	}
}
