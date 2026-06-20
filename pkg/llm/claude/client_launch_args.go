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
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/go-json-experiment/json"
)

// effectiveToolsAndSources computes the allowed tools and setting sources after
// applying the Skills coupling, mirroring upstream _apply_skills_defaults
// (subprocess_cli.py:186-219). When Skills is set it injects the skill tools
// into a copy of AllowedTools (the bare "Skill" for [AllSkills], otherwise
// "Skill(name)" per entry, skipping duplicates) and, if SettingSources is
// unset, defaults the sources to user+project so the CLI discovers installed
// skills. When Skills is nil/empty it is a no-op: tools and sources pass
// through unchanged. The receiver is never mutated.
func effectiveToolsAndSources(opts *Options) (tools []string, sources []SettingSource) {
	tools = slices.Clone(opts.AllowedTools)
	// Copy SettingSources too: the Skills-default branch below may append to
	// `sources`, which would otherwise mutate the caller's
	// opts.SettingSources slice if it has spare capacity. This guards against
	// any future code path that appends to `sources`, not only Skills.
	sources = slices.Clone(opts.SettingSources)

	if len(opts.Skills) == 0 {
		return tools, sources
	}

	has := func(t string) bool {
		return slices.Contains(tools, t)
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

// buildLaunchArgs constructs the argument slice for launching the claude CLI
// subprocess, suitable for exec.Command(args[0], args[1:]...).
//
// cliPath is the resolved binary path (from discoverCLI); opts configures the
// CLI flags (nil uses defaults); resumeSessionID, when non-empty, adds
// --resume <id> so the subprocess resumes that session (used by
// [ClaudeSDKClient.Fork], taking precedence over [Options.Resume]).
//
// Agents and prompts are NOT passed as flags: agent definitions travel in the
// streaming initialize request and prompts are sent as stdin envelopes. The
// argument order mirrors upstream subprocess_cli.py and is asserted for
// round-trip parity in client_launch_args_test.go (AC13); the per-group builder
// helpers below preserve that order. An error is returned only when a JSON
// payload (--json-schema, --settings, --mcp-config) fails to marshal.
func buildLaunchArgs(cliPath string, opts *Options, resumeSessionID string) ([]string, error) {
	if opts == nil {
		opts = &Options{}
	}
	args := []string{cliPath}

	formatArgs, err := buildFormatArgs(opts)
	if err != nil {
		return nil, err
	}
	args = append(args, formatArgs...)

	args = append(args, buildSystemPromptArgs(opts)...)

	// Allowed tools and setting sources are coupled through Skills (see
	// effectiveToolsAndSources). Compute both once: tool args are emitted here,
	// the setting-sources flag near the end (preserving upstream order).
	effTools, effSources := effectiveToolsAndSources(opts)
	args = append(args, buildToolArgs(opts, effTools)...)

	args = append(args, buildLimitArgs(opts)...)

	settingsArgs, err := buildSettingsArgs(opts)
	if err != nil {
		return nil, err
	}
	args = append(args, settingsArgs...)

	args = append(args, buildSessionScopeArgs(opts)...)
	args = append(args, buildThinkingArgs(opts)...)

	mcpArgs, err := buildMCPArgs(opts)
	if err != nil {
		return nil, err
	}
	args = append(args, mcpArgs...)

	args = append(args, buildPluginArgs(opts)...)
	args = append(args, buildSettingSourcesArgs(effSources)...)
	args = append(args, buildExtraArgs(opts)...)
	args = append(args, buildResumeArgs(opts, resumeSessionID)...)

	return args, nil
}

// buildFormatArgs builds the output/input-format, structured-schema, verbosity,
// partial-message, and model flags. Output format defaults to stream-json and
// --verbose is emitted unconditionally for SDK streaming (subprocess_cli.py:225,
// 395-404). It errors only if the --json-schema payload fails to marshal.
func buildFormatArgs(opts *Options) ([]string, error) {
	outputFmt := "stream-json"
	if opts.OutputFormat != "" {
		outputFmt = opts.OutputFormat
	}
	args := []string{"--output-format", outputFmt}

	if opts.JSONSchema != nil {
		schemaJSON, err := json.Marshal(opts.JSONSchema)
		if err != nil {
			return nil, &CLIConnectionError{Message: "marshal --json-schema: " + err.Error()}
		}
		args = append(args, "--json-schema", string(schemaJSON))
	}

	inputFmt := "stream-json"
	if opts.InputFormat != "" {
		inputFmt = opts.InputFormat
	}
	// --verbose follows --input-format unconditionally; emit them together.
	args = append(args, "--input-format", inputFmt, "--verbose")

	if opts.IncludePartialMessages {
		args = append(args, "--include-partial-messages")
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	return args, nil
}

// buildSystemPromptArgs type-switches the SystemPromptSource sum type
// (subprocess_cli.py:227-238): nil → --system-prompt ""; Text → --system-prompt;
// File → --system-prompt-file; Preset → --append-system-prompt.
func buildSystemPromptArgs(opts *Options) []string {
	switch sp := opts.SystemPrompt.(type) {
	case nil:
		return []string{"--system-prompt", ""}
	case SystemPromptText:
		return []string{"--system-prompt", string(sp)}
	case SystemPromptFile:
		return []string{"--system-prompt-file", sp.Path}
	case SystemPromptPreset:
		return []string{"--append-system-prompt", sp.Append}
	default:
		return nil
	}
}

// buildToolArgs emits --allowedTools (the Skills-adjusted effTools) and the base
// --tools set (subprocess_cli.py:241-257). ToolsPreset wins over Tools; a
// non-nil empty Tools emits --tools "" while a nil Tools omits the flag.
func buildToolArgs(opts *Options, effTools []string) []string {
	var args []string
	if len(effTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(effTools, ","))
	}
	switch {
	case opts.ToolsPreset != "":
		args = append(args, "--tools", opts.ToolsPreset)
	case opts.Tools != nil:
		args = append(args, "--tools", strings.Join(opts.Tools, ","))
	}
	return args
}

// buildLimitArgs emits the turn/permission/budget/identity flags that sit
// between the tool flags and the settings value (subprocess_cli.py:259-295).
// TaskBudget is gated on nil so an explicit Total:0 forwards --task-budget 0.
func buildLimitArgs(opts *Options) []string {
	var args []string
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(opts.MaxTurns))
	}
	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", string(opts.PermissionMode))
	}
	if opts.APIKeyHelper != "" {
		args = append(args, "--api-key-helper", opts.APIKeyHelper)
	}
	if opts.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(opts.MaxBudgetUSD, 'f', -1, 64))
	}
	if len(opts.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(opts.DisallowedTools, ","))
	}
	if opts.TaskBudget != nil {
		args = append(args, "--task-budget", strconv.Itoa(opts.TaskBudget.Total))
	}
	if opts.FallbackModel != "" {
		args = append(args, "--fallback-model", opts.FallbackModel)
	}
	if len(opts.Betas) > 0 {
		args = append(args, "--betas", strings.Join(opts.Betas, ","))
	}
	if opts.PermissionPromptToolName != "" {
		args = append(args, "--permission-prompt-tool", opts.PermissionPromptToolName)
	}
	if opts.ContinueConversation {
		args = append(args, "--continue")
	}
	if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}
	return args
}

