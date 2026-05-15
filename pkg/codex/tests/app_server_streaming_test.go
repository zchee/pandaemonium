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
	"iter"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/zchee/pandaemonium/pkg/codex"
)

type streamingPortSummary struct {
	Deltas             []string
	AgentMessages      []string
	CompletedStatuses  []codex.TurnStatus
	CompletedTurnID    string
	CompletedTurnItems int
}

func TestAppServerStreamingPortSyncStreamRoutesTextDeltasAndCompletion(t *testing.T) {
	sdk := newHelperCodex(t, "streaming_port")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	turn, err := thread.Turn(ctx, codex.TextInput{Text: "stream please"}, nil)
	if err != nil {
		t.Fatalf("Thread.Turn(stream please) error = %v", err)
	}

	got := collectStreamingPortSummary(t, turn.Stream(ctx))
	want := streamingPortSummary{
		Deltas:            []string{"he", "llo"},
		AgentMessages:     []string{"hello"},
		CompletedStatuses: []codex.TurnStatus{codex.TurnStatusCompleted},
		CompletedTurnID:   turn.ID(),
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("stream summary mismatch (-want +got):\n%s", diff)
	}
}

func TestAppServerStreamingPortTurnRunReturnsCompletedTurn(t *testing.T) {
	sdk := newHelperCodex(t, "streaming_port")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	turn, err := thread.Turn(ctx, codex.TextInput{Text: "complete this turn"}, nil)
	if err != nil {
		t.Fatalf("Thread.Turn(complete this turn) error = %v", err)
	}

	completed, err := turn.Run(ctx)
	if err != nil {
		t.Fatalf("TurnHandle.Run() error = %v", err)
	}
	if completed.Turn.ID != turn.ID() {
		t.Fatalf("RunResult.Turn.ID = %q, want %q", completed.Turn.ID, turn.ID())
	}
	if completed.Turn.Status != codex.TurnStatusCompleted {
		t.Fatalf("RunResult.Turn.Status = %q, want completed", completed.Turn.Status)
	}
	if len(completed.Turn.Items) != 0 {
		t.Fatalf("RunResult.Turn.Items len = %d, want 0", len(completed.Turn.Items))
	}
}

func TestAppServerStreamingPortAsyncEquivalentRoutesTextDeltasAndCompletion(t *testing.T) {
	sdk := newHelperCodex(t, "streaming_port")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	turn, err := thread.Turn(ctx, codex.TextInput{Text: "async stream please"}, nil)
	if err != nil {
		t.Fatalf("Thread.Turn(async stream please) error = %v", err)
	}

	got := collectStreamingPortSummary(t, turn.Stream(ctx))
	want := streamingPortSummary{
		Deltas:            []string{"as", "ync"},
		AgentMessages:     []string{"async"},
		CompletedStatuses: []codex.TurnStatus{codex.TurnStatusCompleted},
		CompletedTurnID:   turn.ID(),
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("async-equivalent stream summary mismatch (-want +got):\n%s", diff)
	}
}

func TestAppServerStreamingPortLowLevelSyncStreamTextUsesRealTurnRouting(t *testing.T) {
	sdk := newHelperCodex(t, "streaming_port")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}

	deltas := collectStreamingPortTextDeltas(t, sdk.Client().StreamText(ctx, thread.ID(), "low-level sync", nil))
	if diff := cmp.Diff([]string{"fir", "st"}, deltas); diff != "" {
		t.Fatalf("StreamText deltas mismatch (-want +got):\n%s", diff)
	}
}

func TestAppServerStreamingPortLowLevelStreamAllowsParallelModelList(t *testing.T) {
	sdk := newHelperCodex(t, "streaming_port")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}

	next, stop := iter.Pull2(sdk.Client().StreamText(ctx, thread.ID(), "low-level async", nil))
	defer stop()

	first := nextStreamingPortDelta(t, next)
	if first != "one" {
		t.Fatalf("first StreamText delta = %q, want one", first)
	}

	includeHidden := true
	models, err := sdk.Models(ctx, &codex.ModelListParams{IncludeHidden: &includeHidden})
	if err != nil {
		t.Fatalf("Models() while stream is active error = %v", err)
	}
	if len(models.Data) != 1 || models.Data[0].ID != "gpt-stream" {
		t.Fatalf("Models() while stream is active = %#v, want one gpt-stream model", models.Data)
	}

	remaining := collectStreamingPortPulledDeltas(t, next)
	if diff := cmp.Diff([]string{"two", "three"}, remaining); diff != "" {
		t.Fatalf("remaining StreamText deltas mismatch (-want +got):\n%s", diff)
	}
}

