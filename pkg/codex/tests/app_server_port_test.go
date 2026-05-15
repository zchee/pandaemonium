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

	"github.com/google/go-cmp/cmp"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestAppServerHarnessLifecycleApprovalsAndInputsPort(t *testing.T) {
	sdk := newHelperCodex(t, "lifecycle_inputs")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	metadata := sdk.Metadata()
	if metadata.ServerInfo == nil || metadata.ServerInfo.Name != "codex-test" || metadata.ServerInfo.Version != "1.2.3" {
		t.Fatalf("Metadata() = %#v, want codex-test/1.2.3 parsed from user agent", metadata)
	}

	approval, reviewer, err := codex.ApprovalModeSettings(codex.ApprovalModeDenyAll)
	if err != nil {
		t.Fatalf("ApprovalModeSettings(deny_all) error = %v", err)
	}
	thread, err := sdk.ThreadStart(ctx, &codex.ThreadStartParams{
		ApprovalPolicy:    &approval,
		ApprovalsReviewer: reviewer,
	})
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	if thread.ID() != "thread-1" {
		t.Fatalf("ThreadStart().ID() = %q, want thread-1", thread.ID())
	}

	model := "gpt-test-override"
	result, err := thread.Run(ctx, []codex.InputItem{
		codex.TextInput{Text: "Describe these inputs."},
		codex.ImageInput{URL: "https://example.com/codex.png"},
		codex.LocalImageInput{Path: "/tmp/codex.png"},
		codex.SkillInput{Name: "demo", Path: "/tmp/demo/SKILL.md"},
		codex.MentionInput{Name: "README", Path: "/tmp/README.md"},
	}, &codex.TurnStartParams{Model: &model})
	if err != nil {
		t.Fatalf("Thread.Run() with normalized inputs error = %v", err)
	}
	if result.FinalResponse != "final for turn-life-1" {
		t.Fatalf("RunResult.FinalResponse = %q, want final for turn-life-1", result.FinalResponse)
	}
	if result.Usage == nil || result.Usage.Last.InputTokens != 13 || result.Usage.Last.TotalTokens != 20 {
		t.Fatalf("RunResult.Usage = %#v, want token usage from helper app-server", result.Usage)
	}

	resumed, err := sdk.ThreadResume(ctx, thread.ID(), &codex.ThreadResumeParams{ThreadID: thread.ID()})
	if err != nil {
		t.Fatalf("ThreadResume() error = %v", err)
	}
	forked, err := sdk.ThreadFork(ctx, thread.ID(), nil)
	if err != nil {
		t.Fatalf("ThreadFork() error = %v", err)
	}
	if got := []string{resumed.ID(), forked.ID()}; !cmp.Equal(got, []string{"thread-1", "thread-1-fork"}) {
		t.Fatalf("resume/fork ids mismatch (-want +got):\n%s", cmp.Diff([]string{"thread-1", "thread-1-fork"}, got))
	}

	if _, err := sdk.ThreadArchive(ctx, thread.ID()); err != nil {
		t.Fatalf("ThreadArchive() error = %v", err)
	}
	unarchived, err := sdk.ThreadUnarchive(ctx, thread.ID())
	if err != nil {
		t.Fatalf("ThreadUnarchive() error = %v", err)
	}
	if unarchived.ID() != thread.ID() {
		t.Fatalf("ThreadUnarchive().ID() = %q, want %q", unarchived.ID(), thread.ID())
	}
	if _, err := thread.SetName(ctx, "ported Python SDK tests"); err != nil {
		t.Fatalf("Thread.SetName() error = %v", err)
	}
	includeTurns := true
	read, err := thread.Read(ctx, &codex.ThreadReadParams{IncludeTurns: &includeTurns})
	if err != nil {
		t.Fatalf("Thread.Read() error = %v", err)
	}
	if read.Thread.ID != thread.ID() {
		t.Fatalf("Thread.Read().Thread.ID = %q, want %q", read.Thread.ID, thread.ID())
	}
	if _, err := thread.Compact(ctx); err != nil {
		t.Fatalf("Thread.Compact() error = %v", err)
	}

	list, err := sdk.ThreadList(ctx, &codex.ThreadListParams{})
	if err != nil {
		t.Fatalf("ThreadList() error = %v", err)
	}
	includeHidden := true
	models, err := sdk.Models(ctx, &codex.ModelListParams{IncludeHidden: &includeHidden})
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	if got := map[string]int{"threads": len(list.Data), "models": len(models.Data)}; !cmp.Equal(got, map[string]int{"threads": 2, "models": 1}) {
		t.Fatalf("list/model counts mismatch (-want +got):\n%s", cmp.Diff(map[string]int{"threads": 2, "models": 1}, got))
	}
}

