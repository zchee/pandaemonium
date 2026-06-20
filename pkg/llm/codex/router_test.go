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
	"fmt"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/go-json-experiment/json/jsontext"
	gocmp "github.com/google/go-cmp/cmp"
)

func TestNotificationRingPreservesFIFOAcrossWraparound(t *testing.T) {
	t.Parallel()

	ring := newNotificationRing(3)
	for _, method := range []string{"one", "two", "three"} {
		if ok := ring.push(Notification{Method: method}); !ok {
			t.Fatalf("push(%q) ok = false, want true", method)
		}
	}
	if got, ok := ring.pop(); !ok || got.Method != "one" {
		t.Fatalf("first pop = (%q, %t), want (one, true)", got.Method, ok)
	}
	if ok := ring.push(Notification{Method: "four"}); !ok {
		t.Fatal("push(four) after wrap ok = false, want true")
	}

	var got []string
	for {
		notification, ok := ring.pop()
		if !ok {
			break
		}
		got = append(got, notification.Method)
	}
	if diff := gocmp.Diff([]string{"two", "three", "four"}, got); diff != "" {
		t.Fatalf("wrapped FIFO mismatch (-want +got):\n%s", diff)
	}
	if ring.len() != 0 {
		t.Fatalf("ring len after draining = %d, want 0", ring.len())
	}
	if notification, ok := ring.pop(); ok || notification.Method != "" {
		t.Fatalf("empty pop = (%#v, %t), want zero notification and false", notification, ok)
	}
}

func TestNotificationRingRejectsOverflowAndClearsPoppedSlots(t *testing.T) {
	t.Parallel()

	ring := newNotificationRing(1)
	if ok := ring.push(Notification{Method: "one", Params: mustJSON(t, Object{"payload": "large"})}); !ok {
		t.Fatal("first push ok = false, want true")
	}
	if ok := ring.push(Notification{Method: "two"}); ok {
		t.Fatal("second push ok = true, want false for full ring")
	}
	notification, ok := ring.pop()
	if !ok || notification.Method != "one" {
		t.Fatalf("pop after full = (%q, %t), want (one, true)", notification.Method, ok)
	}
	if got := ring.values[0]; got.Method != "" || len(got.Params) != 0 {
		t.Fatalf("popped slot retained notification = %#v, want zero value", got)
	}
	if ok := ring.push(Notification{Method: "three"}); !ok {
		t.Fatal("push after clearing slot ok = false, want true")
	}
}

func TestNotificationRingAppendsPendingInOrder(t *testing.T) {
	t.Parallel()

	ring := newNotificationRing(notificationQueueCapacity)
	pending := []Notification{
		{Method: "pending/one"},
		{Method: "pending/two"},
		{Method: "pending/three"},
	}
	if ok := ring.appendAll(pending); !ok {
		t.Fatal("appendAll(pending) ok = false, want true")
	}
	if ring.len() != len(pending) {
		t.Fatalf("ring len after append = %d, want %d", ring.len(), len(pending))
	}

	var got []string
	for range pending {
		notification, ok := ring.pop()
		if !ok {
			t.Fatal("pop after append ok = false, want true")
		}
		got = append(got, notification.Method)
	}
	if diff := gocmp.Diff([]string{"pending/one", "pending/two", "pending/three"}, got); diff != "" {
		t.Fatalf("appended pending order mismatch (-want +got):\n%s", diff)
	}
}

func TestNotificationRingRejectsAppendPastCapacity(t *testing.T) {
	t.Parallel()

	ring := newNotificationRing(notificationQueueCapacity)
	for i := range notificationQueueCapacity {
		if ok := ring.push(Notification{Method: "queued"}); !ok {
			t.Fatalf("push(%d) ok = false, want true before capacity", i)
		}
	}
	if ok := ring.push(Notification{Method: "overflow"}); ok {
		t.Fatal("push past notificationQueueCapacity ok = true, want false")
	}
	if ok := ring.appendAll([]Notification{{Method: "overflow"}}); ok {
		t.Fatal("appendAll past notificationQueueCapacity ok = true, want false")
	}
	if ring.len() != notificationQueueCapacity {
		t.Fatalf("ring len after rejected append = %d, want %d", ring.len(), notificationQueueCapacity)
	}
}

