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
	"iter"
	"sync"
	"time"
)

// abortOnCancelTimeout bounds the best-effort server-side abort issued when a
// caller cancels a sync Session.Run (divergence: canceling the HTTP request
// alone would leave the server generating).
const abortOnCancelTimeout = 5 * time.Second

// Opencode is the high-level synchronous facade for one OpenCode server,
// mirroring the pkg/llm/codex Codex facade. NewOpencode spawns and owns an
// `opencode serve` process; NewRemoteOpencode attaches to a running one.
// Both dial the shared event bus eagerly so the client-lifetime permission
// consumer is live before any prompt is issued.
type Opencode struct {
	client *Client
}

// NewOpencode spawns `opencode serve`, confirms health, and eagerly dials the
// shared event bus (failing fast if the dial or the server.connected
// handshake does not complete within Config.DialTimeout).
func NewOpencode(ctx context.Context, config *Config) (*Opencode, error) {
	client := NewClient(config)
	if err := client.Start(ctx); err != nil {
		return nil, err
	}
	if _, err := client.ensureBus(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Opencode{client: client}, nil
}

// Health fetches the server health document (GET /global/health).
func (o *Opencode) Health(ctx context.Context) (Health, error) {
	return o.client.Health(ctx)
}

// Close shuts down the client and terminates the spawned server, if any.
func (o *Opencode) Close() error {
	if o == nil || o.client == nil {
		return nil
	}
	return o.client.Close()
}

// Client exposes the lower-level HTTP + SSE client.
func (o *Opencode) Client() *Client {
	return o.client
}

// SessionStart creates a new session.
func (o *Opencode) SessionStart(ctx context.Context, params *SessionNewParams) (*Session, error) {
	info, err := o.client.SessionNew(ctx, params)
	if err != nil {
		return nil, err
	}
	return &Session{client: o.client, id: info.ID}, nil
}

// SessionResume attaches to an existing session, verifying it exists.
func (o *Opencode) SessionResume(ctx context.Context, sessionID string) (*Session, error) {
	info, err := o.client.SessionGet(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return &Session{client: o.client, id: info.ID}, nil
}

// SessionFork forks an existing session (POST /session/{id}/fork),
// optionally at a specific message.
func (o *Opencode) SessionFork(ctx context.Context, sessionID string, params *SessionForkParams) (*Session, error) {
	info, err := o.client.SessionFork(ctx, sessionID, params)
	if err != nil {
		return nil, err
	}
	return &Session{client: o.client, id: info.ID}, nil
}

// SessionList lists sessions.
func (o *Opencode) SessionList(ctx context.Context) ([]SessionInfo, error) {
	return o.client.SessionList(ctx)
}

// SessionDelete deletes a session.
func (o *Opencode) SessionDelete(ctx context.Context, sessionID string) (bool, error) {
	return o.client.SessionDelete(ctx, sessionID)
}

// Providers lists configured providers and per-provider default models.
func (o *Opencode) Providers(ctx context.Context) (ProvidersResponse, error) {
	return o.client.Providers(ctx)
}

// Session is a high-level handle for one OpenCode session (the codex Thread
// analog). Each Session supports at most one active turn at a time.
type Session struct {
	client *Client
	id     string
}

// ID returns the session id.
func (s *Session) ID() string { return s.id }

// promptParams normalizes input into the prompt request body, merging
// caller-supplied params (Parts always comes from input; NoReply is
// rejected because a no-reply injection produces no turn to consume — use
// Client.Prompt directly for history injections).
func (s *Session) promptParams(input RunInput, params *PromptParams) (*PromptParams, error) {
	parts, err := normalizeInput(input)
	if err != nil {
		return nil, err
	}
	merged := PromptParams{}
	if params != nil {
		merged = *params
	}
	if merged.NoReply {
		return nil, errors.New("opencode: NoReply is not a turn; use Client.Prompt for history injections")
	}
	merged.Parts = parts
	return &merged, nil
}

// Run sends a prompt and blocks until the turn completes, returning the
// aggregated result. Canceling ctx aborts only the HTTP request — the server
// keeps generating — so Run installs a best-effort POST /session/{id}/abort
// on cancellation, bounded by the client lifetime.
func (s *Session) Run(ctx context.Context, input RunInput, params *PromptParams) (RunResult, error) {
	prompt, err := s.promptParams(input, params)
	if err != nil {
		return RunResult{}, err
	}
	if err := s.client.beginTurn(s.id); err != nil {
		return RunResult{}, err
	}
	defer s.client.endTurn(s.id)

	resp, err := s.client.Prompt(ctx, s.id, prompt)
	if err != nil && ctx.Err() != nil && s.client.lifetime.Err() == nil {
		_ = s.client.goWork(func() {
			abortCtx, cancel := context.WithTimeout(s.client.lifetime, abortOnCancelTimeout)
			defer cancel()
			_, _ = s.client.Abort(abortCtx, s.id)
		})
	}
	return promptOutcome(s.id, &resp, err)
}

// Turn starts an asynchronous turn and returns a handle for streaming,
// interrupting, or running it to completion.
//
// OpenCode has no server-side turn resource: the handle is synthesized by
// registering a session-scoped consumer on the live shared event bus first,
// then running the blocking prompt in a goroutine bound to the client
// lifetime (not ctx, which governs only turn startup).
func (s *Session) Turn(ctx context.Context, input RunInput, params *PromptParams) (*TurnHandle, error) {
	prompt, err := s.promptParams(input, params)
	if err != nil {
		return nil, err
	}

	bus, err := s.client.ensureBus(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.client.beginTurn(s.id); err != nil {
		return nil, err
	}
	consumer, err := bus.subscribe(s.id)
	if err != nil {
		s.client.endTurn(s.id)
		return nil, err
	}

	handle := &TurnHandle{
		client:    s.client,
		bus:       bus,
		sessionID: s.id,
		consumer:  consumer,
		outcome:   make(chan turnOutcome, 1),
	}

	accepted := s.client.goWork(func() {
		resp, err := s.client.Prompt(s.client.lifetime, s.id, prompt)
		if err == nil {
			handle.setMessageID(resp.Info.ID)
		}
		// The server-side turn is over once the blocking prompt returns;
		// release the session's one-active-turn slot even if the handle is
		// never consumed.
		s.client.endTurn(s.id)
		handle.outcome <- turnOutcome{resp: resp, err: err}
	})
	if !accepted {
		s.client.endTurn(s.id)
		bus.unsubscribe(consumer)
		return nil, errClientClosed
	}

	return handle, nil
}

// Read returns the session state and its messages with parts
// (GET /session/{id} + GET /session/{id}/message).
func (s *Session) Read(ctx context.Context) (SessionRead, error) {
	info, err := s.client.SessionGet(ctx, s.id)
	if err != nil {
		return SessionRead{}, err
	}
	messages, err := s.client.SessionMessages(ctx, s.id)
	if err != nil {
		return SessionRead{}, err
	}
	return SessionRead{Info: info, Messages: messages}, nil
}

// SessionRead aggregates session state with its message history.
type SessionRead struct {
	Info     SessionInfo
	Messages []MessageWithParts
}

// SetTitle renames the session (PATCH /session/{id}).
func (s *Session) SetTitle(ctx context.Context, title string) (SessionInfo, error) {
	return s.client.SessionUpdate(ctx, s.id, &SessionUpdateParams{Title: title})
}

// Summarize compacts the session history (POST /session/{id}/summarize).
// The server requires an explicit provider/model pair.
func (s *Session) Summarize(ctx context.Context, providerID, modelID string) (bool, error) {
	return s.client.Summarize(ctx, s.id, &SummarizeParams{ProviderID: providerID, ModelID: modelID})
}

// Revert stages a revert to messageID (POST /session/{id}/revert); partID
// optionally narrows the revert point to one part.
func (s *Session) Revert(ctx context.Context, messageID, partID string) (SessionInfo, error) {
	return s.client.Revert(ctx, s.id, &RevertParams{MessageID: messageID, PartID: partID})
}

// Unrevert clears a staged revert (POST /session/{id}/unrevert).
func (s *Session) Unrevert(ctx context.Context) (SessionInfo, error) {
	return s.client.Unrevert(ctx, s.id)
}

// Shell runs a shell command in the session context
// (POST /session/{id}/shell). Agent and command are required by the server.
func (s *Session) Shell(ctx context.Context, agent, command string) (MessageWithParts, error) {
	return s.client.Shell(ctx, s.id, &ShellParams{Agent: agent, Command: command})
}

// Command runs a configured slash command (POST /session/{id}/command).
func (s *Session) Command(ctx context.Context, params *CommandParams) (PromptResponse, error) {
	return s.client.Command(ctx, s.id, params)
}

// Share publishes the session and returns its state including the share URL
// (POST /session/{id}/share). OpenCode has no archive; share/unshare is the
// closest lifecycle analog and is named for what it actually does.
func (s *Session) Share(ctx context.Context) (SessionInfo, error) {
	return s.client.Share(ctx, s.id)
}

// Unshare unpublishes the session (DELETE /session/{id}/share).
func (s *Session) Unshare(ctx context.Context) (SessionInfo, error) {
	return s.client.Unshare(ctx, s.id)
}

// Fork forks this session (POST /session/{id}/fork), optionally at
// messageID ("" forks the whole session).
func (s *Session) Fork(ctx context.Context, messageID string) (*Session, error) {
	info, err := s.client.SessionFork(ctx, s.id, &SessionForkParams{MessageID: messageID})
	if err != nil {
		return nil, err
	}
	return &Session{client: s.client, id: info.ID}, nil
}

// turnOutcome is the prompt goroutine's result: the authoritative terminal
// signal for a synthesized turn.
type turnOutcome struct {
	resp PromptResponse
	err  error
}

// TurnHandle controls or consumes one wrapper-synthesized turn. It has no
// server-side identity: OpenCode exposes only a blocking prompt, so the
// handle pairs the prompt goroutine's outcome (authoritative) with the
// session-scoped event subscription (enrichment). There is no Steer:
// OpenCode cannot steer an active turn (a mid-turn prompt only becomes
// visible to the next turn).
type TurnHandle struct {
	client    *Client
	bus       *eventBus
	sessionID string
	consumer  *busConsumer
	outcome   chan turnOutcome

	mu        sync.Mutex
	messageID string
	consuming bool
	finished  bool
	// out persists the prompt goroutine's result once received, so an
	// interrupted consumer (early stream stop, ctx cancel) never strands the
	// turn: the next Stream/Run resumes from it instead of waiting on the
	// already-drained outcome channel.
	out *turnOutcome
	// progress persists the session-scoped terminal evidence across consumer
	// generations for the same reason.
	progress turnProgress
}

// setOutcome persists the prompt result exactly once.
func (h *TurnHandle) setOutcome(out *turnOutcome) {
	h.mu.Lock()
	if h.out == nil {
		h.out = out
	}
	h.mu.Unlock()
}

// storedOutcome returns the persisted prompt result, nil until received.
func (h *TurnHandle) storedOutcome() *turnOutcome {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.out
}

// SessionID returns the session this turn runs on.
func (h *TurnHandle) SessionID() string { return h.sessionID }

// ID returns the assistant message id of this turn once it is known (from a
// message.updated event or the completed prompt response); "" until then.
func (h *TurnHandle) ID() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.messageID
}

func (h *TurnHandle) setMessageID(id string) {
	if id == "" {
		return
	}
	h.mu.Lock()
	if h.messageID == "" {
		h.messageID = id
	}
	h.mu.Unlock()
}

// Interrupt aborts the session's active turn (POST /session/{id}/abort).
// The turn then terminates through its normal path, surfacing
// MessageAbortedError.
func (h *TurnHandle) Interrupt(ctx context.Context) (bool, error) {
	return h.client.Abort(ctx, h.sessionID)
}

// acquireConsumer enforces the one-active-stream-consumer contract.
func (h *TurnHandle) acquireConsumer() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.finished {
		return fmt.Errorf("opencode: turn on session %s already completed", h.sessionID)
	}
	if h.consuming {
		return fmt.Errorf("opencode: turn stream already active for session %s", h.sessionID)
	}
	h.consuming = true
	return nil
}

func (h *TurnHandle) releaseConsumer() {
	h.mu.Lock()
	h.consuming = false
	h.mu.Unlock()
}

// finish marks the turn terminal exactly once and deregisters its consumer.
func (h *TurnHandle) finish() {
	h.mu.Lock()
	if h.finished {
		h.mu.Unlock()
		return
	}
	h.finished = true
	h.mu.Unlock()
	h.bus.unsubscribe(h.consumer)
}

// Stream returns an iterator over this turn's session-scoped events until
// the turn terminates.
//
// The iterator yields (Event{}, err) once and stops on failure: the mapped
// turn error, a TransportClosedError when bus reconnect is exhausted, or ctx
// errors. It yields no error on normal completion. A gap event
// (EventTypeGap) is yielded after a bus reconnect: events may have been
// lost. At most one Stream/Run consumer may be active at a time; stopping
// iteration early releases the consumer without terminating the turn.
func (h *TurnHandle) Stream(ctx context.Context) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		_, err := h.consume(ctx, func(ev Event) bool {
			return yield(ev, nil)
		})
		if err != nil && !errors.Is(err, errEarlyStop) {
			yield(Event{}, err)
		}
	}
}

