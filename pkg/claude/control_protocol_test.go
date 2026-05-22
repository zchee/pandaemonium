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
	"slices"
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
		CanUseTool: func(_ context.Context, tool string, _ jsontext.Value, _ ToolPermissionContext) (PermissionResult, error) {
			gotTool = tool
			return PermissionResultAllow{}, nil
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
		CanUseTool: func(context.Context, string, jsontext.Value, ToolPermissionContext) (PermissionResult, error) {
			return PermissionResultDeny{}, nil
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

// ─── M11d can_use_tool wire-format tests ─────────────────────────────────────

// TestControlProtocol_HandleCanUseTool_AllowPreservesOriginalInput verifies
// that PermissionResultAllow with no UpdatedInput sends the original input
// byte-for-byte. Pins the contract a future refactor must not lose.
func TestControlProtocol_HandleCanUseTool_AllowPreservesOriginalInput(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value, _ ToolPermissionContext) (PermissionResult, error) {
			return PermissionResultAllow{}, nil
		},
	}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Read","input":{"path":"/etc/hosts","extra":42}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	_, _, _, resp := awaitControlResponse(t, out)
	updated, ok := resp["updatedInput"].(map[string]any)
	if !ok {
		t.Fatalf("updatedInput not an object: %v", resp["updatedInput"])
	}
	if updated["path"] != "/etc/hosts" || updated["extra"] != float64(42) {
		t.Errorf("updatedInput = %v, want original input preserved", updated)
	}
}

// TestControlProtocol_HandleCanUseTool_AllowUpdatesInput verifies that
// PermissionResultAllow.UpdatedInput, when non-empty, replaces the original
// input on the wire (parity with upstream's tool-input modification path).
func TestControlProtocol_HandleCanUseTool_AllowUpdatesInput(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value, _ ToolPermissionContext) (PermissionResult, error) {
			return PermissionResultAllow{
				UpdatedInput: jsontext.Value(`{"path":"/safe/path","sanitized":true}`),
			}, nil
		},
	}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Read","input":{"path":"/etc/hosts"}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	_, _, _, resp := awaitControlResponse(t, out)
	updated, ok := resp["updatedInput"].(map[string]any)
	if !ok {
		t.Fatalf("updatedInput not an object: %v", resp["updatedInput"])
	}
	if updated["path"] != "/safe/path" || updated["sanitized"] != true {
		t.Errorf("updatedInput = %v, want modified", updated)
	}
}

// TestControlProtocol_HandleCanUseTool_AllowEmitsUpdatedPermissions verifies
// that PermissionResultAllow.UpdatedPermissions is serialized through
// PermissionUpdate.ToWire onto the response.
func TestControlProtocol_HandleCanUseTool_AllowEmitsUpdatedPermissions(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value, _ ToolPermissionContext) (PermissionResult, error) {
			return PermissionResultAllow{
				UpdatedPermissions: []PermissionUpdate{
					{Type: PermissionUpdateTypeSetMode, Mode: PermissionModeAcceptEdits},
				},
			}, nil
		},
	}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Read","input":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	_, _, _, resp := awaitControlResponse(t, out)
	ups, ok := resp["updatedPermissions"].([]any)
	if !ok || len(ups) != 1 {
		t.Fatalf("updatedPermissions = %v, want one entry", resp["updatedPermissions"])
	}
	first, _ := ups[0].(map[string]any)
	if first["type"] != "setMode" || first["mode"] != "acceptEdits" {
		t.Errorf("updatedPermissions[0] = %v, want type=setMode mode=acceptEdits", first)
	}
}

// TestControlProtocol_HandleCanUseTool_AllowNoUpdatedPermissionsOmitsKey
// verifies that PermissionResultAllow with nil UpdatedPermissions does NOT
// emit the updatedPermissions key (parity with upstream conditional emission).
func TestControlProtocol_HandleCanUseTool_AllowNoUpdatedPermissionsOmitsKey(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value, _ ToolPermissionContext) (PermissionResult, error) {
			return PermissionResultAllow{}, nil
		},
	}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Read","input":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	_, _, _, resp := awaitControlResponse(t, out)
	if _, has := resp["updatedPermissions"]; has {
		t.Errorf("nil UpdatedPermissions must omit updatedPermissions key: %v", resp)
	}
	if _, has := resp["interrupt"]; has {
		t.Errorf("Allow has no interrupt field; key must be absent: %v", resp)
	}
}

