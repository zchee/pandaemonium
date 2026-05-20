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
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"

	"github.com/zchee/pandaemonium/pkg/claude/internal/fakecli"
)

// ── M1: route ────────────────────────────────────────────────────────────────

// TestControlProtocol_Route_ControlResponse verifies that route delivers a
// control_response line to the channel registered for its request_id.
func TestControlProtocol_Route_ControlResponse(t *testing.T) {
	t.Parallel()

	cp := newControlProtocol(nil, func(context.Context, []byte) error { return nil })

	// Register a pending waiter manually, as sendControlRequest would.
	ch := make(chan controlResult, 1)
	cp.pendingMu.Lock()
	cp.pending["req_1_abcd"] = ch
	cp.pendingMu.Unlock()

	line := []byte(`{"type":"control_response","response":{"subtype":"success","request_id":"req_1_abcd","response":{"commands":["a","b"]}}}`)
	consumed, err := cp.route(t.Context(), line)
	if err != nil {
		t.Fatalf("route error = %v", err)
	}
	if !consumed {
		t.Fatalf("route consumed = false, want true for control_response")
	}

	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("controlResult.err = %v, want nil", res.err)
		}
		want := `{"commands":["a","b"]}`
		if string(res.response) != want {
			t.Fatalf("controlResult.response = %s, want %s", res.response, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for control_response delivery")
	}
}

// TestControlProtocol_Route_ControlResponseError verifies that an error-subtype
// control_response is delivered as a controlResult carrying the error.
func TestControlProtocol_Route_ControlResponseError(t *testing.T) {
	t.Parallel()

	cp := newControlProtocol(nil, func(context.Context, []byte) error { return nil })
	ch := make(chan controlResult, 1)
	cp.pendingMu.Lock()
	cp.pending["req_2_beef"] = ch
	cp.pendingMu.Unlock()

	line := []byte(`{"type":"control_response","response":{"subtype":"error","request_id":"req_2_beef","error":"boom"}}`)
	consumed, err := cp.route(t.Context(), line)
	if err != nil || !consumed {
		t.Fatalf("route = (%v, %v), want (true, nil)", consumed, err)
	}

	select {
	case res := <-ch:
		if res.err == nil {
			t.Fatal("controlResult.err = nil, want error")
		}
		var cerr *CLIConnectionError
		if !errors.As(res.err, &cerr) {
			t.Fatalf("controlResult.err type = %T, want *CLIConnectionError", res.err)
		}
		if cerr.Message != "boom" {
			t.Fatalf("error message = %q, want %q", cerr.Message, "boom")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error control_response delivery")
	}
}

// TestControlProtocol_Route_PassThrough verifies that route does NOT consume a
// regular SDK message such as an assistant line.
func TestControlProtocol_Route_PassThrough(t *testing.T) {
	t.Parallel()

	cp := newControlProtocol(nil, func(context.Context, []byte) error { return nil })
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`)
	consumed, err := cp.route(t.Context(), line)
	if err != nil {
		t.Fatalf("route error = %v", err)
	}
	if consumed {
		t.Fatal("route consumed = true for assistant line, want false")
	}
}

// TestControlProtocol_Route_TranscriptMirror verifies that transcript_mirror is
// consumed (and dropped) rather than surfaced to the consumer stream.
func TestControlProtocol_Route_TranscriptMirror(t *testing.T) {
	t.Parallel()

	cp := newControlProtocol(nil, func(context.Context, []byte) error { return nil })
	line := []byte(`{"type":"transcript_mirror","filePath":"/x","entries":[]}`)
	consumed, err := cp.route(t.Context(), line)
	if err != nil {
		t.Fatalf("route error = %v", err)
	}
	if !consumed {
		t.Fatal("route consumed = false for transcript_mirror, want true")
	}
}

// TestControlProtocol_Route_ControlRequestStub verifies that an inbound
// control_request and control_cancel_request are consumed (M1 stubs) without
// error.
func TestControlProtocol_Route_ControlRequestStub(t *testing.T) {
	t.Parallel()

	cp := newControlProtocol(nil, func(context.Context, []byte) error { return nil })
	for _, line := range [][]byte{
		[]byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool"}}`),
		[]byte(`{"type":"control_cancel_request","request_id":"r1"}`),
	} {
		consumed, err := cp.route(t.Context(), line)
		if err != nil || !consumed {
			t.Fatalf("route(%s) = (%v, %v), want (true, nil)", line, consumed, err)
		}
	}
}

