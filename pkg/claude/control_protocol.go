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

package claude

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// controlProtocol implements the bidirectional control protocol layer on top of
// the raw line-oriented transport, porting the upstream Query class
// (claude-agent-sdk-python _internal/query.py).
//
// It owns no transport and no write mutex: outbound writes are funnelled
// through the injected writeFn (ClaudeSDKClient.writeMessage, guarded by
// writeMu) so the single exclusion discipline lives in one place. See the
// race-safety documentation on [ClaudeSDKClient].
type controlProtocol struct {
	// writeFn writes a single stream-JSON payload to the CLI subprocess stdin.
	// It is ClaudeSDKClient.writeMessage, which serializes writes under writeMu.
	writeFn func(context.Context, []byte) error

	// opts is the session configuration. Read-only after construction.
	opts *Options

	// pendingMu guards pending.
	pendingMu sync.Mutex
	// pending maps an outbound request_id to the buffered channel awaiting its
	// control_response. Each channel has capacity 1 so route can deliver under
	// pendingMu without blocking.
	pending map[string]chan controlResult

	// counter generates monotonic request IDs for outbound control requests.
	counter atomic.Uint64

	// hookCallbacks maps a callback ID (assigned at initialize time) to its Go
	// hook. Populated in M4; allocated now so the map is never nil.
	hookCallbacks map[string]Hook

	// mcpServers maps a server name to its in-process MCP bridge. Populated in
	// M5; allocated now so the map is never nil.
	mcpServers map[string]*inProcessMCPServer

	// inflightMu guards inflight.
	inflightMu sync.Mutex
	// inflight maps an inbound (CLI→SDK) request_id to its handler's cancel
	// function so a control_cancel_request can abort it. Populated in M4;
	// allocated now so the map is never nil.
	inflight map[string]context.CancelFunc

	// initializationResult stores the CLI's response to the initialize
	// handshake (the inner response object). Returned by GetServerInfo (M6).
	initializationResult jsontext.Value
}

// controlResult is the outcome of an outbound control request: exactly one of
// response (on subtype=="success") or err (on subtype=="error" or transport
// failure) is set.
type controlResult struct {
	response jsontext.Value
	err      error
}

// newControlProtocol constructs a controlProtocol bound to writeFn (the
// writeMu-guarded writer) and opts. All maps are pre-allocated so later
// milestones can populate them without nil checks.
func newControlProtocol(opts *Options, writeFn func(context.Context, []byte) error) *controlProtocol {
	return &controlProtocol{
		writeFn:       writeFn,
		opts:          opts,
		pending:       make(map[string]chan controlResult),
		hookCallbacks: make(map[string]Hook),
		mcpServers:    make(map[string]*inProcessMCPServer),
		inflight:      make(map[string]context.CancelFunc),
	}
}

// controlResponseEnvelope is the wire shape of an inbound control_response.
// The inner response object is captured as raw JSON so success payloads can be
// delivered without an intermediate decode.
type controlResponseEnvelope struct {
	Response struct {
		RequestID string         `json:"request_id"`
		Subtype   string         `json:"subtype"`
		Response  jsontext.Value `json:"response"`
		Error     string         `json:"error"`
	} `json:"response"`
}

// route inspects a single stream-JSON line read from the CLI and dispatches it
// if it is a control-protocol message.
//
// It returns consumed=true when the line was a control message that route
// handled (and the caller must not treat it as a regular SDK message), and
// consumed=false when the line is an ordinary message (assistant/user/result/
// system/etc.) that the caller should parse and surface to the consumer.
//
// Malformed JSON returns (false, nil) so the existing rawMessages path still
// surfaces it downstream as a CLIJSONDecodeError; route never swallows it.
func (cp *controlProtocol) route(ctx context.Context, line []byte) (consumed bool, err error) {
	// Peek the type discriminator, mirroring parseMessage's peek pattern.
	var env struct {
		Type string `json:"type"`
	}
	if uerr := json.Unmarshal(line, &env); uerr != nil {
		// Malformed JSON is not a control message; let the normal path report it.
		return false, nil
	}

	switch env.Type {
	case "control_response":
		cp.routeControlResponse(line)
		return true, nil

	case "control_request":
		// M1 stub: consume and do nothing. M4 dispatches inbound requests.
		return true, nil

	case "control_cancel_request":
		// M1 stub: consume and do nothing. M4 cancels the matching handler.
		return true, nil

	case "transcript_mirror":
		// Consumed and ignored: per the Decision Log, transcript mirroring is a
		// separate concern and must NOT be forwarded to the consumer stream.
		return true, nil

	default:
		// Not a control message — caller handles it as a regular SDK message.
		return false, nil
	}
}

// routeControlResponse delivers an inbound control_response to the channel
// registered for its request_id, if any. A malformed envelope is ignored
// (treated as consumed by the caller) since it cannot be matched to a waiter.
func (cp *controlProtocol) routeControlResponse(line []byte) {
	var env controlResponseEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return
	}
	reqID := env.Response.RequestID
	if reqID == "" {
		return
	}

	cp.pendingMu.Lock()
	ch, ok := cp.pending[reqID]
	cp.pendingMu.Unlock()
	if !ok {
		return
	}

	var res controlResult
	if env.Response.Subtype == "error" {
		msg := env.Response.Error
		if msg == "" {
			msg = "unknown error"
		}
		res.err = &CLIConnectionError{Message: msg}
	} else {
		res.response = env.Response.Response
	}

	// The channel has capacity 1 and a single producer per request_id, so this
	// send never blocks; the non-blocking select guards against an unexpected
	// duplicate response (which is harmless to drop).
	select {
	case ch <- res:
	default:
	}
}

