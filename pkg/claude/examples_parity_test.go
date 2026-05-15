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

// Package claude (test) — hermetic parity tests for pkg/claude/examples/.
//
// Each test mirrors one example program's API surface against a scripted
// [fakecli.FakeCLI] transport, verifying correct behaviour without a real
// claude CLI subprocess. Single-scenario tests are intentionally not wrapped
// in a table-driven map because their assertions are scenario-specific; only
// the hooks and permission-callback tests use a table (same function, multiple
// input/outcome pairs).
package claude

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/zchee/pandaemonium/pkg/claude/internal/fakecli"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// assistantTextFrame returns a single FakeCLI Frame that emits a system-init
// line, one AssistantMessage with a TextBlock, and a ResultMessage.
func assistantTextFrame(sessionID, text string) fakecli.Frame {
	return fakecli.Frame{Lines: []string{
		fmt.Sprintf(`{"type":"system","subtype":"init","session_id":%q}`, sessionID),
		fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"text","text":%s}],"model":"claude-opus-4-5"},"session_id":%q}`,
			mustJSON(text), sessionID),
		fmt.Sprintf(`{"type":"result","subtype":"success","duration_ms":100,"duration_api_ms":80,"is_error":false,"num_turns":1,"session_id":%q,"total_cost_usd":0.001}`,
			sessionID),
	}}
}

// mustJSON returns the JSON encoding of v, panicking on error.
func mustJSON(v any) string {
	b, err := stdjson.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// ── TestExampleParity_QuickStart ─────────────────────────────────────────────

// TestExampleParity_QuickStart verifies the quick_start example pattern:
// Query → AssistantMessage with a TextBlock. Mirrors examples/quick_start.py.
func TestExampleParity_QuickStart(t *testing.T) {
	t.Parallel()

	script := []fakecli.Frame{assistantTextFrame("qs-sess", "Paris is the capital of France.")}

	c := &ClaudeSDKClient{}
	startWithFakeCLI(t, c, fakecli.New(t, script))
	defer c.Close()

	var got string
	for msg, err := range c.ReceiveResponse(t.Context()) {
		if err != nil {
			t.Fatalf("ReceiveResponse error = %v", err)
		}
		if am, ok := msg.(AssistantMessage); ok {
			for _, b := range am.Content {
				if tb, ok := b.(TextBlock); ok {
					got += tb.Text
				}
			}
		}
	}

	if got != "Paris is the capital of France." {
		t.Errorf("text = %q, want Paris is the capital of France.", got)
	}
}

// ── TestExampleParity_StreamingMode ──────────────────────────────────────────

// TestExampleParity_StreamingMode verifies the streaming_mode example pattern:
// every message kind arrives in order and ResultMessage carries cost metadata.
// Mirrors examples/streaming_mode.py.
func TestExampleParity_StreamingMode(t *testing.T) {
	t.Parallel()

	const wantText = "A goroutine is a lightweight thread managed by the Go runtime."
	const wantCost = 0.00025

	script := []fakecli.Frame{{Lines: []string{
		`{"type":"system","subtype":"init","session_id":"sm-sess"}`,
		fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"text","text":%s}],"model":"claude-opus-4-5"},"session_id":"sm-sess"}`,
			mustJSON(wantText)),
		fmt.Sprintf(`{"type":"result","subtype":"success","duration_ms":200,"duration_api_ms":150,"is_error":false,"num_turns":1,"session_id":"sm-sess","total_cost_usd":%s}`,
			mustJSON(wantCost)),
	}}}

	c := &ClaudeSDKClient{}
	startWithFakeCLI(t, c, fakecli.New(t, script))
	defer c.Close()

	var (
		gotText   string
		gotResult ResultMessage
		sawResult bool
	)
	for msg, err := range c.ReceiveResponse(t.Context()) {
		if err != nil {
			t.Fatalf("ReceiveResponse error = %v", err)
		}
		switch m := msg.(type) {
		case AssistantMessage:
			for _, b := range m.Content {
				if tb, ok := b.(TextBlock); ok {
					gotText += tb.Text
				}
			}
		case ResultMessage:
			gotResult = m
			sawResult = true
		}
	}

	if gotText != wantText {
		t.Errorf("text = %q, want %q", gotText, wantText)
	}
	if !sawResult {
		t.Fatal("no ResultMessage received")
	}
	if gotResult.TotalCostUSD != wantCost {
		t.Errorf("TotalCostUSD = %v, want %v", gotResult.TotalCostUSD, wantCost)
	}
	if gotResult.NumTurns != 1 {
		t.Errorf("NumTurns = %d, want 1", gotResult.NumTurns)
	}
}

