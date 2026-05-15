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
	"errors"
	"os"
	"testing"

	"github.com/zchee/pandaemonium/pkg/claude/internal/fakecli"
)

// ── NewClient tests ──────────────────────────────────────────────────────────

func TestNewClient_Validate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		opts    *Options
		wantErr bool
	}{
		"success: nil opts": {
			opts:    nil,
			wantErr: false,
		},
		"success: zero-value opts": {
			opts:    &Options{},
			wantErr: false,
		},
		"success: populated opts": {
			opts: &Options{
				SystemPrompt: "be concise",
				MaxTurns:     5,
				MaxBudgetUSD: 1.0,
				Model:        "claude-opus-4-5",
			},
			wantErr: false,
		},
		"error: negative MaxTurns": {
			opts:    &Options{MaxTurns: -1},
			wantErr: true,
		},
		"error: negative MaxBudgetUSD": {
			opts:    &Options{MaxBudgetUSD: -0.01},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cli, err := NewClient(t.Context(), tt.opts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if cli != nil {
					t.Error("NewClient() returned non-nil client on error")
				}
				return
			}
			if cli == nil {
				t.Fatal("NewClient() = nil, want non-nil")
			}
			// Transport must not be started at construction time (AC-i1).
			if cli.transport != nil {
				t.Error("NewClient() started transport at construction time, want nil")
			}
		})
	}
}

// ── ReceiveResponse via FakeCLI ──────────────────────────────────────────────

// startWithFakeCLI injects cli as the transport into c under closeMu, then
// triggers the first FakeCLI frame by writing a probe message. Helper tests
// call this before invoking c.ReceiveResponse.
func startWithFakeCLI(t *testing.T, c *ClaudeSDKClient, cli *fakecli.FakeCLI) {
	t.Helper()
	ctx := t.Context()

	c.closeMu.Lock()
	c.start(ctx, cli, nil, nil, nil)
	c.closeMu.Unlock()

	// Trigger the first frame. FakeCLI requires a WriteJSON to advance frames.
	if err := c.writeMessage(ctx, []byte("probe")); err != nil {
		t.Fatalf("writeMessage(probe): %v", err)
	}
}

func TestClaudeSDKClient_ReceiveResponse_AssistantText(t *testing.T) {
	t.Parallel()

	script := []fakecli.Frame{
		{Lines: []string{
			`{"type":"system","subtype":"init","session_id":"s1"}`,
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello!"}],"model":"claude-opus-4-5"},"session_id":"s1"}`,
			`{"type":"result","subtype":"success","duration_ms":100,"is_error":false,"num_turns":1,"session_id":"s1","total_cost_usd":0.001}`,
		}},
	}

	c := &ClaudeSDKClient{}
	startWithFakeCLI(t, c, fakecli.New(t, script))
	defer c.Close()

	var msgs []Message
	for msg, err := range c.ReceiveResponse(t.Context()) {
		if err != nil {
			t.Fatalf("ReceiveResponse error = %v", err)
		}
		msgs = append(msgs, msg)
	}

	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3", len(msgs))
	}
	if _, ok := msgs[0].(SystemMessage); !ok {
		t.Errorf("msgs[0] = %T, want SystemMessage", msgs[0])
	}
	am, ok := msgs[1].(AssistantMessage)
	if !ok {
		t.Fatalf("msgs[1] = %T, want AssistantMessage", msgs[1])
	}
	if len(am.Content) != 1 {
		t.Fatalf("len(am.Content) = %d, want 1", len(am.Content))
	}
	tb, ok := am.Content[0].(TextBlock)
	if !ok {
		t.Fatalf("am.Content[0] = %T, want TextBlock", am.Content[0])
	}
	if tb.Text != "Hello!" {
		t.Errorf("TextBlock.Text = %q, want Hello!", tb.Text)
	}
	rm, ok := msgs[2].(ResultMessage)
	if !ok {
		t.Fatalf("msgs[2] = %T, want ResultMessage", msgs[2])
	}
	if rm.Subtype != "success" {
		t.Errorf("ResultMessage.Subtype = %q, want success", rm.Subtype)
	}
}

