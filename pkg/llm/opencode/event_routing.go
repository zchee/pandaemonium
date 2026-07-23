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

package opencode

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/go-json-experiment/json"

	"github.com/zchee/pandaemonium/pkg/llm"
)

// eventQueueCapacity bounds each consumer's buffered event queue. On
// overflow the oldest event is dropped and a gap event marks the loss.
const eventQueueCapacity = 128

// busConsumer is one router registration on the shared event bus, scoped to
// a session. It is not a connection: all consumers share the bus's single
// GET /event stream.
//
// Queueing is the shared [llm.NotifyQueue] bounded to eventQueueCapacity with
// the gap-marker overflow policy; this wrapper pins the opencode close
// contract (a default TransportClosedError when closed without a cause).
type busConsumer struct {
	id        uint64
	sessionID string
	q         *llm.NotifyQueue[Event]
}

func newBusConsumer(id uint64, sessionID string) *busConsumer {
	return &busConsumer{
		id:        id,
		sessionID: sessionID,
		q:         llm.NewNotifyQueue[Event](),
	}
}

// push enqueues ev. On overflow the oldest events are dropped — never the
// newest — and the loss is coalesced into a single gap marker at the head so
// the consumer learns events were lost.
func (bc *busConsumer) push(ev Event) {
	bc.q.PushBounded(ev, eventQueueCapacity, func(int) (Event, bool) {
		return Event{Type: EventTypeGap}, true
	})
}

// close terminates the consumer with err; pending queued events remain
// readable before the error is surfaced.
func (bc *busConsumer) close(err error) {
	bc.q.Close(err)
}

// next returns the next queued event, blocking until one arrives, the
// consumer is closed (its terminal error is returned after the queue
// drains), or ctx is done.
func (bc *busConsumer) next(ctx context.Context) (Event, error) {
	ev, err := bc.q.Next(ctx)
	if err != nil {
		if errors.Is(err, llm.ErrQueueClosed) {
			return Event{}, &TransportClosedError{Message: "opencode event bus consumer closed"}
		}
		return Event{}, err
	}
	return ev, nil
}

// eventBus owns the single client-lifetime GET /event connection and fans
// events out to registered consumers (locked topology: per-turn
// subscriptions are registrations here, never new connections). The bus owns
// bounded auto-reconnect; on every reconnect all registered consumers
// receive a gap event because /event has no resume cursor.
type eventBus struct {
	client *Client

	ctx    context.Context //nolint:containedctx // the bus run loop and its SSE request must bind to the client lifetime, not any caller ctx.
	cancel context.CancelCauseFunc

	dialOnce  sync.Once
	connected chan struct{} // closed once the first server.connected arrives

	mu        sync.Mutex
	consumers map[uint64]*busConsumer
	nextID    uint64
	body      io.Closer // current stream body; closed to interrupt reads
	closed    bool
	err       error

	done chan struct{} // run loop exited
}

func newEventBus(c *Client) *eventBus {
	ctx, cancel := context.WithCancelCause(c.lifetime)
	return &eventBus{
		client:    c,
		ctx:       ctx,
		cancel:    cancel,
		connected: make(chan struct{}),
		consumers: map[uint64]*busConsumer{},
		done:      make(chan struct{}),
	}
}

// dial starts the run loop (once) and waits for the initial dial to complete:
// stream established and the first server.connected observed. The initial
// dial fails fast — reconnect budgets apply only to an established bus. A
// dial deadline or a canceled caller ctx only ever tears down a bus that has
// never connected: an established shared bus is sacrosanct (other sessions'
// consumers and the permission router depend on it).
func (b *eventBus) dial(ctx context.Context) error {
	b.dialOnce.Do(func() {
		go b.run()
	})

	// Fast path: already connected — never disturb the shared bus.
	select {
	case <-b.connected:
		return nil
	default:
	}

	timer := time.NewTimer(b.client.config.DialTimeout)
	defer timer.Stop()
	select {
	case <-b.connected:
		return nil
	case <-b.done:
		return b.closedErr()
	case <-timer.C:
		return b.failDial(&TransportClosedError{Message: "opencode event bus dial timed out waiting for server.connected"})
	case <-ctx.Done():
		return b.failDial(&TransportClosedError{Message: "opencode event bus dial canceled: " + ctx.Err().Error()})
	}
}

