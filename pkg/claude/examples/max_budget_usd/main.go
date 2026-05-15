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

// Command max_budget_usd demonstrates capping the total spend for a session
// via [claude.Options].MaxBudgetUSD. When the cumulative cost exceeds the
// budget the CLI terminates the session.
//
// Port of examples/max_budget_usd.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/max_budget_usd
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
		fmt.Fprintln(os.Stderr, "max_budget_usd: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	ctx := context.Background()

	// Cap the session at one US cent.
	opts := &claude.Options{
		MaxBudgetUSD: 0.01,
		MaxTurns:     5,
	}

	for msg, err := range claude.Query(ctx, "Write a short poem about Go programming.", opts) {
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
			fmt.Printf("\nCost: $%.6f (budget: $%.4f)\n", m.TotalCostUSD, opts.MaxBudgetUSD)
		}
	}
}