// buildSettingsArgs emits --settings with [Options.Sandbox] merged in, or
// nothing when the resolved value is empty (subprocess_cli.py:129-181).
func buildSettingsArgs(opts *Options) ([]string, error) {
	v, err := buildSettingsValue(opts)
	if err != nil {
		return nil, err
	}
	if v == "" {
		return nil, nil
	}
	return []string{"--settings", v}, nil
}

// buildSessionScopeArgs emits the accessible-directory, hook-event, and
// fork-session flags (subprocess_cli.py:305, 338, 344), one --add-dir per entry.
func buildSessionScopeArgs(opts *Options) []string {
	args := make([]string, 0, len(opts.AddDirs)*2+2)
	for _, dir := range opts.AddDirs {
		args = append(args, "--add-dir", dir)
	}
	if opts.IncludeHookEvents {
		args = append(args, "--include-hook-events")
	}
	if opts.ForkSession {
		args = append(args, "--fork-session")
	}
	return args
}

// buildThinkingArgs maps the Thinking sum type and Effort to flags
// (subprocess_cli.py:372-393). Thinking takes precedence over the deprecated
// MaxThinkingTokens: Adaptive → --thinking adaptive; Enabled →
// --max-thinking-tokens N (no --thinking flag); Disabled → --thinking disabled
// (no --thinking-display). The nil case falls through to MaxThinkingTokens.
func buildThinkingArgs(opts *Options) []string {
	var args []string
	switch t := opts.Thinking.(type) {
	case nil:
		if opts.MaxThinkingTokens > 0 {
			args = append(args, "--max-thinking-tokens", strconv.Itoa(opts.MaxThinkingTokens))
		}
	case ThinkingConfigAdaptive:
		args = append(args, "--thinking", "adaptive")
		if t.Display != "" {
			args = append(args, "--thinking-display", string(t.Display))
		}
	case ThinkingConfigEnabled:
		args = append(args, "--max-thinking-tokens", strconv.Itoa(t.BudgetTokens))
		if t.Display != "" {
			args = append(args, "--thinking-display", string(t.Display))
		}
	case ThinkingConfigDisabled:
		args = append(args, "--thinking", "disabled")
	}
	if opts.Effort != "" {
		args = append(args, "--effort", string(opts.Effort))
	}
	return args
}

