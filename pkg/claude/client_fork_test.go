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

package claude

import (
	"errors"
	"strings"
	"testing"
)

func TestClientFork_RequiresSessionStore(t *testing.T) {
	t.Parallel()

	c, err := NewClient(t.Context(), nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.sessionID = "some-session"

	_, err = c.Fork(t.Context(), "")
	if err == nil {
		t.Fatal("Fork() with nil SessionStore expected error, got nil")
	}
	var connErr *CLIConnectionError
	if !errors.As(err, &connErr) {
		t.Errorf("Fork() error = %T(%v), want *CLIConnectionError", err, err)
	}
}

func TestClientFork_RequiresActiveSession(t *testing.T) {
	t.Parallel()

	store := NewInMemorySessionStore()
	c, err := NewClient(t.Context(), &Options{SessionStore: store})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	// c.sessionID is "" — no active session.

	_, err = c.Fork(t.Context(), "")
	if err == nil {
		t.Fatal("Fork() with empty sessionID expected error, got nil")
	}
	var connErr *CLIConnectionError
	if !errors.As(err, &connErr) {
		t.Errorf("Fork() error = %T(%v), want *CLIConnectionError", err, err)
	}
}

func TestClientFork_SnapshotsHistory(t *testing.T) {
	t.Parallel()

	store := NewInMemorySessionStore()
	msgs := []Message{
		UserMessage{Content: []ContentBlock{TextBlock{Text: "msg1"}}},
		AssistantMessage{Content: []ContentBlock{TextBlock{Text: "msg2"}}},
	}
	if err := store.Save(t.Context(), &Session{ID: "parent-snap", Messages: msgs}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	c, err := NewClient(t.Context(), &Options{SessionStore: store})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.sessionID = "parent-snap"

	child, err := c.Fork(t.Context(), "")
	if err != nil {
		t.Fatalf("Fork() error = %v", err)
	}
	if child == nil {
		t.Fatal("Fork() returned nil client")
	}
	if child.sessionID == "" {
		t.Error("Fork() child.sessionID is empty, want non-empty forked ID")
	}
	if child.sessionID == c.sessionID {
		t.Errorf("Fork() child.sessionID = %q matches parent, want distinct", child.sessionID)
	}

	// The forked session must be stored and retrievable.
	forkedSess, err := store.Load(t.Context(), child.sessionID)
	if err != nil {
		t.Fatalf("Load(child.sessionID) error = %v", err)
	}
	if forkedSess.ParentID != "parent-snap" {
		t.Errorf("forked.ParentID = %q, want parent-snap", forkedSess.ParentID)
	}
	if len(forkedSess.Messages) != 2 {
		t.Errorf("forked.Messages len = %d, want 2", len(forkedSess.Messages))
	}
}

func TestClientFork_DoesNotMutateParentTransport(t *testing.T) {
	t.Parallel()

	store := NewInMemorySessionStore()
	if err := store.Save(t.Context(), &Session{ID: "parent-mut"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	c, err := NewClient(t.Context(), &Options{SessionStore: store})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.sessionID = "parent-mut"

	child, err := c.Fork(t.Context(), "")
	if err != nil {
		t.Fatalf("Fork() error = %v", err)
	}

	// Parent transport must remain nil — Fork must not start a subprocess (AC-i5).
	if c.transport != nil {
		t.Error("Fork() mutated parent transport, want nil")
	}
	// Child must not share the parent transport (both are nil before start,
	// but they must be independent structs).
	if child == c {
		t.Error("Fork() returned the same client pointer as the parent")
	}
	// Parent sessionID must be unchanged.
	if c.sessionID != "parent-mut" {
		t.Errorf("Fork() changed parent.sessionID to %q, want parent-mut", c.sessionID)
	}
}

func TestClientFork_ChildOptsAreIndependent(t *testing.T) {
	t.Parallel()

	store := NewInMemorySessionStore()
	if err := store.Save(t.Context(), &Session{ID: "parent-opts"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	parentOpts := &Options{
		SessionStore: store,
		Model:        "claude-opus-4-5",
		MaxTurns:     3,
	}
	c, err := NewClient(t.Context(), parentOpts)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.sessionID = "parent-opts"

	child, err := c.Fork(t.Context(), "")
	if err != nil {
		t.Fatalf("Fork() error = %v", err)
	}

	// Child opts must be a distinct copy, not the same pointer.
	if child.opts == c.opts {
		t.Error("Fork() child.opts is the same pointer as parent.opts, want independent copy")
	}
	// But the values must match the parent's at fork time.
	if child.opts.Model != "claude-opus-4-5" {
		t.Errorf("child.opts.Model = %q, want claude-opus-4-5", child.opts.Model)
	}
	if child.opts.MaxTurns != 3 {
		t.Errorf("child.opts.MaxTurns = %d, want 3", child.opts.MaxTurns)
	}
}

func TestClientFork_ResumeFlagInLaunchArgs(t *testing.T) {
	t.Parallel()

	store := NewInMemorySessionStore()
	if err := store.Save(t.Context(), &Session{ID: "fork-resume-src"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	c, err := NewClient(t.Context(), &Options{SessionStore: store})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.sessionID = "fork-resume-src"

	child, err := c.Fork(t.Context(), "")
	if err != nil {
		t.Fatalf("Fork() error = %v", err)
	}

	// The child's launch args must include --resume <forkedID> so the CLI
	// subprocess loads the forked session history on start.
	args := buildLaunchArgs("/bin/claude", "", child.opts, child.sessionID)
	joined := strings.Join(args, " ")
	wantFlag := "--resume " + child.sessionID
	if !strings.Contains(joined, wantFlag) {
		t.Errorf("buildLaunchArgs for forked child does not contain %q\n  got: %v", wantFlag, args)
	}
}

func TestClientFork_StoreNotFoundPropagatesError(t *testing.T) {
	t.Parallel()

	store := NewInMemorySessionStore()
	// Do NOT save session "nonexistent" — Fork should get ErrSessionNotFound from the store.
	c, err := NewClient(t.Context(), &Options{SessionStore: store})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.sessionID = "nonexistent"

	_, err = c.Fork(t.Context(), "")
	if err == nil {
		t.Fatal("Fork() with missing session expected error, got nil")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Fork() error = %v, want to wrap ErrSessionNotFound", err)
	}
}
