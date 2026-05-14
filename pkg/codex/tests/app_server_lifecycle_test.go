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

package codex_test

import (
	"context"
	"slices"
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestAppServerLifecyclePort(t *testing.T) {
	t.Run("set_name_and_read", func(t *testing.T) {
		sdk, ctx := newLifecycleCodex(t)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		if _, err := thread.SetName(ctx, "sdk integration thread"); err != nil {
			t.Fatalf("Thread.SetName() error = %v", err)
		}
		named, err := thread.Read(ctx, true)
		if err != nil {
			t.Fatalf("Thread.Read(includeTurns=true) error = %v", err)
		}

		if named.Thread.Name == nil || *named.Thread.Name != "sdk integration thread" {
			t.Fatalf("Thread.Read().Thread.Name = %#v, want sdk integration thread", named.Thread.Name)
		}
	})

	t.Run("thread_list_filters_archived_threads", func(t *testing.T) {
		sdk, ctx := newLifecycleCodex(t)

		activeThread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart(active) error = %v", err)
		}
		archivedThread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart(archived) error = %v", err)
		}
		if _, err := activeThread.Run(ctx, "keep this listed", nil); err != nil {
			t.Fatalf("active Thread.Run() error = %v", err)
		}
		if _, err := archivedThread.Run(ctx, "archive this", nil); err != nil {
			t.Fatalf("archived Thread.Run() error = %v", err)
		}
		if _, err := sdk.ThreadArchive(ctx, archivedThread.ID()); err != nil {
			t.Fatalf("ThreadArchive(%q) error = %v", archivedThread.ID(), err)
		}

		showArchived := true
		activeList, err := sdk.ThreadList(ctx, &codex.ThreadListParams{Archived: new(bool)})
		if err != nil {
			t.Fatalf("ThreadList(archived=false) error = %v", err)
		}
		archivedList, err := sdk.ThreadList(ctx, &codex.ThreadListParams{Archived: &showArchived})
		if err != nil {
			t.Fatalf("ThreadList(archived=true) error = %v", err)
		}

		expected := map[string][]string{
			"active_ids":   {activeThread.ID()},
			"archived_ids": {archivedThread.ID()},
		}
		got := map[string][]string{
			"active_ids":   filteredThreadIDs(activeList.Data, activeThread.ID(), archivedThread.ID()),
			"archived_ids": filteredThreadIDs(archivedList.Data, activeThread.ID(), archivedThread.ID()),
		}
		if diff := gocmp.Diff(expected, got); diff != "" {
			t.Fatalf("thread list archive filters mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("read_include_turns_returns_persisted_history", func(t *testing.T) {
		sdk, ctx := newLifecycleCodex(t)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		if _, err := thread.Run(ctx, "first question", nil); err != nil {
			t.Fatalf("first Thread.Run() error = %v", err)
		}
		if _, err := thread.Run(ctx, "second question", nil); err != nil {
			t.Fatalf("second Thread.Run() error = %v", err)
		}
		read, err := thread.Read(ctx, true)
		if err != nil {
			t.Fatalf("Thread.Read(includeTurns=true) error = %v", err)
		}

		want := []messageSummary{
			{Role: "user", Text: "first question"},
			{Role: "agent", Text: "first answer"},
			{Role: "user", Text: "second question"},
			{Role: "agent", Text: "second answer"},
		}
		if diff := gocmp.Diff(want, threadMessageSummary(t, read)); diff != "" {
			t.Fatalf("thread persisted history mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("async_lifecycle_equivalent_methods_round_trip", func(t *testing.T) {
		sdk, ctx := newLifecycleCodex(t)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		runResult, err := thread.Run(ctx, "materialize async thread", nil)
		if err != nil {
			t.Fatalf("Thread.Run() error = %v", err)
		}
		if _, err := thread.SetName(ctx, "async lifecycle"); err != nil {
			t.Fatalf("Thread.SetName() error = %v", err)
		}
		named, err := thread.Read(ctx, false)
		if err != nil {
			t.Fatalf("Thread.Read(includeTurns=false) error = %v", err)
		}
		resumed, err := sdk.ThreadResume(ctx, thread.ID(), nil)
		if err != nil {
			t.Fatalf("ThreadResume() error = %v", err)
		}
		forked, err := sdk.ThreadFork(ctx, thread.ID(), nil)
		if err != nil {
			t.Fatalf("ThreadFork() error = %v", err)
		}
		archiveResponse, err := sdk.ThreadArchive(ctx, thread.ID())
		if err != nil {
			t.Fatalf("ThreadArchive() error = %v", err)
		}
		unarchived, err := sdk.ThreadUnarchive(ctx, thread.ID())
		if err != nil {
			t.Fatalf("ThreadUnarchive() error = %v", err)
		}

		got := map[string]any{
			"run_final_response": runResult.FinalResponse,
			"named_thread":       stringValue(named.Thread.Name),
			"resumed_id":         resumed.ID(),
			"forked_is_distinct": forked.ID() != thread.ID(),
			"archive_response":   archiveResponse,
			"unarchived_id":      unarchived.ID(),
		}
		want := map[string]any{
			"run_final_response": "async materialized",
			"named_thread":       "async lifecycle",
			"resumed_id":         thread.ID(),
			"forked_is_distinct": true,
			"archive_response":   codex.ThreadArchiveResponse{},
			"unarchived_id":      thread.ID(),
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("async-equivalent lifecycle mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("thread_fork_returns_distinct_thread", func(t *testing.T) {
		sdk, ctx := newLifecycleCodex(t)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		seeded, err := thread.Run(ctx, "materialize this thread before fork", nil)
		if err != nil {
			t.Fatalf("Thread.Run(seed) error = %v", err)
		}
		forked, err := sdk.ThreadFork(ctx, thread.ID(), nil)
		if err != nil {
			t.Fatalf("ThreadFork() error = %v", err)
		}

		got := map[string]any{
			"seeded_response":    seeded.FinalResponse,
			"forked_is_distinct": forked.ID() != thread.ID(),
		}
		want := map[string]any{
			"seeded_response":    "materialized",
			"forked_is_distinct": true,
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("fork mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("archive_unarchive_round_trip_uses_materialized_rollout", func(t *testing.T) {
		sdk, ctx := newLifecycleCodex(t)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		seeded, err := thread.Run(ctx, "materialize this thread before archive", nil)
		if err != nil {
			t.Fatalf("Thread.Run(seed) error = %v", err)
		}
		archived, err := sdk.ThreadArchive(ctx, thread.ID())
		if err != nil {
			t.Fatalf("ThreadArchive() error = %v", err)
		}
		unarchived, err := sdk.ThreadUnarchive(ctx, thread.ID())
		if err != nil {
			t.Fatalf("ThreadUnarchive() error = %v", err)
		}
		read, err := unarchived.Read(ctx, false)
		if err != nil {
			t.Fatalf("Thread.Read() after unarchive error = %v", err)
		}

		got := map[string]any{
			"seeded_response":  seeded.FinalResponse,
			"archive_response": archived,
			"unarchived_id":    unarchived.ID(),
			"read_id":          read.Thread.ID,
		}
		want := map[string]any{
			"seeded_response":  "materialized",
			"archive_response": codex.ThreadArchiveResponse{},
			"unarchived_id":    thread.ID(),
			"read_id":          thread.ID(),
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("archive/unarchive mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("models_rpc", func(t *testing.T) {
		sdk, ctx := newLifecycleCodex(t)

		models, err := sdk.Models(ctx, true)
		if err != nil {
			t.Fatalf("Models(includeHidden=true) error = %v", err)
		}
		if len(models.Data) == 0 {
			t.Fatalf("Models(includeHidden=true) returned no data")
		}
	})

	t.Run("compact_rpc_hits_helper_boundary", func(t *testing.T) {
		sdk, ctx := newLifecycleCodex(t)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		runResult, err := thread.Run(ctx, "create history", nil)
		if err != nil {
			t.Fatalf("Thread.Run(create history) error = %v", err)
		}
		compactResponse, err := thread.Compact(ctx)
		if err != nil {
			t.Fatalf("Thread.Compact() error = %v", err)
		}

		got := map[string]any{
			"run_final_response": runResult.FinalResponse,
			"compact_response":   compactResponse,
		}
		want := map[string]any{
			"run_final_response": "history",
			"compact_response":   codex.ThreadCompactStartResponse{},
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("compact mismatch (-want +got):\n%s", diff)
		}
	})
}

type messageSummary struct {
	Role string
	Text string
}

func newLifecycleCodex(t *testing.T) (*codex.Codex, context.Context) {
	t.Helper()
	sdk := newHelperCodex(t, "lifecycle_persistence")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return sdk, ctx
}

func threadMessageSummary(t *testing.T, read codex.ThreadReadResponse) []messageSummary {
	t.Helper()
	var messages []messageSummary
	for _, turn := range read.Thread.Turns {
		for _, raw := range turn.Items {
			switch item := raw.(type) {
			case codex.UserMessageThreadItem:
				var text string
				for _, input := range item.Content {
					if textInput, ok := input.(codex.TextUserInput); ok {
						if text != "" {
							text += "\n"
						}
						text += textInput.Text
					}
				}
				messages = append(messages, messageSummary{Role: "user", Text: text})
			case codex.AgentMessageThreadItem:
				messages = append(messages, messageSummary{Role: "agent", Text: item.Text})
			default:
				t.Fatalf("unexpected thread item type %T in turn %q", raw, turn.ID)
			}
		}
	}
	return messages
}

func filteredThreadIDs(threads []codex.ThreadPayload, ids ...string) []string {
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	var got []string
	for _, thread := range threads {
		if wanted[thread.ID] {
			got = append(got, thread.ID)
		}
	}
	slices.Sort(got)
	return got
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
