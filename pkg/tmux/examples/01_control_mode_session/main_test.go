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

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRunSkipsWithoutRealTmuxOptIn(t *testing.T) {
	t.Setenv(runRealTmuxTestsEnv, "")

	var out bytes.Buffer
	if err := run(t.Context(), &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, runRealTmuxTestsEnv) {
		t.Fatalf("run() output = %q, want opt-in instructions containing %s", got, runRealTmuxTestsEnv)
	}
}

func TestRunRealTmux(t *testing.T) {
	if os.Getenv(runRealTmuxTestsEnv) != "1" {
		t.Skipf("%s=1 is required for isolated real tmux example tests", runRealTmuxTestsEnv)
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skipf("tmux executable not found on PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	var out bytes.Buffer
	if err := run(ctx, &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"session: pandaemonium-example-", "message: hello from pkg/tmux", "panes: ", "first pane: pandaemonium-example-"} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() output = %q, want substring %q", got, want)
		}
	}
}
