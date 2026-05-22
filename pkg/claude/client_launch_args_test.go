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

	"github.com/go-json-experiment/json"
	"github.com/google/jsonschema-go/jsonschema"
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
		"success: allowed tools are comma-joined into one flag": {
			cliPath: fakeCLI,
			opts:    &Options{AllowedTools: []string{"Bash", "Write"}},
			wantIn:  []string{"--allowedTools", "Bash,Write"},
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
			opts:    &Options{PermissionMode: PermissionModeBypassPermissions},
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
		"alpha-opt": new("v1"),
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

// TestBuildLaunchArgs_Skills covers the M9c Skills coupling: skill-tool
// injection into --allowedTools and the user,project setting-sources default,
// mirroring upstream _apply_skills_defaults.
func TestBuildLaunchArgs_Skills(t *testing.T) {
	t.Parallel()

	const fakeCLI = "/usr/local/bin/claude"

	tests := map[string]struct {
		opts        *Options
		wantAllowed string // expected --allowedTools value, "" to skip
		wantSources string // expected --setting-sources= value, "" means flag absent
	}{
		"AllSkills injects bare Skill and defaults sources": {
			opts:        &Options{Skills: AllSkills()},
			wantAllowed: "Skill",
			wantSources: "user,project",
		},
		"named skills inject Skill(name) and default sources": {
			opts:        &Options{Skills: []string{"foo", "bar"}},
			wantAllowed: "Skill(foo),Skill(bar)",
			wantSources: "user,project",
		},
		"explicit SettingSources wins over the default": {
			opts:        &Options{Skills: AllSkills(), SettingSources: []SettingSource{SettingSourceLocal}},
			wantAllowed: "Skill",
			wantSources: "local",
		},
		"skill injection is additive to existing AllowedTools": {
			opts:        &Options{Skills: AllSkills(), AllowedTools: []string{"Read"}},
			wantAllowed: "Read,Skill",
			wantSources: "user,project",
		},
		"no skills: no injection and no sources default": {
			opts:        &Options{AllowedTools: []string{"Read"}},
			wantAllowed: "Read",
			wantSources: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			args := mustLaunchArgs(t, fakeCLI, tt.opts, "")

			if tt.wantAllowed != "" {
				ai := extraArgIndex(args, "--allowedTools")
				if ai < 0 || ai+1 >= len(args) || args[ai+1] != tt.wantAllowed {
					t.Errorf("--allowedTools = %q, want %q (args=%v)", argValue(args, "--allowedTools"), tt.wantAllowed, args)
				}
			}

			si := extraArgIndex(args, "--setting-sources="+tt.wantSources)
			if tt.wantSources != "" {
				if si < 0 {
					t.Errorf("missing --setting-sources=%s (args=%v)", tt.wantSources, args)
				}
			} else {
				for _, a := range args {
					if strings.HasPrefix(a, "--setting-sources=") {
						t.Errorf("unexpected %q with no Skills/SettingSources", a)
					}
				}
			}
		})
	}
}

// argValue returns the token following flag in args, or "" if absent.
func argValue(args []string, flag string) string {
	i := extraArgIndex(args, flag)
	if i < 0 || i+1 >= len(args) {
		return ""
	}
	return args[i+1]
}

// TestBuildLaunchArgs_Tools covers the base-tool-set --tools flag, distinct
// from --allowedTools (subprocess_cli.py:241-250). Pins the tri-state: nil omits
// the flag, non-nil empty emits --tools "", a list joins comma-separated, and a
// preset takes precedence over Tools.
func TestBuildLaunchArgs_Tools(t *testing.T) {
	t.Parallel()
	const fakeCLI = "/usr/local/bin/claude"

	t.Run("nil omits --tools", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{}, "")
		if extraArgIndex(got, "--tools") != -1 {
			t.Errorf("--tools emitted for nil Tools; args = %v", got)
		}
	})

	t.Run("non-nil empty emits --tools empty-string", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{Tools: []string{}}, "")
		i := extraArgIndex(got, "--tools")
		if i == -1 {
			t.Fatalf("--tools not emitted for empty Tools; args = %v", got)
		}
		if i+1 >= len(got) || got[i+1] != "" {
			t.Errorf("--tools value = %q, want empty string", argValue(got, "--tools"))
		}
	})

	t.Run("list joins comma-separated", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{Tools: []string{"Read", "Bash"}}, "")
		if v := argValue(got, "--tools"); v != "Read,Bash" {
			t.Errorf("--tools = %q, want Read,Bash", v)
		}
	})

	t.Run("preset takes precedence over Tools", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{Tools: []string{"Read"}, ToolsPreset: "default"}, "")
		if v := argValue(got, "--tools"); v != "default" {
			t.Errorf("--tools = %q, want default (preset wins)", v)
		}
	})

	t.Run("tools is independent of allowedTools", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{AllowedTools: []string{"Edit"}, Tools: []string{"Read"}}, "")
		if v := argValue(got, "--allowedTools"); v != "Edit" {
			t.Errorf("--allowedTools = %q, want Edit", v)
		}
		if v := argValue(got, "--tools"); v != "Read" {
			t.Errorf("--tools = %q, want Read", v)
		}
	})
}

