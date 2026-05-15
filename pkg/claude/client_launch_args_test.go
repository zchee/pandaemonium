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

func TestBuildLaunchArgs(t *testing.T) {
	t.Parallel()

	const fakeCLI = "/usr/local/bin/claude"

	tests := map[string]struct {
		cliPath string
		prompt  string
		opts    *Options
		wantIn  []string // substrings / tokens that MUST appear in joined args
		wantOut []string // substrings / tokens that MUST NOT appear in joined args
	}{
		"success: nil opts uses stream-json output format": {
			cliPath: fakeCLI,
			prompt:  "hello",
			opts:    nil,
			wantIn:  []string{fakeCLI, "--output-format", "stream-json", "--print", "hello"},
		},
		"success: empty opts uses stream-json output format": {
			cliPath: fakeCLI,
			prompt:  "hi",
			opts:    &Options{},
			wantIn:  []string{"--output-format", "stream-json"},
		},
		"success: custom output format is used": {
			cliPath: fakeCLI,
			prompt:  "",
			opts:    &Options{OutputFormat: "json"},
			wantIn:  []string{"--output-format", "json"},
			wantOut: []string{"stream-json"},
		},
		"success: model flag appears when set": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{Model: "claude-opus-4-5"},
			wantIn:  []string{"--model", "claude-opus-4-5"},
		},
		"success: system prompt flag appears when set": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{SystemPrompt: "Be concise."},
			wantIn:  []string{"--system-prompt", "Be concise."},
		},
		"success: multiple allowed tools each get their own flag": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{AllowedTools: []string{"Bash", "Write"}},
			wantIn:  []string{"--allowedTools", "Bash", "--allowedTools", "Write"},
		},
		"success: max turns flag appears when positive": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{MaxTurns: 5},
			wantIn:  []string{"--max-turns", "5"},
		},
		"success: zero max turns omits the flag": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{MaxTurns: 0},
			wantOut: []string{"--max-turns"},
		},
		"success: permission mode flag appears when set": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{PermissionMode: "bypassPermissions"},
			wantIn:  []string{"--permission-mode", "bypassPermissions"},
		},
		"success: api key helper flag appears when set": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{APIKeyHelper: "/usr/local/bin/apikey"},
			wantIn:  []string{"--api-key-helper", "/usr/local/bin/apikey"},
		},
		"success: max budget flag appears when positive": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{MaxBudgetUSD: 1.5},
			wantIn:  []string{"--max-budget", "1.5"},
		},
		"success: zero max budget omits flag": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{MaxBudgetUSD: 0},
			wantOut: []string{"--max-budget"},
		},
		"success: verbose flag appears when true": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{Verbose: true},
			wantIn:  []string{"--verbose"},
		},
		"success: verbose flag absent when false": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{Verbose: false},
			wantOut: []string{"--verbose"},
		},
		"success: include partial messages flag appears when true": {
			cliPath: fakeCLI,
			prompt:  "x",
			opts:    &Options{IncludePartialMessages: true},
			wantIn:  []string{"--include-partial-messages"},
		},
		"success: empty prompt omits print flag": {
			cliPath: fakeCLI,
			prompt:  "",
			opts:    nil,
			wantOut: []string{"--print"},
		},
		"success: first arg is always the cli path": {
			cliPath: fakeCLI,
			prompt:  "test",
			opts:    nil,
			wantIn:  []string{fakeCLI},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := buildLaunchArgs(tt.cliPath, tt.prompt, tt.opts, "")
			if len(got) == 0 {
				t.Fatal("buildLaunchArgs() returned empty slice")
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

func TestBuildLaunchArgs_CLIPathIsFirst(t *testing.T) {
	t.Parallel()
	args := buildLaunchArgs("/path/to/claude", "prompt", nil, "")
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
			got := buildLaunchArgs(fakeCLI, "x", tt.opts, "")
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
		"success: single path setting source": {
			opts:   &Options{SettingSources: []SettingSource{{Path: "/etc/claude/settings.json"}}},
			wantIn: []string{"--setting-sources=/etc/claude/settings.json"},
		},
		"success: multiple path sources are comma-joined": {
			opts: &Options{SettingSources: []SettingSource{
				{Path: "/etc/claude/a.json"},
				{Path: "/etc/claude/b.json"},
			}},
			wantIn: []string{"--setting-sources=/etc/claude/a.json,/etc/claude/b.json"},
		},
		"success: URL source is included": {
			opts:   &Options{SettingSources: []SettingSource{{URL: "https://example.com/settings.json"}}},
			wantIn: []string{"--setting-sources=https://example.com/settings.json"},
		},
		"success: empty SettingSources omits the flag": {
			opts:    &Options{SettingSources: []SettingSource{}},
			wantOut: []string{"--setting-sources"},
		},
		"success: setting source with both Path and URL uses Path": {
			opts: &Options{SettingSources: []SettingSource{
				{Path: "/local/settings.json", URL: "https://example.com/settings.json"},
			}},
			wantIn:  []string{"--setting-sources=/local/settings.json"},
			wantOut: []string{"https://example.com"},
		},
		"success: setting source with empty path and empty URL is skipped": {
			opts:    &Options{SettingSources: []SettingSource{{}}},
			wantOut: []string{"--setting-sources"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := buildLaunchArgs(fakeCLI, "x", tt.opts, "")
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
	got := buildLaunchArgs("/usr/local/bin/claude", "x", &Options{
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
			got := buildLaunchArgs(fakeCLI, "x", nil, tt.resumeID)
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
