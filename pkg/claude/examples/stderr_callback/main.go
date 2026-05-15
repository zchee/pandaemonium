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

// Command stderr_callback demonstrates surfacing subprocess stderr output when
// the claude CLI exits non-zero. The [claude.ProcessError] type carries a
// StderrTail field with the last ≤40 lines of stderr, populated by the SDK's
// drainStderr goroutine (mirrors pkg/codex/client.go:737).
//
// Port of examples/stderr_callback_example.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/stderr_callback
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/zchee/pandaemonium/pkg/claude"
)

func main() {
	if os.Getenv("RUN_REAL_CLAUDE_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "stderr_callback: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	ctx := context.Background()

	// Use a deliberately invalid CLI path to trigger a ProcessError so we can
	// demonstrate StderrTail surfacing. In real usage this would be any query
	// that causes the CLI subprocess to exit non-zero.
	opts := &claude.Options{
		CLIPath:  "/usr/local/bin/claude",
		MaxTurns: 1,
	}

	var gotProcessErr *claude.ProcessError
	for _, err := range claude.Query(ctx, "Hello", opts) {
		if err == nil {
			continue
		}
		if errors.As(err, &gotProcessErr) {
			fmt.Printf("CLI exited with code %d\n", gotProcessErr.ExitCode)
			if gotProcessErr.StderrTail != "" {
				fmt.Println("--- stderr tail ---")
				fmt.Println(gotProcessErr.StderrTail)
				fmt.Println("---")
			}
			return
		}
		// Surface other errors normally.
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return
	}

	fmt.Println("Query completed without error.")
}
