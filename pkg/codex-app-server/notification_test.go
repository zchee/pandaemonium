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

package codexappserver

import (
	"testing"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	gocmp "github.com/google/go-cmp/cmp"
)

func TestDecodeNotificationHelpers(t *testing.T) {
	t.Parallel()

	itemParams, err := json.Marshal(ItemCompletedNotification{
		ThreadID: "thr-1",
		TurnID:   "turn-1",
		Item:     RawThreadItem(`{"type":"agentMessage","text":"hello"}`),
	})
	if err != nil {
		t.Fatalf("json.Marshal() item params error = %v", err)
	}
	turnParams, err := json.Marshal(TurnCompletedNotification{
		Turn: Turn{
			ID:     "turn-1",
			Status: TurnStatusCompleted,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() turn params error = %v", err)
	}

	tests := map[string]struct {
		notification Notification
	}{
		"success: decode typed item completed helper": {
			notification: Notification{Method: NotificationMethodItemCompleted, Params: itemParams},
		},
		"success: decode typed turn completed helper": {
			notification: Notification{Method: NotificationMethodTurnCompleted, Params: turnParams},
		},
		"success: preserve raw unknown notification": {
			notification: Notification{Method: "thread/custom", Params: jsontext.Value([]byte(`{"hello":"world"}`))},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			switch tt.notification.Method {
			case NotificationMethodItemCompleted:
				got, ok, err := DecodeItemCompletedNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeItemCompletedNotification() error = %v", err)
				}
				if !ok {
					t.Fatalf("DecodeItemCompletedNotification() ok = false, want true")
				}
				if got.ThreadID != "thr-1" || got.TurnID != "turn-1" {
					t.Fatalf("decoded item notification = %#v, want thr-1/turn-1", got)
				}
				value, ok, err := tt.notification.ItemCompleted()
				if err != nil || !ok {
					t.Fatalf("Notification.ItemCompleted() = (%#v, %v, %v), want success", value, ok, err)
				}
				if diff := gocmp.Diff(got, value); diff != "" {
					t.Fatalf("wrapper mismatch (-want +got):\n%s", diff)
				}
				known, matched, err := DecodeKnownNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeKnownNotification() error = %v", err)
				}
				if !matched {
					t.Fatalf("DecodeKnownNotification() matched = false, want true")
				}
				if known.Method != NotificationMethodItemCompleted {
					t.Fatalf("known.Method = %q, want %q", known.Method, NotificationMethodItemCompleted)
				}
				if diff := gocmp.Diff(got, known.Value); diff != "" {
					t.Fatalf("known.Value mismatch (-want +got):\n%s", diff)
				}
				if diff := gocmp.Diff(tt.notification, known.Raw); diff != "" {
					t.Fatalf("known.Raw mismatch (-want +got):\n%s", diff)
				}
			case NotificationMethodTurnCompleted:
				got, ok, err := DecodeTurnCompletedNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeTurnCompletedNotification() error = %v", err)
				}
				if !ok {
					t.Fatalf("DecodeTurnCompletedNotification() ok = false, want true")
				}
				if got.Turn.ID != "turn-1" || got.Turn.Status != TurnStatusCompleted {
					t.Fatalf("decoded turn notification = %#v, want completed turn-1", got)
				}
				value, ok, err := tt.notification.TurnCompleted()
				if err != nil || !ok {
					t.Fatalf("Notification.TurnCompleted() = (%#v, %v, %v), want success", value, ok, err)
				}
				if diff := gocmp.Diff(got, value); diff != "" {
					t.Fatalf("wrapper mismatch (-want +got):\n%s", diff)
				}
				known, matched, err := DecodeKnownNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeKnownNotification() error = %v", err)
				}
				if !matched {
					t.Fatalf("DecodeKnownNotification() matched = false, want true")
				}
				if diff := gocmp.Diff(got, known.Value); diff != "" {
					t.Fatalf("known.Value mismatch (-want +got):\n%s", diff)
				}
			default:
				known, matched, err := DecodeKnownNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeKnownNotification() error = %v", err)
				}
				if matched {
					t.Fatalf("DecodeKnownNotification() matched = true, want false")
				}
				if diff := gocmp.Diff(tt.notification, known.Raw); diff != "" {
					t.Fatalf("unknown raw mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestDecodeProcessNotifications(t *testing.T) {
	t.Parallel()

	exitedParams := mustJSON(t, ProcessExitedNotification{
		ProcessHandle:    "proc-1",
		ExitCode:         7,
		Stdout:           "stdout",
		Stderr:           "stderr",
		StdoutCapReached: true,
	})
	outputDeltaParams := mustJSON(t, ProcessOutputDeltaNotification{
		ProcessHandle: "proc-1",
		Stream:        ProcessOutputStream(`"stdout"`),
		DeltaBase64:   "aGVsbG8=",
		CapReached:    true,
	})

	tests := map[string]struct {
		notification Notification
		assert       func(*testing.T, Notification, KnownNotification)
	}{
		"process exited": {
			notification: Notification{Method: NotificationMethodProcessExited, Params: exitedParams},
			assert: func(t *testing.T, notification Notification, known KnownNotification) {
				t.Helper()

				got, ok, err := DecodeProcessExitedNotification(notification)
				if err != nil {
					t.Fatalf("DecodeProcessExitedNotification() error = %v", err)
				}
				if !ok {
					t.Fatalf("DecodeProcessExitedNotification() ok = false, want true")
				}
				if got.ProcessHandle != "proc-1" || got.ExitCode != 7 || !got.StdoutCapReached {
					t.Fatalf("decoded process exited notification = %#v, want proc-1 exit 7 stdout cap", got)
				}
				value, ok, err := notification.ProcessExited()
				if err != nil || !ok {
					t.Fatalf("Notification.ProcessExited() = (%#v, %v, %v), want success", value, ok, err)
				}
				if diff := gocmp.Diff(got, value); diff != "" {
					t.Fatalf("wrapper mismatch (-want +got):\n%s", diff)
				}
				if diff := gocmp.Diff(got, known.Value); diff != "" {
					t.Fatalf("known.Value mismatch (-want +got):\n%s", diff)
				}
			},
		},
		"process output delta": {
			notification: Notification{Method: NotificationMethodProcessOutputDelta, Params: outputDeltaParams},
			assert: func(t *testing.T, notification Notification, known KnownNotification) {
				t.Helper()

				got, ok, err := DecodeProcessOutputDeltaNotification(notification)
				if err != nil {
					t.Fatalf("DecodeProcessOutputDeltaNotification() error = %v", err)
				}
				if !ok {
					t.Fatalf("DecodeProcessOutputDeltaNotification() ok = false, want true")
				}
				if got.ProcessHandle != "proc-1" || got.DeltaBase64 != "aGVsbG8=" || !got.CapReached {
					t.Fatalf("decoded process output delta notification = %#v, want proc-1 capped hello chunk", got)
				}
				if string(got.Stream) != `"stdout"` {
					t.Fatalf("decoded process output stream = %s, want stdout", got.Stream)
				}
				value, ok, err := notification.ProcessOutputDelta()
				if err != nil || !ok {
					t.Fatalf("Notification.ProcessOutputDelta() = (%#v, %v, %v), want success", value, ok, err)
				}
				if diff := gocmp.Diff(got, value); diff != "" {
					t.Fatalf("wrapper mismatch (-want +got):\n%s", diff)
				}
				if diff := gocmp.Diff(got, known.Value); diff != "" {
					t.Fatalf("known.Value mismatch (-want +got):\n%s", diff)
				}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			known, matched, err := DecodeKnownNotification(tt.notification)
			if err != nil {
				t.Fatalf("DecodeKnownNotification() error = %v", err)
			}
			if !matched {
				t.Fatalf("DecodeKnownNotification() matched = false, want true")
			}
			if known.Method != tt.notification.Method {
				t.Fatalf("known.Method = %q, want %q", known.Method, tt.notification.Method)
			}
			if diff := gocmp.Diff(tt.notification, known.Raw); diff != "" {
				t.Fatalf("known.Raw mismatch (-want +got):\n%s", diff)
			}
			tt.assert(t, tt.notification, known)
		})
	}
}

func TestDecodeKnownProcessNotificationMalformedParamsPreservesRaw(t *testing.T) {
	t.Parallel()

	notification := Notification{
		Method: NotificationMethodProcessExited,
		Params: jsontext.Value([]byte(`{"processHandle":true}`)),
	}

	known, matched, err := DecodeKnownNotification(notification)
	if !matched {
		t.Fatalf("DecodeKnownNotification() matched = false, want true")
	}
	if err == nil {
		t.Fatalf("DecodeKnownNotification() err = nil, want malformed params error")
	}
	if known.Method != NotificationMethodProcessExited {
		t.Fatalf("known.Method = %q, want %q", known.Method, NotificationMethodProcessExited)
	}
	if diff := gocmp.Diff(notification, known.Raw); diff != "" {
		t.Fatalf("DecodeKnownNotification() raw mismatch (-want +got):\n%s", diff)
	}
}

func TestKnownNotificationMethodsMatchesExpectedInventory(t *testing.T) {
	t.Parallel()

	if diff := gocmp.Diff(expectedNotificationMethods, KnownNotificationMethods()); diff != "" {
		t.Fatalf("KnownNotificationMethods() mismatch (-want +got):\n%s", diff)
	}
}

func TestDecodeNotificationMethodMismatchAndMalformedParams(t *testing.T) {
	t.Parallel()

	mismatch, ok, err := DecodeNotificationAs[ErrorNotification](
		Notification{Method: NotificationMethodTurnCompleted, Params: jsontext.Value([]byte(`{"message":"nope"}`))},
		NotificationMethodError,
	)
	if err != nil {
		t.Fatalf("DecodeNotification() mismatch error = %v", err)
	}
	if ok {
		t.Fatalf("DecodeNotification() mismatch ok = true, want false")
	}
	if diff := gocmp.Diff(ErrorNotification{}, mismatch); diff != "" {
		t.Fatalf("DecodeNotification() mismatch value (-want +got):\n%s", diff)
	}

	_, ok, err = DecodeErrorNotification(Notification{
		Method: NotificationMethodError,
		Params: jsontext.Value([]byte(`{"missing":"fields"`)),
	})
	if !ok {
		t.Fatalf("DecodeErrorNotification() malformed ok = false, want true")
	}
	if err == nil {
		t.Fatalf("DecodeErrorNotification() malformed err = nil, want error")
	}
}

func TestDecodeKnownNotificationUnknownMethodPreservesNestedRaw(t *testing.T) {
	t.Parallel()

	notification := Notification{
		Method: "item/custom",
		Params: jsontext.Value([]byte(`{"items":[{"id":"one"},{"id":"two"}],"nested":{"values":[1,null,2]}}`)),
	}

	known, matched, err := DecodeKnownNotification(notification)
	if err != nil {
		t.Fatalf("DecodeKnownNotification() error = %v", err)
	}
	if matched {
		t.Fatalf("DecodeKnownNotification() matched = true, want false")
	}
	if diff := gocmp.Diff(notification, known.Raw); diff != "" {
		t.Fatalf("DecodeKnownNotification() raw mismatch (-want +got):\n%s", diff)
	}
}

func TestTurnCompletedNotificationRoundTripPreservesThreadItemSlices(t *testing.T) {
	t.Parallel()

	original := TurnCompletedNotification{
		ThreadID: "thr-1",
		Turn: Turn{
			ID:     "turn-1",
			Status: TurnStatusCompleted,
			Items: []ThreadItem{
				RawThreadItem(jsontext.Value(`{"type":"agentMessage","text":"hello","meta":{"source":"assistant"}}`)),
				RawThreadItem(jsontext.Value(`["nested",{"kind":"union"}]`)),
			},
		},
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded TurnCompletedNotification
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if diff := gocmp.Diff(original, decoded); diff != "" {
		t.Fatalf("turn completed round-trip mismatch (-want +got):\n%s", diff)
	}
	nestedRaw, err := json.Marshal(decoded.Turn.Items[1])
	if err != nil {
		t.Fatalf("json.Marshal() nested item error = %v", err)
	}
	if got := string(nestedRaw); got != `["nested",{"kind":"union"}]` {
		t.Fatalf("nested slice item = %s, want preserved raw json", got)
	}
}

func mustJSON(t *testing.T, value any) jsontext.Value {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return jsontext.Value(raw)
}
