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
	"bufio"
	"context"
	"errors"
	"io"
	"slices"
)

// transport is the low-level read/write interface for the claude CLI subprocess.
//
// The interface is intentionally unexported so callers cannot bypass the
// race-safety discipline enforced by ClaudeSDKClient. Implementations must be
// safe for concurrent calls to WriteJSON; ReadJSON is called only from the
// single readLoop goroutine.
//
// Mirrors pkg/codex/client_transport.go:35-39.
type transport interface {
	io.Closer

	// WriteJSON writes a newline-terminated JSON payload to the subprocess stdin.
	WriteJSON(ctx context.Context, p []byte) error

	// ReadJSON reads the next newline-terminated JSON payload from the subprocess
	// stdout. It returns io.EOF when the stream ends cleanly.
	ReadJSON(ctx context.Context) ([]byte, error)
}

// stdioTransport is a transport backed by a subprocess stdin/stdout pair.
//
// It is created by ClaudeSDKClient.start after the subprocess is launched.
// Concurrent safety for WriteJSON is provided by ClaudeSDKClient.writeMu, not
// by an internal mutex — the same discipline used by pkg/codex/client_transport.go:41.
type stdioTransport struct {
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

var _ transport = (*stdioTransport)(nil)

// Close closes the subprocess stdin pipe, signalling EOF to the CLI.
func (t *stdioTransport) Close() error {
	if t.stdin == nil {
		return nil
	}
	return t.stdin.Close()
}

// WriteJSON writes p followed by a newline to the subprocess stdin.
// The data is cloned so the caller may reuse the slice immediately.
func (t *stdioTransport) WriteJSON(_ context.Context, p []byte) error {
	if t.stdin == nil {
		return &CLIConnectionError{Message: "CLI is not running"}
	}
	line := append(slices.Clone(p), '\n')
	if _, err := t.stdin.Write(line); err != nil {
		return &CLIConnectionError{Message: err.Error()}
	}
	return nil
}

// ReadJSON reads the next newline-terminated line from the subprocess stdout.
// Returns io.EOF when the subprocess closes its stdout.
func (t *stdioTransport) ReadJSON(_ context.Context) ([]byte, error) {
	if t.stdout == nil {
		return nil, &CLIConnectionError{Message: "CLI is not running"}
	}
	line, err := t.stdout.ReadBytes('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	return line, nil
}
