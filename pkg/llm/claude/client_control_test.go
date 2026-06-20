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
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"

	"github.com/zchee/pandaemonium/pkg/llm/claude/internal/fakecli"
)

// startConnectedClient brings up a ClaudeSDKClient over a FakeCLI with the
// initialize handshake completed, so control methods see a connected CLI and
// GetServerInfo has a cached result. The returned cli auto-answers initialize.
func startConnectedClient(t *testing.T, opts *Options) (*ClaudeSDKClient, *fakecli.FakeCLI) {
	t.Helper()
	cli := fakecli.New(t, nil)
	c := &ClaudeSDKClient{opts: opts}

	ctx := t.Context()
	c.closeMu.Lock()
	c.start(ctx, cli, nil, nil, nil)
	cp := c.cp
	c.closeMu.Unlock()

	cli.OnWrite(
		func(p []byte) bool { return bytes.Contains(p, []byte(`"subtype":"initialize"`)) },
		func() []string {
			id := extractRequestID(t, lastWriteWithSubtype(cli, "initialize"))
			return []string{`{"type":"control_response","response":{"subtype":"success","request_id":"` + id +
				`","response":{"commands":["compact"],"output_style":"default"}}}`}
		},
	)
	if _, err := cp.initialize(ctx); err != nil {
		t.Fatalf("initialize error = %v", err)
	}
	return c, cli
}

// lastWriteWithSubtype returns the most recent payload carrying the given
// control subtype, for an OnWrite respond closure to echo its request_id.
func lastWriteWithSubtype(cli *fakecli.FakeCLI, subtype string) []byte {
	needle := []byte(`"subtype":"` + subtype + `"`)
	written := cli.Written()
	for _, w := range slices.Backward(written) {
		if bytes.Contains(w, needle) {
			return w
		}
	}
	return nil
}

// autoAnswer registers an OnWrite hook that replies to control requests of the
// given subtype with a success response carrying responseJSON (a raw JSON
// object string) as the inner response.
func autoAnswer(t *testing.T, cli *fakecli.FakeCLI, subtype, responseJSON string) {
	t.Helper()
	cli.OnWrite(
		func(p []byte) bool { return bytes.Contains(p, []byte(`"subtype":"`+subtype+`"`)) },
		func() []string {
			id := extractRequestID(t, lastWriteWithSubtype(cli, subtype))
			return []string{`{"type":"control_response","response":{"subtype":"success","request_id":"` + id +
				`","response":` + responseJSON + `}}`}
		},
	)
}

// autoAnswerError registers an OnWrite hook that replies to the given subtype
// with an error control response.
func autoAnswerError(t *testing.T, cli *fakecli.FakeCLI, subtype, errMsg string) {
	t.Helper()
	cli.OnWrite(
		func(p []byte) bool { return bytes.Contains(p, []byte(`"subtype":"`+subtype+`"`)) },
		func() []string {
			id := extractRequestID(t, lastWriteWithSubtype(cli, subtype))
			return []string{`{"type":"control_response","response":{"subtype":"error","request_id":"` + id +
				`","error":"` + errMsg + `"}}`}
		},
	)
}

// requestBySubtype parses the request object of the last written control
// request of the given subtype.
func requestBySubtype(t *testing.T, cli *fakecli.FakeCLI, subtype string) map[string]any {
	t.Helper()
	payload := lastWriteWithSubtype(cli, subtype)
	if payload == nil {
		t.Fatalf("no write with subtype %q", subtype)
	}
	var env struct {
		Request map[string]any `json:"request"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("unmarshal %s request: %v", subtype, err)
	}
	return env.Request
}

// ── Interrupt ────────────────────────────────────────────────────────────────

func TestClient_Interrupt_Success(t *testing.T) {
	t.Parallel()

	c, cli := startConnectedClient(t, &Options{})
	defer c.Close()
	autoAnswer(t, cli, "interrupt", `{}`)

	if err := c.Interrupt(t.Context()); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}
	req := requestBySubtype(t, cli, "interrupt")
	if req["subtype"] != "interrupt" {
		t.Errorf("request subtype = %v, want interrupt", req["subtype"])
	}
}

func TestClient_Interrupt_NotRunning(t *testing.T) {
	t.Parallel()

	c := &ClaudeSDKClient{}
	err := c.Interrupt(t.Context())
	if _, ok := errors.AsType[*CLIConnectionError](err); !ok {
		t.Errorf("Interrupt() error = %T(%v), want *CLIConnectionError", err, err)
	}
}

func TestClient_Interrupt_CLIError(t *testing.T) {
	t.Parallel()

	c, cli := startConnectedClient(t, &Options{})
	defer c.Close()
	autoAnswerError(t, cli, "interrupt", "nothing to interrupt")

	err := c.Interrupt(t.Context())
	var connErr *CLIConnectionError
	if !errors.As(err, &connErr) {
		t.Fatalf("Interrupt() error = %T(%v), want *CLIConnectionError", err, err)
	}
	if connErr.Message != "nothing to interrupt" {
		t.Errorf("error message = %q, want %q", connErr.Message, "nothing to interrupt")
	}
}

func TestClient_ControlResponseRoutesWhenDataConsumerIsBackedUp(t *testing.T) {
	t.Parallel()

	cli := fakecli.New(t, nil)
	c := &ClaudeSDKClient{opts: &Options{}}

	ctx := t.Context()
	c.closeMu.Lock()
	c.start(ctx, cli, nil, nil, nil)
	c.closeMu.Unlock()
	defer c.Close()

	lines := make([]string, 0, 320)
	for i := range 320 {
		lines = append(lines, fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"text","text":"msg-%03d"}]}}`, i))
	}
	cli.Inject(lines...)
	autoAnswer(t, cli, "set_model", `{}`)

	controlCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := c.SetModel(controlCtx, "claude-opus-4-7"); err != nil {
		t.Fatalf("SetModel() while data messages are unconsumed = %v", err)
	}
}

