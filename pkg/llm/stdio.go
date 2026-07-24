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

package llm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"slices"
)

// ErrorFunc returns a package-specific transport error.
type ErrorFunc func() error

// WrapErrorFunc converts an underlying transport write error into a
// package-specific error value.
type WrapErrorFunc func(error) error

// WriteJSONLine writes data followed by a newline to stdin.
//
// The data slice is cloned before appending the newline so callers may reuse it
// immediately after the call returns.
func WriteJSONLine(stdin io.Writer, data []byte, closedErr ErrorFunc, writeErr WrapErrorFunc) error {
	if stdin == nil {
		return closedErr()
	}
	line := append(slices.Clone(data), '\n')
	if _, err := stdin.Write(line); err != nil {
		return writeErr(err)
	}
	return nil
}

// readJSONLine reads the next newline-terminated JSON line from stdout.
//
// EOF is normalized to io.EOF so SDK packages can share their clean shutdown
// handling while still supplying package-specific closed-stream errors.
func readJSONLine(stdout *bufio.Reader) ([]byte, error) {
	line, err := stdout.ReadBytes('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	return line, nil
}

// ReadJSONLine reads one JSON line and returns ctx.Err if ctx is
// cancelled before the line is available.
//
// A context that is already cancelled on entry returns ctx.Err without
// consuming a line; both channels being ready would otherwise let select
// pick the read result over the cancellation nondeterministically.
//
// The read goroutine exits naturally when the underlying reader is closed.
func ReadJSONLine(ctx context.Context, stdout *bufio.Reader, closedErr ErrorFunc) ([]byte, error) {
	if stdout == nil {
		return nil, closedErr()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := readJSONLine(stdout)
		done <- result{data: line, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		return r.data, r.err
	}
}
