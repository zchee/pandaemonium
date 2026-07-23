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

package llm

import (
	"context"
	"errors"
	"sync"
)

// ErrQueueClosed is returned by [NotifyQueue.Next] when the queue was closed
// without a terminal error and every queued item has been delivered.
var ErrQueueClosed = errors.New("llm: notify queue closed")

// NotifyQueue is a mutex-guarded FIFO delivering items from any number of
// producers to a single consumer through a 1-buffered wakeup channel.
//
// Queued items are always drained before the close outcome is surfaced, and
// the backing slice is recycled with head-index compaction so a long-lived
// queue does not reallocate per item.
type NotifyQueue[T any] struct {
	mu     sync.Mutex
	items  []T
	head   int
	notify chan struct{}
	closed bool
	err    error
}

// NewNotifyQueue returns an empty open queue.
func NewNotifyQueue[T any]() *NotifyQueue[T] {
	return &NotifyQueue[T]{notify: make(chan struct{}, 1)}
}

// Push enqueues item and reports whether it was accepted; a closed queue
// discards item and reports false.
func (q *NotifyQueue[T]) Push(item T) bool {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return false
	}
	q.items = append(q.items, item)
	q.mu.Unlock()
	q.signal()
	return true
}

// PushBounded enqueues item while bounding the pending queue to capacity
// items (capacity must be >= 2). On overflow the oldest pending items are
// dropped — never the newest — keeping capacity-2 entries and prepending
// the marker returned by gap so the consumer learns items were lost. The
// dropped count passed to gap counts evicted queue slots, which include
// earlier gap markers. It reports whether item was accepted; a closed
// queue discards item and reports false.
func (q *NotifyQueue[T]) PushBounded(item T, capacity int, gap func(dropped int) (T, bool)) bool {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return false
	}
	if q.head > 0 {
		q.compactLocked()
	}
	if len(q.items) >= capacity {
		keep := max(capacity-2, 0)
		dropped := len(q.items) - keep
		tail := q.items[len(q.items)-keep:]
		next := make([]T, 0, capacity)
		if marker, ok := gap(dropped); ok {
			next = append(next, marker)
		}
		next = append(next, tail...)
		q.items = next
	}
	q.items = append(q.items, item)
	q.mu.Unlock()
	q.signal()
	return true
}

// Next returns the next queued item, blocking until one arrives, the queue is
// closed (whose terminal error is surfaced only after the queue drains), or
// ctx is done. A queue closed without an error yields [ErrQueueClosed].
func (q *NotifyQueue[T]) Next(ctx context.Context) (T, error) {
	var zero T
	for {
		q.mu.Lock()
		if q.head < len(q.items) {
			item := q.items[q.head]
			q.items[q.head] = zero
			q.head++
			if q.head == len(q.items) {
				q.items = q.items[:0]
				q.head = 0
			} else if q.head > 1024 && q.head*2 >= len(q.items) {
				q.compactLocked()
			}
			rearm := q.head < len(q.items) || q.closed
			q.mu.Unlock()
			if rearm {
				// Keep the wakeup armed for remaining work.
				q.signal()
			}
			return item, nil
		}
		if q.closed {
			err := q.err
			q.mu.Unlock()
			// Re-arm so every other waiter blocked in Next also observes
			// the close, matching broadcast-style close wakeups.
			q.signal()
			if err == nil {
				err = ErrQueueClosed
			}
			return zero, err
		}
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-q.notify:
		}
	}
}

// Close closes the queue with err as the terminal Next outcome once drained.
// The first close wins; later calls are no-ops.
func (q *NotifyQueue[T]) Close(err error) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	q.err = err
	q.mu.Unlock()
	q.signal()
}

// signal arms the 1-buffered wakeup channel without blocking.
func (q *NotifyQueue[T]) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// compactLocked drops the consumed prefix, zeroing vacated slots so the
// backing array does not pin delivered items. q.mu must be held.
func (q *NotifyQueue[T]) compactLocked() {
	var zero T
	copy(q.items, q.items[q.head:])
	for i := len(q.items) - q.head; i < len(q.items); i++ {
		q.items[i] = zero
	}
	q.items = q.items[:len(q.items)-q.head]
	q.head = 0
}