// TestControlProtocol_Route_Malformed verifies that malformed JSON is NOT
// consumed, so the regular path can surface it as a CLIJSONDecodeError.
func TestControlProtocol_Route_Malformed(t *testing.T) {
	t.Parallel()

	cp := newControlProtocol(nil, func(context.Context, []byte) error { return nil })
	consumed, err := cp.route(t.Context(), []byte(`{not json`))
	if err != nil {
		t.Fatalf("route error = %v, want nil", err)
	}
	if consumed {
		t.Fatal("route consumed = true for malformed JSON, want false")
	}
}

// ── M2: sendControlRequest / initialize ──────────────────────────────────────

// TestControlProtocol_SendControlRequest_Timeout verifies that a request whose
// response never arrives returns a *CLIConnectionError timeout and reaps its
// pending entry.
func TestControlProtocol_SendControlRequest_Timeout(t *testing.T) {
	t.Parallel()

	cp := newControlProtocol(nil, func(context.Context, []byte) error { return nil })
	_, err := cp.sendControlRequest(t.Context(), "interrupt", nil, 20*time.Millisecond)
	if err == nil {
		t.Fatal("sendControlRequest err = nil, want timeout error")
	}
	var cerr *CLIConnectionError
	if !errors.As(err, &cerr) {
		t.Fatalf("err type = %T, want *CLIConnectionError", err)
	}
	if cerr.Message != "control request timeout: interrupt" {
		t.Fatalf("err message = %q", cerr.Message)
	}

	cp.pendingMu.Lock()
	n := len(cp.pending)
	cp.pendingMu.Unlock()
	if n != 0 {
		t.Fatalf("pending entries after timeout = %d, want 0", n)
	}
}

// TestControlProtocol_Initialize_Handshake exercises the full initialize
// round-trip through a FakeCLI: the SDK writes a control_request{initialize},
// the FakeCLI auto-answers with a control_response echoing the request_id, and
// initialize returns the inner response object.
func TestControlProtocol_Initialize_Handshake(t *testing.T) {
	t.Parallel()

	cli := fakecli.New(t, nil)
	c := &ClaudeSDKClient{opts: &Options{
		Agents: []AgentDefinition{{
			Name:         "reviewer",
			Description:  "reviews code",
			SystemPrompt: "be terse",
			AllowedTools: []string{"Read", "Grep"},
			Model:        "claude-opus-4-7",
		}},
	}}

	ctx := t.Context()
	c.closeMu.Lock()
	c.start(ctx, cli, nil, nil, nil)
	cp := c.cp
	c.closeMu.Unlock()

	// Auto-answer the initialize control_request by echoing its request_id.
	cli.OnWrite(
		func(p []byte) bool { return bytes.Contains(p, []byte(`"subtype":"initialize"`)) },
		func() []string {
			id := extractRequestID(t, lastInitializeWrite(cli))
			return []string{
				`{"type":"control_response","response":{"subtype":"success","request_id":"` + id +
					`","response":{"commands":["compact"],"output_style":"default"}}}`,
			}
		},
	)

	resp, err := cp.initialize(ctx)
	if err != nil {
		t.Fatalf("initialize error = %v", err)
	}

	// The inner response object must come back intact.
	var got map[string]any
	if err := json.Unmarshal(resp, &got); err != nil {
		t.Fatalf("unmarshal initialize response: %v", err)
	}
	if got["output_style"] != "default" {
		t.Fatalf("initialize response output_style = %v, want default", got["output_style"])
	}

	// GetServerInfo-equivalent state must be stored.
	if len(cp.initializationResult) == 0 {
		t.Fatal("initializationResult not stored")
	}

	// The first thing written must be the initialize control_request.
	written := cli.Written()
	if len(written) == 0 {
		t.Fatal("no payloads written")
	}
	t.Logf("initialize request: %s", written[0])

	var env struct {
		Type    string `json:"type"`
		Request struct {
			Subtype string                    `json:"subtype"`
			Hooks   jsontext.Value            `json:"hooks"`
			Agents  map[string]jsontext.Value `json:"agents"`
		} `json:"request"`
	}
	if err := json.Unmarshal(written[0], &env); err != nil {
		t.Fatalf("unmarshal written[0]: %v", err)
	}
	if env.Type != "control_request" {
		t.Fatalf("written[0] type = %q, want control_request", env.Type)
	}
	if env.Request.Subtype != "initialize" {
		t.Fatalf("written[0] request.subtype = %q, want initialize", env.Request.Subtype)
	}
	// hooks must be JSON null for M2 (no hook wiring yet).
	if string(env.Request.Hooks) != "null" {
		t.Fatalf("written[0] request.hooks = %s, want null", env.Request.Hooks)
	}

	// Agents must use the upstream wire field names (prompt/tools), not the Go
	// struct json tags (systemPrompt/allowedTools).
	agent, ok := env.Request.Agents["reviewer"]
	if !ok {
		t.Fatalf("agents missing 'reviewer': %v", env.Request.Agents)
	}
	var adef map[string]any
	if err := json.Unmarshal(agent, &adef); err != nil {
		t.Fatalf("unmarshal agent def: %v", err)
	}
	if adef["prompt"] != "be terse" {
		t.Fatalf("agent.prompt = %v, want 'be terse'", adef["prompt"])
	}
	if _, has := adef["systemPrompt"]; has {
		t.Fatal("agent def used 'systemPrompt'; want upstream wire name 'prompt'")
	}
	tools, ok := adef["tools"].([]any)
	if !ok || len(tools) != 2 {
		t.Fatalf("agent.tools = %v, want [Read Grep]", adef["tools"])
	}
}

