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
