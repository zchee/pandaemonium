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

type notificationQueue struct {
	mu        sync.Mutex
	notifies  notificationRing
	notify    chan struct{}
	err       error
	scopeID   string
	dropped   uint64 // count of evicted notifications since last drain
	dropError func(scopeID string, dropped int) error
}

func newTurnNotificationQueue(turnID string) *notificationQueue {
	return &notificationQueue{
		scopeID:  turnID,
		notifies: newNotificationRing(notificationQueueCapacity),
		notify:   make(chan struct{}, 1),
		dropError: func(scopeID string, dropped int) error {
			return &NotificationDroppedError{TurnID: scopeID, Dropped: dropped}
		},
	}
}

func newLoginNotificationQueue(loginID string) *notificationQueue {
	return &notificationQueue{
		scopeID:  loginID,
		notifies: newNotificationRing(notificationQueueCapacity),
		notify:   make(chan struct{}, 1),
		dropError: func(scopeID string, dropped int) error {
			return &LoginNotificationDroppedError{LoginID: scopeID, Dropped: dropped}
		},
	}
}

func newProcessNotificationQueue(processHandle string) *notificationQueue {
	return &notificationQueue{
		scopeID:  processHandle,
		notifies: newNotificationRing(notificationQueueCapacity),
		notify:   make(chan struct{}, 1),
		dropError: func(scopeID string, dropped int) error {
			return &ProcessNotificationDroppedError{ProcessHandle: scopeID, Dropped: dropped}
		},
	}
}

// push enqueues notification. On queue overflow the oldest entry is evicted and
// the drop counter incremented. Never errors on overflow; only a closed queue
// (q.err != nil) suppresses the push silently.
func (q *notificationQueue) push(notif Notification) {
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

func (q *notificationQueue) pop() (Notification, bool) {
	q.mu.Lock()
	notification, ok := q.notifies.pop()
	q.mu.Unlock()
	return notification, ok
}

func (q *notificationQueue) next(ctx context.Context) (Notification, error) {
	for {
		q.mu.Lock()
		// Surface any accumulated drops before delivering the next notification.
		// Resets counter so the error fires exactly once per drop event.
		if q.dropped > 0 {
			dropped := q.dropped
			q.dropped = 0
			q.mu.Unlock()
			return Notification{}, q.dropError(q.scopeID, int(dropped))
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

func (q *notificationQueue) close(err error) {
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