// Run consumes this turn to completion and returns the aggregated result.
// The result comes from the blocking prompt response; events enrich the
// terminal decision but their loss (bus reconnect exhaustion) degrades only
// streaming, never the result.
func (h *TurnHandle) Run(ctx context.Context) (RunResult, error) {
	return h.consume(ctx, nil)
}

// errEarlyStop is an internal sentinel: the stream consumer stopped
// iterating before the turn terminated.
var errEarlyStop = errors.New("opencode: stream consumer stopped early")

// turnProgress accumulates the session-scoped terminal evidence observed
// while a turn is being consumed. It lives on the handle (guarded by h.mu)
// so evidence survives an interrupted consumer.
type turnProgress struct {
	sawIdle bool
	sessErr *SessionErrorProperties
}

// note records one bus event's contribution to terminal detection and the
// handle's assistant message id.
func (h *TurnHandle) note(ev Event) {
	switch ev.Type {
	case EventTypeMessageUpdated:
		if props, ok := ev.MessageUpdated(); ok && props.Info.Role == "assistant" {
			h.setMessageID(props.Info.ID)
		}
	case EventTypeSessionError:
		if props, ok := ev.SessionError(); ok {
			h.mu.Lock()
			h.progress.sessErr = &props
			h.mu.Unlock()
		}
	case EventTypeSessionIdle:
		h.mu.Lock()
		h.progress.sawIdle = true
		h.mu.Unlock()
	}
}

