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
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zchee/pandaemonium/cmd/agu/env"
)

// Options represents the configuration for the agu command,
// allowing for injection of environment variables, working directory, and stdio streams.
type Options struct {
	Env    map[string]string
	Cwd    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Execute runs the agu command using the process environment and stdio.
func Execute(ctx context.Context) error {
	return NewRootCommand(Options{}).ExecuteContext(ctx)
}

// NewRootCommand builds the agu command tree.
func NewRootCommand(opts Options) *cobra.Command {
	cwd := opts.Cwd
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			panic(fmt.Errorf("get current working directory: %w", err))
		}
		cwd = wd
	}

	envMap := opts.Env
	if envMap == nil {
		envMap = environMap(os.Environ())
	}

	loadConfig := env.ConfigLoader(func(ctx context.Context) *env.Config {
		return env.ProcessConfigMap(ctx, envMap)
	})

	c := &cobra.Command{
		Use:   "agu",
		Short: "Go frontend for the Agent Gu workflow runtime",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isRootHelpRequest(args) {
				return cmd.Help()
			}
			return nil
		},
	}
	c.SetIn(cmp.Or(opts.Stdin, io.Reader(os.Stdin)))
	c.SetOut(cmp.Or(opts.Stdout, io.Writer(os.Stdout)))
	c.SetErr(cmp.Or(opts.Stderr, io.Writer(os.Stderr)))
	c.SetHelpCommand(newHelpCommand(c))

	c.AddCommand(newAPICommand(loadConfig))

	return c
}

func isRootHelpRequest(args []string) bool {
	switch args[0] {
	case "--help", "-h", "help":
		return true
	default:
		return false
	}
}

func newHelpCommand(c *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Show help for agu or a subcommand",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := c
			if len(args) > 0 {
				found, _, err := c.Find(args)
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

func environMap(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, env := range environ {
		key, value, ok := strings.Cut(env, "=")
		if !ok {
			out[env] = ""
			continue
		}
		out[key] = value
	}
	return out
}
