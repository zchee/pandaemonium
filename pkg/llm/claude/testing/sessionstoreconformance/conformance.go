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

// Package sessionstoreconformance provides a reusable conformance test harness
// for [claude.SessionStore] implementations.
//
// Call [Run] from a *testing.T to validate that a [SessionStore] implementation
// satisfies the full contract expected by pkg/llm/claude.
//
// Usage:
//
//	func TestMyStore(t *testing.T) {
//	    sessionstoreconformance.Run(t, func() claude.SessionStore {
//	        return mystore.New()
//	    })
//	}
package sessionstoreconformance

import (
	"errors"
	"testing"

	"github.com/zchee/pandaemonium/pkg/llm/claude"
)

// newStoreFunc constructs a fresh [claude.SessionStore] for a single sub-test.
type newStoreFunc func() claude.SessionStore

// Run exercises the full [claude.SessionStore] contract against the store
// returned by newStore. newStore is called once per sub-test so each test gets
// a clean instance. Each contract case is an independent parallel sub-test.
func Run(t *testing.T, newStore newStoreFunc) {
	t.Helper()

	cases := []struct {
		name string
		fn   func(t *testing.T, newStore newStoreFunc)
	}{
		{"Load_NotFound", testLoadNotFound},
		{"Save_Load_roundtrip", testSaveLoadRoundtrip},
		{"Save_emptyID_error", testSaveEmptyIDError},
		{"Append_Load", testAppendLoad},
		{"Append_NotFound", testAppendNotFound},
		{"List_empty", testListEmpty},
		{"List_sorted", testListSorted},
		{"Delete_removes_session", testDeleteRemovesSession},
		{"Delete_NotFound", testDeleteNotFound},
		{"Fork_copies_messages", testForkCopiesMessages},
		{"Fork_cuts_at_promoted_message_id", testForkCutsAtPromotedMessageID},
		{"Fork_NotFound", testForkNotFound},
		{"Fork_isolates_from_parent", testForkIsolatesFromParent},
		{"Summary_not_empty", testSummaryNotEmpty},
		{"Summary_NotFound", testSummaryNotFound},
		{"Save_overwrites", testSaveOverwrites},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.fn(t, newStore)
		})
	}
}

func testLoadNotFound(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	_, err := store.Load(t.Context(), "missing-id")
	if !errors.Is(err, claude.ErrSessionNotFound) {
		t.Errorf("Load(missing) error = %v, want ErrSessionNotFound", err)
	}
}

func testSaveLoadRoundtrip(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	sess := &claude.Session{
		ID: "sess-1",
		Messages: []claude.Message{
			claude.AssistantMessage{Content: []claude.ContentBlock{claude.TextBlock{Text: "hello"}}},
		},
	}
	if err := store.Save(t.Context(), sess); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load(t.Context(), "sess-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.ID != sess.ID {
		t.Errorf("Load().ID = %q, want %q", got.ID, sess.ID)
	}
	if len(got.Messages) != 1 {
		t.Errorf("Load().Messages len = %d, want 1", len(got.Messages))
	}
}

func testSaveEmptyIDError(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	err := store.Save(t.Context(), &claude.Session{ID: ""})
	if err == nil {
		t.Error("Save(emptyID) expected error, got nil")
	}
}

func testAppendLoad(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	if err := store.Save(t.Context(), &claude.Session{ID: "sess-a"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	msgs := []claude.Message{
		claude.UserMessage{Content: []claude.ContentBlock{claude.TextBlock{Text: "hi"}}},
		claude.AssistantMessage{Content: []claude.ContentBlock{claude.TextBlock{Text: "hello"}}},
	}
	if err := store.Append(t.Context(), "sess-a", msgs); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	got, err := store.Load(t.Context(), "sess-a")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.Messages) != 2 {
		t.Errorf("Load().Messages len = %d, want 2", len(got.Messages))
	}
}

func testAppendNotFound(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	err := store.Append(t.Context(), "missing-id", nil)
	if !errors.Is(err, claude.ErrSessionNotFound) {
		t.Errorf("Append(missing) error = %v, want ErrSessionNotFound", err)
	}
}

func testListEmpty(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	ids, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("List() = %v, want empty", ids)
	}
}

func testListSorted(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	for _, id := range []string{"c", "a", "b"} {
		if err := store.Save(t.Context(), &claude.Session{ID: id}); err != nil {
			t.Fatalf("Save(%q) error = %v", id, err)
		}
	}
	ids, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("List() len = %d, want 3", len(ids))
	}
	want := []string{"a", "b", "c"}
	for i, w := range want {
		if ids[i] != w {
			t.Errorf("List()[%d] = %q, want %q", i, ids[i], w)
		}
	}
}