// ── TestExampleParity_Hooks ───────────────────────────────────────────────────

// TestExampleParity_Hooks verifies the hooks example pattern:
// a PreToolUse Bash hook blocks dangerous commands and passes safe ones.
// Mirrors examples/hooks.py (parity with AC8).
func TestExampleParity_Hooks(t *testing.T) {
	t.Parallel()

	dangerousPatterns := []string{"rm ", "rmdir", " dd ", "mkfs", "> /dev"}

	bashGuard := func(_ context.Context, event HookEvent) (HookDecision, error) {
		if event.Kind != HookEventPreToolUse || event.ToolName != "Bash" {
			return HookDecision{}, nil
		}
		var inp struct {
			Command string `json:"command"`
		}
		if len(event.ToolInput) > 0 {
			_ = stdjson.Unmarshal(event.ToolInput, &inp)
		}
		for _, pat := range dangerousPatterns {
			if strings.Contains(inp.Command, pat) {
				return HookDecision{
					HookSpecificOutput: HookSpecificOutput{
						PermissionDecision:       PermissionDeny,
						PermissionDecisionReason: "dangerous pattern: " + pat,
					},
				}, nil
			}
		}
		return HookDecision{}, nil
	}

	regs := []HookRegistration{
		{Kind: HookEventPreToolUse, ToolGlob: "Bash", Fn: bashGuard},
	}

	tests := map[string]struct {
		command    string
		wantDeny   bool
		wantReason string
	}{
		"success: safe ls command is allowed": {
			command:  "ls -la",
			wantDeny: false,
		},
		"error: rm command is blocked": {
			command:    "rm -rf /tmp/test",
			wantDeny:   true,
			wantReason: "dangerous pattern: rm ",
		},
		"error: rmdir command is blocked": {
			command:    "rmdir /tmp/foo",
			wantDeny:   true,
			wantReason: "dangerous pattern: rmdir",
		},
		"success: echo command is allowed": {
			command:  "echo hello",
			wantDeny: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			input, _ := stdjson.Marshal(map[string]string{"command": tt.command})
			event := HookEvent{
				Kind:      HookEventPreToolUse,
				ToolName:  "Bash",
				ToolInput: jsontext.Value(input),
			}

			dec, err := dispatchHooks(t.Context(), regs, event)
			if err != nil {
				t.Fatalf("dispatchHooks() error = %v", err)
			}

			isDeny := dec.HookSpecificOutput.PermissionDecision == PermissionDeny
			if isDeny != tt.wantDeny {
				t.Errorf("Deny = %v, want %v (command=%q)", isDeny, tt.wantDeny, tt.command)
			}
			if tt.wantDeny && !strings.Contains(dec.HookSpecificOutput.PermissionDecisionReason, tt.wantReason) {
				t.Errorf("Reason = %q, want to contain %q", dec.HookSpecificOutput.PermissionDecisionReason, tt.wantReason)
			}
		})
	}
}

// ── TestExampleParity_ToolPermissionCallback ──────────────────────────────────

