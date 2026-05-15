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
	"strings"
	"testing"

	"github.com/go-json-experiment/json/jsontext"
)

// ── dispatchHooks tests ──────────────────────────────────────────────────────

func TestDispatchHooks_NoRegistrations(t *testing.T) {
	t.Parallel()

	dec, err := dispatchHooks(t.Context(), nil, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if err != nil {
		t.Fatalf("dispatchHooks() error = %v", err)
	}
	if dec.HookSpecificOutput.PermissionDecision != PermissionAsk {
		t.Errorf("PermissionDecision = %q, want %q", dec.HookSpecificOutput.PermissionDecision, PermissionAsk)
	}
	if dec.SystemMessage != "" {
		t.Errorf("SystemMessage = %q, want empty", dec.SystemMessage)
	}
}

func TestDispatchHooks_KindFiltering(t *testing.T) {
	t.Parallel()

	called := false
	regs := []HookRegistration{
		{
			Kind: HookEventPostToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				called = true
				return HookDecision{}, nil
			},
		},
	}
	// Send a PreToolUse event — PostToolUse hook must NOT fire.
	_, err := dispatchHooks(t.Context(), regs, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if err != nil {
		t.Fatalf("dispatchHooks() error = %v", err)
	}
	if called {
		t.Error("hook was called for mismatched Kind, want not called")
	}
}

func TestDispatchHooks_ToolGlobFiltering(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		glob     string
		toolName string
		wantCall bool
	}{
		"success: glob matches": {
			glob:     "Ba*",
			toolName: "Bash",
			wantCall: true,
		},
		"success: glob no-match": {
			glob:     "Write",
			toolName: "Bash",
			wantCall: false,
		},
		"success: empty glob matches all": {
			glob:     "",
			toolName: "Bash",
			wantCall: true,
		},
		"success: wildcard matches all": {
			glob:     "*",
			toolName: "AnyTool",
			wantCall: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			called := false
			regs := []HookRegistration{
				{
					Kind:     HookEventPreToolUse,
					ToolGlob: tt.glob,
					Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
						called = true
						return HookDecision{}, nil
					},
				},
			}
			_, err := dispatchHooks(t.Context(), regs, HookEvent{Kind: HookEventPreToolUse, ToolName: tt.toolName})
			if err != nil {
				t.Fatalf("dispatchHooks() error = %v", err)
			}
			if called != tt.wantCall {
				t.Errorf("hook called = %v, want %v", called, tt.wantCall)
			}
		})
	}
}

func TestDispatchHooks_InvalidGlob(t *testing.T) {
	t.Parallel()

	regs := []HookRegistration{
		{
			Kind:     HookEventPreToolUse,
			ToolGlob: "[invalid", // malformed glob
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				return HookDecision{}, nil
			},
		},
	}
	_, err := dispatchHooks(t.Context(), regs, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if err == nil {
		t.Fatal("dispatchHooks() = nil error, want error for invalid glob")
	}
	var connErr *CLIConnectionError
	if !errors.As(err, &connErr) {
		t.Errorf("error type = %T, want *CLIConnectionError", err)
	}
}

func TestDispatchHooks_NilFnSkipped(t *testing.T) {
	t.Parallel()

	regs := []HookRegistration{
		{Kind: HookEventPreToolUse, Fn: nil}, // nil fn — must not panic
		{
			Kind: HookEventPreToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				return HookDecision{SystemMessage: "from-second"}, nil
			},
		},
	}
	dec, err := dispatchHooks(t.Context(), regs, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if err != nil {
		t.Fatalf("dispatchHooks() error = %v", err)
	}
	if dec.SystemMessage != "from-second" {
		t.Errorf("SystemMessage = %q, want from-second", dec.SystemMessage)
	}
}

func TestDispatchHooks_DenyStopsEarly(t *testing.T) {
	t.Parallel()

	secondCalled := false
	regs := []HookRegistration{
		{
			Kind: HookEventPreToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				return HookDecision{
					HookSpecificOutput: HookSpecificOutput{
						PermissionDecision:       PermissionDeny,
						PermissionDecisionReason: "unsafe command",
					},
				}, nil
			},
		},
		{
			Kind: HookEventPreToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				secondCalled = true
				return HookDecision{}, nil
			},
		},
	}

	dec, err := dispatchHooks(t.Context(), regs, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if err != nil {
		t.Fatalf("dispatchHooks() error = %v", err)
	}
	if dec.HookSpecificOutput.PermissionDecision != PermissionDeny {
		t.Errorf("PermissionDecision = %q, want deny", dec.HookSpecificOutput.PermissionDecision)
	}
	if dec.HookSpecificOutput.PermissionDecisionReason != "unsafe command" {
		t.Errorf("Reason = %q, want 'unsafe command'", dec.HookSpecificOutput.PermissionDecisionReason)
	}
	if secondCalled {
		t.Error("second hook was called after deny, want early stop")
	}
}