// failDial tears the bus down with cause unless the handshake won the race,
// in which case the dial succeeds and the established bus stays untouched.
func (b *eventBus) failDial(cause *TransportClosedError) error {
	if !b.closeInternal(cause, true) {
		return nil
	}
	<-b.done
	return b.closedErr()
}

func (b *eventBus) closedErr() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	return &TransportClosedError{Message: "opencode event bus closed"}
}

// subscribe registers a session-scoped consumer.
func (b *eventBus) subscribe(sessionID string) (*busConsumer, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		if b.err != nil {
			return nil, b.err
		}
		return nil, &TransportClosedError{Message: "opencode event bus closed"}
	}
	b.nextID++
	consumer := newBusConsumer(b.nextID, sessionID)
	b.consumers[consumer.id] = consumer
	return consumer, nil
}

// unsubscribe removes a consumer registration.
func (b *eventBus) unsubscribe(consumer *busConsumer) {
	if consumer == nil {
		return
	}
	b.mu.Lock()
	delete(b.consumers, consumer.id)
	b.mu.Unlock()
	consumer.close(&TransportClosedError{Message: "opencode event bus consumer unsubscribed"})
}

// close terminates the bus and every registered consumer with err.
func (b *eventBus) close(err error) {
	b.closeInternal(err, false)
}

// closeInternal terminates the bus. With onlyIfUnconnected set it refuses to
// touch a bus whose server.connected handshake already completed (returning
// false); the handshake mark and this check share b.mu, so the race between
// a failing dial waiter and a completing handshake has a strict winner.
// It reports whether the bus is closed on return.
func (b *eventBus) closeInternal(err error, onlyIfUnconnected bool) bool {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return true
	}
	if onlyIfUnconnected {
		select {
		case <-b.connected:
			b.mu.Unlock()
			return false
		default:
		}
	}
	b.closed = true
	b.err = err
	body := b.body
	b.body = nil
	consumers := make([]*busConsumer, 0, len(b.consumers))
	for _, consumer := range b.consumers {
		consumers = append(consumers, consumer)
	}
	b.consumers = map[uint64]*busConsumer{}
	b.mu.Unlock()

	b.cancel(err)
	if body != nil {
		body.Close()
	}
	for _, consumer := range consumers {
		consumer.close(err)
	}
	return true
}

// isClosed reports whether close has been called.
func (b *eventBus) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// setBody records the active stream body so close can interrupt reads. It
// reports false (and closes body) when the bus is already closed.
func (b *eventBus) setBody(body io.ReadCloser) bool {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		body.Close()
		return false
	}
	b.body = body
	b.mu.Unlock()
	return true
}

// broadcastGap delivers a gap event to every registered consumer (events may
// have been lost while the bus was disconnected).
func (b *eventBus) broadcastGap() {
	b.mu.Lock()
	consumers := make([]*busConsumer, 0, len(b.consumers))
	for _, consumer := range b.consumers {
		consumers = append(consumers, consumer)
	}
	b.mu.Unlock()

	b.client.counters.gapNotifications.Add(uint64(len(consumers)))
	for _, consumer := range consumers {
		consumer.push(Event{Type: EventTypeGap})
	}
}

// run is the bus goroutine: dial, stream, and bounded reconnect. The initial
// connection reports readiness through b.connected; reconnect attempts are
// bounded by the client RetryConfig and reset after every established
// connection.
func (b *eventBus) run() {
	defer close(b.done)

	retry := llm.RetryConfig(b.client.config.Retry).WithDefaults()
	firstConnection := true
	attempt := 0
	delay := retry.InitialDelay

	for {
		if b.isClosed() {
			return
		}

		handshaken, streamErr := b.streamOnce(&firstConnection)
		if handshaken {
			// The outage budget is per-outage: an established connection
			// resets it.
			attempt = 0
			delay = retry.InitialDelay
		}
		if b.isClosed() || b.ctx.Err() != nil {
			return
		}

		// Initial dial failures fail fast: the facade contract is "dial
		// eagerly, fail fast", so reconnect budgets only apply once a
		// connection has been established.
		if firstConnection {
			b.close(&TransportClosedError{Message: "opencode event bus dial failed: " + streamErr.Error()})
			return
		}

		attempt++
		if attempt >= retry.MaxAttempts {
			b.close(&TransportClosedError{
				Message: fmt.Sprintf("opencode event bus reconnect exhausted after %d attempts: %s", attempt, streamErr),
			})
			return
		}
		var err error
		delay, err = retry.SleepDelay(b.ctx, delay)
		if err != nil {
			b.close(&TransportClosedError{Message: "opencode event bus closed during reconnect backoff"})
			return
		}
	}
}