// TestExampleParity_ToolPermissionCallback verifies the tool_permission_callback
// example pattern: CanUseTool allows Read and safe Bash, denies everything else.
// Mirrors examples/tool_permission_callback.py (parity with AC9).
func TestExampleParity_ToolPermissionCallback(t *testing.T) {
	t.Parallel()

	// Mirrors the permissionCallback in examples/tool_permission_callback/main.go.
	permCb := func(_ context.Context, toolName string, input jsontext.Value) (PermissionDecision, error) {
		switch toolName {
		case "Read":
			return PermissionAllow, nil
		case "Bash":
			var inp struct {
				Command string `json:"command"`
			}
			if len(input) > 0 {
				_ = stdjson.Unmarshal(input, &inp)
			}
			if strings.HasPrefix(inp.Command, "ls") || strings.HasPrefix(inp.Command, "echo") {
				return PermissionAllow, nil
			}
			return PermissionDeny, nil
		default:
			return PermissionDeny, nil
		}
	}

	tests := map[string]struct {
		toolName string
		input    string
		want     PermissionDecision
	}{
		"success: Read is allowed":      {toolName: "Read", input: `{}`, want: PermissionAllow},
		"success: Bash ls is allowed":   {toolName: "Bash", input: `{"command":"ls -la"}`, want: PermissionAllow},
		"success: Bash echo is allowed": {toolName: "Bash", input: `{"command":"echo hello"}`, want: PermissionAllow},
		"error: Bash rm is denied":      {toolName: "Bash", input: `{"command":"rm -rf /tmp/x"}`, want: PermissionDeny},
		"error: Write tool is denied":   {toolName: "Write", input: `{}`, want: PermissionDeny},
		"error: unknown tool is denied": {toolName: "WebSearch", input: `{}`, want: PermissionDeny},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			event := HookEvent{
				Kind:      HookEventPreToolUse,
				ToolName:  tt.toolName,
				ToolInput: jsontext.Value(tt.input),
			}
			dec, err := applyCanUseTool(t.Context(), permCb, event)
			if err != nil {
				t.Fatalf("applyCanUseTool() error = %v", err)
			}
			if dec.HookSpecificOutput.PermissionDecision != tt.want {
				t.Errorf("PermissionDecision = %q, want %q (tool=%q)", dec.HookSpecificOutput.PermissionDecision, tt.want, tt.toolName)
			}
		})
	}
}

// ── TestExampleParity_MCPCalculator ──────────────────────────────────────────

// TestExampleParity_MCPCalculator verifies the mcp_calculator example pattern:
// Tool[I any] constructors return correct metadata and the typed handler
// function computes arithmetic results correctly.
// Mirrors examples/mcp_calculator.py (parity with AC10).
func TestExampleParity_MCPCalculator(t *testing.T) {
	t.Parallel()

	type calcInput struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
	}

	addFn := func(_ context.Context, in calcInput) (ToolResult, error) {
		return ToolResult{Content: fmt.Sprintf("%g", in.A+in.B)}, nil
	}
	subFn := func(_ context.Context, in calcInput) (ToolResult, error) {
		return ToolResult{Content: fmt.Sprintf("%g", in.A-in.B)}, nil
	}
	mulFn := func(_ context.Context, in calcInput) (ToolResult, error) {
		return ToolResult{Content: fmt.Sprintf("%g", in.A*in.B)}, nil
	}
	divFn := func(_ context.Context, in calcInput) (ToolResult, error) {
		if in.B == 0 {
			return ToolResult{Content: "error: division by zero", IsError: true}, nil
		}
		return ToolResult{Content: fmt.Sprintf("%g", in.A/in.B)}, nil
	}

	addTool := Tool("add", "Add two numbers.", nil, addFn)
	subTool := Tool("subtract", "Subtract b from a.", nil, subFn)
	mulTool := Tool("multiply", "Multiply two numbers.", nil, mulFn)
	divTool := Tool("divide", "Divide a by b.", nil, divFn)

	// Verify tool metadata.
	if addTool.Name() != "add" {
		t.Errorf("add tool name = %q, want add", addTool.Name())
	}
	if subTool.Name() != "subtract" {
		t.Errorf("subtract tool name = %q, want subtract", subTool.Name())
	}
	if mulTool.Name() != "multiply" {
		t.Errorf("multiply tool name = %q, want multiply", mulTool.Name())
	}
	if divTool.Name() != "divide" {
		t.Errorf("divide tool name = %q, want divide", divTool.Name())
	}

	// Verify arithmetic via the typed functions directly.
	tests := map[string]struct {
		fn   func(context.Context, calcInput) (ToolResult, error)
		a, b float64
		want string
	}{
		"success: add 10+5":      {fn: addFn, a: 10, b: 5, want: "15"},
		"success: subtract 10-3": {fn: subFn, a: 10, b: 3, want: "7"},
		"success: multiply 4*3":  {fn: mulFn, a: 4, b: 3, want: "12"},
		"success: divide 10/2":   {fn: divFn, a: 10, b: 2, want: "5"},
		"error: divide by zero":  {fn: divFn, a: 1, b: 0, want: "error: division by zero"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res, err := tt.fn(t.Context(), calcInput{A: tt.a, B: tt.b})
			if err != nil {
				t.Fatalf("%s: fn error = %v", name, err)
			}
			if res.Content != tt.want {
				t.Errorf("%s: Content = %q, want %q", name, res.Content, tt.want)
			}
		})
	}
}

