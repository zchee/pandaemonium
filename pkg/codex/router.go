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
	"context"
	"fmt"
	"sync"

	"github.com/go-json-experiment/json"
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

type turnNotificationQueue struct {
	mu            sync.Mutex
	notifications notificationRing
	notify        chan struct{}
	err           error
}

type notificationRing struct {
	values []Notification
	head   int
	length int
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

func (r *turnNotificationRouter) queue(turnID string) (*turnNotificationQueue, error) {
	if turnID == "" {
		return nil, fmt.Errorf("turn notification router: empty turn id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, r.err
	}
	queue := r.queues[turnID]
	if queue == nil {
		return nil, fmt.Errorf("turn notification router: no active consumer for %s", turnID)
	}
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

func (q *turnNotificationQueue) push(notification Notification) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	if !q.notifications.push(notification) {
		return fmt.Errorf("turn notification queue full")
	}
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return nil
}

func (q *turnNotificationQueue) pop() (Notification, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.notifications.pop()
}

func (q *turnNotificationQueue) next(ctx context.Context) (Notification, error) {
	for {
		if notification, ok := q.pop(); ok {
			return notification, nil
		}
		q.mu.Lock()
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

func newNotificationRing(capacity int) notificationRing {
	return notificationRing{values: make([]Notification, capacity)}
}

func (r *notificationRing) len() int {
	if r == nil {
		return 0
	}
	return r.length
}

func (r *notificationRing) push(notification Notification) bool {
	if r == nil || len(r.values) == 0 || r.length >= len(r.values) {
		return false
	}
	r.values[(r.head+r.length)%len(r.values)] = notification
	r.length++
	return true
}

func (r *notificationRing) appendAll(notifications []Notification) bool {
	if r == nil || len(notifications) > len(r.values)-r.length {
		return false
	}
	for _, notification := range notifications {
		if !r.push(notification) {
			return false
		}
	}
	return true
}

func (r *notificationRing) pop() (Notification, bool) {
	if r == nil || r.length == 0 {
		return Notification{}, false
	}
	notification := r.values[r.head]
	r.values[r.head] = Notification{}
	r.head = (r.head + 1) % len(r.values)
	r.length--
	if r.length == 0 {
		r.head = 0
	}
	return notification, true
}

func notificationTurnID(notification Notification) string {
	var envelope struct {
		TurnID  string `json:"turnId"`
		TurnID2 string `json:"turn_id"`
		Turn    *struct {
			ID      string `json:"id"`
			TurnID  string `json:"turnId"`
			TurnID2 string `json:"turn_id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(notification.Params, &envelope); err != nil {
		return ""
	}
	if envelope.TurnID != "" {
		return envelope.TurnID
	}
	if envelope.TurnID2 != "" {
		return envelope.TurnID2
	}
	if envelope.Turn != nil {
		if envelope.Turn.TurnID != "" {
			return envelope.Turn.TurnID
		}
		if envelope.Turn.TurnID2 != "" {
			return envelope.Turn.TurnID2
		}
		return envelope.Turn.ID
	}
	return ""
}
