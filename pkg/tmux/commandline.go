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

package tmux

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Command is a canonical tmux command token such as "display-message".
type Command string

// Arg is a single tmux command argument.
type Arg struct {
	value string
	raw   bool
}

// StringArg returns a normal argument rendered with tmux-safe quoting.
func StringArg(value string) Arg { return Arg{value: value} }

// RawArg returns an explicit raw tmux syntax fragment.
func RawArg(value string) Arg { return Arg{value: value, raw: true} }

// Args converts values to normal tmux command arguments.
func Args(values ...string) []Arg {
	args := make([]Arg, 0, len(values))
	for _, value := range values {
		args = append(args, StringArg(value))
	}
	return args
}

// CommandLine is one rendered tmux command plus arguments.
type CommandLine struct {
	Command Command
	Args    []Arg
}

// NewCommandLine returns a command line with a defensive copy of args.
func NewCommandLine(command Command, args ...Arg) CommandLine {
	return CommandLine{Command: command, Args: slices.Clone(args)}
}

// String renders l as one newline-free tmux command line.
func (l CommandLine) String() (string, error) {
	command := string(l.Command)
	if err := validateCommandToken(command); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(command)
	for i, arg := range l.Args {
		rendered, err := renderArg(arg)
		if err != nil {
			return "", fmt.Errorf("argument %d: %w", i, err)
		}
		b.WriteByte(' ')
		b.WriteString(rendered)
	}
	return b.String(), nil
}

func validateCommandToken(command string) error {
	if command == "" {
		return fmt.Errorf("tmux: command must not be empty")
	}
	if !utf8.ValidString(command) {
		return fmt.Errorf("tmux: command %q must be valid UTF-8", command)
	}
	if strings.TrimSpace(command) != command || strings.ContainsAny(command, " \t\r\n") {
		return fmt.Errorf("tmux: command %q must be a plain token", command)
	}
	return nil
}

func renderArg(arg Arg) (string, error) {
	if err := validateArgument(arg.value); err != nil {
		return "", err
	}
	if arg.raw {
		if strings.TrimSpace(arg.value) == "" {
			return "", fmt.Errorf("tmux: raw argument must not be empty")
		}
		return arg.value, nil
	}
	return quoteArg(arg.value), nil
}

func validateArgument(value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("tmux: argument must be valid UTF-8")
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("tmux: argument %q contains a newline", value)
	}
	return nil
}

func quoteArg(value string) string {
	if value == "" {
		return "''"
	}
	if isBareArg(value) {
		return value
	}
	if !strings.Contains(value, "'") {
		return "'" + value + "'"
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

func isBareArg(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("_@%+=:,./-", r):
		default:
			return false
		}
	}
	return true
}

func validateRawLine(line string) error {
	if strings.TrimSpace(line) == "" {
		return fmt.Errorf("tmux: command line must not be empty")
	}
	if !utf8.ValidString(line) {
		return fmt.Errorf("tmux: command line must be valid UTF-8")
	}
	if strings.ContainsAny(line, "\r\n") {
		return fmt.Errorf("tmux: command line contains a newline")
	}
	return nil
}
