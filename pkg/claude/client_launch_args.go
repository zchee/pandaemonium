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
		args = append(args, "--max-budget-usd", strconv.FormatFloat(opts.MaxBudgetUSD, 'f', -1, 64))
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
	// passed as a single --setting-sources= flag. Mirrors upstream
	// subprocess_cli.py:353. Empty entries are skipped so a zero-value
	// SettingSource does not produce a trailing empty token.
	//
	// Upstream defaults setting_sources to ["user","project"] only when
	// Options.Skills is set (subprocess_cli.py:205-217); that coupling is
	// deferred until the Skills option lands, so an unset SettingSources emits
	// nothing here.
	if len(opts.SettingSources) > 0 {
		parts := make([]string, 0, len(opts.SettingSources))
		for _, ss := range opts.SettingSources {
			if ss != "" {
				parts = append(parts, string(ss))
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

	return args, nil
}