func TestDispatchHooks_SystemMessageMerge(t *testing.T) {
	t.Parallel()

	regs := []HookRegistration{
		{
			Kind: HookEventPreToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				return HookDecision{SystemMessage: "line-1"}, nil
			},
		},
		{
			Kind: HookEventPreToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				return HookDecision{SystemMessage: "line-2"}, nil
			},
		},
		{
			Kind: HookEventPreToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				return HookDecision{AdditionalContext: "ctx-a"}, nil
			},
		},
		{
			Kind: HookEventPreToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				return HookDecision{AdditionalContext: "ctx-b"}, nil
			},
		},
	}

	dec, err := dispatchHooks(t.Context(), regs, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if err != nil {
		t.Fatalf("dispatchHooks() error = %v", err)
	}
	if !strings.Contains(dec.SystemMessage, "line-1") || !strings.Contains(dec.SystemMessage, "line-2") {
		t.Errorf("SystemMessage = %q, want both line-1 and line-2", dec.SystemMessage)
	}
	if !strings.Contains(dec.AdditionalContext, "ctx-a") || !strings.Contains(dec.AdditionalContext, "ctx-b") {
		t.Errorf("AdditionalContext = %q, want both ctx-a and ctx-b", dec.AdditionalContext)
	}
}

func TestDispatchHooks_HookReturnsError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("hook failed")
	regs := []HookRegistration{
		{
			Kind: HookEventPreToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				return HookDecision{}, sentinel
			},
		},
	}
	_, err := dispatchHooks(t.Context(), regs, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want sentinel", err)
	}
}

func TestDispatchHooks_AllowDoesNotStop(t *testing.T) {
	t.Parallel()

	callCount := 0
	regs := []HookRegistration{
		{
			Kind: HookEventPreToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				callCount++
				return HookDecision{HookSpecificOutput: HookSpecificOutput{PermissionDecision: PermissionAllow}}, nil
			},
		},
		{
			Kind: HookEventPreToolUse,
			Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
				callCount++
				return HookDecision{SystemMessage: "from-second"}, nil
			},
		},
	}
	dec, err := dispatchHooks(t.Context(), regs, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if err != nil {
		t.Fatalf("dispatchHooks() error = %v", err)
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2 (allow must not stop iteration)", callCount)
	}
	if dec.HookSpecificOutput.PermissionDecision != PermissionAllow {
		t.Errorf("PermissionDecision = %q, want allow", dec.HookSpecificOutput.PermissionDecision)
	}
}

// ── applyCanUseTool tests ────────────────────────────────────────────────────

func TestApplyCanUseTool_NilCallback(t *testing.T) {
	t.Parallel()

	dec, err := applyCanUseTool(t.Context(), nil, HookEvent{Kind: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("applyCanUseTool() error = %v", err)
	}
	if dec.HookSpecificOutput.PermissionDecision != PermissionAsk {
		t.Errorf("PermissionDecision = %q, want %q", dec.HookSpecificOutput.PermissionDecision, PermissionAsk)
	}
}

func TestApplyCanUseTool_NonPreToolUse(t *testing.T) {
	t.Parallel()

	called := false
	fn := func(_ context.Context, _ string, _ jsontext.Value) (PermissionDecision, error) {
		called = true
		return PermissionAllow, nil
	}
	dec, err := applyCanUseTool(t.Context(), fn, HookEvent{Kind: HookEventStop})
	if err != nil {
		t.Fatalf("applyCanUseTool() error = %v", err)
	}
	if called {
		t.Error("callback was called for non-PreToolUse event, want not called")
	}
	if dec.HookSpecificOutput.PermissionDecision != PermissionAsk {
		t.Errorf("PermissionDecision = %q, want %q", dec.HookSpecificOutput.PermissionDecision, PermissionAsk)
	}
}

func TestApplyCanUseTool_Decisions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		returns PermissionDecision
		want    PermissionDecision
	}{
		"success: allow": {returns: PermissionAllow, want: PermissionAllow},
		"success: deny":  {returns: PermissionDeny, want: PermissionDeny},
		"success: ask":   {returns: PermissionAsk, want: PermissionAsk},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fn := func(_ context.Context, _ string, _ jsontext.Value) (PermissionDecision, error) {
				return tt.returns, nil
			}
			dec, err := applyCanUseTool(t.Context(), fn, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
			if err != nil {
				t.Fatalf("applyCanUseTool() error = %v", err)
			}
			if dec.HookSpecificOutput.PermissionDecision != tt.want {
				t.Errorf("PermissionDecision = %q, want %q", dec.HookSpecificOutput.PermissionDecision, tt.want)
			}
		})
	}
}

func TestApplyCanUseTool_PassesToolNameAndInput(t *testing.T) {
	t.Parallel()

	var gotTool string
	var gotInput jsontext.Value
	fn := func(_ context.Context, toolName string, input jsontext.Value) (PermissionDecision, error) {
		gotTool = toolName
		gotInput = input
		return PermissionAllow, nil
	}

	event := HookEvent{
		Kind:      HookEventPreToolUse,
		ToolName:  "Bash",
		ToolInput: jsontext.Value(`{"command":"ls"}`),
	}
	if _, err := applyCanUseTool(t.Context(), fn, event); err != nil {
		t.Fatalf("applyCanUseTool() error = %v", err)
	}
	if gotTool != "Bash" {
		t.Errorf("toolName = %q, want Bash", gotTool)
	}
	if string(gotInput) != `{"command":"ls"}` {
		t.Errorf("input = %q, want {\"command\":\"ls\"}", gotInput)
	}
}