// TestControlProtocol_Initialize_NoAgents verifies the agents field is omitted
// entirely when Options.Agents is empty.
func TestControlProtocol_Initialize_NoAgents(t *testing.T) {
	t.Parallel()

	cli := fakecli.New(t, nil)
	c := &ClaudeSDKClient{opts: &Options{}}

	ctx := t.Context()
	c.closeMu.Lock()
	c.start(ctx, cli, nil, nil, nil)
	cp := c.cp
	c.closeMu.Unlock()

	cli.OnWrite(
		func(p []byte) bool { return bytes.Contains(p, []byte(`"subtype":"initialize"`)) },
		func() []string {
			id := extractRequestID(t, lastInitializeWrite(cli))
			return []string{
				`{"type":"control_response","response":{"subtype":"success","request_id":"` + id + `","response":{}}}`,
			}
		},
	)

	if _, err := cp.initialize(ctx); err != nil {
		t.Fatalf("initialize error = %v", err)
	}

	written := cli.Written()
	var env struct {
		Request map[string]jsontext.Value `json:"request"`
	}
	if err := json.Unmarshal(written[0], &env); err != nil {
		t.Fatalf("unmarshal written[0]: %v", err)
	}
	if _, has := env.Request["agents"]; has {
		t.Fatalf("agents present with no Options.Agents: %s", written[0])
	}
}

// ── M4: inbound control_request dispatch ─────────────────────────────────────

// collectWriter returns a writeFn that captures each written payload on a
// buffered channel, so a test can block until the handler goroutine writes its
// control_response without polling.
func collectWriter(n int) (func(context.Context, []byte) error, <-chan []byte) {
	ch := make(chan []byte, n)
	fn := func(_ context.Context, p []byte) error {
		ch <- append([]byte(nil), p...)
		return nil
	}
	return fn, ch
}

// awaitControlResponse blocks for the next written control_response and parses
// its inner response object, subtype, request_id, and error string.
func awaitControlResponse(t *testing.T, ch <-chan []byte) (subtype, reqID, errMsg string, resp map[string]any) {
	t.Helper()
	select {
	case p := <-ch:
		var env struct {
			Type     string `json:"type"`
			Response struct {
				Subtype   string         `json:"subtype"`
				RequestID string         `json:"request_id"`
				Error     string         `json:"error"`
				Response  map[string]any `json:"response"`
			} `json:"response"`
		}
		if err := json.Unmarshal(p, &env); err != nil {
			t.Fatalf("awaitControlResponse unmarshal %s: %v", p, err)
		}
		if env.Type != "control_response" {
			t.Fatalf("written type = %q, want control_response: %s", env.Type, p)
		}
		return env.Response.Subtype, env.Response.RequestID, env.Response.Error, env.Response.Response
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for control_response")
		return "", "", "", nil
	}
}

