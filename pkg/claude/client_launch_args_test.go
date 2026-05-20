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
	"strings"
	"testing"
)

// mustLaunchArgs calls buildLaunchArgs and fails the test if it returns an
// error. buildLaunchArgs only errors when the --mcp-config payload fails to
// marshal, which the well-formed test inputs never trigger.
func mustLaunchArgs(t *testing.T, cliPath string, opts *Options, resumeSessionID string) []string {
	t.Helper()
	args, err := buildLaunchArgs(cliPath, opts, resumeSessionID)
	if err != nil {
		t.Fatalf("buildLaunchArgs() error = %v", err)
	}
	return args
}

func TestBuildLaunchArgs(t *testing.T) {
	t.Parallel()

	const fakeCLI = "/usr/local/bin/claude"

	tests := map[string]struct {
		cliPath string
		opts    *Options
		wantIn  []string // substrings / tokens that MUST appear in joined args
		wantOut []string // substrings / tokens that MUST NOT appear in joined args
	}{
		"success: minimal nil opts is streaming with no --print": {
			cliPath: fakeCLI,
			opts:    nil,
			wantIn:  []string{fakeCLI, "--output-format", "stream-json", "--input-format", "--verbose"},
			wantOut: []string{"--print"},
		},
		"success: empty opts uses stream-json output format": {
			cliPath: fakeCLI,
			opts:    &Options{},
			wantIn:  []string{"--output-format", "stream-json"},
			wantOut: []string{"--print"},
		},
		"success: custom output format is used": {
			cliPath: fakeCLI,
			opts:    &Options{OutputFormat: "json"},
			// stream-json must not appear as the output format. The input format
			// still defaults to stream-json, so assert --output-format json and
			// that --print is never present.
			wantIn:  []string{"--output-format", "json"},
			wantOut: []string{"--print"},
		},
		"success: input format always defaults to stream-json": {
			cliPath: fakeCLI,
			opts:    &Options{},
			wantIn:  []string{"--input-format", "stream-json"},
		},
		"success: custom input format overrides stream-json default": {
			cliPath: fakeCLI,
			opts:    &Options{InputFormat: "text"},
			wantIn:  []string{"--input-format", "text"},
		},
		"success: model flag appears when set": {
			cliPath: fakeCLI,
			opts:    &Options{Model: "claude-opus-4-5"},
			wantIn:  []string{"--model", "claude-opus-4-5"},
		},
		"success: system prompt flag appears when set": {
			cliPath: fakeCLI,
			opts:    &Options{SystemPrompt: "Be concise."},
			wantIn:  []string{"--system-prompt", "Be concise."},
		},
		"success: multiple allowed tools each get their own flag": {
			cliPath: fakeCLI,
			opts:    &Options{AllowedTools: []string{"Bash", "Write"}},
			wantIn:  []string{"--allowedTools", "Bash", "--allowedTools", "Write"},
		},
		"success: max turns flag appears when positive": {
			cliPath: fakeCLI,
			opts:    &Options{MaxTurns: 5},
			wantIn:  []string{"--max-turns", "5"},
		},
		"success: zero max turns omits the flag": {
			cliPath: fakeCLI,
			opts:    &Options{MaxTurns: 0},
			wantOut: []string{"--max-turns"},
		},
		"success: permission mode flag appears when set": {
			cliPath: fakeCLI,
			opts:    &Options{PermissionMode: "bypassPermissions"},
			wantIn:  []string{"--permission-mode", "bypassPermissions"},
		},
		"success: api key helper flag appears when set": {
			cliPath: fakeCLI,
			opts:    &Options{APIKeyHelper: "/usr/local/bin/apikey"},
			wantIn:  []string{"--api-key-helper", "/usr/local/bin/apikey"},
		},
		"success: max budget flag appears when positive": {
			cliPath: fakeCLI,
			opts:    &Options{MaxBudgetUSD: 1.5},
			wantIn:  []string{"--max-budget-usd", "1.5"},
		},
		"success: zero max budget omits flag": {
			cliPath: fakeCLI,
			opts:    &Options{MaxBudgetUSD: 0},
			wantOut: []string{"--max-budget-usd"},
		},
		"success: verbose flag always present with explicit true": {
			cliPath: fakeCLI,
			opts:    &Options{Verbose: true},
			wantIn:  []string{"--verbose"},
		},
		"success: verbose flag always present even when Verbose is false": {
			cliPath: fakeCLI,
			// --verbose is now emitted unconditionally to match upstream
			// subprocess_cli.py:225, regardless of opts.Verbose.
			opts:   &Options{Verbose: false},
			wantIn: []string{"--verbose"},
		},
		"success: include partial messages flag appears when true": {
			cliPath: fakeCLI,
			opts:    &Options{IncludePartialMessages: true},
			wantIn:  []string{"--include-partial-messages"},
		},
		"success: print flag is never emitted": {
			cliPath: fakeCLI,
			opts:    &Options{Model: "claude-opus-4-5", SystemPrompt: "hi"},
			wantOut: []string{"--print"},
		},
		"success: first arg is always the cli path": {
			cliPath: fakeCLI,
			opts:    nil,
			wantIn:  []string{fakeCLI},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := mustLaunchArgs(t, tt.cliPath, tt.opts, "")
			if len(got) == 0 {
				t.Fatal("mustLaunchArgs(t, ) returned empty slice")
			}
			if got[0] != tt.cliPath {
				t.Fatalf("got[0] = %q, want %q", got[0], tt.cliPath)
			}

			joined := strings.Join(got, " ")
			for _, want := range tt.wantIn {
				if !strings.Contains(joined, want) {
					t.Errorf("args %v missing %q", got, want)
				}
			}
			for _, reject := range tt.wantOut {
				if strings.Contains(joined, reject) {
					t.Errorf("args %v should not contain %q", got, reject)
				}
			}
		})
	}
}

