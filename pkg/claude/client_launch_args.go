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
	"sort"
	"strconv"
	"strings"

	"github.com/go-json-experiment/json"
)

// buildLaunchArgs constructs the argument slice for launching the claude CLI
// subprocess. The resulting slice is suitable for exec.Command(args[0], args[1:]...).
//
// cliPath is the resolved binary path (from discoverCLI).
// opts configures additional CLI flags; nil opts uses defaults.
// resumeSessionID, when non-empty, adds --resume <id> so the CLI subprocess
// resumes from that session (used by [ClaudeSDKClient.Fork]).
//
// Note on Agents: AgentDefinition values in opts.Agents are NOT encoded as CLI
// flags. The claude CLI receives agent definitions via the streaming initialize
// request (same pattern as the TypeScript and Python SDKs). No --agents flag
// exists in the claude CLI.
//
// Prompts are NOT passed as flags: the CLI is always launched in streaming
// stdin mode, and the user prompt is sent as a JSON envelope on stdin after
// the initialize handshake completes (see [ClaudeSDKClient.Query]). There is
// no --print flag.
//
// It returns an error only if the --mcp-config payload fails to marshal, which
// cannot happen for the well-formed maps configForCLI produces; the error path
// exists so a future MCPServer config that is not JSON-encodable surfaces a
// CLIConnectionError instead of silently launching without its servers.
//
// Mirrors the structure of pkg/codex/client.go:563 buildAppServerArgs.
// Round-trip parity is tested in client_launch_args_test.go (AC13).
// effectiveToolsAndSources computes the allowed tools and setting sources after
// applying the Skills coupling, mirroring upstream _apply_skills_defaults
// (subprocess_cli.py:186-219). When Skills is set it injects the skill tools
// into a copy of AllowedTools (the bare "Skill" for [AllSkills], otherwise
// "Skill(name)" per entry, skipping duplicates) and, if SettingSources is
// unset, defaults the sources to user+project so the CLI discovers installed
// skills. When Skills is nil/empty it is a no-op: tools and sources pass
// through unchanged. The receiver is never mutated.
func effectiveToolsAndSources(opts *Options) (tools []string, sources []SettingSource) {
	tools = append([]string(nil), opts.AllowedTools...)
	sources = opts.SettingSources

	if len(opts.Skills) == 0 {
		return tools, sources
	}

	has := func(t string) bool {
		for _, x := range tools {
			if x == t {
				return true
			}
		}
		return false
	}
	if len(opts.Skills) == 1 && opts.Skills[0] == skillsAll {
		if !has("Skill") {
			tools = append(tools, "Skill")
		}
	} else {
		for _, name := range opts.Skills {
			pattern := "Skill(" + name + ")"
			if !has(pattern) {
				tools = append(tools, pattern)
			}
		}
	}

	if len(sources) == 0 {
		sources = []SettingSource{SettingSourceUser, SettingSourceProject}
	}
	return tools, sources
}