func TestPushPendingNotification(t *testing.T) {
	t.Parallel()

	t.Run("success: appends before capacity", func(t *testing.T) {
		t.Parallel()

		pending, dropped := pushPendingNotification(nil, Notification{Method: "pending/one"})
		if dropped {
			t.Fatal("pushPendingNotification() dropped = true, want false before capacity")
		}
		if diff := gocmp.Diff([]string{"pending/one"}, notificationMethods(pending)); diff != "" {
			t.Fatalf("pending methods mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("success: drops oldest in place at capacity", func(t *testing.T) {
		t.Parallel()

		pending := make([]Notification, 0, notificationQueueCapacity)
		for i := range notificationQueueCapacity {
			pending = append(pending, Notification{
				Method: fmt.Sprintf("pending/%03d", i),
				Params: mustJSON(t, Object{
					"payload": fmt.Sprintf("retention-sentinel-%03d", i),
				}),
			})
		}
		backing := &pending[0]

		updated, dropped := pushPendingNotification(pending, Notification{Method: "pending/overflow"})
		if !dropped {
			t.Fatal("pushPendingNotification() dropped = false, want true at capacity")
		}
		if len(updated) != notificationQueueCapacity {
			t.Fatalf("updated len = %d, want %d", len(updated), notificationQueueCapacity)
		}
		if &updated[0] != backing {
			t.Fatal("pushPendingNotification() moved backing array, want in-place reuse")
		}
		if updated[0].Method != "pending/001" {
			t.Fatalf("updated first method = %q, want pending/001", updated[0].Method)
		}
		if updated[len(updated)-1].Method != "pending/overflow" {
			t.Fatalf("updated last method = %q, want pending/overflow", updated[len(updated)-1].Method)
		}
		for i, notification := range updated {
			if notification.Method == "pending/000" {
				t.Fatalf("updated[%d] retained oldest notification, want it evicted", i)
			}
		}
	})
}

func TestNotificationTurnIDExtractsSupportedShapes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		params jsontext.Value
		want   string
	}{
		"success: top-level camel case": {
			params: jsontext.Value(`{"threadId":"thread","turnId":"turn-camel"}`),
			want:   "turn-camel",
		},
		"success: top-level snake case": {
			params: jsontext.Value(`{"threadId":"thread","turn_id":"turn-snake"}`),
			want:   "turn-snake",
		},
		"success: nested turn id": {
			params: jsontext.Value(`{"threadId":"thread","turn":{"id":"turn-nested","status":"completed"}}`),
			want:   "turn-nested",
		},
		"success: nested explicit turn id wins over nested id": {
			params: jsontext.Value(`{"threadId":"thread","turn":{"id":"turn-id","turnId":"turn-explicit","status":"completed"}}`),
			want:   "turn-explicit",
		},
		"success: escaped field falls back to full decoder": {
			params: jsontext.Value(`{"threadId":"thread","turn\u0049d":"turn-\u0031"}`),
			want:   "turn-1",
		},
		"success: no turn id remains global": {
			params: jsontext.Value(`{"threadId":"thread","phase":"global","payload":[1,2,3]}`),
			want:   "",
		},
		"success: null params remain global": {
			params: jsontext.Value(`null`),
			want:   "",
		},
		"error: malformed params remain global": {
			params: jsontext.Value(`{"threadId":"thread","turnId":`),
			want:   "",
		},
		"error: non-string turn id remains global": {
			params: jsontext.Value(`{"threadId":"thread","turnId":42}`),
			want:   "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := notificationTurnID(Notification{Method: NotificationMethodItemCompleted, Params: tt.params})
			if got != tt.want {
				t.Fatalf("notificationTurnID() = %q, want %q", got, tt.want)
			}
		})
	}
}

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

