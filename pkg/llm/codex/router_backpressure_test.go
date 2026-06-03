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
	"strings"
	"testing"
)

// TestRouterBackpressureLocal validates per-turn drop isolation (AC-1.7):
// turn A overflows, turn B is unaffected, drop error surfaced exactly once,
// and the router remains functional after overflow.
func TestRouterBackpressureLocal(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)

	// Register both consumers before routing.
	if _, err := client.openTurnConsumer("turn-a"); err != nil {
		t.Fatalf("openTurnConsumer(turn-a) error = %v", err)
	}
	if _, err := client.openTurnConsumer("turn-b"); err != nil {
		t.Fatalf("openTurnConsumer(turn-b) error = %v", err)
	}

	// Route 2× capacity for turn-a — no error returned (drop-oldest).
	for i := range notificationQueueCapacity * 2 {
		if err := client.routeNotification(Notification{
			Method: NotificationMethodItemCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-a",
				"turnId":   "turn-a",
				"item":     Object{"type": "agentMessage", "text": fmt.Sprintf("a-%d", i)},
			}),
		}); err != nil {
			t.Fatalf("routeNotification(turn-a, %d) error = %v, want nil", i, err)
		}
	}

	// Route capacity notifications for turn-b — all succeed, turn-b unaffected.
	for i := range notificationQueueCapacity {
		if err := client.routeNotification(Notification{
			Method: NotificationMethodItemCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-b",
				"turnId":   "turn-b",
				"item":     Object{"type": "agentMessage", "text": fmt.Sprintf("b-%d", i)},
			}),
		}); err != nil {
			t.Fatalf("routeNotification(turn-b, %d) error = %v, want nil", i, err)
		}
	}

	// Turn-A: first next() surfaces drop error for exactly the overflowed count.
	_, err := client.nextTurnNotification(t.Context(), "turn-a")
	var dropErr *NotificationDroppedError
	if !errors.As(err, &dropErr) {
		t.Fatalf("nextTurnNotification(turn-a) error = %v (%T), want *NotificationDroppedError", err, err)
	}
	if dropErr.Dropped != notificationQueueCapacity {
		t.Fatalf("NotificationDroppedError.Dropped = %d, want %d", dropErr.Dropped, notificationQueueCapacity)
	}
	if dropErr.TurnID != "turn-a" {
		t.Fatalf("NotificationDroppedError.TurnID = %q, want turn-a", dropErr.TurnID)
	}

	// Turn-B: all capacity notifications received with no drop error.
	for i := range notificationQueueCapacity {
		notification, err := client.nextTurnNotification(t.Context(), "turn-b")
		if err != nil {
			t.Fatalf("nextTurnNotification(turn-b, %d) error = %v, want notification", i, err)
		}
		if notification.Method != NotificationMethodItemCompleted {
			t.Fatalf("turn-b notification[%d] method = %q, want %s", i, notification.Method, NotificationMethodItemCompleted)
		}
	}

	// Router still functional — Close() returns nil.
	if err := client.Close(); err != nil {
		t.Fatalf("Close() after backpressure error = %v", err)
	}
}

// TestRouterPendingOverflowSurvives routes 2× capacity for a turn ID before any
// consumer registers. Asserts no error from route(), drop error on first next(),
// and surviving notification on second next().
func TestRouterPendingOverflowSurvives(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	const turnID = "pre-consumer-turn"

	// Route 2× capacity BEFORE registering a consumer.
	for i := range notificationQueueCapacity * 2 {
		if err := client.routeNotification(Notification{
			Method: NotificationMethodItemCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-pre",
				"turnId":   turnID,
				"item":     Object{"type": "agentMessage", "text": fmt.Sprintf("item-%d", i)},
			}),
		}); err != nil {
			t.Fatalf("routeNotification(%d) error = %v, want nil", i, err)
		}
	}

	// Register consumer — migrates pending + pendingDropped into the queue.
	if _, err := client.openTurnConsumer(turnID); err != nil {
		t.Fatalf("openTurnConsumer() error = %v", err)
	}

	// First next() must return NotificationDroppedError.
	_, err := client.nextTurnNotification(t.Context(), turnID)
	var dropErr *NotificationDroppedError
	if !errors.As(err, &dropErr) {
		t.Fatalf("first nextTurnNotification() error = %v (%T), want *NotificationDroppedError", err, err)
	}
	if dropErr.Dropped != notificationQueueCapacity {
		t.Fatalf("NotificationDroppedError.Dropped = %d, want %d", dropErr.Dropped, notificationQueueCapacity)
	}
	if dropErr.TurnID != turnID {
		t.Fatalf("NotificationDroppedError.TurnID = %q, want %s", dropErr.TurnID, turnID)
	}

	// Second next() returns first surviving notification without blocking.
	notification, err := client.nextTurnNotification(t.Context(), turnID)
	if err != nil {
		t.Fatalf("second nextTurnNotification() error = %v, want surviving notification", err)
	}
	if notification.Method != NotificationMethodItemCompleted {
		t.Fatalf("surviving notification method = %q, want %s", notification.Method, NotificationMethodItemCompleted)
	}
}

// TestRouterGlobalDropDoesNotKillTurns fills the global channel, verifies the
// overflow is swallowed without calling failLocked, and confirms per-turn routing
// and NextNotification remain functional.
func TestRouterGlobalDropDoesNotKillTurns(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)

	// Register a turn consumer so we can verify it remains healthy.
	if _, err := client.openTurnConsumer("turn-healthy"); err != nil {
		t.Fatalf("openTurnConsumer() error = %v", err)
	}

	// Fill the global channel to capacity (no turn ID → global).
	for i := range notificationQueueCapacity {
		if err := client.routeNotification(Notification{
			Method: fmt.Sprintf("global/%d", i),
		}); err != nil {
			t.Fatalf("routeNotification(global/%d) error = %v", i, err)
		}
	}

	// Overflow the global channel — must NOT error (drop-oldest).
	if err := client.routeNotification(Notification{Method: "global/overflow"}); err != nil {
		t.Fatalf("routeNotification(global/overflow) error = %v, want nil (drop-oldest)", err)
	}

	// Per-turn routing still works after global overflow.
	if err := client.routeNotification(Notification{
		Method: NotificationMethodItemCompleted,
		Params: mustJSON(t, Object{
			"threadId": "thread-healthy",
			"turnId":   "turn-healthy",
			"item":     Object{"type": "agentMessage", "text": "healthy"},
		}),
	}); err != nil {
		t.Fatalf("routeNotification(turn-healthy) error = %v, want nil (router healthy)", err)
	}

	// failLocked was NOT called — router is not closed.
	client.turnRouter.mu.Lock()
	routerClosed := client.turnRouter.closed
	client.turnRouter.mu.Unlock()
	if routerClosed {
		t.Fatal("router.closed = true after global overflow, want false (no failLocked)")
	}

	// NextNotification returns from the global channel (router healthy).
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	notification, err := client.NextNotification(ctx)
	if err != nil {
		t.Fatalf("NextNotification() error = %v, want global notification", err)
	}
	if !strings.HasPrefix(notification.Method, "global/") {
		t.Fatalf("NextNotification() method = %q, want global/*", notification.Method)
	}
}
