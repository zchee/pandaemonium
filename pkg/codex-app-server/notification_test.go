// Copyright 2026 The omxx Authors.
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

	json "github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/google/go-cmp/cmp"

	"github.com/zchee/omxx/pkg/codex-app-server/protocol"
)

func TestDecodeNotificationHelpers(t *testing.T) {
	t.Parallel()

	itemParams, err := json.Marshal(protocol.ItemCompletedNotification{
		ThreadID: "thr-1",
		TurnID:   "turn-1",
		Item:     protocol.ThreadItem(`{"type":"agentMessage","text":"hello"}`),
	})
	if err != nil {
		t.Fatalf("json.Marshal() item params error = %v", err)
	}
	turnParams, err := json.Marshal(protocol.TurnCompletedNotification{
		Turn: protocol.Turn{
			ID:     "turn-1",
			Status: protocol.TurnStatusCompleted,
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
				if diff := cmp.Diff(got, value); diff != "" {
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
				if diff := cmp.Diff(got, known.Value); diff != "" {
					t.Fatalf("known.Value mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tt.notification, known.Raw); diff != "" {
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
				if got.Turn.ID != "turn-1" || got.Turn.Status != protocol.TurnStatusCompleted {
					t.Fatalf("decoded turn notification = %#v, want completed turn-1", got)
				}
				value, ok, err := tt.notification.TurnCompleted()
				if err != nil || !ok {
					t.Fatalf("Notification.TurnCompleted() = (%#v, %v, %v), want success", value, ok, err)
				}
				if diff := cmp.Diff(got, value); diff != "" {
					t.Fatalf("wrapper mismatch (-want +got):\n%s", diff)
				}
				known, matched, err := DecodeKnownNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeKnownNotification() error = %v", err)
				}
				if !matched {
					t.Fatalf("DecodeKnownNotification() matched = false, want true")
				}
				if diff := cmp.Diff(got, known.Value); diff != "" {
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
				if diff := cmp.Diff(tt.notification, known.Raw); diff != "" {
					t.Fatalf("unknown raw mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestDecodeNotificationMethodMismatchAndMalformedParams(t *testing.T) {
	t.Parallel()

	mismatch, ok, err := DecodeNotification[protocol.ErrorNotification](
		Notification{Method: NotificationMethodTurnCompleted, Params: jsontext.Value([]byte(`{"message":"nope"}`))},
		NotificationMethodError,
	)
	if err != nil {
		t.Fatalf("DecodeNotification() mismatch error = %v", err)
	}
	if ok {
		t.Fatalf("DecodeNotification() mismatch ok = true, want false")
	}
	if diff := cmp.Diff(protocol.ErrorNotification{}, mismatch); diff != "" {
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

func mustJSON(t *testing.T, value any) jsontext.Value {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return jsontext.Value(raw)
}
