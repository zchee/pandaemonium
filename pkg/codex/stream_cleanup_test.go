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
	"testing"
	"testing/synctest"
)

// TestTurnHandleStreamEarlyBreakClearsPending verifies AC-3.8: stopping iteration
// of TurnHandle.Stream before TurnCompleted releases both the turn consumer
// (queues entry) and the pending buffer. Without defer clearTurnPending, the
// pending buffer could accumulate notifications for the released turn if the
// server keeps streaming after the client breaks.
func TestTurnHandleStreamEarlyBreakClearsPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)
		handle := &TurnHandle{
			client:   client,
			threadID: "thread-cleanup",
			turnID:   "turn-cleanup",
		}

		// Route a notification pre-consumer (goes to r.pending[turnID]).
		if err := client.routeNotification(Notification{
			Method: NotificationMethodItemCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-cleanup",
				"turnId":   "turn-cleanup",
				"item":     Object{"type": "agentMessage", "text": "pre"},
			}),
		}); err != nil {
			t.Fatalf("routeNotification(pre-consumer) error = %v", err)
		}

		// Break out of Stream after the first notification (before TurnCompleted).
		var received int
		for notification, err := range handle.Stream(t.Context()) {
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			received++
			_ = notification
			break // early break — TurnCompleted never received
		}
		synctest.Wait()

		if received != 1 {
			t.Fatalf("received = %d, want 1", received)
		}

		// After early break: consumer must be released (no active consumer).
		if got := activeTurnConsumers(client); len(got) != 0 {
			t.Fatalf("active turn consumers after early break = %v, want none (releaseTurnConsumer ran)", got)
		}

		// After early break: pending buffer must be empty (clearTurnPending ran).
		client.turnRouter.mu.Lock()
		pending := client.turnRouter.pending["turn-cleanup"]
		client.turnRouter.mu.Unlock()
		if len(pending) != 0 {
			t.Fatalf("pending[turn-cleanup] len = %d, want 0 (clearTurnPending ran)", len(pending))
		}

		// Simulate server sending a post-break notification (goes to pending again).
		if err := client.routeNotification(Notification{
			Method: NotificationMethodItemCompleted,
			Params: mustJSON(t, Object{
				"threadId": "thread-cleanup",
				"turnId":   "turn-cleanup",
				"item":     Object{"type": "agentMessage", "text": "post"},
			}),
		}); err != nil {
			t.Fatalf("routeNotification(post-break) error = %v", err)
		}

		// A second Stream on the same turn can now register without conflict.
		var notifications []Notification
		for notification, err := range handle.Stream(t.Context()) {
			if err != nil {
				t.Fatalf("second Stream() error = %v", err)
			}
			notifications = append(notifications, notification)
			break // stop after the post-break notification
		}
		synctest.Wait()

		if len(notifications) != 1 {
			t.Fatalf("second Stream received %d notifications, want 1", len(notifications))
		}
	})
}
