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
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestAppServerApprovalsPort(t *testing.T) {
	t.Run("resume inherits deny_all", func(t *testing.T) {
		sdk, ctx := newApprovalPersistenceCodex(t)
		never, reviewer := mustApprovalModeSettings(t, codex.ApprovalModeDenyAll)

		source, err := sdk.ThreadStart(ctx, &codex.ThreadStartParams{ApprovalPolicy: &never, ApprovalsReviewer: reviewer})
		if err != nil {
			t.Fatalf("ThreadStart(deny_all) error = %v", err)
		}
		result, err := source.Run(ctx, "seed the source rollout", nil)
		if err != nil {
			t.Fatalf("Thread.Run(seed) error = %v", err)
		}
		resumed, err := sdk.Client().ThreadResume(ctx, source.ID(), &codex.ThreadResumeParams{ThreadID: source.ID()})
		if err != nil {
			t.Fatalf("Client.ThreadResume() error = %v", err)
		}

		got := map[string]string{
			"final_response": result.FinalResponse,
			"resumed_policy": approvalPolicyString(t, resumed.ApprovalPolicy),
		}
		want := map[string]string{"final_response": "source seeded", "resumed_policy": string(codex.AskForApprovalValueNever)}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("resume approval state mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("fork inherits deny_all", func(t *testing.T) {
		sdk, ctx := newApprovalPersistenceCodex(t)
		never, reviewer := mustApprovalModeSettings(t, codex.ApprovalModeDenyAll)

		source, err := sdk.ThreadStart(ctx, &codex.ThreadStartParams{ApprovalPolicy: &never, ApprovalsReviewer: reviewer})
		if err != nil {
			t.Fatalf("ThreadStart(deny_all) error = %v", err)
		}
		result, err := source.Run(ctx, "seed the source rollout", nil)
		if err != nil {
			t.Fatalf("Thread.Run(seed) error = %v", err)
		}
		forked, err := sdk.ThreadFork(ctx, source.ID(), nil)
		if err != nil {
			t.Fatalf("ThreadFork() error = %v", err)
		}
		forkedState, err := sdk.Client().ThreadResume(ctx, forked.ID(), &codex.ThreadResumeParams{ThreadID: forked.ID()})
		if err != nil {
			t.Fatalf("Client.ThreadResume(forked) error = %v", err)
		}

		got := map[string]any{
			"final_response":     result.FinalResponse,
			"forked_is_distinct": forked.ID() != source.ID(),
			"forked_policy":      approvalPolicyString(t, forkedState.ApprovalPolicy),
		}
		want := map[string]any{"final_response": "source seeded", "forked_is_distinct": true, "forked_policy": string(codex.AskForApprovalValueNever)}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("fork inherited approval state mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("fork can override approval mode", func(t *testing.T) {
		sdk, ctx := newApprovalPersistenceCodex(t)
		never, denyReviewer := mustApprovalModeSettings(t, codex.ApprovalModeDenyAll)
		onRequest, autoReviewer := mustApprovalModeSettings(t, codex.ApprovalModeAutoReview)

		source, err := sdk.ThreadStart(ctx, &codex.ThreadStartParams{ApprovalPolicy: &never, ApprovalsReviewer: denyReviewer})
		if err != nil {
			t.Fatalf("ThreadStart(deny_all) error = %v", err)
		}
		result, err := source.Run(ctx, "seed the source rollout", nil)
		if err != nil {
			t.Fatalf("Thread.Run(seed) error = %v", err)
		}
		forked, err := sdk.ThreadFork(ctx, source.ID(), &codex.ThreadForkParams{ApprovalPolicy: &onRequest, ApprovalsReviewer: autoReviewer})
		if err != nil {
			t.Fatalf("ThreadFork(auto_review) error = %v", err)
		}
		forkedState, err := sdk.Client().ThreadResume(ctx, forked.ID(), &codex.ThreadResumeParams{ThreadID: forked.ID()})
		if err != nil {
			t.Fatalf("Client.ThreadResume(forked) error = %v", err)
		}

		got := map[string]string{
			"final_response":  result.FinalResponse,
			"forked_policy":   approvalPolicyString(t, forkedState.ApprovalPolicy),
			"forked_reviewer": string(forkedState.ApprovalsReviewer),
		}
		want := map[string]string{"final_response": "source seeded", "forked_policy": string(codex.AskForApprovalValueOnRequest), "forked_reviewer": string(codex.ApprovalsReviewerAutoReview)}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("fork override approval state mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("turn approval mode persists until next turn", func(t *testing.T) {
		sdk, ctx := newApprovalPersistenceCodex(t)
		never, reviewer := mustApprovalModeSettings(t, codex.ApprovalModeDenyAll)

		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart(default) error = %v", err)
		}
		firstResult, err := thread.Run(ctx, "deny this and later turns", &codex.TurnStartParams{ApprovalPolicy: &never, ApprovalsReviewer: reviewer})
		if err != nil {
			t.Fatalf("Thread.Run(deny override) error = %v", err)
		}
		afterTurnOverride, err := sdk.Client().ThreadResume(ctx, thread.ID(), &codex.ThreadResumeParams{ThreadID: thread.ID()})
		if err != nil {
			t.Fatalf("Client.ThreadResume(after turn override) error = %v", err)
		}
		secondResult, err := thread.Run(ctx, "inherit previous approval mode", nil)
		if err != nil {
			t.Fatalf("Thread.Run(omitted approval) error = %v", err)
		}
		afterOmittedTurn, err := sdk.Client().ThreadResume(ctx, thread.ID(), &codex.ThreadResumeParams{ThreadID: thread.ID()})
		if err != nil {
			t.Fatalf("Client.ThreadResume(after omitted turn) error = %v", err)
		}

		got := map[string]any{
			"after_turn_override": approvalPolicyString(t, afterTurnOverride.ApprovalPolicy),
			"after_omitted_turn":  approvalPolicyString(t, afterOmittedTurn.ApprovalPolicy),
			"final_responses":     []string{firstResult.FinalResponse, secondResult.FinalResponse},
		}
		want := map[string]any{"after_turn_override": string(codex.AskForApprovalValueNever), "after_omitted_turn": string(codex.AskForApprovalValueNever), "final_responses": []string{"turn override", "turn inherited"}}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("turn approval persistence mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("thread run approval mode persists until explicit override", func(t *testing.T) {
		sdk, ctx := newApprovalPersistenceCodex(t)
		never, denyReviewer := mustApprovalModeSettings(t, codex.ApprovalModeDenyAll)
		onRequest, autoReviewer := mustApprovalModeSettings(t, codex.ApprovalModeAutoReview)

		thread, err := sdk.ThreadStart(ctx, &codex.ThreadStartParams{ApprovalPolicy: &never, ApprovalsReviewer: denyReviewer})
		if err != nil {
			t.Fatalf("ThreadStart(deny_all) error = %v", err)
		}
		firstResult, err := thread.Run(ctx, "keep approvals denied", nil)
		if err != nil {
			t.Fatalf("Thread.Run(default approval) error = %v", err)
		}
		afterDefaultRun, err := sdk.Client().ThreadResume(ctx, thread.ID(), &codex.ThreadResumeParams{ThreadID: thread.ID()})
		if err != nil {
			t.Fatalf("Client.ThreadResume(after default run) error = %v", err)
		}
		secondResult, err := thread.Run(ctx, "allow auto review now", &codex.TurnStartParams{ApprovalPolicy: &onRequest, ApprovalsReviewer: autoReviewer})
		if err != nil {
			t.Fatalf("Thread.Run(auto_review override) error = %v", err)
		}
		afterOverrideRun, err := sdk.Client().ThreadResume(ctx, thread.ID(), &codex.ThreadResumeParams{ThreadID: thread.ID()})
		if err != nil {
			t.Fatalf("Client.ThreadResume(after override run) error = %v", err)
		}

		got := map[string]any{
			"after_default_policy":    approvalPolicyString(t, afterDefaultRun.ApprovalPolicy),
			"after_override_policy":   approvalPolicyString(t, afterOverrideRun.ApprovalPolicy),
			"after_override_reviewer": string(afterOverrideRun.ApprovalsReviewer),
			"final_responses":         []string{firstResult.FinalResponse, secondResult.FinalResponse},
		}
		want := map[string]any{
			"after_default_policy":    string(codex.AskForApprovalValueNever),
			"after_override_policy":   string(codex.AskForApprovalValueOnRequest),
			"after_override_reviewer": string(codex.ApprovalsReviewerAutoReview),
			"final_responses":         []string{"locked down", "reviewable"},
		}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("thread run approval persistence mismatch (-want +got):\n%s", diff)
		}
	})
}

func newApprovalPersistenceCodex(t *testing.T) (*codex.Codex, context.Context) {
	t.Helper()
	sdk := newHelperCodex(t, "approval_persistence")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return sdk, ctx
}

func mustApprovalModeSettings(t *testing.T, mode codex.ApprovalMode) (codex.AskForApproval, *codex.ApprovalsReviewer) {
	t.Helper()
	approval, reviewer, err := codex.ApprovalModeSettings(mode)
	if err != nil {
		t.Fatalf("ApprovalModeSettings(%q) error = %v", mode, err)
	}
	return approval, reviewer
}

func approvalPolicyString(t *testing.T, policy codex.AskForApproval) string {
	t.Helper()
	value, ok := policy.(codex.AskForApprovalValue)
	if !ok {
		t.Fatalf("approval policy = %T(%#v), want AskForApprovalValue", policy, policy)
	}
	return string(value)
}