// streamOnce dials /event once and pumps events until the stream fails. On a
// successful (re)connection it handles the server.connected handshake, gap
// notification, and reconnect counting. It reports whether the handshake
// completed (the connection was established) and the stream failure; callers
// decide whether to retry.
func (b *eventBus) streamOnce(firstConnection *bool) (bool, error) {
	body, err := dialEvents(b.ctx, b.client.httpClient, b.client.BaseURL())
	if err != nil {
		return false, err
	}
	if !b.setBody(body) {
		return false, &TransportClosedError{Message: "opencode event bus closed"}
	}

	scanner := newSSEScanner(body)
	handshaken := false
	for {
		frame, err := scanner.next()
		if err != nil {
			return handshaken, fmt.Errorf("opencode event stream read: %w", err)
		}

		var ev Event
		if err := json.Unmarshal(frame.data, &ev); err != nil || ev.Type == "" {
			// Undecodable frames are tolerated: the stream stays healthy and
			// typed routing simply cannot apply.
			continue
		}

		if !handshaken {
			if err := b.handshake(ev, firstConnection); err != nil {
				return false, err
			}
			handshaken = true
			continue
		}

		b.route(ev)
	}
}

// handshake processes the mandatory server.connected first event of a
// (re)connection. The first-connection mark shares b.mu with
// closeInternal's unconnected check, so a concurrent failing dial waiter
// either sees the bus connected (and succeeds) or wins and this connection
// stops here.
func (b *eventBus) handshake(ev Event, firstConnection *bool) error {
	if ev.Type != EventTypeServerConnected {
		return fmt.Errorf("opencode event stream: expected server.connected handshake, got %q", ev.Type)
	}
	if !*firstConnection {
		b.client.counters.sseReconnects.Add(1)
		b.broadcastGap()
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return &TransportClosedError{Message: "opencode event bus closed during handshake"}
	}
	*firstConnection = false
	close(b.connected)
	return nil
}

// route fans one decoded event out: the client-lifetime permission consumer
// sees permission requests for every session; session-scoped consumers see
// events whose sessionID matches; a session.error without a sessionID is
// counted and never guessed onto a session.
func (b *eventBus) route(ev Event) {
	switch ev.Type {
	case EventTypePermissionAsked, EventTypePermissionV2Asked:
		b.client.respondPermission(ev)
	case EventTypeSessionError:
		if ev.SessionID() == "" {
			b.client.counters.unattributedSessionErrors.Add(1)
			return
		}
	}

	sessionID := ev.SessionID()
	if sessionID == "" {
		return
	}

	b.mu.Lock()
	consumers := make([]*busConsumer, 0, len(b.consumers))
	for _, consumer := range b.consumers {
		if consumer.sessionID == sessionID {
			consumers = append(consumers, consumer)
		}
	}
	b.mu.Unlock()

	for _, consumer := range consumers {
		consumer.push(ev)
	}
}

// ensureBus returns the shared client-lifetime bus, dialing it on first use.
// The facade constructors call this eagerly (fail fast before any prompt);
// low-level Client users get a lazy dial on first subscription.
func (c *Client) ensureBus(ctx context.Context) (*eventBus, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errClientClosed
	}
	bus := c.bus
	if bus == nil {
		bus = newEventBus(c)
		c.bus = bus
	}
	c.mu.Unlock()

	if err := bus.dial(ctx); err != nil {
		c.mu.Lock()
		if c.bus == bus {
			c.bus = nil
		}
		c.mu.Unlock()
		return nil, err
	}
	return bus, nil
}
