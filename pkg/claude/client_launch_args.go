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
	"strconv"
	"strings"
)

// buildLaunchArgs constructs the argument slice for launching the claude CLI
// subprocess. The resulting slice is suitable for exec.Command(args[0], args[1:]...).
//
// cliPath is the resolved binary path (from discoverCLI).
// prompt is the initial user prompt; passed via --print.
// opts configures additional CLI flags; nil opts uses defaults.
// resumeSessionID, when non-empty, adds --resume <id> so the CLI subprocess
// resumes from that session (used by [ClaudeSDKClient.Fork]).
//
// Note on Agents: AgentDefinition values in opts.Agents are NOT encoded as CLI
// flags. The claude CLI receives agent definitions via the streaming initialize
// request (same pattern as the TypeScript and Python SDKs). No --agents flag
// exists in the claude CLI.
//
// Mirrors the structure of pkg/codex/client.go:563 buildAppServerArgs.
// Round-trip parity is tested in client_launch_args_test.go (AC13).
func buildLaunchArgs(cliPath, prompt string, opts *Options, resumeSessionID string) []string {
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

	// Input format.
	if opts.InputFormat != "" {
		args = append(args, "--input-format", opts.InputFormat)
	}

	// Verbose mode.
	if opts.Verbose {
		args = append(args, "--verbose")
	}

	// Include partial messages.
	if opts.IncludePartialMessages {
		args = append(args, "--include-partial-messages")
	}

	// Model selection.
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	// System prompt.
	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}

	// Allowed tools.
	for _, tool := range opts.AllowedTools {
		args = append(args, "--allowedTools", tool)
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
		args = append(args, "--max-budget", strconv.FormatFloat(opts.MaxBudgetUSD, 'f', -1, 64))
	}

	// Plugins — local plugins use --plugin-dir (one flag per plugin).
	// Mirrors upstream Python subprocess_cli.py:
	//   if plugin["type"] == "local": cmd.extend(["--plugin-dir", plugin["path"]])
	for _, p := range opts.Plugins {
		if p.Path != "" {
			args = append(args, "--plugin-dir", p.Path)
		}
	}

	// Setting sources — comma-joined list passed as a single --setting-sources= flag.
	// Mirrors upstream Python subprocess_cli.py:
	//   cmd.append(f"--setting-sources={','.join(effective_setting_sources)}")
	if len(opts.SettingSources) > 0 {
		parts := make([]string, 0, len(opts.SettingSources))
		for _, ss := range opts.SettingSources {
			switch {
			case ss.Path != "":
				parts = append(parts, ss.Path)
			case ss.URL != "":
				parts = append(parts, ss.URL)
			}
		}
		if len(parts) > 0 {
			args = append(args, "--setting-sources="+strings.Join(parts, ","))
		}
	}

	// Resume session — used by Fork to replay a branched session in the new
	// subprocess. Mirrors upstream Python: cmd.extend(["--resume", options.resume]).
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	}

	// Prompt — passed as the --print flag so the CLI runs in non-interactive mode.
	if prompt != "" {
		args = append(args, "--print", prompt)
	}

	return args
}
