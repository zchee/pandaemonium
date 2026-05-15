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
	"sync"
)

type turnNotificationQueue struct {
	mu       sync.Mutex
	notifies notificationRing
	notify   chan struct{}
	err      error
	turnID   string // stored so next() can form NotificationDroppedError
	dropped  uint64 // count of evicted notifications since last drain
}

// push enqueues notification. On queue overflow the oldest entry is evicted and
// the drop counter incremented. Never errors on overflow; only a closed queue
// (q.err != nil) suppresses the push silently.
func (q *turnNotificationQueue) push(notif Notification) {
	q.mu.Lock()
	if q.err != nil {
		q.mu.Unlock()
		return // closed queue; discard silently
	}
	if q.notifies.pushDisplacing(notif) {
		q.dropped++
	}
	select {
	case q.notify <- struct{}{}:
	default:
	}
	q.mu.Unlock()
}

func (q *turnNotificationQueue) pop() (Notification, bool) {
	q.mu.Lock()
	notification, ok := q.notifies.pop()
	q.mu.Unlock()
	return notification, ok
}

func (q *turnNotificationQueue) next(ctx context.Context) (Notification, error) {
	for {
		q.mu.Lock()
		// Surface any accumulated drops before delivering the next notification.
		// Resets counter so the error fires exactly once per drop event.
		if q.dropped > 0 {
			dropped := q.dropped
			q.dropped = 0
			q.mu.Unlock()
			return Notification{}, &NotificationDroppedError{TurnID: q.turnID, Dropped: int(dropped)}
		}
		if notification, ok := q.notifies.pop(); ok {
			q.mu.Unlock()
			return notification, nil
		}
		if q.err != nil {
			err := q.err
			q.mu.Unlock()
			return Notification{}, err
		}
		notify := q.notify
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return Notification{}, ctx.Err()
		case <-notify:
		}
	}
}

func (q *turnNotificationQueue) close(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return
	}
	q.err = err
	select {
	case q.notify <- struct{}{}:
	default:
	}
}