// TestControlProtocol_Initialize_HooksWire verifies that registered hooks are
// converted into the upstream {event: [{matcher, hookCallbackIds, timeout?}]}
// wire shape, grouped by event in registration order, and that each callback ID
// is recorded in hookCallbacks before the request is written.
func TestControlProtocol_Initialize_HooksWire(t *testing.T) {
	t.Parallel()

	noop := func(context.Context, HookEvent) (HookDecision, error) { return HookDecision{}, nil }
	cli := fakecli.New(t, nil)
	c := &ClaudeSDKClient{opts: &Options{Hooks: []HookRegistration{
		{Kind: HookEventPreToolUse, ToolGlob: "Bash", Fn: noop},
		{Kind: HookEventPreToolUse, ToolGlob: "Write", Fn: noop},
		{Kind: HookEventStop, Fn: noop},
	}}}

	ctx := t.Context()
	c.closeMu.Lock()
	c.start(ctx, cli, nil, nil, nil)
	cp := c.cp
	c.closeMu.Unlock()

	cli.OnWrite(
		func(p []byte) bool { return bytes.Contains(p, []byte(`"subtype":"initialize"`)) },
		func() []string {
			id := extractRequestID(t, lastInitializeWrite(cli))
			return []string{`{"type":"control_response","response":{"subtype":"success","request_id":"` + id + `","response":{}}}`}
		},
	)

	if _, err := cp.initialize(ctx); err != nil {
		t.Fatalf("initialize error = %v", err)
	}

	// Three distinct callback IDs must be registered.
	if len(cp.hookCallbacks) != 3 {
		t.Fatalf("hookCallbacks count = %d, want 3", len(cp.hookCallbacks))
	}
	for _, id := range []string{"hook_0", "hook_1", "hook_2"} {
		if cp.hookCallbacks[id] == nil {
			t.Errorf("hookCallbacks missing %q", id)
		}
	}

	var env struct {
		Request struct {
			Hooks map[string][]struct {
				Matcher         *string  `json:"matcher"`
				HookCallbackIDs []string `json:"hookCallbackIds"`
			} `json:"hooks"`
		} `json:"request"`
	}
	if err := json.Unmarshal(lastInitializeWrite(cli), &env); err != nil {
		t.Fatalf("unmarshal initialize request: %v", err)
	}

	pre := env.Request.Hooks["PreToolUse"]
	if len(pre) != 2 {
		t.Fatalf("PreToolUse matchers = %d, want 2: %+v", len(pre), pre)
	}
	if pre[0].Matcher == nil || *pre[0].Matcher != "Bash" {
		t.Errorf("PreToolUse[0].matcher = %v, want Bash", pre[0].Matcher)
	}
	if pre[1].Matcher == nil || *pre[1].Matcher != "Write" {
		t.Errorf("PreToolUse[1].matcher = %v, want Write", pre[1].Matcher)
	}
	if len(pre[0].HookCallbackIDs) != 1 || pre[0].HookCallbackIDs[0] != "hook_0" {
		t.Errorf("PreToolUse[0].hookCallbackIds = %v, want [hook_0]", pre[0].HookCallbackIDs)
	}
	stop := env.Request.Hooks["Stop"]
	if len(stop) != 1 {
		t.Fatalf("Stop matchers = %d, want 1", len(stop))
	}
	// An empty ToolGlob must serialize as null, not "".
	if stop[0].Matcher != nil {
		t.Errorf("Stop[0].matcher = %v, want null", *stop[0].Matcher)
	}
	if len(stop[0].HookCallbackIDs) != 1 || stop[0].HookCallbackIDs[0] != "hook_2" {
		t.Errorf("Stop[0].hookCallbackIds = %v, want [hook_2]", stop[0].HookCallbackIDs)
	}
}

