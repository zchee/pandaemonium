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

package cmd

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var _ cobra.Command

type Options struct {
	Env    map[string]string
	Cwd    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Execute runs the pand command using the process environment and stdio.
func Execute(ctx context.Context) error {
	return NewRootCommand(Options{}).ExecuteContext(ctx)
}

// NewRootCommand builds the pand command tree.
func NewRootCommand(opts Options) *cobra.Command {
	env := opts.Env
	if env == nil {
		env = environMap(os.Environ())
	}

	root := &cobra.Command{
		Use:   "pand",
		Short: "Go front door for the OMX workflow runtime",
		RunE: func(cmd *cobra.Command, args []string) error {
			if isRootHelpRequest(args) {
				return cmd.Help()
			}
			return backend.Run(cmd.Context(), BackendRequest{
				Args:   args,
				Env:    env,
				Dir:    opts.Cwd,
				Stdin:  inputFor(cmd, opts.Stdin),
				Stdout: cmd.OutOrStdout(),
				Stderr: cmd.ErrOrStderr(),
			})
		},
	}

	return root
}

func newHelpCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Show help for pand or a subcommand",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := root
			if len(args) > 0 {
				found, _, err := root.Find(args)
				if err == nil && found != nil {
					target = found
				}
			}
			target.SetOut(cmd.OutOrStdout())
			target.SetErr(cmd.ErrOrStderr())
			return target.Help()
		},
	}
}

func isRootHelpRequest(args []string) bool {
	if len(args) != 1 {
		return false
	}
	return args[0] == "--help" || args[0] == "-h" || args[0] == "help"
}

func environMap(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			out[entry] = ""
			continue
		}
		out[key] = value
	}
	return out
}

func delegatedCommandSpecs() []delegatedCommandSpec {
	return []delegatedCommandSpec{
		{Name: "launch", Short: "Launch Codex CLI through the upstream OMX runtime"},
		{Name: "exec", Short: "Run codex exec through the upstream OMX runtime"},
		{Name: "imagegen", Short: "Delegate image generation continuation commands"},
		{Name: "setup", Short: "Delegate OMX setup/install operations"},
		{Name: "update", Short: "Delegate global OMX update operations"},
		{Name: "list", Short: "Delegate packaged skill and agent listing"},
		{Name: "agents", Short: "Delegate native agent TOML management"},
		{Name: "agents-init", Short: "Delegate AGENTS.md bootstrap"},
		{Name: "deepinit", Short: "Delegate AGENTS.md bootstrap alias"},
		{Name: "uninstall", Short: "Delegate OMX uninstall operations"},
		{Name: "doctor", Short: "Delegate OMX diagnostics"},
		{Name: "cleanup", Short: "Delegate OMX process and temp cleanup"},
		{Name: "auth", Short: "Delegate Codex auth-slot operations"},
		{Name: "ask", Short: "Delegate local advisor CLI workflow"},
		{Name: "question", Short: "Delegate OMX question UI/runtime"},
		{Name: "autoresearch", Short: "Delegate deprecated autoresearch compatibility"},
		{Name: "autoresearch-goal", Short: "Delegate research goal workflow"},
		{Name: "explore", Short: "Delegate deprecated explore compatibility"},
		{Name: "api", Short: "Delegate omx-api sidecar commands"},
		{Name: "sparkshell", Short: "Delegate sparkshell sidecar commands"},
		{Name: "team", Short: "Delegate team/tmux orchestration"},
		{Name: "session", Short: "Delegate session history search"},
		{Name: "resume", Short: "Delegate Codex session resume"},
		{Name: "ralph", Short: "Delegate Ralph workflow"},
		{Name: "ultragoal", Short: "Delegate Ultragoal workflow"},
		{Name: "performance-goal", Short: "Delegate performance-goal workflow"},
		{Name: "hud", Short: "Delegate HUD statusline UI"},
		{Name: "sidecar", Short: "Delegate sidecar visualization"},
		{Name: "mcp-serve", Short: "Delegate OMX MCP server targets"},
		{Name: "tmux-hook", Short: "Delegate tmux hook management"},
		{Name: "hooks", Short: "Delegate hook plugin management"},
		{Name: "reasoning", Short: "Delegate Codex reasoning config"},
		{Name: "adapt", Short: "Delegate adapter scaffolding"},
		{Name: "code-intel", Short: "Delegate LSP and ast-grep code intelligence"},
	}
}