func testDeleteRemovesSession(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	if err := store.Save(t.Context(), &claude.Session{ID: "del-me"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.Delete(t.Context(), "del-me"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	_, err := store.Load(t.Context(), "del-me")
	if !errors.Is(err, claude.ErrSessionNotFound) {
		t.Errorf("Load(deleted) error = %v, want ErrSessionNotFound", err)
	}
}

func testDeleteNotFound(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	err := store.Delete(t.Context(), "missing")
	if !errors.Is(err, claude.ErrSessionNotFound) {
		t.Errorf("Delete(missing) error = %v, want ErrSessionNotFound", err)
	}
}

func testForkCopiesMessages(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	msgs := []claude.Message{
		claude.UserMessage{Content: []claude.ContentBlock{claude.TextBlock{Text: "msg1"}}},
		claude.UserMessage{Content: []claude.ContentBlock{claude.TextBlock{Text: "msg2"}}},
	}
	if err := store.Save(t.Context(), &claude.Session{ID: "src", Messages: msgs}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	// Fork with empty fromMessageID copies all messages.
	forked, err := store.Fork(t.Context(), "src", "")
	if err != nil {
		t.Fatalf("Fork() error = %v", err)
	}
	if forked.ID == "src" {
		t.Error("Fork().ID should differ from source")
	}
	if forked.ParentID != "src" {
		t.Errorf("Fork().ParentID = %q, want %q", forked.ParentID, "src")
	}
	if len(forked.Messages) != 2 {
		t.Errorf("Fork().Messages len = %d, want 2", len(forked.Messages))
	}
	// Forked session must be retrievable from the store.
	got, err := store.Load(t.Context(), forked.ID)
	if err != nil {
		t.Fatalf("Load(forked.ID) error = %v", err)
	}
	if got.ParentID != "src" {
		t.Errorf("Load(forked).ParentID = %q, want %q", got.ParentID, "src")
	}
}

func testForkCutsAtPromotedMessageID(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	msgs := []claude.Message{
		claude.AssistantMessage{MessageID: "msg-1", Content: []claude.ContentBlock{claude.TextBlock{Text: "one"}}},
		claude.AssistantMessage{MessageID: "msg-2", Content: []claude.ContentBlock{claude.TextBlock{Text: "two"}}},
		claude.AssistantMessage{MessageID: "msg-3", Content: []claude.ContentBlock{claude.TextBlock{Text: "three"}}},
	}
	if err := store.Save(t.Context(), &claude.Session{ID: "branch-src", Messages: msgs}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	forked, err := store.Fork(t.Context(), "branch-src", "msg-2")
	if err != nil {
		t.Fatalf("Fork() error = %v", err)
	}
	if len(forked.Messages) != 2 {
		t.Fatalf("Fork().Messages len = %d, want 2", len(forked.Messages))
	}
	last, ok := forked.Messages[1].(claude.AssistantMessage)
	if !ok {
		t.Fatalf("forked.Messages[1] = %T, want AssistantMessage", forked.Messages[1])
	}
	if last.MessageID != "msg-2" {
		t.Fatalf("last MessageID = %q, want msg-2", last.MessageID)
	}
}

func testForkNotFound(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	_, err := store.Fork(t.Context(), "missing", "")
	if !errors.Is(err, claude.ErrSessionNotFound) {
		t.Errorf("Fork(missing) error = %v, want ErrSessionNotFound", err)
	}
}

func testForkIsolatesFromParent(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	if err := store.Save(t.Context(), &claude.Session{ID: "parent"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	forked, err := store.Fork(t.Context(), "parent", "")
	if err != nil {
		t.Fatalf("Fork() error = %v", err)
	}
	// Appending to forked must not affect parent.
	_ = store.Append(t.Context(), forked.ID, []claude.Message{
		claude.UserMessage{Content: []claude.ContentBlock{claude.TextBlock{Text: "forked-only"}}},
	})
	parent, err := store.Load(t.Context(), "parent")
	if err != nil {
		t.Fatalf("Load(parent) error = %v", err)
	}
	if len(parent.Messages) != 0 {
		t.Errorf("parent.Messages len = %d, want 0 (fork should be isolated)", len(parent.Messages))
	}
}

func testSummaryNotEmpty(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	if err := store.Save(t.Context(), &claude.Session{ID: "summ-id"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	s, err := store.Summary(t.Context(), "summ-id")
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if s == "" {
		t.Error("Summary() returned empty string")
	}
}

func testSummaryNotFound(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	_, err := store.Summary(t.Context(), "missing")
	if !errors.Is(err, claude.ErrSessionNotFound) {
		t.Errorf("Summary(missing) error = %v, want ErrSessionNotFound", err)
	}
}

func testSaveOverwrites(t *testing.T, newStore newStoreFunc) {
	t.Helper()
	store := newStore()
	if err := store.Save(t.Context(), &claude.Session{ID: "ow", Messages: nil}); err != nil {
		t.Fatalf("Save() #1 error = %v", err)
	}
	newMsgs := []claude.Message{claude.UserMessage{Content: []claude.ContentBlock{claude.TextBlock{Text: "new"}}}}
	if err := store.Save(t.Context(), &claude.Session{ID: "ow", Messages: newMsgs}); err != nil {
		t.Fatalf("Save() #2 error = %v", err)
	}
	got, err := store.Load(t.Context(), "ow")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.Messages) != 1 {
		t.Errorf("Load().Messages len = %d, want 1", len(got.Messages))
	}
}
