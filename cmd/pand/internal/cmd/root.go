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
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	pandVersion     = "0.1.0"
	upstreamVersion = "0.18.4"
	upstreamCommit  = "a31cf5d7866c02fe00cc18c420ae823ecdc352bc"
)

// Options configures the pand command tree.
type Options struct {
	Backend Backend
	Env     map[string]string
	Cwd     string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

// Execute runs the pand command using the process environment and stdio.
func Execute(ctx context.Context) error {
	return NewRootCommand(Options{}).ExecuteContext(ctx)
}

// NewRootCommand builds the pand command tree.
func NewRootCommand(opts Options) *cobra.Command {
	backend := opts.Backend
	if backend == nil {
		backend = SystemBackend{}
	}
	env := opts.Env
	if env == nil {
		env = environMap(os.Environ())
	}

	root := &cobra.Command{
		Use:                "pand",
		Short:              "Go front door for the OMX workflow runtime",
		Long:               rootLongDescription(),
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
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
	root.SetIn(readerOrDefault(opts.Stdin, os.Stdin))
	root.SetOut(writerOrDefault(opts.Stdout, os.Stdout))
	root.SetErr(writerOrDefault(opts.Stderr, os.Stderr))
	root.SetHelpCommand(newHelpCommand(root))
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(newVersionCommand())
	root.AddCommand(newStateCommand(env, opts.Cwd))
	root.AddCommand(newStatusCommand(env, opts.Cwd))
	root.AddCommand(newCancelCommand(env, opts.Cwd))
	root.AddCommand(newMCPParityCommand("notepad", notepadDescriptor(env, opts.Cwd)))
	root.AddCommand(newMCPParityCommand("project-memory", projectMemoryDescriptor(env, opts.Cwd)))
	root.AddCommand(newMCPParityCommand("trace", traceDescriptor(env, opts.Cwd)))
	root.AddCommand(newMCPParityCommand("wiki", wikiDescriptor(env, opts.Cwd)))

	for _, spec := range delegatedCommandSpecs() {
		root.AddCommand(newDelegatedCommand(spec, backend, env, opts))
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

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "pand %s (omx compatibility %s %s)\n", pandVersion, upstreamVersion, upstreamCommit)
			return err
		},
	}
}

type delegatedCommandSpec struct {
	Name  string
	Short string
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

func newDelegatedCommand(spec delegatedCommandSpec, backend Backend, env map[string]string, opts Options) *cobra.Command {
	return &cobra.Command{
		Use:                spec.Name,
		Short:              spec.Short,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			argv := append([]string{spec.Name}, args...)
			return backend.Run(cmd.Context(), BackendRequest{
				Args:   argv,
				Env:    env,
				Dir:    opts.Cwd,
				Stdin:  inputFor(cmd, opts.Stdin),
				Stdout: cmd.OutOrStdout(),
				Stderr: cmd.ErrOrStderr(),
			})
		},
	}
}

func inputFor(cmd *cobra.Command, fallback io.Reader) io.Reader {
	if cmd.InOrStdin() != nil {
		return cmd.InOrStdin()
	}
	return readerOrDefault(fallback, os.Stdin)
}

func readerOrDefault(got, fallback io.Reader) io.Reader {
	if got != nil {
		return got
	}
	return fallback
}

func writerOrDefault(got, fallback io.Writer) io.Writer {
	if got != nil {
		return got
	}
	return fallback
}

func rootLongDescription() string {
	var b strings.Builder
	b.WriteString("pand ports the OMX CLI control plane into Go. Native commands own state, status, cancel, notepad, project-memory, trace, and wiki surfaces; platform-sized upstream runtime commands delegate through the compatibility backend.\n\n")
	b.WriteString("Upstream reference: oh-my-codex ")
	b.WriteString(upstreamVersion)
	b.WriteString(" @ ")
	b.WriteString(upstreamCommit)
	b.WriteByte('.')
	return b.String()
}
