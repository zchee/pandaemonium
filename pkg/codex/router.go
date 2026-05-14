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
	"fmt"
	"sync"
)

const notificationQueueCapacity = 128

type turnNotificationRouter struct {
	mu      sync.Mutex
	global  chan Notification
	queues  map[string]*turnNotificationQueue
	pending map[string][]Notification
	closed  bool
	err     error
}

func newTurnNotificationRouter() *turnNotificationRouter {
	return &turnNotificationRouter{
		global:  make(chan Notification, notificationQueueCapacity),
		queues:  map[string]*turnNotificationQueue{},
		pending: map[string][]Notification{},
	}
}

func (r *turnNotificationRouter) nextGlobal(ctx context.Context, legacy <-chan Notification) (Notification, error) {
	r.mu.Lock()
	if r.closed {
		err := r.err
		r.mu.Unlock()
		if err != nil {
			return Notification{}, err
		}
		return Notification{}, &TransportClosedError{Message: "app-server notification stream closed"}
	}
	global := r.global
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		return Notification{}, ctx.Err()
	case notification, ok := <-legacy:
		if !ok {
			return Notification{}, &TransportClosedError{Message: "app-server notification stream closed"}
		}
		return notification, nil
	case notification, ok := <-global:
		if !ok {
			return Notification{}, r.closedErr(&TransportClosedError{Message: "app-server notification stream closed"})
		}
		return notification, nil
	}
}

func (r *turnNotificationRouter) next(ctx context.Context, turnID string) (Notification, error) {
	r.mu.Lock()
	if r.closed {
		err := r.err
		r.mu.Unlock()
		return Notification{}, err
	}
	queue := r.queues[turnID]
	r.mu.Unlock()
	if queue == nil {
		return Notification{}, fmt.Errorf("turn consumer is not active for %s", turnID)
	}
	return queue.next(ctx)
}

func (r *turnNotificationRouter) register(turnID string) (*turnNotificationQueue, error) {
	if turnID == "" {
		return nil, fmt.Errorf("turn notification router: empty turn id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, r.err
	}
	if _, ok := r.queues[turnID]; ok {
		return nil, fmt.Errorf("turn consumer already active for %s", turnID)
	}
	queue := &turnNotificationQueue{
		notifications: newNotificationRing(notificationQueueCapacity),
		notify:        make(chan struct{}, 1),
	}
	if pending := r.pending[turnID]; len(pending) > 0 {
		if !queue.notifications.appendAll(pending) {
			return nil, fmt.Errorf("turn notification router: pending queue overflow for %s", turnID)
		}
		delete(r.pending, turnID)
	}
	r.queues[turnID] = queue
	return queue, nil
}

func (r *turnNotificationRouter) unregister(turnID string) {
	if turnID == "" {
		return
	}
	r.mu.Lock()
	delete(r.queues, turnID)
	r.mu.Unlock()
}

func (r *turnNotificationRouter) clearPending(turnID string) {
	if turnID == "" {
		return
	}
	r.mu.Lock()
	delete(r.pending, turnID)
	r.mu.Unlock()
}

func (r *turnNotificationRouter) route(notification Notification) error {
	turnID := notificationTurnID(notification)

	r.mu.Lock()
	if r.closed {
		err := r.err
		r.mu.Unlock()
		return err
	}
	if turnID == "" {
		select {
		case r.global <- notification:
			r.mu.Unlock()
			return nil
		default:
		}
		err := fmt.Errorf("notification router: global notification queue full")
		r.failLocked(err)
		r.mu.Unlock()
		return err
	}
	if queue := r.queues[turnID]; queue != nil {
		if err := queue.push(notification); err != nil {
			r.failLocked(err)
			r.mu.Unlock()
			return err
		}
		r.mu.Unlock()
		return nil
	}
	pending := r.pending[turnID]
	if len(pending) >= notificationQueueCapacity {
		err := fmt.Errorf("notification router: pending queue full for %s", turnID)
		r.failLocked(err)
		r.mu.Unlock()
		return err
	}
	r.pending[turnID] = append(pending, notification)
	r.mu.Unlock()
	return nil
}

func (r *turnNotificationRouter) close(err error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.err = err
	global := r.global
	r.global = nil
	queues := make([]*turnNotificationQueue, 0, len(r.queues))
	for _, queue := range r.queues {
		queues = append(queues, queue)
	}
	r.queues = map[string]*turnNotificationQueue{}
	r.pending = map[string][]Notification{}
	r.mu.Unlock()

	if global != nil {
		close(global)
	}
	for _, queue := range queues {
		queue.close(err)
	}
}

func (r *turnNotificationRouter) failLocked(err error) {
	if r.closed {
		return
	}
	r.closed = true
	r.err = err
	global := r.global
	r.global = nil
	queues := make([]*turnNotificationQueue, 0, len(r.queues))
	for _, queue := range r.queues {
		queues = append(queues, queue)
	}
	r.queues = map[string]*turnNotificationQueue{}
	r.pending = map[string][]Notification{}
	if global != nil {
		close(global)
	}
	for _, queue := range queues {
		queue.close(err)
	}
}

func (r *turnNotificationRouter) closedErr(err error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	return err
}
