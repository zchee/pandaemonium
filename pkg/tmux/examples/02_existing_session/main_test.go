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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const runRealTmuxTestsEnv = "RUN_REAL_TMUX_TESTS"

func TestRunSkipsWithoutSessionName(t *testing.T) {
	t.Setenv(sessionEnv, "")
	t.Setenv(socketPathEnv, "")
	t.Setenv(socketNameEnv, "")
	t.Setenv(configFileEnv, "")

	var out bytes.Buffer
	if err := run(t.Context(), &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, sessionEnv) {
		t.Fatalf("run() output = %q, want instructions containing %s", got, sessionEnv)
	}
}

func TestRunExistingSession(t *testing.T) {
	if os.Getenv(runRealTmuxTestsEnv) != "1" {
		t.Skipf("%s=1 is required for isolated real tmux example tests", runRealTmuxTestsEnv)
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		t.Skipf("tmux executable not found on PATH: %v", err)
	}

	tmp := t.TempDir()
	socket := filepath.Join(tmp, "tmux.sock")
	config := filepath.Join(tmp, "tmux.conf")
	if err := os.WriteFile(config, nil, 0o600); err != nil {
		t.Fatalf("write empty tmux config: %v", err)
	}
	session := fmt.Sprintf("pandaemonium-existing-example-%d", os.Getpid())
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	t.Cleanup(func() { _ = exec.Command(path, "-S", socket, "kill-server").Run() })
	if err := exec.CommandContext(ctx, path, "-S", socket, "-f", config, "new-session", "-d", "-s", session).Run(); err != nil {
		t.Fatalf("create existing tmux session: %v", err)
	}

	t.Setenv(sessionEnv, session)
	t.Setenv(socketPathEnv, socket)
	t.Setenv(socketNameEnv, "")
	t.Setenv(configFileEnv, config)

	var out bytes.Buffer
	if err := run(ctx, &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"attached session: " + session,
		"active pane: %",
		"panes: 1",
		"first pane: %",
		"dropped notifications: 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() output = %q, want substring %q", got, want)
		}
	}
}
