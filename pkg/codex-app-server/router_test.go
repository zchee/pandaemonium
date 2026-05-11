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
	"slices"
	"testing"
	"testing/synctest"

	gocmp "github.com/google/go-cmp/cmp"
)

func TestTurnNotificationRouterQueuesPendingBeforeConsumer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)
		handle := &TurnHandle{client: client, threadID: "thread-pending", turnID: "turn-pending"}

		client.routeNotification(Notification{
			Method: NotificationMethodItemCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-pending",
				"turnId":   "turn-pending",
				"item":     Object{"type": "agentMessage", "text": "queued"},
			}),
		})
		client.routeNotification(Notification{
			Method: NotificationMethodTurnCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-pending",
				"turn":     Object{"id": "turn-pending", "status": "completed"},
			}),
		})

		notifications, err := collectStream(handle.Stream(t.Context()))
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		want := []string{NotificationMethodItemCompleted, NotificationMethodTurnCompleted}
		if diff := gocmp.Diff(want, notificationMethods(notifications)); diff != "" {
			t.Fatalf("stream methods mismatch (-want +got):\n%s", diff)
		}
		if got := activeTurnConsumers(client); len(got) != 0 {
			t.Fatalf("active turn consumers = %v, want released after pending stream", got)
		}
	})
}

func TestTurnNotificationRouterKeepsUnscopedEventsGlobal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)
		handle := &TurnHandle{client: client, threadID: "thread-global", turnID: "turn-global"}

		streamResult := collectStreamAsync(handle.Stream(t.Context()))
		synctest.Wait()
		assertActiveTurnConsumer(t, client, handle.ID())

		global := Notification{
			Method: "custom/global",
			Params: mustJSON(t, Object{"phase": "unscoped"}),
		}
		client.routeNotification(global)

		gotGlobal, err := client.NextNotification(t.Context())
		if err != nil {
			t.Fatalf("NextNotification() error = %v", err)
		}
		if gotGlobal.Method != global.Method {
			t.Fatalf("NextNotification() method = %q, want %q", gotGlobal.Method, global.Method)
		}

		client.routeNotification(Notification{
			Method: NotificationMethodTurnCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-global",
				"turn":     Object{"id": "turn-global", "status": "completed"},
			}),
		})
		synctest.Wait()

		result := <-streamResult
		if result.err != nil {
			t.Fatalf("Stream() error = %v", result.err)
		}
		if diff := gocmp.Diff([]string{NotificationMethodTurnCompleted}, notificationMethods(result.notifications)); diff != "" {
			t.Fatalf("stream methods mismatch (-want +got):\n%s", diff)
		}
		assertActiveTurnConsumers(t, client)
	})
}

func TestTurnNotificationRouterAllowsConcurrentDifferentTurnConsumers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)
		first := &TurnHandle{client: client, threadID: "thread-concurrent", turnID: "turn-1"}
		second := &TurnHandle{client: client, threadID: "thread-concurrent", turnID: "turn-2"}

		firstResult := collectStreamAsync(first.Stream(t.Context()))
		secondResult := collectStreamAsync(second.Stream(t.Context()))
		synctest.Wait()
		assertActiveTurnConsumers(t, client, "turn-1", "turn-2")

		client.routeNotification(Notification{
			Method: NotificationMethodItemCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-concurrent",
				"turnId":   "turn-2",
				"item":     Object{"type": "agentMessage", "text": "second"},
			}),
		})
		client.routeNotification(Notification{
			Method: NotificationMethodItemCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-concurrent",
				"turnId":   "turn-1",
				"item":     Object{"type": "agentMessage", "text": "first"},
			}),
		})
		client.routeNotification(Notification{
			Method: NotificationMethodTurnCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-concurrent",
				"turn":     Object{"id": "turn-1", "status": "completed"},
			}),
		})
		client.routeNotification(Notification{
			Method: NotificationMethodTurnCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-concurrent",
				"turn":     Object{"id": "turn-2", "status": "completed"},
			}),
		})
		synctest.Wait()

		gotFirst := <-firstResult
		if gotFirst.err != nil {
			t.Fatalf("first Stream() error = %v", gotFirst.err)
		}
		gotSecond := <-secondResult
		if gotSecond.err != nil {
			t.Fatalf("second Stream() error = %v", gotSecond.err)
		}
		want := []string{NotificationMethodItemCompleted, NotificationMethodTurnCompleted}
		if diff := gocmp.Diff(want, notificationMethods(gotFirst.notifications)); diff != "" {
			t.Fatalf("first stream methods mismatch (-want +got):\n%s", diff)
		}
		if diff := gocmp.Diff(want, notificationMethods(gotSecond.notifications)); diff != "" {
			t.Fatalf("second stream methods mismatch (-want +got):\n%s", diff)
		}
		if turnID := mustTurnID(t, gotFirst.notifications[0]); turnID != "turn-1" {
			t.Fatalf("first stream item turnID = %q, want turn-1", turnID)
		}
		if turnID := mustTurnID(t, gotSecond.notifications[0]); turnID != "turn-2" {
			t.Fatalf("second stream item turnID = %q, want turn-2", turnID)
		}
		assertActiveTurnConsumers(t, client)
	})
}

func assertActiveTurnConsumers(t *testing.T, client *Client, want ...string) {
	t.Helper()
	got := activeTurnConsumers(client)
	if want == nil {
		want = []string{}
	}
	slices.Sort(want)
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Fatalf("active turn consumers mismatch (-want +got):\n%s", diff)
	}
}

func mustTurnID(t *testing.T, notification Notification) string {
	t.Helper()
	item, ok, err := notification.ItemCompleted()
	if err != nil {
		t.Fatalf("ItemCompleted() error = %v", err)
	}
	if !ok {
		t.Fatalf("ItemCompleted() ok = false for %s", notification.Method)
	}
	return item.TurnID
}