func TestAppServerStreamingPortInterleavedSyncTurnStreamsRouteByTurnID(t *testing.T) {
	sdk := newHelperCodex(t, "streaming_port")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	firstThread, firstTurn := startStreamingPortTurn(t, ctx, sdk, "first")
	secondThread, secondTurn := startStreamingPortTurn(t, ctx, sdk, "second")
	if firstThread.ID() == secondThread.ID() {
		t.Fatalf("thread ids unexpectedly equal: %q", firstThread.ID())
	}

	nextFirst, stopFirst := iter.Pull2(firstTurn.Stream(ctx))
	defer stopFirst()
	nextSecond, stopSecond := iter.Pull2(secondTurn.Stream(ctx))
	defer stopSecond()

	if got := nextStreamingPortNotificationDelta(t, nextFirst); got != "one-" {
		t.Fatalf("first stream first delta = %q, want one-", got)
	}
	if got := nextStreamingPortNotificationDelta(t, nextSecond); got != "two-" {
		t.Fatalf("second stream first delta = %q, want two-", got)
	}

	firstTail := collectStreamingPortPulledNotifications(t, nextFirst)
	secondTail := collectStreamingPortPulledNotifications(t, nextSecond)

	wantFirst := streamingPortSummary{
		Deltas:            []string{"done"},
		AgentMessages:     []string{"one-done"},
		CompletedStatuses: []codex.TurnStatus{codex.TurnStatusCompleted},
		CompletedTurnID:   firstTurn.ID(),
	}
	wantSecond := streamingPortSummary{
		Deltas:            []string{"done"},
		AgentMessages:     []string{"two-done"},
		CompletedStatuses: []codex.TurnStatus{codex.TurnStatusCompleted},
		CompletedTurnID:   secondTurn.ID(),
	}
	if diff := cmp.Diff(wantFirst, firstTail); diff != "" {
		t.Fatalf("first stream tail mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantSecond, secondTail); diff != "" {
		t.Fatalf("second stream tail mismatch (-want +got):\n%s", diff)
	}
}

func TestAppServerStreamingPortInterleavedAsyncEquivalentTurnStreamsRouteByTurnID(t *testing.T) {
	sdk := newHelperCodex(t, "streaming_port")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	firstThread, firstTurn := startStreamingPortTurn(t, ctx, sdk, "async first")
	secondThread, secondTurn := startStreamingPortTurn(t, ctx, sdk, "async second")
	if firstThread.ID() == secondThread.ID() {
		t.Fatalf("thread ids unexpectedly equal: %q", firstThread.ID())
	}

	nextFirst, stopFirst := iter.Pull2(firstTurn.Stream(ctx))
	defer stopFirst()
	nextSecond, stopSecond := iter.Pull2(secondTurn.Stream(ctx))
	defer stopSecond()

	if got := nextStreamingPortNotificationDelta(t, nextFirst); got != "a1" {
		t.Fatalf("first async-equivalent stream first delta = %q, want a1", got)
	}
	if got := nextStreamingPortNotificationDelta(t, nextSecond); got != "a2" {
		t.Fatalf("second async-equivalent stream first delta = %q, want a2", got)
	}

	firstSummary := collectStreamingPortPulledNotifications(t, nextFirst)
	secondSummary := collectStreamingPortPulledNotifications(t, nextSecond)

	wantFirst := streamingPortSummary{
		Deltas:            []string{"-done"},
		AgentMessages:     []string{"a1-done"},
		CompletedStatuses: []codex.TurnStatus{codex.TurnStatusCompleted},
		CompletedTurnID:   firstTurn.ID(),
	}
	wantSecond := streamingPortSummary{
		Deltas:            []string{"-done"},
		AgentMessages:     []string{"a2-done"},
		CompletedStatuses: []codex.TurnStatus{codex.TurnStatusCompleted},
		CompletedTurnID:   secondTurn.ID(),
	}
	if diff := cmp.Diff(wantFirst, firstSummary); diff != "" {
		t.Fatalf("first async-equivalent stream summary mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantSecond, secondSummary); diff != "" {
		t.Fatalf("second async-equivalent stream summary mismatch (-want +got):\n%s", diff)
	}
}

func startStreamingPortTurn(t *testing.T, ctx context.Context, sdk *codex.Codex, text string) (*codex.Thread, *codex.TurnHandle) {
	t.Helper()
	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart(%q) error = %v", text, err)
	}
	turn, err := thread.Turn(ctx, codex.TextInput{Text: text}, nil)
	if err != nil {
		t.Fatalf("Thread.Turn(%q) error = %v", text, err)
	}
	return thread, turn
}

func collectStreamingPortSummary(t *testing.T, stream iter.Seq2[codex.Notification, error]) streamingPortSummary {
	t.Helper()
	var summary streamingPortSummary
	for notification, err := range stream {
		if err != nil {
			t.Fatalf("stream error = %v", err)
		}
		summary.add(t, notification)
	}
	return summary
}

func collectStreamingPortPulledNotifications(t *testing.T, next func() (codex.Notification, error, bool)) streamingPortSummary {
	t.Helper()
	var summary streamingPortSummary
	for {
		notification, err, ok := next()
		if err != nil {
			t.Fatalf("stream error = %v", err)
		}
		if !ok {
			return summary
		}
		summary.add(t, notification)
	}
}

func nextStreamingPortNotificationDelta(t *testing.T, next func() (codex.Notification, error, bool)) string {
	t.Helper()
	for {
		notification, err, ok := next()
		if err != nil {
			t.Fatalf("stream error = %v", err)
		}
		if !ok {
			t.Fatal("stream ended before the next agent-message delta")
		}
		delta, ok, err := notification.ItemAgentMessageDelta()
		if err != nil {
			t.Fatalf("AgentMessageDelta() error = %v", err)
		}
		if ok {
			return delta.Delta
		}
	}
}

func collectStreamingPortTextDeltas(t *testing.T, stream iter.Seq2[codex.AgentMessageDeltaNotification, error]) []string {
	t.Helper()
	next, stop := iter.Pull2(stream)
	defer stop()
	return collectStreamingPortPulledDeltas(t, next)
}

func collectStreamingPortPulledDeltas(t *testing.T, next func() (codex.AgentMessageDeltaNotification, error, bool)) []string {
	t.Helper()
	var deltas []string
	for {
		delta, err, ok := next()
		if err != nil {
			t.Fatalf("StreamText() error = %v", err)
		}
		if !ok {
			return deltas
		}
		deltas = append(deltas, delta.Delta)
	}
}

func nextStreamingPortDelta(t *testing.T, next func() (codex.AgentMessageDeltaNotification, error, bool)) string {
	t.Helper()
	delta, err, ok := next()
	if err != nil {
		t.Fatalf("StreamText() error = %v", err)
	}
	if !ok {
		t.Fatal("StreamText() ended before the next delta")
	}
	return delta.Delta
}

func (s *streamingPortSummary) add(t *testing.T, notification codex.Notification) {
	t.Helper()
	if delta, ok, err := notification.ItemAgentMessageDelta(); err != nil {
		t.Fatalf("ItemAgentMessageDelta() error = %v", err)
	} else if ok {
		s.Deltas = append(s.Deltas, delta.Delta)
		return
	}
	if item, ok, err := notification.ItemCompleted(); err != nil {
		t.Fatalf("ItemCompleted() error = %v", err)
	} else if ok {
		switch agentMessage := item.Item.(type) {
		case codex.AgentMessageThreadItem:
			s.AgentMessages = append(s.AgentMessages, agentMessage.Text)
		case *codex.AgentMessageThreadItem:
			s.AgentMessages = append(s.AgentMessages, agentMessage.Text)
		}
		return
	}
	if completed, ok, err := notification.TurnCompleted(); err != nil {
		t.Fatalf("TurnCompleted() error = %v", err)
	} else if ok {
		s.CompletedStatuses = append(s.CompletedStatuses, completed.Turn.Status)
		s.CompletedTurnID = completed.Turn.ID
		s.CompletedTurnItems = len(completed.Turn.Items)
	}
}