// TestBuildLaunchArgs_SystemPromptAlwaysEmitted verifies that --system-prompt is
// emitted even when Options.SystemPrompt is empty, with the empty string as its
// attached value, matching upstream subprocess_cli.py:228. The check is an
// index-pair (not substring) so a flag emitted without its value would fail.
func TestBuildLaunchArgs_SystemPromptAlwaysEmitted(t *testing.T) {
	t.Parallel()

	args := mustLaunchArgs(t, "/usr/local/bin/claude", &Options{}, "")
	found := false
	for i, a := range args {
		if a == "--system-prompt" {
			if i+1 >= len(args) {
				t.Fatalf("--system-prompt is the last arg with no value: %v", args)
			}
			if args[i+1] != "" {
				t.Errorf("--system-prompt value = %q, want empty string", args[i+1])
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("--system-prompt not emitted for empty SystemPrompt: %v", args)
	}
}

func TestBuildLaunchArgs_CLIPathIsFirst(t *testing.T) {
	t.Parallel()
	args := mustLaunchArgs(t, "/path/to/claude", nil, "")
	if args[0] != "/path/to/claude" {
		t.Fatalf("args[0] = %q, want /path/to/claude", args[0])
	}
}

func TestBuildLaunchArgs_Plugins(t *testing.T) {
	t.Parallel()

	const fakeCLI = "/usr/local/bin/claude"

	tests := map[string]struct {
		opts    *Options
		wantIn  []string
		wantOut []string
	}{
		"success: single plugin emits plugin-dir flag": {
			opts:   &Options{Plugins: []Plugin{{Name: "myplugin", Path: "/opt/plugins/myplugin"}}},
			wantIn: []string{"--plugin-dir", "/opt/plugins/myplugin"},
		},
		"success: multiple plugins each get their own plugin-dir flag": {
			opts: &Options{Plugins: []Plugin{
				{Name: "alpha", Path: "/plugins/alpha"},
				{Name: "beta", Path: "/plugins/beta"},
			}},
			wantIn: []string{"--plugin-dir", "/plugins/alpha", "--plugin-dir", "/plugins/beta"},
		},
		"success: plugin with empty path omits plugin-dir": {
			opts:    &Options{Plugins: []Plugin{{Name: "nopath"}}},
			wantOut: []string{"--plugin-dir"},
		},
		"success: empty Plugins slice omits plugin-dir": {
			opts:    &Options{Plugins: []Plugin{}},
			wantOut: []string{"--plugin-dir"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := mustLaunchArgs(t, fakeCLI, tt.opts, "")
			joined := strings.Join(got, " ")
			for _, want := range tt.wantIn {
				if !strings.Contains(joined, want) {
					t.Errorf("args %v missing %q", got, want)
				}
			}
			for _, reject := range tt.wantOut {
				if strings.Contains(joined, reject) {
					t.Errorf("args %v should not contain %q", got, reject)
				}
			}
		})
	}
}

func TestBuildLaunchArgs_SettingSources(t *testing.T) {
	t.Parallel()

	const fakeCLI = "/usr/local/bin/claude"

	tests := map[string]struct {
		opts    *Options
		wantIn  []string
		wantOut []string
	}{
		"success: single literal setting source": {
			opts:   &Options{SettingSources: []SettingSource{SettingSourceUser}},
			wantIn: []string{"--setting-sources=user"},
		},
		"success: multiple literals are comma-joined in order": {
			opts: &Options{SettingSources: []SettingSource{
				SettingSourceUser,
				SettingSourceProject,
			}},
			wantIn: []string{"--setting-sources=user,project"},
		},
		"success: local literal is included": {
			opts:   &Options{SettingSources: []SettingSource{SettingSourceLocal}},
			wantIn: []string{"--setting-sources=local"},
		},
		"success: empty SettingSources omits the flag": {
			opts:    &Options{SettingSources: []SettingSource{}},
			wantOut: []string{"--setting-sources"},
		},
		"success: nil SettingSources omits the flag": {
			opts:    &Options{},
			wantOut: []string{"--setting-sources"},
		},
		"success: empty-string entry is skipped": {
			opts:    &Options{SettingSources: []SettingSource{""}},
			wantOut: []string{"--setting-sources"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := mustLaunchArgs(t, fakeCLI, tt.opts, "")
			joined := strings.Join(got, " ")
			for _, want := range tt.wantIn {
				if !strings.Contains(joined, want) {
					t.Errorf("args %v missing %q", got, want)
				}
			}
			for _, reject := range tt.wantOut {
				if strings.Contains(joined, reject) {
					t.Errorf("args %v should not contain %q", got, reject)
				}
			}
		})
	}
}

func TestBuildLaunchArgs_Agents(t *testing.T) {
	t.Parallel()

	// Agents are sent via the streaming initialize request (not CLI flags).
	// Verify that no --agent flag (or similar) appears in the launch args
	// regardless of how many AgentDefinitions are configured.
	got := mustLaunchArgs(t, "/usr/local/bin/claude", &Options{
		Agents: []AgentDefinition{
			{Name: "helper", Description: "A helper agent", SystemPrompt: "You help."},
			{Name: "coder", Description: "A coding agent", AllowedTools: []string{"Bash"}},
		},
	}, "")
	joined := strings.Join(got, " ")
	for _, banned := range []string{"--agent", "--agents"} {
		if strings.Contains(joined, banned) {
			t.Errorf("args contain %q, but agents must be sent via streaming initialize (not CLI flags): %v", banned, got)
		}
	}
}

func TestBuildLaunchArgs_Resume(t *testing.T) {
	t.Parallel()

	const fakeCLI = "/usr/local/bin/claude"

	tests := map[string]struct {
		resumeID string
		wantIn   []string
		wantOut  []string
	}{
		"success: non-empty resumeSessionID adds resume flag": {
			resumeID: "sess-abc123",
			wantIn:   []string{"--resume", "sess-abc123"},
		},
		"success: empty resumeSessionID omits resume flag": {
			resumeID: "",
			wantOut:  []string{"--resume"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := mustLaunchArgs(t, fakeCLI, nil, tt.resumeID)
			joined := strings.Join(got, " ")
			for _, want := range tt.wantIn {
				if !strings.Contains(joined, want) {
					t.Errorf("args %v missing %q", got, want)
				}
			}
			for _, reject := range tt.wantOut {
				if strings.Contains(joined, reject) {
					t.Errorf("args %v should not contain %q", got, reject)
				}
			}
		})
	}
}

// TestBuildLaunchArgs_M9PassthroughFlags covers the bucket-2 flag-passthrough
// Options fields added in M9a, each mapping to a single CLI flag.
func TestBuildLaunchArgs_M9PassthroughFlags(t *testing.T) {
	t.Parallel()

	const fakeCLI = "/usr/local/bin/claude"

	tests := map[string]struct {
		opts    *Options
		wantIn  []string
		wantOut []string
	}{
		"disallowed tools are comma-joined into one flag": {
			opts:   &Options{DisallowedTools: []string{"Bash", "Write"}},
			wantIn: []string{"--disallowedTools", "Bash,Write"},
		},
		"fallback model": {
			opts:   &Options{FallbackModel: "claude-haiku-4-5"},
			wantIn: []string{"--fallback-model", "claude-haiku-4-5"},
		},
		"betas are comma-joined": {
			opts:   &Options{Betas: []string{"beta-a", "beta-b"}},
			wantIn: []string{"--betas", "beta-a,beta-b"},
		},
		"permission prompt tool": {
			opts:   &Options{PermissionPromptToolName: "mcp__auth__prompt"},
			wantIn: []string{"--permission-prompt-tool", "mcp__auth__prompt"},
		},
		"continue conversation": {
			opts:   &Options{ContinueConversation: true},
			wantIn: []string{"--continue"},
		},
		"continue absent when false": {
			opts:    &Options{},
			wantOut: []string{"--continue"},
		},
		"session id": {
			opts:   &Options{SessionID: "sess-123"},
			wantIn: []string{"--session-id", "sess-123"},
		},
		"settings string": {
			opts:   &Options{Settings: "/etc/claude/settings.json"},
			wantIn: []string{"--settings", "/etc/claude/settings.json"},
		},
		"add dirs each get their own flag": {
			opts:   &Options{AddDirs: []string{"/a", "/b"}},
			wantIn: []string{"--add-dir", "/a", "--add-dir", "/b"},
		},
		"include hook events": {
			opts:   &Options{IncludeHookEvents: true},
			wantIn: []string{"--include-hook-events"},
		},
		"fork session": {
			opts:   &Options{ForkSession: true},
			wantIn: []string{"--fork-session"},
		},
		"resume from opts": {
			opts:   &Options{Resume: "resume-me"},
			wantIn: []string{"--resume", "resume-me"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := mustLaunchArgs(t, fakeCLI, tt.opts, "")
			joined := strings.Join(got, " ")
			for _, want := range tt.wantIn {
				if !strings.Contains(joined, want) {
					t.Errorf("args %v missing %q", got, want)
				}
			}
			for _, reject := range tt.wantOut {
				if strings.Contains(joined, reject) {
					t.Errorf("args %v should not contain %q", got, reject)
				}
			}
		})
	}
}

// TestBuildLaunchArgs_ResumePrecedence verifies the Fork-driven resumeSessionID
// parameter wins over opts.Resume.
func TestBuildLaunchArgs_ResumePrecedence(t *testing.T) {
	t.Parallel()

	args := mustLaunchArgs(t, "/usr/local/bin/claude", &Options{Resume: "from-opts"}, "from-fork")
	var got string
	for i, a := range args {
		if a == "--resume" && i+1 < len(args) {
			got = args[i+1]
		}
	}
	if got != "from-fork" {
		t.Errorf("--resume = %q, want from-fork (Fork param wins over opts.Resume)", got)
	}
}

// TestBuildLaunchArgs_ExtraArgs covers the M9b ExtraArgs passthrough: a nil
// value emits a bare flag, a non-nil value emits --key value, and keys are
// emitted in sorted (deterministic) order.
func TestBuildLaunchArgs_ExtraArgs(t *testing.T) {
	t.Parallel()

	opts := &Options{ExtraArgs: map[string]*string{
		"zeta-flag": nil,
		"alpha-opt": ExtraFlag("v1"),
	}}
	args := mustLaunchArgs(t, "/usr/local/bin/claude", opts, "")

	zi := extraArgIndex(args, "--zeta-flag")
	if zi < 0 {
		t.Errorf("args %v missing bare --zeta-flag", args)
	}
	ai := extraArgIndex(args, "--alpha-opt")
	if ai < 0 || ai+1 >= len(args) || args[ai+1] != "v1" {
		t.Errorf("args %v missing --alpha-opt v1 pair", args)
	}
	if ai >= 0 && zi >= 0 && ai > zi {
		t.Errorf("extra args not sorted: --alpha-opt at %d after --zeta-flag at %d", ai, zi)
	}
}

// extraArgIndex returns the index of v in s, or -1.
func extraArgIndex(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
