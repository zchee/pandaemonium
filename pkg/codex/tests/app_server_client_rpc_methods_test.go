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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/google/go-cmp/cmp"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestAppServerClientRPCMethodsPortGeneratedParamsAndSharedPlanType(t *testing.T) {
	t.Parallel()

	searchTerm := "needle"
	limit := int32(5)
	params := codex.ThreadListParams{SearchTerm: &searchTerm, Limit: &limit}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal(ThreadListParams) error = %v", err)
	}
	var dumped map[string]any
	if err := json.Unmarshal(encoded, &dumped); err != nil {
		t.Fatalf("json.Unmarshal(ThreadListParams) error = %v", err)
	}
	if diff := cmp.Diff(map[string]any{"searchTerm": "needle", "limit": float64(5)}, dumped); diff != "" {
		t.Fatalf("ThreadListParams JSON mismatch (-want +got):\n%s", diff)
	}

	protocolSource := readProtocolGenSource(t)
	if count := strings.Count(protocolSource, "type PlanType string"); count != 1 {
		t.Fatalf("protocol_gen.go PlanType definitions = %d, want 1", count)
	}
}

func TestAppServerClientRPCMethodsPortResponseAndNotificationDecoding(t *testing.T) {
	t.Parallel()

	var resumed codex.ThreadResumeResponse
	if err := json.Unmarshal([]byte(`{
		"approvalPolicy":"on-request",
		"approvalsReviewer":"auto_review",
		"cwd":"/tmp",
		"model":"gpt-5",
		"modelProvider":"openai",
		"sandbox":{"type":"dangerFullAccess"},
		"thread":{
			"id":"thread-1",
			"sessionId":"session-1",
			"createdAt":1,
			"updatedAt":1,
			"cwd":"/tmp",
			"modelProvider":"openai",
			"preview":"",
			"source":"cli",
			"status":{"type":"idle"},
			"turns":[],
			"ephemeral":false,
			"cliVersion":"1.0.0"
		}
	}`), &resumed); err != nil {
		t.Fatalf("json.Unmarshal(ThreadResumeResponse) error = %v", err)
	}
	if resumed.ApprovalsReviewer != codex.ApprovalsReviewerAutoReview {
		t.Fatalf("ApprovalsReviewer = %q, want %q", resumed.ApprovalsReviewer, codex.ApprovalsReviewerAutoReview)
	}

	usageEvent := codex.Notification{
		Method: codex.NotificationMethodThreadTokenUsageUpdated,
		Params: mustJSONValue(t, map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"tokenUsage": map[string]any{
				"last":  map[string]any{"cachedInputTokens": 0, "inputTokens": 1, "outputTokens": 2, "reasoningOutputTokens": 0, "totalTokens": 3},
				"total": map[string]any{"cachedInputTokens": 0, "inputTokens": 1, "outputTokens": 2, "reasoningOutputTokens": 0, "totalTokens": 3},
			},
		}),
	}
	usage, ok, err := usageEvent.ThreadTokenUsageUpdated()
	if err != nil {
		t.Fatalf("ThreadTokenUsageUpdated() error = %v", err)
	}
	if !ok || usage.TurnID != "turn-1" || usage.TokenUsage.Last.TotalTokens != 3 {
		t.Fatalf("ThreadTokenUsageUpdated() = (%#v, %v), want typed turn-1 usage", usage, ok)
	}

	unknownEvent := codex.Notification{
		Method: "unknown/notification",
		Params: mustJSONValue(t, map[string]any{
			"id":             "evt-1",
			"conversationId": "thread-1",
			"msg":            map[string]any{"type": "turn_aborted"},
		}),
	}
	unknown, matched, err := codex.DecodeNotification(unknownEvent)
	if err != nil {
		t.Fatalf("DecodeNotification(unknown) error = %v", err)
	}
	if matched {
		t.Fatalf("DecodeNotification(unknown) matched = true, want false")
	}
	if unknown.Raw.Method != unknownEvent.Method || string(unknown.Raw.Params) != string(unknownEvent.Params) {
		t.Fatalf("DecodeNotification(unknown) raw = %#v, want original event", unknown.Raw)
	}

	invalidKnown := codex.Notification{
		Method: codex.NotificationMethodThreadTokenUsageUpdated,
		Params: mustJSONValue(t, map[string]any{
			"threadId":   "thread-1",
			"turnId":     "turn-1",
			"tokenUsage": "not-an-object",
		}),
	}
	invalid, matched, err := codex.DecodeNotification(invalidKnown)
	if !matched {
		t.Fatalf("DecodeNotification(invalid known) matched = false, want true")
	}
	if err == nil {
		t.Fatalf("DecodeNotification(invalid known) error = nil, want malformed payload error")
	}
	if invalid.Raw.Method != invalidKnown.Method || string(invalid.Raw.Params) != string(invalidKnown.Params) {
		t.Fatalf("DecodeNotification(invalid known) raw = %#v, want original event", invalid.Raw)
	}
}

