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
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zchee/pandaemonium/pkg/llm"
)

// startFakeOpencode serves fake over httptest and attaches a facade to it.
func startFakeOpencode(t *testing.T, fake *fakeOpencode, mutate func(*RemoteConfig)) *Opencode {
	t.Helper()

	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)

	cfg := &RemoteConfig{
		BaseURL:     server.URL,
		DialTimeout: 10 * time.Second,
		DrainWindow: 300 * time.Millisecond,
		Retry:       llm.RetryConfig{MaxAttempts: 2, InitialDelay: 10 * time.Millisecond, MaxDelay: 20 * time.Millisecond, JitterRatio: -1},
	}
	if mutate != nil {
		mutate(cfg)
	}
	oc, err := NewRemoteOpencode(t.Context(), cfg)
	if err != nil {
		t.Fatalf("NewRemoteOpencode: %v", err)
	}
	t.Cleanup(func() { _ = oc.Close() })
	return oc
}

// startBlockedTurn creates a session and starts a turn whose prompt blocks
// until fake.promptBlock is closed (or an abort fires).
func startBlockedTurn(t *testing.T, oc *Opencode, fake *fakeOpencode) (*Session, *TurnHandle) {
	t.Helper()

	fake.mu.Lock()
	if fake.promptBlock == nil {
		fake.promptBlock = make(chan struct{})
	}
	fake.mu.Unlock()

	session, err := oc.SessionStart(t.Context(), nil)
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	handle, err := session.Turn(t.Context(), "do something", nil)
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	select {
	case <-fake.promptStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("prompt never reached the fake server")
	}
	return session, handle
}

func releasePrompt(fake *fakeOpencode) {
	fake.mu.Lock()
	block := fake.promptBlock
	fake.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		default:
			close(block)
		}
	}
}

// TestTurnTruthTableSuccessWithIdle covers truth-table row 1: prompt success
// plus session.idle ends the stream cleanly exactly once, yielding the
// emitted events in order.
func TestTurnTruthTableSuccessWithIdle(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, nil)
	session, handle := startBlockedTurn(t, oc, fake)

	fake.emit(fakeEvent(EventTypeMessageUpdated, map[string]any{
		"sessionID": session.ID(),
		"info": map[string]any{
			"id": "msg_live", "sessionID": session.ID(), "role": "assistant",
			"time": map[string]any{"created": 1},
		},
	}))
	fake.emit(fakeEvent(EventTypeMessagePartUpdated, map[string]any{
		"sessionID": session.ID(),
		"part":      map[string]any{"id": "prt_live", "sessionID": session.ID(), "messageID": "msg_live", "type": "text", "text": "partial"},
	}))

	fake.emit(fakeEvent(EventTypeSessionIdle, map[string]any{"sessionID": session.ID()}))

	// Hold the prompt until the event-sourced message id is observed by the
	// stream loop below, so the event capture (not the response fallback) is
	// what this test verifies. The early idle is not terminal until then
	// (truth-table row 5).
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for handle.ID() == "" && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		releasePrompt(fake)
	}()

	var types []string
	var streamErr error
	for ev, err := range handle.Stream(t.Context()) {
		if err != nil {
			streamErr = err
			break
		}
		types = append(types, ev.Type)
	}
	if streamErr != nil {
		t.Fatalf("stream error: %v", streamErr)
	}
	want := []string{EventTypeMessageUpdated, EventTypeMessagePartUpdated}
	for i, wantType := range want {
		if i >= len(types) || types[i] != wantType {
			t.Fatalf("event order = %v, want prefix %v", types, want)
		}
	}
	if handle.ID() != "msg_live" {
		t.Errorf("handle.ID() = %q, want msg_live (from message.updated)", handle.ID())
	}
	if got := oc.Client().Counters().StreamsWithoutTerminal; got != 0 {
		t.Errorf("StreamsWithoutTerminal = %d, want 0", got)
	}

	// The turn is terminal: a second consumption attempt errors immediately.
	if _, err := handle.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "already completed") {
		t.Errorf("Run after terminal = %v, want already-completed error", err)
	}
}