// newRequestID generates a unique outbound request ID of the form
// "req_<counter>_<8 hex chars>", matching the upstream
// f"req_{counter}_{urandom(4).hex()}" scheme.
func (cp *controlProtocol) newRequestID() string {
	n := cp.counter.Add(1)
	var b [4]byte
	_, _ = rand.Read(b[:])
	return "req_" + strconv.FormatUint(n, 10) + "_" + hex.EncodeToString(b[:])
}

// sendControlRequest sends an outbound control_request{subtype} carrying the
// given payload fields and blocks until the matching control_response arrives,
// ctx is cancelled, or timeout elapses.
//
// The request body is payload with "subtype" added; payload may be nil. On a
// success response the inner response object is returned as raw JSON. On an
// error response a *CLIConnectionError carrying the CLI error text is returned.
// On timeout a *CLIConnectionError describing the timed-out subtype is
// returned. The pending entry is always reaped on every exit path.
func (cp *controlProtocol) sendControlRequest(ctx context.Context, subtype string, payload map[string]any, timeout time.Duration) (jsontext.Value, error) {
	// Build the request body: payload plus the subtype.
	req := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		req[k] = v
	}
	req["subtype"] = subtype

	id := cp.newRequestID()
	envelope := map[string]any{
		"type":       "control_request",
		"request_id": id,
		"request":    req,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, &CLIConnectionError{Message: fmt.Sprintf("marshal control request %q: %v", subtype, err)}
	}

	// Register the waiter before writing so a fast response cannot race ahead.
	ch := make(chan controlResult, 1)
	cp.pendingMu.Lock()
	cp.pending[id] = ch
	cp.pendingMu.Unlock()
	defer func() {
		cp.pendingMu.Lock()
		delete(cp.pending, id)
		cp.pendingMu.Unlock()
	}()

	if err := cp.writeFn(ctx, body); err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		return res.response, nil
	case <-reqCtx.Done():
		// Distinguish a timeout from a caller cancellation: only the former
		// (deadline exceeded while the parent ctx is still live) is a timeout.
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			return nil, &CLIConnectionError{Message: "control request timeout: " + subtype}
		}
		return nil, ctx.Err()
	}
}

// initializeTimeout returns the initialize handshake timeout: the larger of 60s
// and CLAUDE_CODE_STREAM_CLOSE_TIMEOUT (interpreted as milliseconds), matching
// upstream client.py's initialize_timeout computation.
func initializeTimeout() time.Duration {
	const floor = 60 * time.Second
	v := os.Getenv("CLAUDE_CODE_STREAM_CLOSE_TIMEOUT")
	if v == "" {
		return floor
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms <= 0 {
		return floor
	}
	d := time.Duration(ms) * time.Millisecond
	if d < floor {
		return floor
	}
	return d
}

// initialize performs the control-protocol initialize handshake: it sends a
// control_request{subtype:"initialize"} carrying the hooks configuration and
// agent definitions, waits for the CLI's response, stores it for later access
// via GetServerInfo, and returns the inner response object.
//
// For M2 the hooks field is sent as null (full hook callback-id wiring is M4).
// Agent definitions from opts.Agents are converted to the upstream wire shape
// (keyed by agent name) and included when present.
func (cp *controlProtocol) initialize(ctx context.Context) (jsontext.Value, error) {
	payload := map[string]any{
		// Upstream sends "hooks": null when there are no hook matchers. M4 will
		// populate this with {event: [{matcher, hookCallbackIds, timeout?}]}.
		"hooks": nil,
	}
	if agents := cp.agentsWire(); agents != nil {
		payload["agents"] = agents
	}

	resp, err := cp.sendControlRequest(ctx, "initialize", payload, initializeTimeout())
	if err != nil {
		return nil, err
	}
	cp.initializationResult = resp
	return resp, nil
}

// agentsWire converts opts.Agents into the upstream initialize "agents" wire
// shape: a map keyed by agent Name whose value carries the dataclass field
// names used by the Python SDK (description, prompt, tools, model), omitting
// empty values. It returns nil when there are no agents.
//
// NOTE: the wire field names are the upstream Python dataclass field names
// (asdict), NOT the Go struct's json tags. In particular SystemPrompt maps to
// "prompt" and AllowedTools maps to "tools". See Surprises & Discoveries in the
// ExecPlan.
func (cp *controlProtocol) agentsWire() map[string]any {
	if cp.opts == nil || len(cp.opts.Agents) == 0 {
		return nil
	}
	out := make(map[string]any, len(cp.opts.Agents))
	for _, a := range cp.opts.Agents {
		def := make(map[string]any, 4)
		if a.Description != "" {
			def["description"] = a.Description
		}
		if a.SystemPrompt != "" {
			def["prompt"] = a.SystemPrompt
		}
		if len(a.AllowedTools) > 0 {
			def["tools"] = a.AllowedTools
		}
		if a.Model != "" {
			def["model"] = a.Model
		}
		out[a.Name] = def
	}
	return out
}