// ── applyPermissions tests ───────────────────────────────────────────────────

func TestApplyPermissions_NilOpts(t *testing.T) {
	t.Parallel()

	dec, err := applyPermissions(t.Context(), nil, HookEvent{Kind: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("applyPermissions() error = %v", err)
	}
	if dec.HookSpecificOutput.PermissionDecision != PermissionAsk {
		t.Errorf("PermissionDecision = %q, want %q", dec.HookSpecificOutput.PermissionDecision, PermissionAsk)
	}
}

func TestApplyPermissions_HookDenySkipsCanUseTool(t *testing.T) {
	t.Parallel()

	canUseToolCalled := false
	opts := &Options{
		Hooks: []HookRegistration{
			{
				Kind: HookEventPreToolUse,
				Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
					return HookDecision{
						HookSpecificOutput: HookSpecificOutput{
							PermissionDecision:       PermissionDeny,
							PermissionDecisionReason: "hook denied",
						},
					}, nil
				},
			},
		},
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value) (PermissionDecision, error) {
			canUseToolCalled = true
			return PermissionAllow, nil
		},
	}

	dec, err := applyPermissions(t.Context(), opts, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if err != nil {
		t.Fatalf("applyPermissions() error = %v", err)
	}
	if dec.HookSpecificOutput.PermissionDecision != PermissionDeny {
		t.Errorf("PermissionDecision = %q, want deny", dec.HookSpecificOutput.PermissionDecision)
	}
	if dec.HookSpecificOutput.PermissionDecisionReason != "hook denied" {
		t.Errorf("Reason = %q, want 'hook denied'", dec.HookSpecificOutput.PermissionDecisionReason)
	}
	if canUseToolCalled {
		t.Error("CanUseTool was called after hook deny, want skipped")
	}
}

func TestApplyPermissions_CanUseToolDenyOverridesHookAllow(t *testing.T) {
	t.Parallel()

	opts := &Options{
		Hooks: []HookRegistration{
			{
				Kind: HookEventPreToolUse,
				Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
					return HookDecision{
						HookSpecificOutput: HookSpecificOutput{PermissionDecision: PermissionAllow},
					}, nil
				},
			},
		},
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value) (PermissionDecision, error) {
			return PermissionDeny, nil
		},
	}

	dec, err := applyPermissions(t.Context(), opts, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if err != nil {
		t.Fatalf("applyPermissions() error = %v", err)
	}
	if dec.HookSpecificOutput.PermissionDecision != PermissionDeny {
		t.Errorf("PermissionDecision = %q, want deny", dec.HookSpecificOutput.PermissionDecision)
	}
}

func TestApplyPermissions_BothAllow(t *testing.T) {
	t.Parallel()

	opts := &Options{
		Hooks: []HookRegistration{
			{
				Kind: HookEventPreToolUse,
				Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
					return HookDecision{
						SystemMessage:      "hook-sys",
						HookSpecificOutput: HookSpecificOutput{PermissionDecision: PermissionAllow},
					}, nil
				},
			},
		},
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value) (PermissionDecision, error) {
			return PermissionAllow, nil
		},
	}

	dec, err := applyPermissions(t.Context(), opts, HookEvent{Kind: HookEventPreToolUse, ToolName: "Bash"})
	if err != nil {
		t.Fatalf("applyPermissions() error = %v", err)
	}
	if dec.HookSpecificOutput.PermissionDecision != PermissionAllow {
		t.Errorf("PermissionDecision = %q, want allow", dec.HookSpecificOutput.PermissionDecision)
	}
	if dec.SystemMessage != "hook-sys" {
		t.Errorf("SystemMessage = %q, want hook-sys", dec.SystemMessage)
	}
}

func TestApplyPermissions_NonPreToolUseEventSkipsCanUseTool(t *testing.T) {
	t.Parallel()

	canUseToolCalled := false
	opts := &Options{
		Hooks: []HookRegistration{
			{
				Kind: HookEventStop,
				Fn: func(_ context.Context, _ HookEvent) (HookDecision, error) {
					return HookDecision{SystemMessage: "stop-msg"}, nil
				},
			},
		},
		CanUseTool: func(_ context.Context, _ string, _ jsontext.Value) (PermissionDecision, error) {
			canUseToolCalled = true
			return PermissionDeny, nil
		},
	}

	dec, err := applyPermissions(t.Context(), opts, HookEvent{Kind: HookEventStop})
	if err != nil {
		t.Fatalf("applyPermissions() error = %v", err)
	}
	if canUseToolCalled {
		t.Error("CanUseTool was called for Stop event, want skipped")
	}
	if dec.SystemMessage != "stop-msg" {
		t.Errorf("SystemMessage = %q, want stop-msg", dec.SystemMessage)
	}
}