// turnOver reports whether the truth table declares the turn terminal, given
// that the prompt goroutine has returned (out non-nil is the caller's
// responsibility): any prompt error, a turn-level error on the response, an
// explicit session.error, or the session-idle terminal event.
func (h *TurnHandle) turnOver(out *turnOutcome) bool {
	h.mu.Lock()
	progress := h.progress
	h.mu.Unlock()
	return out.err != nil || out.resp.Info.Error != nil || progress.sessErr != nil || progress.sawIdle
}

// resolve produces the terminal (RunResult, error) once turnOver (or the
// bounded drain) declared the turn over, and deregisters the consumer.
func (h *TurnHandle) resolve(out *turnOutcome) (RunResult, error) {
	h.mu.Lock()
	progress := h.progress
	h.mu.Unlock()

	h.finish()
	switch {
	case progress.sessErr != nil:
		// An explicit session.error for our session outweighs HTTP success
		// (and enriches an opaque prompt failure).
		turnErr := MessageError{Name: errorNameUnknown}
		if progress.sessErr.Error != nil {
			turnErr = *progress.sessErr.Error
		}
		return RunResult{}, mapTurnError(h.sessionID, h.ID(), turnErr)
	case out.err != nil:
		return RunResult{}, h.classifyPromptError(out.err)
	default:
		return promptOutcome(h.sessionID, &out.resp, nil)
	}
}

