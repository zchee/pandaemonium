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

	"github.com/google/go-cmp/cmp"

	"github.com/zchee/pandaemonium/pkg/codex"
)

type turnControlsSteerSummary struct {
	SteeredTurnID string
	TurnID        string
	AgentMessages []string
}

type turnControlsInterruptSummary struct {
	InterruptResponse codex.TurnInterruptResponse
	InterruptedStatus codex.TurnStatus
	FollowUp          string
}

func TestAppServerTurnControlsPortSteerAddsFollowUpInput(t *testing.T) {
	sdk := newHelperCodex(t, "turn_controls")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	turn, err := thread.Turn(ctx, codex.TextInput{Text: "Start a steerable turn."}, nil)
	if err != nil {
		t.Fatalf("Thread.Turn(steerable) error = %v", err)
	}
	steer, err := turn.Steer(ctx, codex.TextInput{Text: "Use this steering input."})
	if err != nil {
		t.Fatalf("TurnHandle.Steer() error = %v", err)
	}
	events := collectStreamingPortSummary(t, turn.Stream(ctx))

	got := turnControlsSteerSummary{
		SteeredTurnID: steer.TurnID,
		TurnID:        turn.ID(),
		AgentMessages: events.AgentMessages,
	}
	want := turnControlsSteerSummary{
		SteeredTurnID: turn.ID(),
		TurnID:        turn.ID(),
		AgentMessages: []string{"before steer", "after steer"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("steer result mismatch (-want +got):\n%s", diff)
	}
}

func TestAppServerTurnControlsPortInterruptStopsActiveTurnAndFollowUpRuns(t *testing.T) {
	sdk := newHelperCodex(t, "turn_controls")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	interruptedTurn, err := thread.Turn(ctx, codex.TextInput{Text: "Start a long turn."}, nil)
	if err != nil {
		t.Fatalf("Thread.Turn(long) error = %v", err)
	}
	interruptResponse, err := interruptedTurn.Interrupt(ctx)
	if err != nil {
		t.Fatalf("TurnHandle.Interrupt() error = %v", err)
	}
	completed, err := interruptedTurn.Run(ctx)
	if err != nil {
		t.Fatalf("TurnHandle.Run() interrupted turn error = %v", err)
	}
	followUp, err := thread.Run(ctx, "Continue after the interrupt.", nil)
	if err != nil {
		t.Fatalf("Thread.Run(follow-up) error = %v", err)
	}

	got := turnControlsInterruptSummary{
		InterruptResponse: interruptResponse,
		InterruptedStatus: completed.Turn.Status,
		FollowUp:          followUp.FinalResponse,
	}
	want := turnControlsInterruptSummary{
		InterruptResponse: codex.TurnInterruptResponse{},
		InterruptedStatus: codex.TurnStatusInterrupted,
		FollowUp:          "after interrupt",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("interrupt result mismatch (-want +got):\n%s", diff)
	}
}
