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
	"context"
	"io"
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
