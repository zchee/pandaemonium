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
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"
)

func drainQueue(t *testing.T, q *NotifyQueue[int]) ([]int, error) {
	t.Helper()
	var got []int
	for {
		item, err := q.Next(t.Context())
		if err != nil {
			return got, err
		}
		got = append(got, item)
	}
}

func TestNotifyQueueFIFO(t *testing.T) {
	t.Parallel()

	q := NewNotifyQueue[int]()
	for i := range 5 {
		if !q.Push(i) {
			t.Fatalf("Push(%d) rejected on open queue", i)
		}
	}
	q.Close(nil)

	got, err := drainQueue(t, q)
	if !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("drain error = %v, want ErrQueueClosed", err)
	}
	if diff := gocmp.Diff([]int{0, 1, 2, 3, 4}, got); diff != "" {
		t.Errorf("drained items mismatch (-want +got):\n%s", diff)
	}
}

func TestNotifyQueueDrainThenError(t *testing.T) {
	t.Parallel()

	terminal := errors.New("terminal")
	q := NewNotifyQueue[int]()
	q.Push(1)
	q.Push(2)
	q.Close(terminal)
	if q.Push(3) {
		t.Fatal("Push accepted on closed queue")
	}

	got, err := drainQueue(t, q)
	if !errors.Is(err, terminal) {
		t.Fatalf("drain error = %v, want terminal error", err)
	}
	if diff := gocmp.Diff([]int{1, 2}, got); diff != "" {
		t.Errorf("drained items mismatch (-want +got):\n%s", diff)
	}
}

func TestNotifyQueueBlockingWakeup(t *testing.T) {
	t.Parallel()

	q := NewNotifyQueue[int]()
	got := make(chan int, 1)
	go func() {
		item, err := q.Next(context.WithoutCancel(t.Context()))
		if err != nil {
			return
		}
		got <- item
	}()

	time.Sleep(10 * time.Millisecond) // let Next block first
	q.Push(42)

	select {
	case item := <-got:
		if item != 42 {
			t.Fatalf("Next() = %d, want 42", item)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Next did not wake on Push")
	}
}

func TestNotifyQueueContextCancel(t *testing.T) {
	t.Parallel()

	q := NewNotifyQueue[int]()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := q.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Next() error = %v, want context.Canceled", err)
	}
}

func TestNotifyQueuePushBoundedGap(t *testing.T) {
	t.Parallel()

	const capacity = 4
	gapMarker := -1
	q := NewNotifyQueue[int]()
	var droppedTotal int
	gap := func(dropped int) (int, bool) {
		droppedTotal += dropped
		return gapMarker, true
	}
	for i := 1; i <= 6; i++ {
		if !q.PushBounded(i, capacity, gap) {
			t.Fatalf("PushBounded(%d) rejected on open queue", i)
		}
	}
	q.Close(nil)

	got, err := drainQueue(t, q)
	if !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("drain error = %v, want ErrQueueClosed", err)
	}
	// Overflow at push 5 keeps [3 4], then overflow at push 6 keeps [4 5]:
	// the newest survive and one gap marker leads the queue.
	if diff := gocmp.Diff([]int{gapMarker, 4, 5, 6}, got); diff != "" {
		t.Errorf("drained items mismatch (-want +got):\n%s", diff)
	}
	if droppedTotal == 0 {
		t.Error("gap callback never observed dropped items")
	}
}

func TestNotifyQueuePushBoundedPendingAccounting(t *testing.T) {
	t.Parallel()

	const capacity = 8
	q := NewNotifyQueue[int]()
	gapCalls := 0
	gap := func(int) (int, bool) {
		gapCalls++
		return -1, true
	}
	// Fill to capacity-1 pending, then drain part of it so head > 0 while
	// the backing slice still holds the consumed prefix.
	for i := range capacity - 1 {
		q.PushBounded(i, capacity, gap)
	}
	for range 4 {
		if _, err := q.Next(t.Context()); err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	}
	// Pending is 3; four more pushes reach pending 7 < capacity. The gap
	// policy must count true pending items only — consumed head slots left
	// in the backing array must not trigger premature gap events.
	for i := range 4 {
		q.PushBounded(100+i, capacity, gap)
	}
	if gapCalls != 0 {
		t.Fatalf("gap invoked %d times below capacity; pending accounting counts consumed head slots", gapCalls)
	}
	// Two more pushes cross capacity: exactly one overflow event.
	for i := range 2 {
		q.PushBounded(200+i, capacity, gap)
	}
	if gapCalls != 1 {
		t.Fatalf("gap calls = %d, want 1 after true overflow", gapCalls)
	}
}

func TestNotifyQueueHeadCompaction(t *testing.T) {
	t.Parallel()

	q := NewNotifyQueue[int]()
	const total = 3000
	for i := range total {
		q.Push(i)
	}
	// Consume enough to trip the head>1024 compaction branch while items
	// remain pending, then drain the rest and verify order integrity.
	for i := range total {
		item, err := q.Next(t.Context())
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if item != i {
			t.Fatalf("Next() = %d, want %d", item, i)
		}
	}
	q.Close(nil)
	if _, err := q.Next(t.Context()); !errors.Is(err, ErrQueueClosed) {
		t.Fatal("queue not empty after full drain")
	}
}

func TestNotifyQueueCloseWakesAllWaiters(t *testing.T) {
	t.Parallel()

	q := NewNotifyQueue[int]()
	const waiters = 4
	errs := make(chan error, waiters)
	for range waiters {
		go func() {
			_, err := q.Next(context.WithoutCancel(t.Context()))
			errs <- err
		}()
	}

	time.Sleep(10 * time.Millisecond) // let the waiters block first
	q.Close(nil)

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for range waiters {
		select {
		case err := <-errs:
			if !errors.Is(err, ErrQueueClosed) {
				t.Fatalf("Next() error = %v, want ErrQueueClosed", err)
			}
		case <-deadline.C:
			t.Fatal("close did not wake every blocked waiter")
		}
	}
}

func TestNotifyQueueConcurrentProducers(t *testing.T) {
	t.Parallel()

	const producers, perProducer = 4, 250
	q := NewNotifyQueue[int]()
	var wg sync.WaitGroup
	for p := range producers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perProducer {
				q.Push(p*perProducer + i)
			}
		}()
	}
	go func() {
		wg.Wait()
		q.Close(nil)
	}()

	seen := make(map[int]bool, producers*perProducer)
	for {
		item, err := q.Next(t.Context())
		if err != nil {
			if !errors.Is(err, ErrQueueClosed) {
				t.Fatalf("Next() error = %v", err)
			}
			break
		}
		if seen[item] {
			t.Fatalf("item %d delivered twice", item)
		}
		seen[item] = true
	}
	if len(seen) != producers*perProducer {
		t.Fatalf("delivered %d items, want %d", len(seen), producers*perProducer)
	}
}