// TestControlProtocol_HandleHookCallback verifies that an inbound hook_callback
// request invokes the Go hook registered under its callback_id, delivers the
// parsed HookEvent, and replies with a success response carrying the hook's
// decision in hookSpecificOutput shape.
func TestControlProtocol_HandleHookCallback(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{}, writeFn)

	var gotEvent HookEvent
	cp.hookCallbacks["hook_0"] = func(_ context.Context, ev HookEvent) (HookDecision, error) {
		gotEvent = ev
		return HookDecision{
			HookSpecificOutput: HookSpecificOutput{
				PermissionDecision:       PermissionDeny,
				PermissionDecisionReason: "blocked by policy",
			},
		}, nil
	}

	line := []byte(`{"type":"control_request","request_id":"r_hc","request":{"subtype":"hook_callback","callback_id":"hook_0","input":{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"rm -rf /"}}}}`)
	consumed, err := cp.route(t.Context(), line)
	if err != nil || !consumed {
		t.Fatalf("route = (%v, %v), want (true, nil)", consumed, err)
	}

	subtype, reqID, errMsg, resp := awaitControlResponse(t, out)
	if subtype != "success" {
		t.Fatalf("response subtype = %q (err=%q), want success", subtype, errMsg)
	}
	if reqID != "r_hc" {
		t.Errorf("response request_id = %q, want r_hc", reqID)
	}
	if gotEvent.Kind != HookEventPreToolUse || gotEvent.ToolName != "Bash" {
		t.Errorf("hook received event = %+v, want PreToolUse/Bash", gotEvent)
	}
	hso, ok := resp["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("response missing hookSpecificOutput: %v", resp)
	}
	if hso["permissionDecision"] != "deny" {
		t.Errorf("permissionDecision = %v, want deny", hso["permissionDecision"])
	}
	if hso["permissionDecisionReason"] != "blocked by policy" {
		t.Errorf("permissionDecisionReason = %v", hso["permissionDecisionReason"])
	}
}

// TestControlProtocol_HandleHookCallback_UnknownID verifies that a hook_callback
// for an unregistered callback_id produces an error response.
func TestControlProtocol_HandleHookCallback_UnknownID(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"hook_callback","callback_id":"hook_99","input":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	subtype, _, errMsg, _ := awaitControlResponse(t, out)
	if subtype != "error" {
		t.Fatalf("subtype = %q, want error", subtype)
	}
	if !strings.Contains(errMsg, "hook_99") {
		t.Errorf("error = %q, want to mention hook_99", errMsg)
	}
}

// TestControlProtocol_HandleCanUseTool_Allow verifies that a can_use_tool
// request whose CanUseTool callback allows the call replies with the
// permission-result allow shape, preserving the original input as updatedInput.
func TestControlProtocol_HandleCanUseTool_Allow(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	var gotTool string
	cp := newControlProtocol(&Options{
		CanUseTool: func(_ context.Context, tool string, _ jsontext.Value) (PermissionDecision, error) {
			gotTool = tool
			return PermissionAllow, nil
		},
	}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r_allow","request":{"subtype":"can_use_tool","tool_name":"Read","input":{"path":"/etc/hosts"}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}

	subtype, reqID, errMsg, resp := awaitControlResponse(t, out)
	if subtype != "success" {
		t.Fatalf("subtype = %q (err=%q), want success", subtype, errMsg)
	}
	if reqID != "r_allow" {
		t.Errorf("request_id = %q, want r_allow", reqID)
	}
	if gotTool != "Read" {
		t.Errorf("callback tool = %q, want Read", gotTool)
	}
	if resp["behavior"] != "allow" {
		t.Errorf("behavior = %v, want allow", resp["behavior"])
	}
	updated, ok := resp["updatedInput"].(map[string]any)
	if !ok || updated["path"] != "/etc/hosts" {
		t.Errorf("updatedInput = %v, want {path:/etc/hosts}", resp["updatedInput"])
	}
}

// TestControlProtocol_HandleCanUseTool_Deny verifies the deny path replies with
// behavior=deny.
func TestControlProtocol_HandleCanUseTool_Deny(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{
		CanUseTool: func(context.Context, string, jsontext.Value) (PermissionDecision, error) {
			return PermissionDeny, nil
		},
	}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r_deny","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"rm -rf /"}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	subtype, _, _, resp := awaitControlResponse(t, out)
	if subtype != "success" {
		t.Fatalf("subtype = %q, want success", subtype)
	}
	if resp["behavior"] != "deny" {
		t.Errorf("behavior = %v, want deny", resp["behavior"])
	}
}

// TestControlProtocol_HandleCanUseTool_NoCallback verifies that a can_use_tool
// request with no CanUseTool configured replies with an error, matching
// upstream's "canUseTool callback is not provided".
func TestControlProtocol_HandleCanUseTool_NoCallback(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Read","input":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	subtype, _, errMsg, _ := awaitControlResponse(t, out)
	if subtype != "error" {
		t.Fatalf("subtype = %q, want error", subtype)
	}
	if !strings.Contains(errMsg, "CanUseTool") {
		t.Errorf("error = %q, want to mention CanUseTool", errMsg)
	}
}