// TestControlProtocol_HandleCanUseTool_DenyInterruptFalseOmitsKey verifies
// that Deny with Interrupt=false omits the interrupt wire key. Pins parity
// with upstream's `if response.interrupt` gate.
func TestControlProtocol_HandleCanUseTool_DenyInterruptFalseOmitsKey(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value, _ ToolPermissionContext) (PermissionResult, error) {
			return PermissionResultDeny{Message: "no"}, nil
		},
	}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	_, _, _, resp := awaitControlResponse(t, out)
	if resp["behavior"] != "deny" {
		t.Errorf("behavior = %v, want deny", resp["behavior"])
	}
	if resp["message"] != "no" {
		t.Errorf("message = %v, want \"no\"", resp["message"])
	}
	if _, has := resp["interrupt"]; has {
		t.Errorf("Interrupt=false must omit the interrupt key: %v", resp)
	}
}

// TestControlProtocol_HandleCanUseTool_DenyInterruptTrueEmitsKey verifies
// that Deny with Interrupt=true emits {"interrupt": true} on the wire.
func TestControlProtocol_HandleCanUseTool_DenyInterruptTrueEmitsKey(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value, _ ToolPermissionContext) (PermissionResult, error) {
			return PermissionResultDeny{Message: "abort", Interrupt: true}, nil
		},
	}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	_, _, _, resp := awaitControlResponse(t, out)
	if resp["interrupt"] != true {
		t.Errorf("interrupt = %v, want true", resp["interrupt"])
	}
}

// TestControlProtocol_HandleCanUseTool_ContextDecode verifies that every
// ToolPermissionContext field decodes from its snake_case wire key, and that
// permission_suggestions (snake_case rename to Go's Suggestions) goes through
// PermissionUpdateFromWire correctly.
func TestControlProtocol_HandleCanUseTool_ContextDecode(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	var got ToolPermissionContext
	cp := newControlProtocol(&Options{
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value, ctx ToolPermissionContext) (PermissionResult, error) {
			got = ctx
			return PermissionResultAllow{}, nil
		},
	}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{` +
		`"subtype":"can_use_tool",` +
		`"tool_name":"Bash",` +
		`"input":{"command":"ls"},` +
		`"tool_use_id":"tu_123",` +
		`"agent_id":"agent_42",` +
		`"blocked_path":"/restricted/x",` +
		`"decision_reason":"path outside cwd",` +
		`"title":"Approve Bash?",` +
		`"display_name":"Bash",` +
		`"description":"Run shell command",` +
		`"permission_suggestions":[` +
		`{"type":"addRules","behavior":"allow","rules":[{"toolName":"Bash","ruleContent":"ls *"}]}` +
		`]}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	_, _, _, _ = awaitControlResponse(t, out)

	if got.ToolUseID != "tu_123" {
		t.Errorf("ToolUseID = %q, want tu_123", got.ToolUseID)
	}
	if got.AgentID != "agent_42" {
		t.Errorf("AgentID = %q, want agent_42", got.AgentID)
	}
	if got.BlockedPath != "/restricted/x" {
		t.Errorf("BlockedPath = %q, want /restricted/x", got.BlockedPath)
	}
	if got.DecisionReason != "path outside cwd" {
		t.Errorf("DecisionReason = %q, want 'path outside cwd'", got.DecisionReason)
	}
	if got.Title != "Approve Bash?" {
		t.Errorf("Title = %q, want 'Approve Bash?'", got.Title)
	}
	if got.DisplayName != "Bash" {
		t.Errorf("DisplayName = %q, want Bash", got.DisplayName)
	}
	if got.Description != "Run shell command" {
		t.Errorf("Description = %q, want 'Run shell command'", got.Description)
	}
	if len(got.Suggestions) != 1 {
		t.Fatalf("Suggestions len = %d, want 1", len(got.Suggestions))
	}
	s := got.Suggestions[0]
	if s.Type != PermissionUpdateTypeAddRules || s.Behavior != PermissionBehaviorAllow {
		t.Errorf("suggestion type/behavior = %v/%v, want addRules/allow", s.Type, s.Behavior)
	}
	if len(s.Rules) != 1 || s.Rules[0].ToolName != "Bash" || s.Rules[0].RuleContent != "ls *" {
		t.Errorf("suggestion rules = %v, want [{Bash, ls *}]", s.Rules)
	}
}