// TestBuildLaunchArgs_JSONSchema covers the structured-output --json-schema flag
// (subprocess_cli.py:395-404): emitted as the marshaled schema when set, absent
// when nil, and independent of --output-format.
func TestBuildLaunchArgs_JSONSchema(t *testing.T) {
	t.Parallel()
	const fakeCLI = "/usr/local/bin/claude"

	t.Run("nil omits --json-schema", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{}, "")
		if extraArgIndex(got, "--json-schema") != -1 {
			t.Errorf("--json-schema emitted for nil JSONSchema; args = %v", got)
		}
	})

	t.Run("non-nil emits marshaled schema", func(t *testing.T) {
		t.Parallel()
		schema := &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{"answer": {Type: "string"}},
		}
		got := mustLaunchArgs(t, fakeCLI, &Options{JSONSchema: schema}, "")
		v := argValue(got, "--json-schema")
		if v == "" {
			t.Fatalf("--json-schema not emitted; args = %v", got)
		}
		want, err := json.Marshal(schema)
		if err != nil {
			t.Fatalf("marshal schema: %v", err)
		}
		if v != string(want) {
			t.Errorf("--json-schema = %q, want %q", v, want)
		}
		// Independent of --output-format (still the default stream-json).
		if of := argValue(got, "--output-format"); of != "stream-json" {
			t.Errorf("--output-format = %q, want stream-json", of)
		}
	})
}

// ── M10b Thinking group ──────────────────────────────────────────────────────

