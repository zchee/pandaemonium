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

func pushPendingNotification(pending []Notification, notif Notification) ([]Notification, bool) {
	if len(pending) < notificationQueueCapacity {
		return append(pending, notif), false
	}
	copy(pending, pending[1:])
	pending[len(pending)-1] = notif
	return pending, true
}

type turnNotificationRouter struct {
	mu                  sync.Mutex
	global              chan Notification
	queues              map[string]*notificationQueue
	pending             map[string][]Notification
	pendingDropped      map[string]uint64 // drop counts for pre-consumer pending
	loginQueues         map[string]*notificationQueue
	pendingLogin        map[string][]Notification
	pendingLoginDropped map[string]uint64
	processQueues       map[string]*notificationQueue
	closed              bool
	err                 error
}

func newTurnNotificationRouter() *turnNotificationRouter {
	return &turnNotificationRouter{
		global:              make(chan Notification, notificationQueueCapacity),
		queues:              map[string]*notificationQueue{},
		pending:             map[string][]Notification{},
		pendingDropped:      map[string]uint64{},
		loginQueues:         map[string]*notificationQueue{},
		pendingLogin:        map[string][]Notification{},
		pendingLoginDropped: map[string]uint64{},
		processQueues:       map[string]*notificationQueue{},
	}
}