func TestAppServerHarnessStreamAndControlsPort(t *testing.T) {
	sdk := newHelperCodex(t, "stream_controls")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	turn, err := thread.Turn(ctx, codex.TextInput{Text: "stream please"}, nil)
	if err != nil {
		t.Fatalf("Thread.Turn(stream) error = %v", err)
	}
	var deltas []string
	for event, err := range turn.Stream(ctx) {
		if err != nil {
			t.Fatalf("TurnHandle.Stream() error = %v", err)
		}
		delta, ok, err := event.ItemAgentMessageDelta()
		if err != nil {
			t.Fatalf("AgentMessageDelta() error = %v", err)
		}
		if ok {
			deltas = append(deltas, delta.Delta)
		}
	}
	if diff := cmp.Diff([]string{"he", "llo"}, deltas); diff != "" {
		t.Fatalf("stream deltas mismatch (-want +got):\n%s", diff)
	}

	steerTurn, err := thread.Turn(ctx, "start steerable turn", nil)
	if err != nil {
		t.Fatalf("Thread.Turn(steer) error = %v", err)
	}
	steerResponse, err := steerTurn.Steer(ctx, codex.TextInput{Text: "steer now"})
	if err != nil {
		t.Fatalf("TurnHandle.Steer() error = %v", err)
	}
	if steerResponse.TurnID != steerTurn.ID() {
		t.Fatalf("TurnHandle.Steer().TurnID = %q, want %q", steerResponse.TurnID, steerTurn.ID())
	}
	steered, err := steerTurn.Run(ctx)
	if err != nil {
		t.Fatalf("TurnHandle.Run() after steer error = %v", err)
	}
	if steered.FinalResponse != "steered final" {
		t.Fatalf("steered FinalResponse = %q, want steered final", steered.FinalResponse)
	}

	interruptTurn, err := thread.Turn(ctx, "start interrupt turn", nil)
	if err != nil {
		t.Fatalf("Thread.Turn(interrupt) error = %v", err)
	}
	if _, err := interruptTurn.Interrupt(ctx); err != nil {
		t.Fatalf("TurnHandle.Interrupt() error = %v", err)
	}
	interrupted, err := interruptTurn.Run(ctx)
	if err != nil {
		t.Fatalf("TurnHandle.Run() after interrupt error = %v", err)
	}
	if interrupted.Turn.Status != codex.TurnStatusInterrupted {
		t.Fatalf("interrupted status = %q, want interrupted", interrupted.Turn.Status)
	}
	followUp, err := thread.Run(ctx, "continue after interrupt", nil)
	if err != nil {
		t.Fatalf("Thread.Run() after interrupt error = %v", err)
	}
	if followUp.FinalResponse != "after interrupt" {
		t.Fatalf("follow-up FinalResponse = %q, want after interrupt", followUp.FinalResponse)
	}

	_, err = thread.Run(ctx, "fail this turn", nil)
	if err == nil || !strings.Contains(err.Error(), "mock failure") {
		t.Fatalf("Thread.Run(failed) error = %v, want mock failure", err)
	}
}