// TestTurnTruthTableSuccessWithoutIdle covers row 2: prompt success with no
// terminal event ends cleanly after the bounded drain and increments the
// stream-without-terminal counter — never hangs.
func TestTurnTruthTableSuccessWithoutIdle(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, func(cfg *RemoteConfig) { cfg.DrainWindow = 100 * time.Millisecond })
	_, handle := startBlockedTurn(t, oc, fake)

	releasePrompt(fake)

	result, err := handle.Run(t.Context())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.FinalResponse != "hello from fake opencode" {
		t.Errorf("FinalResponse = %q", result.FinalResponse)
	}
	if got := oc.Client().Counters().StreamsWithoutTerminal; got != 1 {
		t.Errorf("StreamsWithoutTerminal = %d, want 1", got)
	}
}

// TestTurnTruthTableSessionErrorOutweighsSuccess covers row 3: an explicit
// session.error for our session turns an HTTP-success prompt into
// TurnFailedError.
func TestTurnTruthTableSessionErrorOutweighsSuccess(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, nil)
	session, handle := startBlockedTurn(t, oc, fake)

	fake.emit(fakeEvent(EventTypeSessionError, map[string]any{
		"sessionID": session.ID(),
		"error":     map[string]any{"name": "ProviderAuthError", "data": map[string]any{"message": "expired key"}},
	}))
	// Give the bus a moment to route the error before the outcome arrives, so
	// this exercises the error-before-outcome ordering deterministically.
	time.Sleep(100 * time.Millisecond)
	releasePrompt(fake)

	_, err := handle.Run(t.Context())
	var failed *TurnFailedError
	if !errors.As(err, &failed) {
		t.Fatalf("err = %v (%T), want TurnFailedError", err, err)
	}
	if failed.Err.Name != "ProviderAuthError" || failed.SessionID != session.ID() {
		t.Errorf("TurnFailedError = %+v", failed)
	}
}

// TestTurnTruthTablePromptError covers row 4: a failed prompt request ends
// the turn with the mapped HTTP error regardless of events.
func TestTurnTruthTablePromptError(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	fake.promptStatus = http.StatusInternalServerError
	oc := startFakeOpencode(t, fake, nil)

	session, err := oc.SessionStart(t.Context(), nil)
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	handle, err := session.Turn(t.Context(), "boom", nil)
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}

	_, err = handle.Run(t.Context())
	var api *APIError
	if !errors.As(err, &api) {
		t.Fatalf("err = %v (%T), want APIError", err, err)
	}
	if api.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", api.StatusCode)
	}
}

// TestTurnTruthTableEarlyIdleNotTerminal covers row 5: a session.idle seen
// while the prompt goroutine is still running must not truncate the stream.
func TestTurnTruthTableEarlyIdleNotTerminal(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, nil)
	session, handle := startBlockedTurn(t, oc, fake)

	fake.emit(fakeEvent(EventTypeSessionIdle, map[string]any{"sessionID": session.ID()}))
	fake.emit(fakeEvent("custom.after.idle", map[string]any{"sessionID": session.ID()}))

	sawAfterIdle := make(chan bool, 1)
	go func() {
		var afterIdle bool
		var idleSeen bool
		for ev, err := range handle.Stream(t.Context()) {
			if err != nil {
				break
			}
			if ev.Type == EventTypeSessionIdle {
				idleSeen = true
			}
			if idleSeen && ev.Type == "custom.after.idle" {
				afterIdle = true
			}
		}
		sawAfterIdle <- afterIdle
	}()

	// The stream must still be alive after the early idle; release the
	// prompt only after the post-idle event had time to flow.
	time.Sleep(150 * time.Millisecond)
	releasePrompt(fake)

	select {
	case afterIdle := <-sawAfterIdle:
		if !afterIdle {
			t.Fatal("stream ended at early idle; row 5 violated (spurious idle truncated the stream)")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("stream never terminated")
	}
}