func buildLaunchArgs(cliPath string, opts *Options, resumeSessionID string) ([]string, error) {
	args := []string{cliPath}

	if opts == nil {
		opts = &Options{}
	}

	// Output format — always stream-json for SDK use unless overridden.
	outputFmt := "stream-json"
	if opts.OutputFormat != "" {
		outputFmt = opts.OutputFormat
	}
	args = append(args, "--output-format", outputFmt)

	// Input format — always stream-json for SDK use unless overridden.
	// Upstream subprocess_cli.py always sends --input-format stream-json.
	inputFmt := "stream-json"
	if opts.InputFormat != "" {
		inputFmt = opts.InputFormat
	}
	args = append(args, "--input-format", inputFmt)

	// Verbose mode — emitted unconditionally to match upstream
	// subprocess_cli.py:225 (the CLI always runs verbose for SDK streaming).
	args = append(args, "--verbose")

	// Include partial messages.
	if opts.IncludePartialMessages {
		args = append(args, "--include-partial-messages")
	}

	// Model selection.
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	// System prompt — always emitted, even when empty, matching upstream
	// subprocess_cli.py:228 (which sends --system-prompt "" when unset). The
	// SystemPrompt Preset/File variants are added in a later milestone.
	args = append(args, "--system-prompt", opts.SystemPrompt)

	// Allowed tools and setting sources are coupled through Skills (see
	// effectiveToolsAndSources). Compute both once; emit allowedTools here as a
	// single comma-joined flag (subprocess_cli.py:257) and setting-sources
	// below.
	effTools, effSources := effectiveToolsAndSources(opts)
	if len(effTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(effTools, ","))
	}

	// Max turns per session.
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(opts.MaxTurns))
	}

	// Permission mode.
	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", opts.PermissionMode)
	}

	// API key helper binary.
	if opts.APIKeyHelper != "" {
		args = append(args, "--api-key-helper", opts.APIKeyHelper)
	}

	// Max spend budget.
	if opts.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(opts.MaxBudgetUSD, 'f', -1, 64))
	}

	// Disallowed tools — single comma-joined flag (subprocess_cli.py:266).
	if len(opts.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(opts.DisallowedTools, ","))
	}

	// Fallback model (subprocess_cli.py:275).
	if opts.FallbackModel != "" {
		args = append(args, "--fallback-model", opts.FallbackModel)
	}

	// Beta feature flags — single comma-joined flag (subprocess_cli.py:278).
	if len(opts.Betas) > 0 {
		args = append(args, "--betas", strings.Join(opts.Betas, ","))
	}

	// Permission-prompt MCP tool name (subprocess_cli.py:281).
	if opts.PermissionPromptToolName != "" {
		args = append(args, "--permission-prompt-tool", opts.PermissionPromptToolName)
	}

	// Continue the most recent conversation (subprocess_cli.py:289).
	if opts.ContinueConversation {
		args = append(args, "--continue")
	}

	// Explicit session ID for a new session (subprocess_cli.py:295).
	if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}

	// Settings JSON or file path (subprocess_cli.py:300). Sandbox merging into
	// settings is added with the Sandbox type group.
	if opts.Settings != "" {
		args = append(args, "--settings", opts.Settings)
	}

	// Additional accessible directories — one flag per entry (subprocess_cli.py:305).
	for _, dir := range opts.AddDirs {
		args = append(args, "--add-dir", dir)
	}

	// Stream hook lifecycle events as messages (subprocess_cli.py:338).
	if opts.IncludeHookEvents {
		args = append(args, "--include-hook-events")
	}

	// Fork into a new session ID when resuming (subprocess_cli.py:344).
	if opts.ForkSession {
		args = append(args, "--fork-session")
	}

	// MCP servers — encoded as a single --mcp-config JSON object keyed by
	// server name, followed by --strict-mcp-config when requested. Mirrors
	// upstream subprocess_cli.py:307 (json.dumps({"mcpServers": servers_for_cli}))
	// and :340 (--strict-mcp-config). In-process servers contribute
	// {"type":"sdk","name":<name>}; the CLI then routes their tool calls back
	// over the control protocol.
	if len(opts.MCPServers) > 0 {
		servers := make(map[string]any, len(opts.MCPServers))
		for _, srv := range opts.MCPServers {
			servers[srv.Name()] = srv.configForCLI()
		}
		cfg, err := json.Marshal(map[string]any{"mcpServers": servers})
		if err != nil {
			return nil, &CLIConnectionError{Message: "marshal --mcp-config: " + err.Error()}
		}
		args = append(args, "--mcp-config", string(cfg))
	}
	if opts.StrictMCPConfig {
		args = append(args, "--strict-mcp-config")
	}

	// Plugins — local plugins use --plugin-dir (one flag per plugin).
	// Mirrors upstream Python subprocess_cli.py:
	//   if plugin["type"] == "local": cmd.extend(["--plugin-dir", plugin["path"]])
	for _, p := range opts.Plugins {
		if p.Path != "" {
			args = append(args, "--plugin-dir", p.Path)
		}
	}

	// Setting sources — comma-joined list of literals (user|project|local)
	// passed as a single --setting-sources= flag (subprocess_cli.py:353). The
	// list is the Skills-adjusted effSources from effectiveToolsAndSources: when
	// Skills is set and the user left SettingSources unset, it defaults to
	// user+project. Empty entries are skipped so a zero-value SettingSource does
	// not produce a trailing empty token.
	if len(effSources) > 0 {
		parts := make([]string, 0, len(effSources))
		for _, ss := range effSources {
			if ss != "" {
				parts = append(parts, string(ss))
			}
		}
		if len(parts) > 0 {
			args = append(args, "--setting-sources="+strings.Join(parts, ","))
		}
	}

	// Extra args — arbitrary additional CLI flags (subprocess_cli.py:364). A nil
	// value emits a bare "--key"; a non-nil value emits "--key <value>". Keys
	// are emitted in sorted order so the argument slice is deterministic (Go
	// map iteration is randomized, unlike the insertion-ordered Python dict).
	if len(opts.ExtraArgs) > 0 {
		keys := make([]string, 0, len(opts.ExtraArgs))
		for k := range opts.ExtraArgs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if v := opts.ExtraArgs[k]; v != nil {
				args = append(args, "--"+k, *v)
			} else {
				args = append(args, "--"+k)
			}
		}
	}

	// Resume session (subprocess_cli.py:292). The Fork-driven resumeSessionID
	// parameter takes precedence over opts.Resume: a forked child must resume
	// the branched session regardless of the user's option. Falls back to
	// opts.Resume for an explicitly-resumed (non-forked) client.
	resume := resumeSessionID
	if resume == "" {
		resume = opts.Resume
	}
	if resume != "" {
		args = append(args, "--resume", resume)
	}

	return args, nil
}
