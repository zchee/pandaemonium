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

package codex

import (
	"context"
	"errors"
	"testing"

	"github.com/go-json-experiment/json"
	gocmp "github.com/google/go-cmp/cmp"
)

func TestDecodeThreadItemRoundTripPreservesNestedSlices(t *testing.T) {
	t.Parallel()

	original := []ThreadItem{
		RawThreadItem(`{"type":"agentMessage","text":"alpha","phase":"draft"}`),
		RawThreadItem(`{"type":"agent_message","text":"beta","phase":"finalAnswer"}`),
		RawThreadItem(`{"type":"unknown","text":"ignored","nested":[{"type":"agentMessage","text":"nested"}]}`),
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded []RawThreadItem
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	got := make([]ThreadItem, len(decoded))
	for i := range decoded {
		got[i] = decoded[i]
	}
	if diff := gocmp.Diff(original, got); diff != "" {
		t.Fatalf("round-trip mismatch (-want +got):\n%s", diff)
	}

	if item, ok := decodeThreadItem(got[2]); !ok {
		t.Fatalf("decodeThreadItem() ok = false, want true for syntactically valid payload")
	} else if item.agentMessage() {
		t.Fatalf("decodeThreadItem() accepted unknown discriminator as agent message: %#v", item)
	}
}

func TestFinalAssistantResponseIgnoresUnknownDiscriminatorAndFallsBack(t *testing.T) {
	t.Parallel()

	items := []ThreadItem{
		RawThreadItem(`{"type":"unknown","text":"ignored"}`),
		RawThreadItem(`{"type":"agentMessage","text":"answer one"}`),
		RawThreadItem(`{"type":"agent_message","text":"answer two","phase":"final_answer"}`),
	}

	if got := finalAssistantResponse(items); got != "answer two" {
		t.Fatalf("finalAssistantResponse() = %q, want %q", got, "answer two")
	}
}

func TestDecodeThreadItemRejectsMalformedPayload(t *testing.T) {
	t.Parallel()

	if _, ok := decodeThreadItem(RawThreadItem(`{"type":"agentMessage","text":"missing brace"`)); ok {
		t.Fatal("decodeThreadItem() ok = true, want false for malformed payload")
	}
}

func TestMergeParamsPreservesBaseThreadTurnFieldsWhenTypedParamsAreZero(t *testing.T) {
	t.Parallel()

	model := "gpt-test"
	got, err := mergeParamsBaseWins(&TurnStartParams{Model: &model}, Object{
		"threadId": "thread-1",
		"input": []Object{
			{"type": "text", "text": "hello"},
		},
	})
	if err != nil {
		t.Fatalf("mergeParamsBaseWins() error = %v", err)
	}
	want := Object{
		"threadId": "thread-1",
		"input": []Object{
			{"type": "text", "text": "hello"},
		},
		"model": "gpt-test",
	}
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Fatalf("mergeParamsBaseWins() mismatch (-want +got):\n%s", diff)
	}
}

func TestMergeParamsBaseWins(t *testing.T) {
	t.Parallel()

	// Caller passes threadId but base must always win — the caller-supplied value
	// must be silently overridden by the injected base value.
	got, err := mergeParamsBaseWins(
		map[string]any{"threadId": "caller-value", "model": "gpt-caller"},
		Object{"threadId": "base-value"},
	)
	if err != nil {
		t.Fatalf("mergeParamsBaseWins() error = %v", err)
	}
	if got["threadId"] != "base-value" {
		t.Fatalf("mergeParamsBaseWins() threadId = %q, want base-value (base must win)", got["threadId"])
	}
	if got["model"] != "gpt-caller" {
		t.Fatalf("mergeParamsBaseWins() model = %q, want gpt-caller (caller value preserved when no conflict)", got["model"])
	}
}

func TestMergeParamsMarshalError(t *testing.T) {
	t.Parallel()

	// channel values are not JSON-serialisable; Marshal must propagate the error.
	_, err := mergeParamsBaseWins(make(chan int), Object{"threadId": "t"})
	if err == nil {
		t.Fatal("mergeParamsBaseWins() error = nil, want marshal error for non-serialisable input")
	}
}

// TestCollectRunResultMissingCompletion verifies AC-3.1: the nil guard on
// completed prevents a nil-pointer dereference when the notification stream
// closes before a TurnCompleted arrives. Previously this would panic; now it
// returns an explicit error.
func TestCollectRunResultMissingCompletion(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	const turnID = "turn-no-complete"
	if err := client.acquireTurnConsumer(turnID); err != nil {
		t.Fatalf("acquireTurnConsumer() error = %v", err)
	}

	// Route one ItemCompleted so the loop makes progress before the stream ends.
	if err := client.routeNotification(Notification{
		Method: NotificationMethodItemCompleted,
		Params: mustJSON(t, Object{
			"threadId": "thread-test",
			"turnId":   turnID,
			"item":     Object{"type": "agentMessage", "text": "hello"},
		}),
	}); err != nil {
		t.Fatalf("routeNotification() error = %v", err)
	}

	// Close the router so the next nextTurnNotification call returns an error
	// rather than blocking — simulating the stream ending before TurnCompleted.
	client.turnRouter.close(&TransportClosedError{Message: "stream ended before TurnCompleted"})

	_, err := collectRunResult(context.Background(), client, turnID)
	if err == nil {
		t.Fatal("collectRunResult() error = nil, want non-nil when stream closes before TurnCompleted")
	}
}