// TestTurnTruthTableBusLossDegradesEventsNotResults covers row 6: reconnect
// exhaustion surfaces TransportClosedError to Stream, while Run still
// resolves from the authoritative prompt outcome.
func TestTurnTruthTableBusLossDegradesEventsNotResults(t *testing.T) {
	t.Parallel()

	t.Run("run resolves despite bus loss", func(t *testing.T) {
		t.Parallel()

		fake := newFakeOpencode()
		oc := startFakeOpencode(t, fake, nil)
		_, handle := startBlockedTurn(t, oc, fake)

		fake.setFailEventDials(true)
		fake.closeSubscribers()

		// Give the bus time to exhaust its bounded reconnect budget.
		time.Sleep(200 * time.Millisecond)
		releasePrompt(fake)

		result, err := handle.Run(t.Context())
		if err != nil {
			t.Fatalf("Run must survive bus loss, got: %v", err)
		}
		if result.FinalResponse == "" {
			t.Error("FinalResponse empty")
		}
	})

	t.Run("stream surfaces transport error", func(t *testing.T) {
		t.Parallel()

		fake := newFakeOpencode()
		oc := startFakeOpencode(t, fake, nil)
		_, handle := startBlockedTurn(t, oc, fake)

		fake.setFailEventDials(true)
		fake.closeSubscribers()

		var streamErr error
		for _, err := range handle.Stream(t.Context()) {
			if err != nil {
				streamErr = err
				break
			}
		}
		if _, ok := errors.AsType[*TransportClosedError](streamErr); !ok {
			t.Fatalf("stream err = %v (%T), want TransportClosedError", streamErr, streamErr)
		}

		// The turn itself still resolves once the prompt returns.
		releasePrompt(fake)
		if _, err := handle.Run(t.Context()); err != nil {
			t.Fatalf("Run after stream transport loss: %v", err)
		}
	})
}

// TestTurnReconnectGapNotification: a dropped-and-redialed /event stream
// delivers EventTypeGap to the live consumer and counts a reconnect.
func TestTurnReconnectGapNotification(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, func(cfg *RemoteConfig) {
		cfg.Retry = llm.RetryConfig{MaxAttempts: 5, InitialDelay: 10 * time.Millisecond, MaxDelay: 20 * time.Millisecond, JitterRatio: -1}
	})
	session, handle := startBlockedTurn(t, oc, fake)

	// Drain the initial connection's signal so the wait below observes the
	// reconnect, not the facade's eager dial.
	for len(fake.subOpened) > 0 {
		<-fake.subOpened
	}

	// Drop the live stream; dials stay allowed so the bus reconnects.
	fake.closeSubscribers()
	select {
	case <-fake.subOpened:
	case <-time.After(10 * time.Second):
		t.Fatal("bus never reconnected")
	}

	fake.emit(fakeEvent("custom.post.reconnect", map[string]any{"sessionID": session.ID()}))
	releasePrompt(fake)
	fake.emit(fakeEvent(EventTypeSessionIdle, map[string]any{"sessionID": session.ID()}))

	var sawGap, sawPostReconnect bool
	for ev, err := range handle.Stream(t.Context()) {
		if err != nil {
			t.Fatalf("stream error after reconnect: %v", err)
		}
		switch ev.Type {
		case EventTypeGap:
			sawGap = true
		case "custom.post.reconnect":
			sawPostReconnect = true
		}
	}
	if !sawGap {
		t.Error("no gap event delivered after reconnect")
	}
	if !sawPostReconnect {
		t.Error("post-reconnect event lost")
	}
	counters := oc.Client().Counters()
	if counters.SSEReconnects != 1 {
		t.Errorf("SSEReconnects = %d, want 1", counters.SSEReconnects)
	}
	if counters.GapNotifications == 0 {
		t.Error("GapNotifications = 0, want > 0")
	}
}