func TestClaudeSDKClient_ReceiveResponse_ToolUse(t *testing.T) {
	t.Parallel()

	script := []fakecli.Frame{
		{Lines: []string{
			`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_001","name":"Bash","input":{"command":"echo hello"}}],"model":"claude-opus-4-5"},"session_id":"s2"}`,
			`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_001","content":[{"type":"text","text":"hello\n"}],"is_error":false}]},"session_id":"s2"}`,
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Done."}],"model":"claude-opus-4-5"},"session_id":"s2"}`,
			`{"type":"result","subtype":"success","duration_ms":200,"is_error":false,"num_turns":2,"session_id":"s2","total_cost_usd":0.002}`,
		}},
	}

	c := &ClaudeSDKClient{}
	startWithFakeCLI(t, c, fakecli.New(t, script))
	defer c.Close()

	var msgs []Message
	for msg, err := range c.ReceiveResponse(t.Context()) {
		if err != nil {
			t.Fatalf("ReceiveResponse error = %v", err)
		}
		msgs = append(msgs, msg)
	}

	if len(msgs) != 4 {
		t.Fatalf("len(msgs) = %d, want 4", len(msgs))
	}

	// msgs[0]: AssistantMessage with ToolUseBlock
	am0, ok := msgs[0].(AssistantMessage)
	if !ok {
		t.Fatalf("msgs[0] = %T, want AssistantMessage", msgs[0])
	}
	tub, ok := am0.Content[0].(ToolUseBlock)
	if !ok {
		t.Fatalf("am0.Content[0] = %T, want ToolUseBlock", am0.Content[0])
	}
	if tub.Name != "Bash" {
		t.Errorf("ToolUseBlock.Name = %q, want Bash", tub.Name)
	}

	// msgs[1]: UserMessage with ToolResultBlock
	um, ok := msgs[1].(UserMessage)
	if !ok {
		t.Fatalf("msgs[1] = %T, want UserMessage", msgs[1])
	}
	trb, ok := um.Content[0].(ToolResultBlock)
	if !ok {
		t.Fatalf("um.Content[0] = %T, want ToolResultBlock", um.Content[0])
	}
	if trb.ToolUseID != "toolu_001" {
		t.Errorf("ToolResultBlock.ToolUseID = %q, want toolu_001", trb.ToolUseID)
	}

	// msgs[2]: AssistantMessage with TextBlock
	if _, ok := msgs[2].(AssistantMessage); !ok {
		t.Errorf("msgs[2] = %T, want AssistantMessage", msgs[2])
	}

	// msgs[3]: ResultMessage
	rm, ok := msgs[3].(ResultMessage)
	if !ok {
		t.Fatalf("msgs[3] = %T, want ResultMessage", msgs[3])
	}
	if rm.NumTurns != 2 {
		t.Errorf("ResultMessage.NumTurns = %d, want 2", rm.NumTurns)
	}
}

func TestClaudeSDKClient_ReceiveResponse_StopsAtResultMessage(t *testing.T) {
	t.Parallel()

	// Emit a ResultMessage followed by extra lines that must NOT be delivered.
	script := []fakecli.Frame{
		{Lines: []string{
			`{"type":"result","subtype":"success","duration_ms":50,"is_error":false,"num_turns":1,"session_id":"s3","total_cost_usd":0.0}`,
			`{"type":"system","subtype":"init","session_id":"s3"}`, // must not be yielded
		}},
	}

	c := &ClaudeSDKClient{}
	startWithFakeCLI(t, c, fakecli.New(t, script))
	defer c.Close()

	var msgs []Message
	for msg, err := range c.ReceiveResponse(t.Context()) {
		if err != nil {
			t.Fatalf("ReceiveResponse error = %v", err)
		}
		msgs = append(msgs, msg)
	}

	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1 (iterator must stop at ResultMessage)", len(msgs))
	}
	if _, ok := msgs[0].(ResultMessage); !ok {
		t.Errorf("msgs[0] = %T, want ResultMessage", msgs[0])
	}
}

