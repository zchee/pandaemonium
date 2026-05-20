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
	// hook. Populated in M4; allocated now so the map is never nil. It is
	// written once during initialize (before any hook_callback request can
	// arrive) and read-only thereafter, so it needs no mutex.
	hookCallbacks map[string]Hook

	// nextCallbackID is the monotonic counter for hook callback IDs of the form
	// "hook_<n>", mirroring upstream Query.next_callback_id. It lives for the
	// controlProtocol's lifetime so IDs never collide across re-initialize.
	nextCallbackID int

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
		// Spawn the handler in its own goroutine so a slow hook/permission
		// callback never stalls the read loop (and thus never blocks the
		// control_response of an in-flight outbound request). Mirrors upstream
		// Query._spawn_control_request_handler.
		cp.spawnControlRequest(ctx, line)
		return true, nil

	case "control_cancel_request":
		cp.cancelControlRequest(line)
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
// The hooks field carries the per-event matcher configuration with callback
// IDs that the CLI uses to invoke individual Go hooks via hook_callback control
// requests; it is JSON null when no hooks are registered (matching upstream).
// Agent definitions from opts.Agents are converted to the upstream wire shape
// (keyed by agent name) and included when present.
//
// Callback IDs are assigned and recorded in cp.hookCallbacks BEFORE the request
// is written, since the CLI may fire hook_callback against an ID as soon as it
// receives the initialize request.
func (cp *controlProtocol) initialize(ctx context.Context) (jsontext.Value, error) {
	// Upstream sends "hooks": null (not {}) when there are no hook matchers. A
	// typed-nil map would marshal to {}, so leave the value as untyped nil in
	// that case and only set the concrete map when matchers exist.
	payload := map[string]any{"hooks": nil}
	if hooks := cp.hooksWire(); hooks != nil {
		payload["hooks"] = hooks
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

// hooksWire converts opts.Hooks into the upstream initialize "hooks" wire shape
// and registers each hook callback under a "hook_<n>" ID in cp.hookCallbacks.
//
// The wire shape is a map keyed by event name (the HookEventKind string) whose
// value is a list of matcher objects {matcher, hookCallbackIds, timeout?},
// mirroring upstream Query.initialize. Each Go HookRegistration maps to exactly
// one matcher with exactly one callback ID: registration order is preserved per
// event, which matters because the CLI merges the per-callback outputs itself.
//
// It returns nil (serialized as JSON null) when there are no registrations, so
// the initialize request matches upstream byte-for-byte in the no-hooks case.
//
// NOTE: in wire mode the CLI decides which matchers fire and invokes each
// callback individually via a hook_callback request; the local-evaluation
// merge in dispatchHooks/applyPermissions is NOT used on this path.
func (cp *controlProtocol) hooksWire() map[string][]map[string]any {
	if cp.opts == nil || len(cp.opts.Hooks) == 0 {
		return nil
	}
	out := make(map[string][]map[string]any)
	for _, reg := range cp.opts.Hooks {
		if reg.Fn == nil {
			continue
		}
		id := "hook_" + strconv.Itoa(cp.nextCallbackID)
		cp.nextCallbackID++
		cp.hookCallbacks[id] = reg.Fn

		matcher := map[string]any{
			// Upstream sends matcher.get("matcher"), which is None when unset; an
			// empty ToolGlob means "match all", so emit null in that case.
			"matcher":         nil,
			"hookCallbackIds": []string{id},
		}
		if reg.ToolGlob != "" {
			matcher["matcher"] = reg.ToolGlob
		}
		event := string(reg.Kind)
		out[event] = append(out[event], matcher)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// spawnControlRequest parses an inbound control_request, registers a
// cancellable context under its request_id, and dispatches the handler in a new
// goroutine so the read loop is never blocked by a callback.
//
// A malformed envelope or one missing request_id is dropped: it cannot be
// answered or cancelled, mirroring upstream which only acts on a present
// request_id.
func (cp *controlProtocol) spawnControlRequest(parent context.Context, line []byte) {
	var env struct {
		RequestID string         `json:"request_id"`
		Request   jsontext.Value `json:"request"`
	}
	if err := json.Unmarshal(line, &env); err != nil || env.RequestID == "" {
		return
	}

	// Register the cancel func BEFORE spawning so a control_cancel_request that
	// arrives while the handler runs always finds it.
	ctx, cancel := context.WithCancel(parent)
	cp.inflightMu.Lock()
	cp.inflight[env.RequestID] = cancel
	cp.inflightMu.Unlock()

	go func() {
		defer func() {
			cp.inflightMu.Lock()
			delete(cp.inflight, env.RequestID)
			cp.inflightMu.Unlock()
			cancel()
		}()
		cp.handleControlRequest(ctx, env.RequestID, env.Request)
	}()
}

// closeInflight cancels every in-flight inbound control-request handler. It is
// called from ClaudeSDKClient.Close so handler goroutines blocked in a slow
// user callback do not outlive the session (the read loop that would route
// their responses has already exited). Each handler removes its own inflight
// entry on return, so this is idempotent and safe to call once at close time.
func (cp *controlProtocol) closeInflight() {
	cp.inflightMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(cp.inflight))
	for _, cancel := range cp.inflight {
		cancels = append(cancels, cancel)
	}
	cp.inflightMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// cancelControlRequest cancels the inflight handler matching the cancel
// request's request_id, if any. The cancelled handler writes no response,
// matching upstream's CancelledError re-raise.
func (cp *controlProtocol) cancelControlRequest(line []byte) {
	var env struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(line, &env); err != nil || env.RequestID == "" {
		return
	}
	cp.inflightMu.Lock()
	cancel, ok := cp.inflight[env.RequestID]
	delete(cp.inflight, env.RequestID)
	cp.inflightMu.Unlock()
	if ok {
		cancel()
	}
}

// handleControlRequest dispatches a single inbound control request by subtype
// and writes the control_response. It mirrors upstream
// Query._handle_control_request: can_use_tool invokes the CanUseTool callback
// and replies with a permission-result object; hook_callback invokes the Go
// hook registered under the request's callback_id and replies with that hook's
// output; any other subtype (including mcp_message until M5) replies with an
// error.
//
// If ctx is cancelled (via control_cancel_request) at any await point, no
// response is written: the CLI has already abandoned the request.
func (cp *controlProtocol) handleControlRequest(ctx context.Context, requestID string, reqBody jsontext.Value) {
	var head struct {
		Subtype string `json:"subtype"`
	}
	if err := json.Unmarshal(reqBody, &head); err != nil {
		cp.writeControlError(ctx, requestID, fmt.Sprintf("malformed control request: %v", err))
		return
	}

	responseData, err := cp.dispatchControlSubtype(ctx, head.Subtype, reqBody)
	if ctx.Err() != nil {
		// Cancelled while handling: the CLI abandoned this request; stay silent.
		return
	}
	if err != nil {
		cp.writeControlError(ctx, requestID, err.Error())
		return
	}
	cp.writeControlSuccess(ctx, requestID, responseData)
}

// dispatchControlSubtype runs the subtype-specific handler and returns the
// inner response object (the "response" field of a success control_response).
func (cp *controlProtocol) dispatchControlSubtype(ctx context.Context, subtype string, reqBody jsontext.Value) (map[string]any, error) {
	switch subtype {
	case "can_use_tool":
		return cp.handleCanUseTool(ctx, reqBody)
	case "hook_callback":
		return cp.handleHookCallback(ctx, reqBody)
	default:
		// Includes mcp_message, which is wired in M5. Until then, an unsupported
		// subtype is an error response, matching upstream.
		return nil, fmt.Errorf("unsupported control request subtype: %s", subtype)
	}
}

// handleCanUseTool invokes the CanUseTool permission callback for a
// can_use_tool request and converts the verdict into the permission-result wire
// shape the CLI expects: {"behavior":"allow","updatedInput":<input>} or
// {"behavior":"deny","message":<reason>}.
//
// PermissionAsk (the zero value, "no opinion from the SDK") maps to allow with
// the original input unchanged, so the CLI's configured permission_mode makes
// the final call — there is no third "ask" behavior on this wire.
func (cp *controlProtocol) handleCanUseTool(ctx context.Context, reqBody jsontext.Value) (map[string]any, error) {
	if cp.opts == nil || cp.opts.CanUseTool == nil {
		return nil, errors.New("CanUseTool callback is not provided")
	}
	var req struct {
		ToolName string         `json:"tool_name"`
		Input    jsontext.Value `json:"input"`
	}
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return nil, fmt.Errorf("decode can_use_tool request: %w", err)
	}

	decision, err := cp.opts.CanUseTool(ctx, req.ToolName, req.Input)
	if err != nil {
		return nil, err
	}

	if decision == PermissionDeny {
		return map[string]any{"behavior": "deny", "message": ""}, nil
	}
	// Allow and Ask both permit the call; preserve the original input.
	input := req.Input
	if len(input) == 0 {
		input = jsontext.Value("{}")
	}
	return map[string]any{"behavior": "allow", "updatedInput": input}, nil
}

// handleHookCallback invokes the Go hook registered under the request's
// callback_id and returns its decision in the hookSpecificOutput envelope shape
// the CLI expects. The full hook event payload is delivered in the request's
// "input" field; unknown fields survive via HookEvent.Raw.
func (cp *controlProtocol) handleHookCallback(ctx context.Context, reqBody jsontext.Value) (map[string]any, error) {
	var req struct {
		CallbackID string         `json:"callback_id"`
		Input      jsontext.Value `json:"input"`
	}
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return nil, fmt.Errorf("decode hook_callback request: %w", err)
	}

	hook := cp.hookCallbacks[req.CallbackID]
	if hook == nil {
		return nil, fmt.Errorf("no hook callback found for ID: %s", req.CallbackID)
	}

	var event HookEvent
	if len(req.Input) > 0 {
		if err := json.Unmarshal(req.Input, &event); err != nil {
			return nil, fmt.Errorf("decode hook input: %w", err)
		}
	}

	decision, err := hook(ctx, event)
	if err != nil {
		return nil, err
	}
	return hookDecisionWire(decision), nil
}

// hookDecisionWire converts a HookDecision into the CLI-expected output map,
// omitting empty fields so a zero decision serializes to {} (proceed
// unchanged). Only the fields the Go HookDecision models are emitted.
func hookDecisionWire(d HookDecision) map[string]any {
	out := make(map[string]any, 3)
	if d.SystemMessage != "" {
		out["systemMessage"] = d.SystemMessage
	}
	if d.AdditionalContext != "" {
		out["additionalContext"] = d.AdditionalContext
	}
	hso := make(map[string]any, 3)
	if d.HookSpecificOutput.HookEventName != "" {
		hso["hookEventName"] = string(d.HookSpecificOutput.HookEventName)
	}
	if d.HookSpecificOutput.PermissionDecision != PermissionAsk {
		hso["permissionDecision"] = string(d.HookSpecificOutput.PermissionDecision)
	}
	if d.HookSpecificOutput.PermissionDecisionReason != "" {
		hso["permissionDecisionReason"] = d.HookSpecificOutput.PermissionDecisionReason
	}
	if len(hso) > 0 {
		out["hookSpecificOutput"] = hso
	}
	return out
}

// writeControlSuccess writes a success control_response carrying responseData
// as its inner response object. A nil responseData serializes as {}.
func (cp *controlProtocol) writeControlSuccess(ctx context.Context, requestID string, responseData map[string]any) {
	if responseData == nil {
		responseData = map[string]any{}
	}
	cp.writeControlResponse(ctx, map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": requestID,
			"response":   responseData,
		},
	})
}

// writeControlError writes an error control_response carrying msg.
func (cp *controlProtocol) writeControlError(ctx context.Context, requestID, msg string) {
	cp.writeControlResponse(ctx, map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "error",
			"request_id": requestID,
			"error":      msg,
		},
	})
}

// writeControlResponse marshals and writes a control_response envelope through
// the writeMu-guarded writeFn. A marshal or transport failure is dropped: the
// read loop must not block, and a failed response will surface as a downstream
// transport error on the next read/write.
func (cp *controlProtocol) writeControlResponse(ctx context.Context, envelope map[string]any) {
	body, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	_ = cp.writeFn(ctx, body)
}