// TestTurnStreamWireFraming: adversarial wire framing (comments, multi-line
// data, CRLF) decodes into ordered events, and unknown types pass through raw.
func TestTurnStreamWireFraming(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, nil)
	session, handle := startBlockedTurn(t, oc, fake)

	fake.emitRaw(": heartbeat comment\n\n")
	fake.emitRaw("data: {\"id\":\"evt_m\",\"type\":\"custom.multiline\",\ndata: \"properties\":{\"sessionID\":\"" + session.ID() + "\"}}\n\n")
	fake.emitRaw("data: {\"id\":\"evt_c\",\"type\":\"custom.crlf\",\"properties\":{\"sessionID\":\"" + session.ID() + "\"}}\r\n\r\n")
	releasePrompt(fake)
	fake.emit(fakeEvent(EventTypeSessionIdle, map[string]any{"sessionID": session.ID()}))

	var got []string
	for ev, err := range handle.Stream(t.Context()) {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		if strings.HasPrefix(ev.Type, "custom.") {
			got = append(got, ev.Type)
		}
	}
	want := []string{"custom.multiline", "custom.crlf"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("decoded custom events = %v, want %v", got, want)
	}
}

// TestTurnSecondConsumerRejected: the one-active-consumer contract (AC4).
func TestTurnSecondConsumerRejected(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, nil)
	_, handle := startBlockedTurn(t, oc, fake)

	runDone := make(chan error, 1)
	go func() {
		_, err := handle.Run(t.Context())
		runDone <- err
	}()

	// Wait until the Run consumer is active, then a concurrent Stream must
	// fail immediately.
	deadline := time.Now().Add(5 * time.Second)
	for {
		handle.mu.Lock()
		consuming := handle.consuming
		handle.mu.Unlock()
		if consuming {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Run consumer never became active")
		}
		time.Sleep(5 * time.Millisecond)
	}

	var streamErr error
	for _, err := range handle.Stream(t.Context()) {
		streamErr = err
		break
	}
	if streamErr == nil || !strings.Contains(streamErr.Error(), "stream already active") {
		t.Fatalf("second consumer error = %v, want stream-already-active", streamErr)
	}

	releasePrompt(fake)
	if err := <-runDone; err != nil {
		t.Fatalf("primary Run failed: %v", err)
	}
}

// TestTurnOneActiveTurnPerSession: a second Turn on the same session is
// rejected while the first is in flight (concurrency contract).
func TestTurnOneActiveTurnPerSession(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, nil)
	session, handle := startBlockedTurn(t, oc, fake)

	if _, err := session.Turn(t.Context(), "second", nil); err == nil || !strings.Contains(err.Error(), "active turn") {
		t.Fatalf("second Turn = %v, want active-turn error", err)
	}
	if _, err := session.Run(t.Context(), "second", nil); err == nil || !strings.Contains(err.Error(), "active turn") {
		t.Fatalf("concurrent sync Run = %v, want active-turn error", err)
	}

	releasePrompt(fake)
	if _, err := handle.Run(t.Context()); err != nil {
		t.Fatalf("first turn failed: %v", err)
	}

	// The slot frees after the turn completes.
	if _, err := session.Run(t.Context(), "third", nil); err != nil {
		t.Fatalf("Run after turn completion: %v", err)
	}
}