func TestClaudeSDKClient_ReceiveResponse_EarlyBreak(t *testing.T) {
	t.Parallel()

	// Script with many messages; caller breaks after the first.
	script := []fakecli.Frame{
		{Lines: []string{
			`{"type":"system","subtype":"init","session_id":"s4"}`,
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Hi"}],"model":"m"},"session_id":"s4"}`,
			`{"type":"result","subtype":"success","duration_ms":10,"is_error":false,"num_turns":1,"session_id":"s4","total_cost_usd":0.0}`,
		}},
	}

	c := &ClaudeSDKClient{}
	startWithFakeCLI(t, c, fakecli.New(t, script))
	defer c.Close()

	count := 0
	for msg, err := range c.ReceiveResponse(t.Context()) {
		if err != nil {
			t.Fatalf("ReceiveResponse error = %v", err)
		}
		_ = msg
		count++
		break // early break after first message
	}

	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestClaudeSDKClient_ReceiveResponse_ContextCancel(t *testing.T) {
	t.Parallel()

	// Empty script: FakeCLI will block on ReadJSON until closed or cancelled.
	c := &ClaudeSDKClient{}
	cli := fakecli.New(t, nil)
	c.closeMu.Lock()
	c.start(t.Context(), cli, nil, nil, nil)
	c.closeMu.Unlock()
	defer c.Close()

	// Cancel the context before calling ReceiveResponse so the iterator
	// immediately surfaces context.Canceled.
	cancelCtx, cancel := context.WithCancel(t.Context())
	cancel()

	var gotErr error
	for _, err := range c.ReceiveResponse(cancelCtx) {
		gotErr = err
		break
	}
	if !errors.Is(gotErr, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", gotErr)
	}
}

func TestClaudeSDKClient_ReceiveResponse_NilRawMessages(t *testing.T) {
	t.Parallel()

	// Client with no Query called — rawMessages is nil.
	c := &ClaudeSDKClient{}

	var gotErr error
	for _, err := range c.ReceiveResponse(t.Context()) {
		gotErr = err
		break
	}

	var connErr *CLIConnectionError
	if !errors.As(gotErr, &connErr) {
		t.Errorf("error = %T(%v), want *CLIConnectionError", gotErr, gotErr)
	}
}

// ── Interrupt tests ──────────────────────────────────────────────────────────

func TestClaudeSDKClient_Interrupt_NoSubprocess(t *testing.T) {
	t.Parallel()

	c := &ClaudeSDKClient{}
	err := c.Interrupt(t.Context())

	var connErr *CLIConnectionError
	if !errors.As(err, &connErr) {
		t.Errorf("Interrupt() error = %T(%v), want *CLIConnectionError", err, err)
	}
}

// ── Multi-turn Query via FakeCLI ─────────────────────────────────────────────

func TestClaudeSDKClient_MultiTurn(t *testing.T) {
	t.Parallel()

	// Two turns: each WriteJSON (Query call) triggers the corresponding frame.
	script := []fakecli.Frame{
		{Lines: []string{
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Turn 1 response"}],"model":"m"},"session_id":"s5"}`,
			`{"type":"result","subtype":"success","duration_ms":10,"is_error":false,"num_turns":1,"session_id":"s5","total_cost_usd":0.0}`,
		}},
		{Lines: []string{
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Turn 2 response"}],"model":"m"},"session_id":"s5"}`,
			`{"type":"result","subtype":"success","duration_ms":10,"is_error":false,"num_turns":2,"session_id":"s5","total_cost_usd":0.0}`,
		}},
	}

	c := &ClaudeSDKClient{}
	cli := fakecli.New(t, script)
	ctx := t.Context()

	c.closeMu.Lock()
	c.start(ctx, cli, nil, nil, nil)
	c.closeMu.Unlock()
	defer c.Close()

	// Turn 1
	if err := c.writeMessage(ctx, []byte("prompt 1")); err != nil {
		t.Fatalf("writeMessage turn 1: %v", err)
	}
	var turn1 []Message
	for msg, err := range c.ReceiveResponse(ctx) {
		if err != nil {
			t.Fatalf("turn1 ReceiveResponse error = %v", err)
		}
		turn1 = append(turn1, msg)
	}
	if len(turn1) != 2 {
		t.Fatalf("turn1 len = %d, want 2", len(turn1))
	}
	am1, ok := turn1[0].(AssistantMessage)
	if !ok {
		t.Fatalf("turn1[0] = %T, want AssistantMessage", turn1[0])
	}
	if tb, ok := am1.Content[0].(TextBlock); !ok || tb.Text != "Turn 1 response" {
		t.Errorf("turn1 text = %q, want Turn 1 response", func() string {
			if ok {
				return tb.Text
			}
			return "not-a-TextBlock"
		}())
	}

	// Turn 2
	if err := c.writeMessage(ctx, []byte("prompt 2")); err != nil {
		t.Fatalf("writeMessage turn 2: %v", err)
	}
	var turn2 []Message
	for msg, err := range c.ReceiveResponse(ctx) {
		if err != nil {
			t.Fatalf("turn2 ReceiveResponse error = %v", err)
		}
		turn2 = append(turn2, msg)
	}
	if len(turn2) != 2 {
		t.Fatalf("turn2 len = %d, want 2", len(turn2))
	}
	am2, ok := turn2[0].(AssistantMessage)
	if !ok {
		t.Fatalf("turn2[0] = %T, want AssistantMessage", turn2[0])
	}
	if tb, ok := am2.Content[0].(TextBlock); !ok || tb.Text != "Turn 2 response" {
		t.Errorf("turn2 text = %q, want Turn 2 response", func() string {
			if ok {
				return tb.Text
			}
			return "not-a-TextBlock"
		}())
	}
}

// ── Real CLI integration test (guarded) ─────────────────────────────────────

func TestQuery_RealCLI(t *testing.T) {
	if os.Getenv("RUN_REAL_CLAUDE_TESTS") != "1" {
		t.Skip("set RUN_REAL_CLAUDE_TESTS=1 to run real CLI tests")
	}

	ctx := t.Context()
	var msgs []Message
	for msg, err := range Query(ctx, "Reply with exactly one word: ok", nil) {
		if err != nil {
			t.Fatalf("Query error = %v", err)
		}
		msgs = append(msgs, msg)
	}

	if len(msgs) == 0 {
		t.Fatal("Query returned no messages")
	}
	// Last message must be a ResultMessage.
	last := msgs[len(msgs)-1]
	rm, ok := last.(ResultMessage)
	if !ok {
		t.Fatalf("last message = %T, want ResultMessage", last)
	}
	if rm.Subtype != "success" {
		t.Errorf("ResultMessage.Subtype = %q, want success", rm.Subtype)
	}
}
