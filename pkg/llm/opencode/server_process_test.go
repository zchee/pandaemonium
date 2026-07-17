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

package opencode

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeOpencodeBin returns a Config that spawns this test binary as the fake
// `opencode` (see TestMain), in the given mode.
func fakeOpencodeBin(t *testing.T, mode, password string) *Config {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return &Config{
		OpencodeBin: executable,
		Password:    password,
		Env:         map[string]string{"FAKE_OPENCODE_MODE": mode},
		DialTimeout: 10 * time.Second,
		DrainWindow: 200 * time.Millisecond,
	}
}

// TestSpawnAnnounceParseAndRun is the AC3 core: spawn the fake binary, parse
// the announced port (past the unsecured-warning line), reach health, create
// a session, and complete a sync run — no real opencode required.
func TestSpawnAnnounceParseAndRun(t *testing.T) {
	t.Parallel()

	oc, err := NewOpencode(t.Context(), fakeOpencodeBin(t, "announce", ""))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	defer func() {
		if err := oc.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	health, err := oc.Health(t.Context())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !health.Healthy || health.Version == "" {
		t.Fatalf("unexpected health: %+v", health)
	}

	session, err := oc.SessionStart(t.Context(), &SessionNewParams{Title: "ac3"})
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if !strings.HasPrefix(session.ID(), "ses_") {
		t.Fatalf("session id = %q, want ses_ prefix", session.ID())
	}

	result, err := session.Run(t.Context(), "hi", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.FinalResponse == "" {
		t.Fatal("RunResult.FinalResponse is empty")
	}
	if result.Usage == nil || result.Usage.Output == 0 {
		t.Fatalf("usage not aggregated: %+v", result.Usage)
	}
}

// TestSpawnAnnounceFallback exercises the announce-parse-failure path: the
// fake binary stays silent, so the wrapper kills it and respawns on a
// self-reserved explicit port.
func TestSpawnAnnounceFallback(t *testing.T) {
	t.Parallel()

	cfg := fakeOpencodeBin(t, "no-announce", "")
	cfg.DialTimeout = 2 * time.Second // bound the announce wait, keep the test fast

	oc, err := NewOpencode(t.Context(), cfg)
	if err != nil {
		t.Fatalf("NewOpencode (fallback path): %v", err)
	}
	defer oc.Close()

	client := oc.Client()
	if client.BaseURL() == "" {
		t.Fatal("base URL empty after fallback")
	}

	session, err := oc.SessionStart(t.Context(), nil)
	if err != nil {
		t.Fatalf("SessionStart after fallback: %v", err)
	}
	result, err := session.Run(t.Context(), "hi", nil)
	if err != nil {
		t.Fatalf("Run after fallback: %v", err)
	}
	if result.FinalResponse == "" {
		t.Fatal("FinalResponse empty after fallback")
	}
}

// TestSpawnPasswordHygiene (AC8, spawn half): the password reaches the child
// via env only — never argv — and every wrapper request authenticates
// against the password-enforcing fake.
func TestSpawnPasswordHygiene(t *testing.T) {
	t.Parallel()

	const password = "sekret-ac8-value"
	oc, err := NewOpencode(t.Context(), fakeOpencodeBin(t, "announce", password))
	if err != nil {
		t.Fatalf("NewOpencode with password: %v", err)
	}
	defer oc.Close()

	client := oc.Client()
	client.mu.Lock()
	proc := client.proc
	client.mu.Unlock()
	if proc == nil {
		t.Fatal("no spawned process")
	}
	for _, arg := range proc.cmd.Args {
		if strings.Contains(arg, password) {
			t.Fatalf("password leaked into argv: %q", arg)
		}
	}

	// The fake enforces basic auth when the env password is set, so a
	// passing health round-trip proves the header flows end to end.
	if _, err := oc.Health(t.Context()); err != nil {
		t.Fatalf("authenticated Health: %v", err)
	}

	// Error strings must not carry the password either.
	_, err = oc.SessionResume(t.Context(), "ses_missing")
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("password leaked into error string: %q", err.Error())
	}
}

// TestCloseTerminatesChild asserts Close reaps the spawned process (no
// orphan): the pid must be gone afterwards.
func TestCloseTerminatesChild(t *testing.T) {
	t.Parallel()

	oc, err := NewOpencode(t.Context(), fakeOpencodeBin(t, "announce", ""))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	client := oc.Client()
	client.mu.Lock()
	pid := client.proc.cmd.Process.Pid
	client.mu.Unlock()

	if err := oc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After Close the process must be reaped: signal 0 to a reaped child
	// fails (ESRCH) because Close owns the single Wait.
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if err != nil {
			break // process gone
		}
		if time.Now().After(deadline) {
			t.Fatalf("child pid %d still alive after Close", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