// buildMCPArgs encodes MCP servers as a single --mcp-config JSON object keyed by
// server name, followed by --strict-mcp-config when requested
// (subprocess_cli.py:307, 340). In-process servers contribute
// {"type":"sdk","name":<name>}; the CLI routes their tool calls back over the
// control protocol. It errors only if the config payload fails to marshal.
func buildMCPArgs(opts *Options) ([]string, error) {
	var args []string
	if len(opts.MCPServers) > 0 {
		servers := make(map[string]any, len(opts.MCPServers))
		for _, srv := range opts.MCPServers {
			servers[srv.Name()] = srv.configForCLI()
		}
		cfg, err := json.Marshal(map[string]any{"mcpServers": servers}, json.Deterministic(true))
		if err != nil {
			return nil, &CLIConnectionError{Message: "marshal --mcp-config: " + err.Error()}
		}
		args = append(args, "--mcp-config", string(cfg))
	}
	if opts.StrictMCPConfig {
		args = append(args, "--strict-mcp-config")
	}
	return args, nil
}

// buildPluginArgs emits one --plugin-dir flag per local plugin path, mirroring
// upstream's `if plugin["type"] == "local"` handling.
func buildPluginArgs(opts *Options) []string {
	if len(opts.Plugins) == 0 {
		return nil
	}
	args := make([]string, 0, len(opts.Plugins)*2)
	for _, p := range opts.Plugins {
		if p.Path != "" {
			args = append(args, "--plugin-dir", p.Path)
		}
	}
	return args
}

// buildSettingSourcesArgs emits the comma-joined --setting-sources= flag from
// the Skills-adjusted effSources (subprocess_cli.py:353). Empty entries are
// skipped so a zero-value SettingSource does not produce a trailing empty token.
func buildSettingSourcesArgs(effSources []SettingSource) []string {
	if len(effSources) == 0 {
		return nil
	}
	parts := make([]string, 0, len(effSources))
	for _, ss := range effSources {
		if ss != "" {
			parts = append(parts, string(ss))
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return []string{"--setting-sources=" + strings.Join(parts, ",")}
}

// buildExtraArgs emits arbitrary additional CLI flags (subprocess_cli.py:364): a
// nil value emits a bare "--key"; a non-nil value emits "--key <value>". Keys
// are sorted so the argument slice is deterministic despite Go map iteration.
func buildExtraArgs(opts *Options) []string {
	if len(opts.ExtraArgs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(opts.ExtraArgs))
	for k := range opts.ExtraArgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		if v := opts.ExtraArgs[k]; v != nil {
			args = append(args, "--"+k, *v)
		} else {
			args = append(args, "--"+k)
		}
	}
	return args
}

// buildResumeArgs emits --resume (subprocess_cli.py:292). The Fork-driven
// resumeSessionID takes precedence over opts.Resume so a forked child resumes
// the branched session regardless of the user's option.
func buildResumeArgs(opts *Options, resumeSessionID string) []string {
	resume := resumeSessionID
	if resume == "" {
		resume = opts.Resume
	}
	if resume != "" {
		return []string{"--resume", resume}
	}
	return nil
}