// TestRunReturnsTurnFailedError verifies AC-3.2: collectRunResult returns a
// *TurnFailedError when the server reports Status=failed, and the Unwrap chain
// successfully maps CodexErrorInfo "serverOverloaded" to *ServerBusyError,
// enabling errors.As to reach *ServerBusyError from the top-level error.
func TestRunReturnsTurnFailedError(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	const turnID = "turn-failed-busy"
	if err := client.acquireTurnConsumer(turnID); err != nil {
		t.Fatalf("acquireTurnConsumer() error = %v", err)
	}
	defer client.releaseTurnConsumer(turnID)

	// Route a TurnCompleted with Status=failed and CodexErrorInfo=serverOverloaded.
	if err := client.routeNotification(Notification{
		Method: NotificationMethodTurnCompleted,
		Params: mustJSON(t, Object{
			"threadId": "thread-test",
			"turn": Object{
				"id":     turnID,
				"status": string(TurnStatusFailed),
				"error": Object{
					"message":        "thread busy",
					"codexErrorInfo": string(CodexErrorInfoValueServerOverloaded),
				},
			},
		}),
	}); err != nil {
		t.Fatalf("routeNotification() error = %v", err)
	}

	_, err := collectRunResult(context.Background(), client, turnID)
	if err == nil {
		t.Fatal("collectRunResult() error = nil, want *TurnFailedError")
	}

	var turnFailed *TurnFailedError
	if !errors.As(err, &turnFailed) {
		t.Fatalf("errors.As(*TurnFailedError) = false; err = %v (%T)", err, err)
	}
	if turnFailed.TurnID != turnID {
		t.Errorf("TurnFailedError.TurnID = %q, want %q", turnFailed.TurnID, turnID)
	}
	if turnFailed.Status != TurnStatusFailed {
		t.Errorf("TurnFailedError.Status = %q, want %q", turnFailed.Status, TurnStatusFailed)
	}
	if turnFailed.Err == nil || turnFailed.Err.Message != "thread busy" {
		t.Errorf("TurnFailedError.Err.Message = %q, want %q", func() string {
			if turnFailed.Err == nil {
				return "<nil>"
			}
			return turnFailed.Err.Message
		}(), "thread busy")
	}

	// The Unwrap chain must surface *ServerBusyError for "serverOverloaded".
	var busy *ServerBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("errors.As(*ServerBusyError) = false; Unwrap chain broken for serverOverloaded CodexErrorInfo")
	}
}

// TestRunFailedErrorWithoutCodexErrorInfo verifies AC-3.2: when the turn fails
// without a CodexErrorInfo, Unwrap() returns nil (no further chain), but
// errors.As still finds the *TurnFailedError wrapper at the top level.
func TestRunFailedErrorWithoutCodexErrorInfo(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	const turnID = "turn-failed-no-info"
	if err := client.acquireTurnConsumer(turnID); err != nil {
		t.Fatalf("acquireTurnConsumer() error = %v", err)
	}
	defer client.releaseTurnConsumer(turnID)

	// Route a TurnCompleted with Status=failed but no CodexErrorInfo.
	if err := client.routeNotification(Notification{
		Method: NotificationMethodTurnCompleted,
		Params: mustJSON(t, Object{
			"threadId": "thread-test",
			"turn": Object{
				"id":     turnID,
				"status": string(TurnStatusFailed),
				"error": Object{
					"message": "unknown failure",
				},
			},
		}),
	}); err != nil {
		t.Fatalf("routeNotification() error = %v", err)
	}

	_, err := collectRunResult(context.Background(), client, turnID)
	if err == nil {
		t.Fatal("collectRunResult() error = nil, want *TurnFailedError")
	}

	var turnFailed *TurnFailedError
	if !errors.As(err, &turnFailed) {
		t.Fatalf("errors.As(*TurnFailedError) = false; err = %v (%T)", err, err)
	}

	// Without CodexErrorInfo, Unwrap() must return nil.
	if unwrapped := turnFailed.Unwrap(); unwrapped != nil {
		t.Errorf("TurnFailedError.Unwrap() = %v (%T), want nil when CodexErrorInfo absent", unwrapped, unwrapped)
	}

	// No *ServerBusyError should be reachable in the chain.
	if _, ok := errors.AsType[*ServerBusyError](err); ok {
		t.Errorf("errors.As(*ServerBusyError) = true, want false when no CodexErrorInfo")
	}
}