// ── TestExampleParity_SystemPrompt_LaunchArgs ─────────────────────────────────

// TestExampleParity_SystemPrompt_LaunchArgs verifies that Options.SystemPrompt
// round-trips into the --system-prompt CLI flag. Mirrors examples/system_prompt.py.
func TestExampleParity_SystemPrompt_LaunchArgs(t *testing.T) {
	t.Parallel()

	const prompt = "You respond only in haiku."
	args := buildLaunchArgs("/usr/local/bin/claude", "", &Options{SystemPrompt: prompt}, "")

	found := false
	for i, a := range args {
		if a == "--system-prompt" && i+1 < len(args) && args[i+1] == prompt {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("buildLaunchArgs args = %v; want --system-prompt %q", args, prompt)
	}
}

// ── TestExampleParity_ToolsOption_LaunchArgs ──────────────────────────────────

// TestExampleParity_ToolsOption_LaunchArgs verifies that Options.AllowedTools
// round-trips into --allowedTools flags. Mirrors examples/tools_option.py.
func TestExampleParity_ToolsOption_LaunchArgs(t *testing.T) {
	t.Parallel()

	opts := &Options{AllowedTools: []string{"Read", "Bash"}}
	args := buildLaunchArgs("/usr/local/bin/claude", "", opts, "")

	var gotTools []string
	for i, a := range args {
		if a == "--allowedTools" && i+1 < len(args) {
			gotTools = append(gotTools, args[i+1])
		}
	}
	if len(gotTools) != 2 {
		t.Fatalf("--allowedTools count = %d, want 2; args = %v", len(gotTools), args)
	}
	if gotTools[0] != "Read" || gotTools[1] != "Bash" {
		t.Errorf("--allowedTools = %v, want [Read Bash]", gotTools)
	}
}

// ── TestExampleParity_MaxBudgetUSD_LaunchArgs ─────────────────────────────────

// TestExampleParity_MaxBudgetUSD_LaunchArgs verifies that Options.MaxBudgetUSD
// round-trips into the --max-budget flag. Mirrors examples/max_budget_usd.py.
func TestExampleParity_MaxBudgetUSD_LaunchArgs(t *testing.T) {
	t.Parallel()

	opts := &Options{MaxBudgetUSD: 0.01}
	args := buildLaunchArgs("/usr/local/bin/claude", "", opts, "")

	found := false
	for i, a := range args {
		if a == "--max-budget" && i+1 < len(args) && args[i+1] == "0.01" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("buildLaunchArgs args = %v; want --max-budget 0.01", args)
	}
}

// ── TestExampleParity_IncludePartialMessages_LaunchArgs ──────────────────────

// TestExampleParity_IncludePartialMessages_LaunchArgs verifies that
// Options.IncludePartialMessages emits --include-partial-messages.
// Mirrors examples/include_partial_messages.py.
func TestExampleParity_IncludePartialMessages_LaunchArgs(t *testing.T) {
	t.Parallel()

	opts := &Options{IncludePartialMessages: true}
	args := buildLaunchArgs("/usr/local/bin/claude", "", opts, "")

	found := slices.Contains(args, "--include-partial-messages")
	if !found {
		t.Errorf("buildLaunchArgs args = %v; want --include-partial-messages", args)
	}

	// Without the flag, --include-partial-messages must be absent.
	argsOff := buildLaunchArgs("/usr/local/bin/claude", "", &Options{}, "")
	if slices.Contains(argsOff, "--include-partial-messages") {
		t.Errorf("buildLaunchArgs: --include-partial-messages present when IncludePartialMessages=false")
	}
}

// ── TestExampleParity_Plugins_LaunchArgs ─────────────────────────────────────

// TestExampleParity_Plugins_LaunchArgs verifies that Options.Plugins entries
// with a Path emit --plugin-dir flags. Mirrors examples/plugin_example.py.
func TestExampleParity_Plugins_LaunchArgs(t *testing.T) {
	t.Parallel()

	opts := &Options{
		Plugins: []Plugin{
			{Name: "my-plugin", Path: "/usr/local/lib/my-plugin"},
		},
	}
	args := buildLaunchArgs("/usr/local/bin/claude", "", opts, "")

	found := false
	for i, a := range args {
		if a == "--plugin-dir" && i+1 < len(args) && args[i+1] == "/usr/local/lib/my-plugin" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("buildLaunchArgs args = %v; want --plugin-dir /usr/local/lib/my-plugin", args)
	}
}

// ── TestExampleParity_SettingSources_LaunchArgs ───────────────────────────────

// TestExampleParity_SettingSources_LaunchArgs verifies that
// Options.SettingSources entries emit a comma-joined --setting-sources= flag.
// Mirrors examples/setting_sources.py.
func TestExampleParity_SettingSources_LaunchArgs(t *testing.T) {
	t.Parallel()

	opts := &Options{
		SettingSources: []SettingSource{
			{Path: "/etc/claude/settings.json"},
			{Path: "/home/user/.claude/settings.json"},
		},
	}
	args := buildLaunchArgs("/usr/local/bin/claude", "", opts, "")

	want := "--setting-sources=/etc/claude/settings.json,/home/user/.claude/settings.json"
	found := slices.Contains(args, want)
	if !found {
		t.Errorf("buildLaunchArgs args = %v; want %q", args, want)
	}
}

// ── TestExampleParity_Agents_Options ─────────────────────────────────────────

// TestExampleParity_Agents_Options verifies that Options with AgentDefinitions
// passes validation and that agent fields are set correctly.
// Mirrors examples/agents.py and examples/filesystem_agents.py.
func TestExampleParity_Agents_Options(t *testing.T) {
	t.Parallel()

	opts := &Options{
		Agents: []AgentDefinition{
			{
				Name:         "researcher",
				Description:  "Searches for and summarises information.",
				SystemPrompt: "You are a concise research assistant.",
				AllowedTools: []string{"WebSearch", "Read"},
			},
			{
				Name:         "coder",
				Description:  "Writes and reviews Go code.",
				SystemPrompt: "You are an expert Go programmer.",
				AllowedTools: []string{"Read", "Write", "Bash"},
			},
		},
		MaxTurns: 5,
	}

	if err := opts.validate(); err != nil {
		t.Fatalf("validate() error = %v", err)
	}
	if len(opts.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(opts.Agents))
	}
	if opts.Agents[0].Name != "researcher" {
		t.Errorf("Agents[0].Name = %q, want researcher", opts.Agents[0].Name)
	}
	if opts.Agents[1].Name != "coder" {
		t.Errorf("Agents[1].Name = %q, want coder", opts.Agents[1].Name)
	}
	if len(opts.Agents[1].AllowedTools) != 3 {
		t.Errorf("Agents[1].AllowedTools len = %d, want 3", len(opts.Agents[1].AllowedTools))
	}
}

// ── TestExampleParity_StderrCallback_ProcessError ────────────────────────────

// TestExampleParity_StderrCallback_ProcessError verifies that ProcessError
// correctly formats its error string with StderrTail.
// Mirrors examples/stderr_callback_example.py.
func TestExampleParity_StderrCallback_ProcessError(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		exitCode   int
		stderrTail string
		wantSubstr string
	}{
		"success: exit code only": {
			exitCode:   1,
			stderrTail: "",
			wantSubstr: "exit with code 1",
		},
		"error: exit code with stderr tail": {
			exitCode:   2,
			stderrTail: "fatal: invalid configuration",
			wantSubstr: "fatal: invalid configuration",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := &ProcessError{ExitCode: tt.exitCode, StderrTail: tt.stderrTail}
			msg := err.Error()
			if !strings.Contains(msg, fmt.Sprintf("%d", tt.exitCode)) {
				t.Errorf("Error() = %q; want exit code %d", msg, tt.exitCode)
			}
			if tt.stderrTail != "" && !strings.Contains(msg, tt.stderrTail) {
				t.Errorf("Error() = %q; want to contain %q", msg, tt.stderrTail)
			}
		})
	}
}
