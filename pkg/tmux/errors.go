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
	"errors"
	"fmt"
	"strings"
)

// ErrClosed is returned when an operation is attempted after client shutdown
// has begun.
var ErrClosed = errors.New("tmux: client closed")

// CommandError reports a tmux command that completed with a `%error` marker.
type CommandError struct {
	Line     string
	Response Response
}

// Error returns a concise command failure message.
func (e *CommandError) Error() string {
	if e == nil {
		return "tmux: command error"
	}
	output := strings.Join(e.Response.Lines, "\n")
	if output == "" {
		return fmt.Sprintf("tmux: command %q failed", e.Line)
	}
	return fmt.Sprintf("tmux: command %q failed: %s", e.Line, output)
}

// ProtocolError reports malformed tmux control-mode data.
type ProtocolError struct {
	Line string
	Err  error
}

// Error returns a concise protocol failure message.
func (e *ProtocolError) Error() string {
	if e == nil {
		return "tmux: protocol error"
	}
	if e.Line == "" {
		return fmt.Sprintf("tmux: protocol error: %v", e.Err)
	}
	return fmt.Sprintf("tmux: protocol error on %q: %v", e.Line, e.Err)
}

// Unwrap returns the underlying protocol error.
func (e *ProtocolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ExitError reports a `%exit` notification from the tmux control client.
type ExitError struct {
	Reason string
}

// Error returns the tmux exit reason.
func (e *ExitError) Error() string {
	if e == nil || e.Reason == "" {
		return "tmux: control client exited"
	}
	return "tmux: control client exited: " + e.Reason
}
