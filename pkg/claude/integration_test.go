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

package claude_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/zchee/pandaemonium/pkg/claude"
)

// runRealClaudeTestsEnv gates the real-CLI integration test lane.
// Set RUN_REAL_CLAUDE_TESTS=1 in the environment to opt in.
// This mirrors the RUN_REAL_CODEX_TESTS=1 gate in pkg/codex/integration_test.go.
const runRealClaudeTestsEnv = "RUN_REAL_CLAUDE_TESTS"

// TestRealCLI_Query sends a minimal prompt to the real claude binary and
// verifies that a [ResultMessage] arrives before the timeout.
func TestRealCLI_Query(t *testing.T) {
	if os.Getenv(runRealClaudeTestsEnv) != "1" {
		t.Skipf("set %s=1 to run real claude CLI integration coverage", runRealClaudeTestsEnv)
	}

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		t.Skipf("real claude integration requires claude binary on PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	opts := &claude.Options{
		CLIPath: claudeBin,
	}

	var gotResult bool
	for msg, err := range claude.Query(ctx, "Reply with only the word PONG and nothing else.", opts) {
		if err != nil {
			t.Fatalf("Query() stream error = %v", err)
		}
		if rm, ok := msg.(claude.ResultMessage); ok {
			gotResult = true
			if rm.IsError {
				t.Errorf("ResultMessage.IsError = true, want false; subtype = %q", rm.Subtype)
			}
			break
		}
	}
	if !gotResult {
		t.Error("stream ended without a ResultMessage")
	}
}

// TestRealCLI_MultiTurn sends two prompts to the real claude binary using
// [ClaudeSDKClient] and verifies that both turns produce a ResultMessage.
func TestRealCLI_MultiTurn(t *testing.T) {
	if os.Getenv(runRealClaudeTestsEnv) != "1" {
		t.Skipf("set %s=1 to run real claude CLI integration coverage", runRealClaudeTestsEnv)
	}

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		t.Skipf("real claude integration requires claude binary on PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()

	opts := &claude.Options{
		CLIPath: claudeBin,
	}

	cli, err := claude.NewClient(ctx, opts)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer cli.Close()

	prompts := []string{
		"Reply with only the word PING and nothing else.",
		"Reply with only the word PONG and nothing else.",
	}

	for turn, prompt := range prompts {
		if err := cli.Query(ctx, prompt); err != nil {
			t.Fatalf("turn %d: Query() error = %v", turn, err)
		}
		var gotResult bool
		for msg, err := range cli.ReceiveResponse(ctx) {
			if err != nil {
				t.Fatalf("turn %d: ReceiveResponse() stream error = %v", turn, err)
			}
			if rm, ok := msg.(claude.ResultMessage); ok {
				gotResult = true
				if rm.IsError {
					t.Errorf("turn %d: ResultMessage.IsError = true; subtype = %q", turn, rm.Subtype)
				}
				break
			}
		}
		if !gotResult {
			t.Errorf("turn %d: stream ended without a ResultMessage", turn)
		}
	}
}
