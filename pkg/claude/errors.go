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
	"fmt"
)

// Error is the base interface implemented by all pkg/claude SDK errors.
// Callers can use errors.As to unwrap to a concrete type.
type Error interface {
	error
	isCLIError()
}

// CLINotFoundError is returned when the claude CLI binary cannot be located.
//
// CLI discovery order: Options.CLIPath → exec.LookPath("claude") →
// ~/.claude/local/claude → /opt/homebrew/bin/claude → /usr/local/bin/claude.
type CLINotFoundError struct {
	// SearchPaths lists every path that was tried before giving up.
	SearchPaths []string
}

func (e *CLINotFoundError) Error() string {
	return fmt.Sprintf("claude CLI not found; searched: %v", e.SearchPaths)
}

func (*CLINotFoundError) isCLIError() {}

// CLIConnectionError is returned when a connection to the claude CLI subprocess
// cannot be established or is lost unexpectedly.
type CLIConnectionError struct {
	// Message describes the connection failure.
	Message string
}

func (e *CLIConnectionError) Error() string {
	return fmt.Sprintf("claude CLI connection error: %s", e.Message)
}

func (*CLIConnectionError) isCLIError() {}

// ProcessError is returned when the claude CLI subprocess exits with a
// non-zero status code.
type ProcessError struct {
	// ExitCode is the subprocess exit code.
	ExitCode int

	// StderrTail contains the last ≤ 40 lines of subprocess stderr output,
	// captured by a drainStderr goroutine mirroring pkg/codex/client.go:737.
	StderrTail string
}

func (e *ProcessError) Error() string {
	if e.StderrTail != "" {
		return fmt.Sprintf("claude CLI exited with code %d: %s", e.ExitCode, e.StderrTail)
	}
	return fmt.Sprintf("claude CLI exited with code %d", e.ExitCode)
}

func (*ProcessError) isCLIError() {}

// CLIJSONDecodeError is returned when a line from the claude CLI subprocess
// stdout cannot be decoded as valid stream-JSON.
type CLIJSONDecodeError struct {
	// Line is the raw bytes that failed to decode.
	Line []byte

	// Offset is the byte offset within the stream where parsing failed.
	Offset int64
}

func (e *CLIJSONDecodeError) Error() string {
	return fmt.Sprintf("claude CLI JSON decode error at offset %d: %q", e.Offset, e.Line)
}

func (*CLIJSONDecodeError) isCLIError() {}