// TestControlProtocol_HandleCanUseTool_NilResultDefaultsToAllow verifies that
// a nil PermissionResult (the SDK expressing no opinion) maps to allow with
// the original input preserved.
func TestControlProtocol_HandleCanUseTool_NilResultDefaultsToAllow(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value, _ ToolPermissionContext) (PermissionResult, error) {
			return nil, nil
		},
	}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Read","input":{"path":"/x"}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	_, _, _, resp := awaitControlResponse(t, out)
	if resp["behavior"] != "allow" {
		t.Errorf("behavior = %v, want allow", resp["behavior"])
	}
	updated, ok := resp["updatedInput"].(map[string]any)
	if !ok || updated["path"] != "/x" {
		t.Errorf("updatedInput = %v, want original input preserved", resp["updatedInput"])
	}
}

// TestControlProtocol_HandleUnsupportedSubtype verifies that a subtype the SDK
// does not handle inbound produces an error response naming the subtype.
func TestControlProtocol_HandleUnsupportedSubtype(t *testing.T) {
	t.Parallel()

	writeFn, out := collectWriter(1)
	cp := newControlProtocol(&Options{}, writeFn)

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"set_permission_mode","mode":"plan"}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	subtype, _, errMsg, _ := awaitControlResponse(t, out)
	if subtype != "error" {
		t.Fatalf("subtype = %q, want error", subtype)
	}
	if !strings.Contains(errMsg, "set_permission_mode") {
		t.Errorf("error = %q, want to mention set_permission_mode", errMsg)
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

// TestControlProtocol_HandlerPanic_DoesNotCrash verifies that a panic in a
// user-supplied callback is recovered on the per-request goroutine and turned
// into an error control_response, rather than crashing the whole process. The
// test surviving to its assertions is itself the primary signal: an unrecovered
// panic on the spawned goroutine would take the test binary down.
func TestControlProtocol_HandlerPanic_DoesNotCrash(t *testing.T) {
	t.Parallel()

	wrote := make(chan []byte, 1)
	writeFn := func(_ context.Context, p []byte) error {
		select {
		case wrote <- append([]byte(nil), p...):
		default:
		}
		return nil
	}

	cp := newControlProtocol(&Options{}, writeFn)
	cp.hookCallbacks["hook_0"] = func(context.Context, HookEvent) (HookDecision, error) {
		panic("boom from user hook")
	}

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"hook_callback","callback_id":"hook_0","input":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}

	select {
	case p := <-wrote:
		var env struct {
			Response struct {
				Subtype string `json:"subtype"`
				Error   string `json:"error"`
			} `json:"response"`
		}
		if err := json.Unmarshal(p, &env); err != nil {
			t.Fatalf("unmarshal control_response: %v (raw=%s)", err, p)
		}
		if env.Response.Subtype != "error" {
			t.Errorf("response subtype = %q, want error", env.Response.Subtype)
		}
		if !strings.Contains(env.Response.Error, "handler panic") {
			t.Errorf("error = %q, want to contain \"handler panic\"", env.Response.Error)
		}
		if !strings.Contains(env.Response.Error, "boom from user hook") {
			t.Errorf("error = %q, want to contain the recovered panic value", env.Response.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no control_response written after handler panic")
	}
}

// TestControlProtocol_WaitInflight_DrainsHandlers is the C6 regression guard:
// closeInflight cancels handler contexts, but Close must also be able to WAIT
// for the handler goroutines to actually exit so they do not leak past the
// session. waitInflight provides that bounded wait; here a ctx-respecting hook
// exits on cancel and waitInflight returns true (drained) within budget.
func TestControlProtocol_WaitInflight_DrainsHandlers(t *testing.T) {
	t.Parallel()

	cp := newControlProtocol(&Options{}, func(context.Context, []byte) error { return nil })

	started := make(chan struct{})
	cp.hookCallbacks["hook_0"] = func(ctx context.Context, _ HookEvent) (HookDecision, error) {
		close(started)
		<-ctx.Done() // respects cancellation
		return HookDecision{}, nil
	}

	line := []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"hook_callback","callback_id":"hook_0","input":{}}}`)
	if _, err := cp.route(t.Context(), line); err != nil {
		t.Fatalf("route error = %v", err)
	}
	<-started

	// Before cancellation the handler is still running, so waitInflight times out.
	if cp.waitInflight(100 * time.Millisecond) {
		t.Fatal("waitInflight returned drained while a handler was still running")
	}

	// Cancel + wait: the handler unblocks and waitInflight reports drained.
	cp.closeInflight()
	if !cp.waitInflight(2 * time.Second) {
		t.Fatal("waitInflight did not drain after closeInflight (handler leaked past Close)")
	}

	// With no handlers left, waitInflight reports drained promptly.
	if !cp.waitInflight(500 * time.Millisecond) {
		t.Error("waitInflight with no inflight handlers should report drained")
	}
	// A negative timeout never waits.
	if cp.waitInflight(-1) {
		t.Error("waitInflight(-1) should return false without waiting")
	}
}

// TestControlProtocol_SpawnAfterClose_NotAdmitted is the C6 follow-up guard:
// once closeInflight has run (Close has begun draining), a control_request that
// readLoop routes from a pre-buffered line must NOT spawn a new handler that
// could outlive Close. Without the closed barrier, spawnControlRequest could
// insert into inflight and Add(1) AFTER waitInflight already observed a zero
// count, leaking a ctx-ignoring handler past Close — the exact class the fix
// claims to eliminate.
func TestControlProtocol_SpawnAfterClose_NotAdmitted(t *testing.T) {
	t.Parallel()

	ran := make(chan struct{}, 1)
	cp := newControlProtocol(&Options{}, func(context.Context, []byte) error { return nil })
	cp.hookCallbacks["hook_0"] = func(context.Context, HookEvent) (HookDecision, error) {
		ran <- struct{}{}
		return HookDecision{}, nil
	}

	// Begin the Close drain: cancel + mark closed.
	cp.closeInflight()

	// A request routed after closeInflight must be dropped, not spawned.
	line := []byte(`{"type":"control_request","request_id":"r-late","request":{"subtype":"hook_callback","callback_id":"hook_0","input":{}}}`)
	cp.spawnControlRequest(t.Context(), line)

	// waitInflight must report drained (no admitted handler), and the handler
	// must never have run.
	if !cp.waitInflight(500 * time.Millisecond) {
		t.Fatal("waitInflight did not drain — a handler was admitted after closeInflight")
	}
	select {
	case <-ran:
		t.Fatal("handler ran after closeInflight; spawnControlRequest admitted it past the close barrier")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestControlProtocol_Envelope_DeterministicKeyOrder is the C9 guard: the
// control-response envelope is a map[string]any, whose Go iteration order is
// non-deterministic. writeControlResponse must marshal it with
// json.Deterministic so the bytes on the wire are stable (sorted keys) across
// repeated writes. (The mcp.go schema/content sites are NOT covered here — they
// use custom marshalers and are already deterministic; only the envelope
// re-marshal of plain maps is at risk.)
func TestControlProtocol_Envelope_DeterministicKeyOrder(t *testing.T) {
	t.Parallel()

	var last string
	first := ""
	writeFn := func(_ context.Context, p []byte) error {
		last = string(p)
		return nil
	}
	cp := newControlProtocol(&Options{}, writeFn)

	// A response payload with several keys exercises map ordering at two nesting
	// levels (the outer envelope and the inner response object).
	payload := map[string]any{
		"zulu": 1, "alpha": 2, "mike": 3, "bravo": 4, "yankee": 5, "charlie": 6,
	}
	for i := range 50 {
		cp.writeControlSuccess(t.Context(), "req-1", payload)
		if i == 0 {
			first = last
			continue
		}
		if last != first {
			t.Fatalf("iteration %d: envelope bytes differ\n first=%s\n  got=%s", i, first, last)
		}
	}

	// Deterministic output for Go maps is sorted key order — verify the inner
	// payload keys come out alphabetically.
	prev := -1
	for _, k := range []string{"alpha", "bravo", "charlie", "mike", "yankee", "zulu"} {
		idx := strings.Index(first, `"`+k+`"`)
		if idx < 0 {
			t.Fatalf("key %q missing: %s", k, first)
		}
		if idx <= prev {
			t.Errorf("key %q out of sorted order (idx=%d, prev=%d): %s", k, idx, prev, first)
		}
		prev = idx
	}
}

// ── test helpers ─────────────────────────────────────────────────────────────

// lastInitializeWrite returns the most recently written payload that contains
// the initialize subtype. It is used by OnWrite respond closures to read back
// the request_id the SDK just generated.
func lastInitializeWrite(cli *fakecli.FakeCLI) []byte {
	written := cli.Written()
	for _, w := range slices.Backward(written) {
		if bytes.Contains(w, []byte(`"subtype":"initialize"`)) {
			return w
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
