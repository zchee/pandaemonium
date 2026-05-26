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
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type mcpTool struct {
	Name        string
	Description string
}

type mcpDescriptor struct {
	CommandName string
	Title       string
	Tools       []mcpTool
	Aliases     map[string]string
	Handle      func(map[string]any) (any, bool)
}

func newMCPParityCommand(name string, descriptor mcpDescriptor) *cobra.Command {
	return &cobra.Command{
		Use:                name + " <tool>",
		Short:              descriptor.Title,
		Long:               mcpCommandHelp(descriptor),
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
				return cmd.Help()
			}
			input, compact, err := parseCommandInput(args[1:])
			if err != nil {
				return err
			}
			toolName := resolveMCPToolName(descriptor, args[0])
			if toolName == "" {
				return fmt.Errorf("unknown %s tool: %s", name, args[0])
			}
			input["tool"] = toolName
			payload, isError := descriptor.Handle(input)
			out := cmd.OutOrStdout()
			if isError {
				out = cmd.ErrOrStderr()
			}
			if err := writeJSON(out, payload, compact); err != nil {
				return err
			}
			if isError {
				return fmt.Errorf("%s %s failed", name, args[0])
			}
			return nil
		},
	}
}

func resolveMCPToolName(descriptor mcpDescriptor, requested string) string {
	if alias, ok := descriptor.Aliases[requested]; ok {
		return alias
	}
	for _, tool := range descriptor.Tools {
		if tool.Name == requested {
			return requested
		}
	}
	return ""
}

func mcpCommandHelp(descriptor mcpDescriptor) string {
	var b strings.Builder
	b.WriteString(descriptor.Title)
	b.WriteString("\n\nTools:\n")
	tools := append([]mcpTool(nil), descriptor.Tools...)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	for _, tool := range tools {
		fmt.Fprintf(&b, "  %s\t%s\n", tool.Name, tool.Description)
	}
	if len(descriptor.Aliases) > 0 {
		b.WriteString("\nAliases:\n")
		aliases := make([]string, 0, len(descriptor.Aliases))
		for alias := range descriptor.Aliases {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			fmt.Fprintf(&b, "  %s -> %s\n", alias, descriptor.Aliases[alias])
		}
	}
	b.WriteString("\nInput: pass JSON object arguments with --input '{...}'. Add --json for compact output.")
	return b.String()
}