// TestTurnStreamBreakThenRunResumes is the regression for the
// stranded-outcome defect: a consumer that receives the prompt outcome and
// then stops early (break inside Stream, mid-drain) must not lose it — a
// follow-up Run resumes from the persisted outcome instead of hanging on the
// already-drained channel.
func TestTurnStreamBreakThenRunResumes(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, func(cfg *RemoteConfig) { cfg.DrainWindow = time.Second })
	session, handle := startBlockedTurn(t, oc, fake)

	releasePrompt(fake)

	// Emit a marker only after the active consumer has persisted the
	// outcome, so the break below deterministically lands inside the
	// post-outcome drain window (the defect's trigger).
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for handle.storedOutcome() == nil && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		fake.emit(fakeEvent("custom.break.marker", map[string]any{"sessionID": session.ID()}))
	}()

	sawMarker := false
	for ev, err := range handle.Stream(t.Context()) {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		if ev.Type == "custom.break.marker" {
			sawMarker = true
			break // early stop while the drain window is armed
		}
	}
	if !sawMarker {
		t.Fatal("marker event never observed; scenario not exercised")
	}

	runCtx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	result, err := handle.Run(runCtx)
	if err != nil {
		t.Fatalf("Run after early-stopped Stream must resume from the persisted outcome, got: %v", err)
	}
	if result.FinalResponse != "hello from fake opencode" {
		t.Errorf("FinalResponse = %q", result.FinalResponse)
	}
}

// TestEnsureBusSurvivesCanceledDialCtx is the regression for the
// shared-bus-teardown defect: a dial with an already-canceled ctx against an
// established bus must succeed via the connected fast path and never close
// the bus other sessions depend on.
func TestEnsureBusSurvivesCanceledDialCtx(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, nil) // eager dial: bus is connected

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	bus, err := oc.Client().ensureBus(canceled)
	if err != nil {
		t.Fatalf("ensureBus with canceled ctx on a connected bus: %v", err)
	}
	if bus.isClosed() {
		t.Fatal("established shared bus was torn down by a canceled dial ctx")
	}

	// The bus still serves turns end to end.
	session, err := oc.SessionStart(t.Context(), nil)
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	handle, err := session.Turn(t.Context(), "still alive?", nil)
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if _, err := handle.Run(t.Context()); err != nil {
		t.Fatalf("Run over the surviving bus: %v", err)
	}
}

// TestBusConsumerOverflowKeepsNewest pins the drop-oldest policy: on
// overflow the newest events survive and the loss is coalesced into one gap
// marker at the head.
func TestBusConsumerOverflowKeepsNewest(t *testing.T) {
	t.Parallel()

	consumer := newBusConsumer(1, "ses_x")
	total := eventQueueCapacity + 8
	for i := range total {
		consumer.push(Event{Type: "custom.seq", ID: strconv.Itoa(i)})
	}

	var drained []Event
	for range eventQueueCapacity {
		ev, err := consumer.next(t.Context())
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		drained = append(drained, ev)
	}
	if drained[0].Type != EventTypeGap {
		t.Fatalf("head = %+v, want gap marker", drained[0])
	}
	newest := drained[len(drained)-1]
	if newest.ID != strconv.Itoa(total-1) {
		t.Fatalf("newest surviving event = %q, want %d (newest must never be dropped)", newest.ID, total-1)
	}
}

// TestTurnInterrupt is AC5: Interrupt triggers POST /session/{id}/abort and
// the turn surfaces MessageAbortedError.
func TestTurnInterrupt(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, nil)
	session, handle := startBlockedTurn(t, oc, fake)

	if _, err := handle.Interrupt(t.Context()); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	_, err := handle.Run(t.Context())
	var aborted *MessageAbortedError
	if !errors.As(err, &aborted) {
		t.Fatalf("err = %v (%T), want MessageAbortedError", err, err)
	}
	if aborted.SessionID != session.ID() {
		t.Errorf("aborted.SessionID = %q, want %q", aborted.SessionID, session.ID())
	}

	var sawAbort bool
	for _, req := range fake.recordedRequests() {
		if req.Method == http.MethodPost && strings.HasSuffix(req.Path, "/abort") {
			sawAbort = true
		}
	}
	if !sawAbort {
		t.Error("no POST /session/{id}/abort recorded")
	}
}