func TestAppServerClientRPCMethodsPortTurnNotificationRoutingShapes(t *testing.T) {
	sdk := newHelperCodex(t, "async_client_behavior")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	turn, err := thread.Turn(ctx, "route direct and nested unknown turn notifications", nil)
	if err != nil {
		t.Fatalf("Thread.Turn() error = %v", err)
	}

	events := collectClientRPCMethodsStream(t, turn.Stream(ctx))
	if diff := cmp.Diff([]string{"unknown/direct", "unknown/nested", codex.NotificationMethodTurnCompleted}, eventMethods(events)); diff != "" {
		t.Fatalf("turn routed methods mismatch (-want +got):\n%s", diff)
	}
	for _, event := range events[:2] {
		if got := turnIDFromRawParams(t, event); got != turn.ID() {
			t.Fatalf("%s raw params turn id = %q, want %q", event.Method, got, turn.ID())
		}
	}

	global, err := sdk.Client().NextNotification(ctx)
	if err != nil {
		t.Fatalf("NextNotification() after turn stream error = %v", err)
	}
	if global.Method != "unknown/global" {
		t.Fatalf("NextNotification() method = %q, want unknown/global", global.Method)
	}
}

func TestAppServerClientRPCMethodsPortReaderRoutesInterleavedTurnsByID(t *testing.T) {
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
	if diff := cmp.Diff([]string{"done"}, firstTail.Deltas); diff != "" {
		t.Fatalf("first routed tail deltas mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"done"}, secondTail.Deltas); diff != "" {
		t.Fatalf("second routed tail deltas mismatch (-want +got):\n%s", diff)
	}
	if firstTail.CompletedTurnID != firstTurn.ID() || secondTail.CompletedTurnID != secondTurn.ID() {
		t.Fatalf("completed turn ids = (%q, %q), want (%q, %q)", firstTail.CompletedTurnID, secondTail.CompletedTurnID, firstTurn.ID(), secondTurn.ID())
	}
}

func TestAppServerClientRPCMethodsPortBuffersEventsBeforeRegistration(t *testing.T) {
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
		t.Fatalf("buffered stream summary mismatch (-want +got):\n%s", diff)
	}
}

func readProtocolGenSource(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "protocol_gen.go"))
	if err != nil {
		t.Fatalf("os.ReadFile(protocol_gen.go) error = %v", err)
	}
	return string(source)
}

func collectClientRPCMethodsStream(t *testing.T, stream iter.Seq2[codex.Notification, error]) []codex.Notification {
	t.Helper()
	var events []codex.Notification
	for event, err := range stream {
		if err != nil {
			t.Fatalf("TurnHandle.Stream() error = %v", err)
		}
		events = append(events, event)
	}
	return events
}

func eventMethods(events []codex.Notification) []string {
	methods := make([]string, 0, len(events))
	for _, event := range events {
		methods = append(methods, event.Method)
	}
	return methods
}

func turnIDFromRawParams(t *testing.T, event codex.Notification) string {
	t.Helper()
	var params struct {
		TurnID string `json:"turnId,omitzero"`
		Turn   *struct {
			ID string `json:"id,omitzero"`
		} `json:"turn,omitzero"`
	}
	if err := json.Unmarshal(event.Params, &params); err != nil {
		t.Fatalf("json.Unmarshal(%s params) error = %v", event.Method, err)
	}
	if params.TurnID != "" {
		return params.TurnID
	}
	if params.Turn != nil {
		return params.Turn.ID
	}
	return ""
}