// TestBuildLaunchArgs_Thinking covers the Thinking-config / MaxThinkingTokens /
// Effort wiring (subprocess_cli.py:372-393). The discriminator cases pin the
// three wire-mapping quirks:
//
//  1. Enabled emits ONLY --max-thinking-tokens (no --thinking flag).
//  2. Disabled does not emit --thinking-display even if it were carried.
//  3. Thinking takes precedence over MaxThinkingTokens (else-if upstream).
func TestBuildLaunchArgs_Thinking(t *testing.T) {
	t.Parallel()

	const fakeCLI = "/usr/local/bin/claude"

	t.Run("adaptive emits --thinking adaptive", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{Thinking: ThinkingConfigAdaptive{}}, "")
		if v := argValue(got, "--thinking"); v != "adaptive" {
			t.Errorf("--thinking = %q, want adaptive", v)
		}
		if extraArgIndex(got, "--max-thinking-tokens") != -1 {
			t.Errorf("adaptive must not emit --max-thinking-tokens: %v", got)
		}
		if extraArgIndex(got, "--thinking-display") != -1 {
			t.Errorf("adaptive with empty Display must not emit --thinking-display: %v", got)
		}
	})

	t.Run("adaptive with Display emits --thinking-display", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{
			Thinking: ThinkingConfigAdaptive{Display: ThinkingDisplaySummarized},
		}, "")
		if v := argValue(got, "--thinking-display"); v != "summarized" {
			t.Errorf("--thinking-display = %q, want summarized", v)
		}
	})

	t.Run("enabled emits only --max-thinking-tokens (no --thinking flag)", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{
			Thinking: ThinkingConfigEnabled{BudgetTokens: 4096},
		}, "")
		if v := argValue(got, "--max-thinking-tokens"); v != "4096" {
			t.Errorf("--max-thinking-tokens = %q, want 4096", v)
		}
		if extraArgIndex(got, "--thinking") != -1 {
			t.Errorf("enabled must NOT emit --thinking (subprocess_cli.py:378): %v", got)
		}
	})

	t.Run("enabled with Display emits --thinking-display", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{
			Thinking: ThinkingConfigEnabled{BudgetTokens: 2048, Display: ThinkingDisplayOmitted},
		}, "")
		if v := argValue(got, "--thinking-display"); v != "omitted" {
			t.Errorf("--thinking-display = %q, want omitted", v)
		}
	})

	t.Run("disabled emits --thinking disabled and no --thinking-display", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{Thinking: ThinkingConfigDisabled{}}, "")
		if v := argValue(got, "--thinking"); v != "disabled" {
			t.Errorf("--thinking = %q, want disabled", v)
		}
		if extraArgIndex(got, "--thinking-display") != -1 {
			t.Errorf("disabled must not emit --thinking-display: %v", got)
		}
	})

	t.Run("MaxThinkingTokens alone emits the flag", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{MaxThinkingTokens: 1024}, "")
		if v := argValue(got, "--max-thinking-tokens"); v != "1024" {
			t.Errorf("--max-thinking-tokens = %q, want 1024", v)
		}
		if extraArgIndex(got, "--thinking") != -1 {
			t.Errorf("MaxThinkingTokens alone must not emit --thinking: %v", got)
		}
	})

	t.Run("MaxThinkingTokens zero omits the flag", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{}, "")
		if extraArgIndex(got, "--max-thinking-tokens") != -1 {
			t.Errorf("unset MaxThinkingTokens must omit --max-thinking-tokens: %v", got)
		}
	})

	t.Run("Thinking precedence over MaxThinkingTokens", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{
			Thinking:          ThinkingConfigAdaptive{},
			MaxThinkingTokens: 9999,
		}, "")
		if v := argValue(got, "--thinking"); v != "adaptive" {
			t.Errorf("--thinking = %q, want adaptive", v)
		}
		// Adaptive does not emit --max-thinking-tokens; MaxThinkingTokens is
		// ignored because Thinking is set (subprocess_cli.py:387 else-if).
		if extraArgIndex(got, "--max-thinking-tokens") != -1 {
			t.Errorf("Thinking set must ignore MaxThinkingTokens: %v", got)
		}
	})

	t.Run("Effort emits independently of Thinking", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{
			Thinking: ThinkingConfigAdaptive{},
			Effort:   EffortLevelHigh,
		}, "")
		if v := argValue(got, "--effort"); v != "high" {
			t.Errorf("--effort = %q, want high", v)
		}
		if v := argValue(got, "--thinking"); v != "adaptive" {
			t.Errorf("--thinking = %q, want adaptive (Effort must not displace it)", v)
		}
	})

	t.Run("Effort zero omits the flag", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{}, "")
		if extraArgIndex(got, "--effort") != -1 {
			t.Errorf("unset Effort must omit --effort: %v", got)
		}
	})
}

// ── M10d TaskBudget ──────────────────────────────────────────────────────────

// TestBuildLaunchArgs_TaskBudget covers the --task-budget wiring
// (subprocess_cli.py:268-269). The explicit-zero case pins parity with
// upstream's `is not None` gate: an explicit Total=0 must reach the wire.
func TestBuildLaunchArgs_TaskBudget(t *testing.T) {
	t.Parallel()
	const fakeCLI = "/usr/local/bin/claude"

	t.Run("nil TaskBudget omits the flag", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{}, "")
		if extraArgIndex(got, "--task-budget") != -1 {
			t.Errorf("nil TaskBudget must omit --task-budget: %v", got)
		}
	})

	t.Run("positive Total emits the value", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{TaskBudget: &TaskBudget{Total: 4096}}, "")
		if v := argValue(got, "--task-budget"); v != "4096" {
			t.Errorf("--task-budget = %q, want 4096", v)
		}
	})

	t.Run("explicit zero Total reaches the wire", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{TaskBudget: &TaskBudget{Total: 0}}, "")
		if v := argValue(got, "--task-budget"); v != "0" {
			t.Errorf("--task-budget = %q, want 0 (parity with upstream is_not_none gate)", v)
		}
	})
}
