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
	"strings"
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestAppServerRunPort(t *testing.T) {
	t.Run("sync_thread_run_uses_mock_responses_equivalent", func(t *testing.T) {
		sdk, ctx := newRunCodex(t)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		result, err := thread.Run(ctx, "hello", nil)
		if err != nil {
			t.Fatalf("Thread.Run(hello) error = %v", err)
		}

		got := map[string]any{
			"final_response": result.FinalResponse,
			"agent_messages": runAgentMessageTexts(t, result.Items),
			"has_usage":      result.Usage != nil,
		}
		want := map[string]any{
			"final_response": "Hello from the mock.",
			"agent_messages": []string{"Hello from the mock."},
			"has_usage":      true,
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("run result mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("run_params_and_usage_cross_app_server_boundary", func(t *testing.T) {
		sdk, ctx := newRunCodex(t)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		model := "mock-model-override"
		result, err := thread.Run(ctx, "use overrides", &codex.TurnStartParams{Model: &model})
		if err != nil {
			t.Fatalf("Thread.Run(use overrides) error = %v", err)
		}
		if result.Usage == nil {
			t.Fatalf("RunResult.Usage = nil, want app-server usage snapshot")
		}

		got := map[string]any{
			"final_response": result.FinalResponse,
			"usage": map[string]int64{
				"last.cachedInputTokens":      result.Usage.Last.CachedInputTokens,
				"last.inputTokens":            result.Usage.Last.InputTokens,
				"last.outputTokens":           result.Usage.Last.OutputTokens,
				"last.reasoningOutputTokens":  result.Usage.Last.ReasoningOutputTokens,
				"last.totalTokens":            result.Usage.Last.TotalTokens,
				"total.cachedInputTokens":     result.Usage.Total.CachedInputTokens,
				"total.inputTokens":           result.Usage.Total.InputTokens,
				"total.outputTokens":          result.Usage.Total.OutputTokens,
				"total.reasoningOutputTokens": result.Usage.Total.ReasoningOutputTokens,
				"total.totalTokens":           result.Usage.Total.TotalTokens,
			},
		}
		want := map[string]any{
			"final_response": "overrides applied",
			"usage": map[string]int64{
				"last.cachedInputTokens":      3,
				"last.inputTokens":            11,
				"last.outputTokens":           7,
				"last.reasoningOutputTokens":  5,
				"last.totalTokens":            18,
				"total.cachedInputTokens":     3,
				"total.inputTokens":           11,
				"total.outputTokens":          7,
				"total.reasoningOutputTokens": 5,
				"total.totalTokens":           18,
			},
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("run overrides/usage mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("async_thread_run_equivalent", func(t *testing.T) {
		sdk, ctx := newRunCodex(t)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		result, err := thread.Run(ctx, "async hello", nil)
		if err != nil {
			t.Fatalf("Thread.Run(async hello) error = %v", err)
		}

		got := map[string]any{
			"final_response": result.FinalResponse,
			"agent_messages": runAgentMessageTexts(t, result.Items),
		}
		want := map[string]any{
			"final_response": "Hello async.",
			"agent_messages": []string{"Hello async."},
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("async-equivalent run mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("sync_run_result_uses_last_unknown_phase_message", func(t *testing.T) {
		result := runText(t, "case: last unknown phase wins")

		got := map[string]any{
			"final_response": result.FinalResponse,
			"agent_messages": runAgentMessageTexts(t, result.Items),
		}
		want := map[string]any{
			"final_response": "Second message",
			"agent_messages": []string{"First message", "Second message"},
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("last unknown phase mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("sync_run_result_preserves_empty_last_message", func(t *testing.T) {
		result := runText(t, "case: empty last message")

		got := map[string]any{
			"final_response": result.FinalResponse,
			"agent_messages": runAgentMessageTexts(t, result.Items),
		}
		want := map[string]any{
			"final_response": "",
			"agent_messages": []string{"First message", ""},
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("empty final message mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("sync_run_result_does_not_promote_commentary_only_to_final", func(t *testing.T) {
		result := runText(t, "case: commentary only")

		got := map[string]any{
			"final_response": result.FinalResponse,
			"agent_messages": runAgentMessageTexts(t, result.Items),
		}
		want := map[string]any{
			"final_response": "",
			"agent_messages": []string{"Commentary"},
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("commentary-only final mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("async_run_result_uses_last_unknown_phase_message_equivalent", func(t *testing.T) {
		result := runText(t, "case: async last unknown phase")

		got := map[string]any{
			"final_response": result.FinalResponse,
			"agent_messages": runAgentMessageTexts(t, result.Items),
		}
		want := map[string]any{
			"final_response": "Second async message",
			"agent_messages": []string{"First async message", "Second async message"},
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("async-equivalent last unknown mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("async_run_result_does_not_promote_commentary_only_to_final_equivalent", func(t *testing.T) {
		result := runText(t, "case: async commentary only")

		got := map[string]any{
			"final_response": result.FinalResponse,
			"agent_messages": runAgentMessageTexts(t, result.Items),
		}
		want := map[string]any{
			"final_response": "",
			"agent_messages": []string{"Async commentary"},
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("async-equivalent commentary-only mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("thread_run_raises_when_app_server_reports_failed_turn", func(t *testing.T) {
		sdk, ctx := newRunCodex(t)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		_, err = thread.Run(ctx, "trigger failure", nil)
		if err == nil || !strings.Contains(err.Error(), "boom from mock model") {
			t.Fatalf("Thread.Run(trigger failure) error = %v, want boom from mock model", err)
		}
	})

	t.Run("final_answer_phase_survives_app_server_mapping", func(t *testing.T) {
		result := runText(t, "choose final answer")

		got := map[string]any{
			"final_response": result.FinalResponse,
			"items":          runAgentMessagePhaseSummaries(t, result.Items),
		}
		want := map[string]any{
			"final_response": "Final answer",
			"items": []runAgentMessagePhaseSummary{
				{Text: "Commentary", Phase: "commentary"},
				{Text: "Final answer", Phase: "final_answer"},
			},
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("final-answer phase mismatch (-want +got):\n%s", diff)
		}
	})
}

type runAgentMessagePhaseSummary struct {
	Text  string
	Phase string
}

func newRunCodex(t *testing.T) (*codex.Codex, context.Context) {
	t.Helper()
	sdk := newHelperCodex(t, "run_result")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return sdk, ctx
}

func runText(t *testing.T, input string) codex.RunResult {
	t.Helper()
	sdk, ctx := newRunCodex(t)
	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	result, err := thread.Run(ctx, input, nil)
	if err != nil {
		t.Fatalf("Thread.Run(%q) error = %v", input, err)
	}
	return result
}

func runAgentMessageTexts(t *testing.T, items []codex.ThreadItem) []string {
	t.Helper()
	var texts []string
	for _, raw := range items {
		if item, ok := raw.(codex.AgentMessageThreadItem); ok {
			texts = append(texts, item.Text)
			continue
		}
		t.Fatalf("RunResult item = %T, want AgentMessageThreadItem", raw)
	}
	return texts
}

func runAgentMessagePhaseSummaries(t *testing.T, items []codex.ThreadItem) []runAgentMessagePhaseSummary {
	t.Helper()
	var summaries []runAgentMessagePhaseSummary
	for _, raw := range items {
		item, ok := raw.(codex.AgentMessageThreadItem)
		if !ok {
			t.Fatalf("RunResult item = %T, want AgentMessageThreadItem", raw)
		}
		summary := runAgentMessagePhaseSummary{Text: item.Text}
		if item.Phase != nil {
			summary.Phase = messagePhaseString(t, *item.Phase)
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

func messagePhaseString(t *testing.T, phase codex.MessagePhase) string {
	t.Helper()
	switch phase := phase.(type) {
	case codex.MessagePhaseValue:
		return string(phase)
	case codex.RawMessagePhase:
		return string(phase)
	default:
		t.Fatalf("unexpected MessagePhase type %T", phase)
		return ""
	}
}