// ── SetModel ─────────────────────────────────────────────────────────────────

func TestClient_SetModel_Success(t *testing.T) {
	t.Parallel()

	c, cli := startConnectedClient(t, &Options{})
	defer c.Close()
	autoAnswer(t, cli, "set_model", `{}`)

	if err := c.SetModel(t.Context(), "claude-opus-4-7"); err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	req := requestBySubtype(t, cli, "set_model")
	if req["model"] != "claude-opus-4-7" {
		t.Errorf("model = %v, want claude-opus-4-7", req["model"])
	}
}

func TestClient_SetModel_EmptyIsNull(t *testing.T) {
	t.Parallel()

	c, cli := startConnectedClient(t, &Options{})
	defer c.Close()
	autoAnswer(t, cli, "set_model", `{}`)

	if err := c.SetModel(t.Context(), ""); err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	// An empty model must serialize as JSON null, matching set_model(None).
	payload := lastWriteWithSubtype(cli, "set_model")
	var env struct {
		Request struct {
			Model jsontext.Value `json:"model"`
		} `json:"request"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("unmarshal set_model request: %v", err)
	}
	if string(env.Request.Model) != "null" {
		t.Errorf("model field = %s, want null", env.Request.Model)
	}
}

// ── SetPermissionMode ────────────────────────────────────────────────────────

func TestClient_SetPermissionMode_Success(t *testing.T) {
	t.Parallel()

	c, cli := startConnectedClient(t, &Options{})
	defer c.Close()
	autoAnswer(t, cli, "set_permission_mode", `{}`)

	if err := c.SetPermissionMode(t.Context(), PermissionModePlan); err != nil {
		t.Fatalf("SetPermissionMode() error = %v", err)
	}
	req := requestBySubtype(t, cli, "set_permission_mode")
	if req["mode"] != "plan" {
		t.Errorf("mode = %v, want plan", req["mode"])
	}
}

// ── GetServerInfo ────────────────────────────────────────────────────────────

func TestClient_GetServerInfo_Cached(t *testing.T) {
	t.Parallel()

	c, _ := startConnectedClient(t, &Options{})
	defer c.Close()

	info, err := c.GetServerInfo(t.Context())
	if err != nil {
		t.Fatalf("GetServerInfo() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(info, &got); err != nil {
		t.Fatalf("unmarshal server info: %v", err)
	}
	if got["output_style"] != "default" {
		t.Errorf("output_style = %v, want default", got["output_style"])
	}
}

func TestClient_GetServerInfo_NotConnected(t *testing.T) {
	t.Parallel()

	// A client whose initialize never ran has an empty cached result.
	cli := fakecli.New(t, nil)
	c := &ClaudeSDKClient{opts: &Options{}}
	ctx := t.Context()
	c.closeMu.Lock()
	c.start(ctx, cli, nil, nil, nil)
	c.closeMu.Unlock()
	defer c.Close()

	_, err := c.GetServerInfo(ctx)
	if _, ok := errors.AsType[*CLIConnectionError](err); !ok {
		t.Errorf("GetServerInfo() error = %T(%v), want *CLIConnectionError", err, err)
	}
}

func TestClient_GetServerInfo_NotRunning(t *testing.T) {
	t.Parallel()

	c := &ClaudeSDKClient{}
	_, err := c.GetServerInfo(t.Context())
	if _, ok := errors.AsType[*CLIConnectionError](err); !ok {
		t.Errorf("GetServerInfo() error = %T(%v), want *CLIConnectionError", err, err)
	}
}

// ── GetMCPStatus / GetContextUsage ───────────────────────────────────────────

func TestClient_GetMCPStatus_Success(t *testing.T) {
	t.Parallel()

	c, cli := startConnectedClient(t, &Options{})
	defer c.Close()
	autoAnswer(t, cli, "mcp_status", `{"servers":[{"name":"calc","status":"connected"}]}`)

	resp, err := c.GetMCPStatus(t.Context())
	if err != nil {
		t.Fatalf("GetMCPStatus() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(resp, &got); err != nil {
		t.Fatalf("unmarshal mcp status: %v", err)
	}
	servers, ok := got["servers"].([]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("servers = %v, want 1 entry", got["servers"])
	}
}

func TestClient_GetContextUsage_Success(t *testing.T) {
	t.Parallel()

	c, cli := startConnectedClient(t, &Options{})
	defer c.Close()
	autoAnswer(t, cli, "get_context_usage", `{"total_tokens":1234}`)

	resp, err := c.GetContextUsage(t.Context())
	if err != nil {
		t.Fatalf("GetContextUsage() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(resp, &got); err != nil {
		t.Fatalf("unmarshal context usage: %v", err)
	}
	if got["total_tokens"] != float64(1234) {
		t.Errorf("total_tokens = %v, want 1234", got["total_tokens"])
	}
}

// ── timeout & close-in-flight (shared properties) ────────────────────────────

func TestClient_Control_Timeout(t *testing.T) {
	t.Parallel()

	// No auto-answer for get_context_usage: the request times out via ctx.
	c, _ := startConnectedClient(t, &Options{})
	defer c.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := c.GetContextUsage(ctx)
	if err == nil {
		t.Fatal("GetContextUsage() error = nil, want timeout/cancel error")
	}
}

func TestClient_Control_HonorsLongerCtxDeadline(t *testing.T) {
	t.Parallel()

	// A ctx deadline beyond the 60s default must be honored: a request with a
	// 90s deadline and no answer must still be pending (not timed out) shortly
	// after it is sent. We assert it does not return within a brief window.
	c, _ := startConnectedClient(t, &Options{})
	defer c.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.GetContextUsage(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("request returned early (%v); 90s deadline should still be pending", err)
	case <-time.After(150 * time.Millisecond):
		// Still pending after 150ms, as expected. cancel() (deferred) unblocks it.
	}
}

func TestClient_Control_CloseInFlight(t *testing.T) {
	t.Parallel()

	// No auto-answer: the request is in flight when Close runs. Close clears
	// the transport and fails pending waiters, so the request must surface an
	// error, not hang.
	c, cli := startConnectedClient(t, &Options{})

	done := make(chan error, 1)
	go func() {
		_, err := c.GetMCPStatus(context.Background())
		done <- err
	}()

	// Wait until the request has actually been written (its pending entry is
	// registered before the write), instead of a flaky fixed sleep.
	waitForSubtypeWritten(t, cli, "mcp_status")
	_ = c.Close()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("in-flight control request returned nil error after Close, want error")
		}
		if _, ok := errors.AsType[*CLIConnectionError](err); !ok {
			t.Errorf("error = %T(%v), want *CLIConnectionError", err, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight control request hung after Close")
	}
}

// waitForSubtypeWritten polls until a control request of the given subtype has
// been written to the FakeCLI, failing the test if it does not appear in time.
func waitForSubtypeWritten(t *testing.T, cli *fakecli.FakeCLI, subtype string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if lastWriteWithSubtype(cli, subtype) != nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("control request %q was never written", subtype)
}

// ── failPending unit test ────────────────────────────────────────────────────

func TestControlProtocol_FailPending(t *testing.T) {
	t.Parallel()

	cp := newControlProtocol(nil, func(context.Context, []byte) error { return nil })
	ch := make(chan controlResult, 1)
	cp.pendingMu.Lock()
	cp.pending["req_1_dead"] = ch
	cp.pendingMu.Unlock()

	cp.failPending(eofForTestError{})

	select {
	case res := <-ch:
		if res.err == nil {
			t.Fatal("failPending delivered nil error, want CLIConnectionError")
		}
		var connErr *CLIConnectionError
		if !errors.As(res.err, &connErr) {
			t.Fatalf("error = %T, want *CLIConnectionError", res.err)
		}
		if !strings.Contains(connErr.Message, "synthetic EOF") {
			t.Errorf("message = %q, want to wrap the cause", connErr.Message)
		}
	default:
		t.Fatal("failPending delivered nothing to the pending channel")
	}
}

// eofForTestError is a stand-in transport error for failPending.
type eofForTestError struct{}

func (eofForTestError) Error() string { return "synthetic EOF" }