// TestControlProtocol_HandleUnsupportedSubtype verifies that an unknown subtype
// (e.g. mcp_message, wired in M5) produces an error response.
func TestControlProtocol_HandleUnsupportedSubtype(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"mcp_message","server_name":"x","message":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	subtype, _, errMsg, _ := awaitControlResponse(t, out)
	if subtype != "error" {
		t.Fatalf("subtype = %q, want error", subtype)
	}
	if !strings.Contains(errMsg, "mcp_message") {
		t.Errorf("error = %q, want to mention mcp_message", errMsg)
	}
}

// TestControlProtocol_CancelControlRequest verifies that a control_cancel_request
// cancels the inflight handler and that the cancelled handler writes NO
// response — matching upstream's CancelledError re-raise.
func TestControlProtocol_CancelControlRequest(t *testing.T) {
	t.Parallel()

	// A writeFn that fails the test if any response is written.
	wrote := make(chan struct{}, 1)
	writeFn := func(context.Context, []byte) error {
		select {
		case wrote <- struct{}{}:
		default:
		}
		return nil
	}

	// The hook blocks until its ctx is cancelled, then returns. Because the
	// handler observes ctx.Err() != nil after the callback returns, it must not
	// write a response.
	started := make(chan struct{})
	cp := newControlProtocol(&Options{}, writeFn)
	cp.hookCallbacks["hook_0"] = func(ctx context.Context, _ HookEvent) (HookDecision, error) {
		close(started)
		<-ctx.Done()
		return HookDecision{}, nil
	}

	line := []byte(`{"type":"control_request","request_id":"r_cancel","request":{"subtype":"hook_callback","callback_id":"hook_0","input":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}

	// Wait for the handler to start, then cancel it.
	<-started
	cp.cancelControlRequest([]byte(`{"type":"control_cancel_request","request_id":"r_cancel"}`))

	// The inflight entry must be cleared and no response written.
	select {
	case <-wrote:
		t.Fatal("cancelled handler wrote a response, want none")
	case <-time.After(200 * time.Millisecond):
	}
	cp.inflightMu.Lock()
	n := len(cp.inflight)
	cp.inflightMu.Unlock()
	if n != 0 {
		t.Fatalf("inflight entries after cancel = %d, want 0", n)
	}
}

// TestControlProtocol_CloseInflight verifies that closeInflight cancels every
// in-flight handler so a goroutine blocked in a slow callback unblocks at
// session close instead of leaking. The cancelled handler still writes no
// response.
func TestControlProtocol_CloseInflight(t *testing.T) {
	t.Parallel()

	wrote := make(chan struct{}, 1)
	writeFn := func(context.Context, []byte) error {
		select {
		case wrote <- struct{}{}:
		default:
		}
		return nil
	}

	started := make(chan struct{})
	done := make(chan struct{})
	cp := newControlProtocol(&Options{}, writeFn)
	cp.hookCallbacks["hook_0"] = func(ctx context.Context, _ HookEvent) (HookDecision, error) {
		close(started)
		<-ctx.Done()
		close(done)
		return HookDecision{}, nil
	}

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"hook_callback","callback_id":"hook_0","input":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}

	<-started
	cp.closeInflight()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not unblock after closeInflight")
	}
	select {
	case <-wrote:
		t.Fatal("cancelled handler wrote a response, want none")
	case <-time.After(100 * time.Millisecond):
	}
}

// ── test helpers ─────────────────────────────────────────────────────────────

// lastInitializeWrite returns the most recently written payload that contains
// the initialize subtype. It is used by OnWrite respond closures to read back
// the request_id the SDK just generated.
func lastInitializeWrite(cli *fakecli.FakeCLI) []byte {
	written := cli.Written()
	for i := len(written) - 1; i >= 0; i-- {
		if bytes.Contains(written[i], []byte(`"subtype":"initialize"`)) {
			return written[i]
		}
	}
	return nil
}

// extractRequestID parses the request_id from a written control_request payload.
func extractRequestID(t *testing.T, payload []byte) string {
	t.Helper()
	var env struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("extractRequestID unmarshal: %v", err)
	}
	if env.RequestID == "" {
		t.Fatalf("extractRequestID: empty request_id in %s", payload)
	}
	return env.RequestID
}