func TestTurnNotificationRouterProcessClosedNilErrorFallback(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		call func(*turnNotificationRouter) error
	}{
		"success: next process": {
			call: func(router *turnNotificationRouter) error {
				_, err := router.nextProcess(t.Context(), "process-closed")
				return err
			},
		},
		"success: register process": {
			call: func(router *turnNotificationRouter) error {
				err := router.registerProcess("process-closed")
				return err
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			router := newTurnNotificationRouter()
			router.close(nil)

			err := tt.call(router)
			assertProcessRouterClosedError(t, err)
		})
	}
}

func assertProcessRouterClosedError(t *testing.T, err error) {
	t.Helper()
	var closedErr *TransportClosedError
	if !errors.As(err, &closedErr) {
		t.Fatalf("process router closed error = %v (%T), want *TransportClosedError", err, err)
	}
	if closedErr.Message != "process notification router closed" {
		t.Fatalf("TransportClosedError.Message = %q, want process notification router closed", closedErr.Message)
	}
}

func TestTurnNotificationRouterProcessCloseNilErrorUnblocksActiveWaiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := newTurnNotificationRouter()
		if err := router.registerProcess("process-waiting"); err != nil {
			t.Fatalf("registerProcess() error = %v", err)
		}

		errs := make(chan error, 1)
		go func() {
			_, err := router.nextProcess(t.Context(), "process-waiting")
			errs <- err
		}()
		synctest.Wait()

		router.close(nil)
		synctest.Wait()

		select {
		case err := <-errs:
			assertProcessRouterClosedError(t, err)
		default:
			t.Fatal("nextProcess() waiter remained blocked after router.close(nil)")
		}
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

func TestTurnNotificationRouterDropsOldestWhenActiveTurnQueueFull(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	if _, err := client.openTurnConsumer("turn-full"); err != nil {
		t.Fatalf("openTurnConsumer() error = %v", err)
	}

	// Fill queue to capacity — all succeed, no error.
	for i := range notificationQueueCapacity {
		if err := client.routeNotification(Notification{
			Method: NotificationMethodItemCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-full",
				"turnId":   "turn-full",
				"item":     Object{"type": "agentMessage", "text": fmt.Sprintf("item-%d", i)},
			}),
		}); err != nil {
			t.Fatalf("routeNotification(%d) error = %v", i, err)
		}
	}

	// One more — overflow. Must NOT error; drops oldest instead.
	if err := client.routeNotification(Notification{
		Method: NotificationMethodItemCompleted,
		Params: mustJSON(t, Object{
			"threadId": "thread-full",
			"turnId":   "turn-full",
			"item":     Object{"type": "agentMessage", "text": "overflow"},
		}),
	}); err != nil {
		t.Fatalf("routeNotification() overflow error = %v, want nil (drop-oldest)", err)
	}

	// Router must NOT be closed — NextNotification on global still works.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := client.NextNotification(ctx)
	// Context was cancelled; router is healthy so we get context.Canceled, NOT a router-failure error.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NextNotification() after overflow error = %v, want context.Canceled (router healthy)", err)
	}

	// Consumer's next call sees the drop error (exactly once).
	_, err = client.nextTurnNotification(t.Context(), "turn-full")
	var dropErr *NotificationDroppedError
	if !errors.As(err, &dropErr) {
		t.Fatalf("nextTurnNotification() after overflow error = %v (%T), want *NotificationDroppedError", err, err)
	}
	if dropErr.Dropped != 1 {
		t.Fatalf("NotificationDroppedError.Dropped = %d, want 1", dropErr.Dropped)
	}
	if dropErr.TurnID != "turn-full" {
		t.Fatalf("NotificationDroppedError.TurnID = %q, want turn-full", dropErr.TurnID)
	}

	// Second call: no more drops pending, returns first surviving notification normally.
	notification, err := client.nextTurnNotification(t.Context(), "turn-full")
	if err != nil {
		t.Fatalf("nextTurnNotification() second call error = %v, want surviving notification", err)
	}
	if notification.Method != NotificationMethodItemCompleted {
		t.Fatalf("surviving notification method = %q, want %s", notification.Method, NotificationMethodItemCompleted)
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