func (r *turnNotificationRouter) nextGlobal(ctx context.Context) (Notification, error) {
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
	case notif, ok := <-global:
		if !ok {
			return Notification{}, r.closedErr(&TransportClosedError{Message: "app-server notification stream closed"})
		}
		return notif, nil
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

func (r *turnNotificationRouter) nextLogin(ctx context.Context, loginID string) (Notification, error) {
	r.mu.Lock()
	if r.closed {
		err := r.err
		r.mu.Unlock()
		return Notification{}, err
	}
	queue := r.loginQueues[loginID]
	r.mu.Unlock()
	if queue == nil {
		return Notification{}, fmt.Errorf("login consumer is not active for %s", loginID)
	}
	return queue.next(ctx)
}

func (r *turnNotificationRouter) nextProcess(ctx context.Context, processHandle string) (Notification, error) {
	r.mu.Lock()
	if r.closed {
		err := r.err
		r.mu.Unlock()
		return Notification{}, err
	}
	queue := r.processQueues[processHandle]
	r.mu.Unlock()
	if queue == nil {
		return Notification{}, fmt.Errorf("process consumer is not active for %s", processHandle)
	}
	return queue.next(ctx)
}

func (r *turnNotificationRouter) register(turnID string) (*notificationQueue, error) {
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
	queue := newTurnNotificationQueue(turnID)
	if pending := r.pending[turnID]; len(pending) > 0 {
		if !queue.notifies.appendAll(pending) {
			return nil, fmt.Errorf("turn notification router: pending queue overflow for %s", turnID)
		}
		delete(r.pending, turnID)
	}
	// Migrate any pre-consumer drop count so the consumer sees it on first next() call.
	if dropped := r.pendingDropped[turnID]; dropped > 0 {
		queue.dropped = dropped
		delete(r.pendingDropped, turnID)
		// Signal so the consumer wakes immediately to surface the drop error.
		select {
		case queue.notify <- struct{}{}:
		default:
		}
	}
	r.queues[turnID] = queue
	return queue, nil
}

func (r *turnNotificationRouter) registerLogin(loginID string) (*notificationQueue, error) {
	if loginID == "" {
		return nil, fmt.Errorf("login notification router: empty login id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, r.err
	}
	if _, ok := r.loginQueues[loginID]; ok {
		return nil, fmt.Errorf("login consumer already active for %s", loginID)
	}
	queue := newLoginNotificationQueue(loginID)
	if pending := r.pendingLogin[loginID]; len(pending) > 0 {
		if !queue.notifies.appendAll(pending) {
			return nil, fmt.Errorf("login notification router: pending queue overflow for %s", loginID)
		}
		delete(r.pendingLogin, loginID)
	}
	if dropped := r.pendingLoginDropped[loginID]; dropped > 0 {
		queue.dropped = dropped
		delete(r.pendingLoginDropped, loginID)
		select {
		case queue.notify <- struct{}{}:
		default:
		}
	}
	r.loginQueues[loginID] = queue
	return queue, nil
}

func (r *turnNotificationRouter) registerProcess(processHandle string) (*notificationQueue, error) {
	if processHandle == "" {
		return nil, fmt.Errorf("process notification router: empty process handle")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, r.err
	}
	if _, ok := r.processQueues[processHandle]; ok {
		return nil, fmt.Errorf("process consumer already active for %s", processHandle)
	}
	queue := newProcessNotificationQueue(processHandle)
	r.processQueues[processHandle] = queue
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

func (r *turnNotificationRouter) unregisterLogin(loginID string) {
	if loginID == "" {
		return
	}
	r.mu.Lock()
	delete(r.loginQueues, loginID)
	r.mu.Unlock()
}

func (r *turnNotificationRouter) unregisterProcess(processHandle string) {
	if processHandle == "" {
		return
	}
	r.mu.Lock()
	queue := r.processQueues[processHandle]
	delete(r.processQueues, processHandle)
	r.mu.Unlock()
	if queue != nil {
		queue.close(fmt.Errorf("process consumer closed for %s", processHandle))
	}
}

func (r *turnNotificationRouter) clearPending(turnID string) {
	if turnID == "" {
		return
	}
	r.mu.Lock()
	delete(r.pending, turnID)
	r.mu.Unlock()
}

func (r *turnNotificationRouter) clearLoginPending(loginID string) {
	if loginID == "" {
		return
	}
	r.mu.Lock()
	delete(r.pendingLogin, loginID)
	delete(r.pendingLoginDropped, loginID)
	r.mu.Unlock()
}

func (r *turnNotificationRouter) route(notif Notification) error {
	loginID, loginScoped := notificationLoginID(notif)
	if loginScoped {
		return r.routeLogin(notif, loginID)
	}

	processHandle := notificationProcessHandle(notif)
	turnID := notificationTurnID(notif)

	r.mu.Lock()
	if r.closed {
		err := r.err
		r.mu.Unlock()
		return err
	}

	if processHandle != "" {
		if queue := r.processQueues[processHandle]; queue != nil {
			queue.push(notif)
			r.mu.Unlock()
			return nil
		}
	}

	// ── Global (no turn ID) ────────────────────────────────────────────────
	if turnID == "" {
		// Fast path: channel has room.
		select {
		case r.global <- notif:
			r.mu.Unlock()
			return nil
		default:
		}
		// Channel full: evict oldest (best-effort; nextGlobal may have already
		// consumed one, which only helps us), then push newest.
		select {
		case <-r.global:
		default:
		}
		// After the drain above, channel has < capacity entries (no other
		// route() can interleave — we hold r.mu). Push always succeeds here.
		select {
		case r.global <- notif:
		default: // paranoid guard; should not fire
		}
		r.mu.Unlock()
		return nil
	}

	// ── Active turn consumer ───────────────────────────────────────────────
	if queue := r.queues[turnID]; queue != nil {
		// push is now void; drop-oldest handled inside the queue.
		queue.push(notif)
		r.mu.Unlock()
		return nil
	}

	// ── Pre-consumer pending ───────────────────────────────────────────────
	pending, dropped := pushPendingNotification(r.pending[turnID], notif)
	r.pending[turnID] = pending
	if dropped {
		// Drop oldest pending entry, track count.
		r.pendingDropped[turnID]++
	}
	r.mu.Unlock()
	return nil
}

func (r *turnNotificationRouter) routeLogin(notif Notification, loginID string) error {
	r.mu.Lock()
	if r.closed {
		err := r.err
		r.mu.Unlock()
		return err
	}

	if loginID == "" {
		r.routeGlobalLocked(notif)
		r.mu.Unlock()
		return nil
	}

	if queue := r.loginQueues[loginID]; queue != nil {
		queue.push(notif)
		r.mu.Unlock()
		return nil
	}

	pending, dropped := pushPendingNotification(r.pendingLogin[loginID], notif)
	r.pendingLogin[loginID] = pending
	if dropped {
		r.pendingLoginDropped[loginID]++
	}
	r.mu.Unlock()
	return nil
}

func (r *turnNotificationRouter) routeGlobalLocked(notif Notification) {
	select {
	case r.global <- notif:
		return
	default:
	}
	select {
	case <-r.global:
	default:
	}
	select {
	case r.global <- notif:
	default:
	}
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
	queues := make([]*notificationQueue, 0, len(r.queues))
	for _, queue := range r.queues {
		queues = append(queues, queue)
	}
	loginQueues := make([]*notificationQueue, 0, len(r.loginQueues))
	for _, queue := range r.loginQueues {
		loginQueues = append(loginQueues, queue)
	}
	processQueues := make([]*notificationQueue, 0, len(r.processQueues))
	for _, queue := range r.processQueues {
		processQueues = append(processQueues, queue)
	}
	r.queues = map[string]*notificationQueue{}
	r.pending = map[string][]Notification{}
	r.pendingDropped = map[string]uint64{}
	r.loginQueues = map[string]*notificationQueue{}
	r.pendingLogin = map[string][]Notification{}
	r.pendingLoginDropped = map[string]uint64{}
	r.processQueues = map[string]*notificationQueue{}
	r.mu.Unlock()

	if global != nil {
		close(global)
	}
	for _, queue := range queues {
		queue.close(err)
	}
	for _, queue := range loginQueues {
		queue.close(err)
	}
	for _, queue := range processQueues {
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