// pumpEvents forwards the consumer's queue into a selectable channel; the
// pump goroutine exits when ctx ends or the consumer closes (its terminal
// error lands on the second channel).
func (h *TurnHandle) pumpEvents(ctx context.Context) (eventCh <-chan Event, errCh <-chan error) {
	events := make(chan Event)
	pumpErr := make(chan error, 1)
	go func() {
		for {
			ev, err := h.consumer.next(ctx)
			if err != nil {
				pumpErr <- err
				return
			}
			select {
			case events <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, pumpErr
}

// consume drives the turn to its terminal state, implementing the
// dual-source truth table (plan §6 step 6): the prompt goroutine's return is
// necessary and authoritative; session-scoped events enrich but never
// preempt it. yield forwards events for Stream mode; nil selects Run mode.
//
//nolint:cyclop // select-loop state machine mirroring the six-row terminal truth table; splitting the select would obscure the contract.
func (h *TurnHandle) consume(ctx context.Context, yield func(Event) bool) (RunResult, error) {
	if err := h.acquireConsumer(); err != nil {
		return RunResult{}, err
	}
	defer h.releaseConsumer()

	pumpCtx, cancelPump := context.WithCancel(ctx)
	defer cancelPump()
	events, pumpErr := h.pumpEvents(pumpCtx)

	streaming := yield != nil
	outcomeCh := h.outcome
	var drainCh <-chan time.Time
	var drainTimer *time.Timer
	defer func() {
		if drainTimer != nil {
			drainTimer.Stop()
		}
	}()

	// A previous, interrupted consumer may already have received the prompt
	// outcome: resume from the persisted state instead of waiting on the
	// already-drained outcome channel (which would hang forever). The
	// one-shot drain grace belonged to the consumer that received the
	// outcome; the authoritative result is in hand, so a resumed consumer
	// resolves immediately — counted when no terminal event was observed.
	if out := h.storedOutcome(); out != nil {
		if !h.turnOver(out) {
			h.client.counters.streamsWithoutTerminal.Add(1)
		}
		return h.resolve(out)
	}

	var out *turnOutcome
	for {
		select {
		case <-ctx.Done():
			return RunResult{}, ctx.Err()

		case outcome := <-outcomeCh:
			h.setOutcome(&outcome)
			out = &outcome
			outcomeCh = nil // the buffered send arrives exactly once
			if h.turnOver(out) {
				return h.resolve(out)
			}
			// Success without a terminal event yet: bounded drain.
			drainTimer = time.NewTimer(h.client.config.DrainWindow)
			drainCh = drainTimer.C

		case <-drainCh:
			// Clean end without session.idle: not an error, but observable.
			h.client.counters.streamsWithoutTerminal.Add(1)
			return h.resolve(out)

		case ev := <-events:
			h.note(ev)
			if streaming && !yield(ev) {
				return RunResult{}, errEarlyStop
			}
			// Events are terminal only after the prompt goroutine returned
			// (an early or spurious idle must not truncate the stream).
			if out != nil && h.turnOver(out) {
				return h.resolve(out)
			}

		case err := <-pumpErr:
			if streaming {
				// Stream mode: the event stream is the product; surface the
				// transport failure. The turn itself is not finished — Run
				// can still resolve from the prompt outcome.
				return RunResult{}, err
			}
			// Run mode: bus loss degrades events, not results — keep
			// waiting for the authoritative prompt outcome.
			events = nil
			pumpErr = nil
			if out != nil {
				return h.resolve(out)
			}
		}
	}
}

// classifyPromptError maps a failed prompt goroutine outcome. A prompt
// aborted by client shutdown surfaces as TransportClosedError.
func (h *TurnHandle) classifyPromptError(err error) error {
	if h.client.lifetime.Err() != nil {
		return &TransportClosedError{Message: "opencode client closed during turn: " + err.Error()}
	}
	return err
}
