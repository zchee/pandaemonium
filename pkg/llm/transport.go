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
)

// Transport is a bidirectional newline-delimited JSON message transport
// between an SDK client and its CLI/server process.
//
// WriteJSON writes one newline-terminated JSON payload; ReadJSON returns the
// next payload and io.EOF when the stream ends cleanly. Implementations rely
// on the owning client to serialize WriteJSON calls (write mutex or gate),
// and ReadJSON is called only from the single read-loop goroutine.
type Transport interface {
	io.Closer
	WriteJSON(ctx context.Context, data []byte) error
	ReadJSON(ctx context.Context) ([]byte, error)
}

// errStdioTransportClosed is the fallback closed-stream error used when a
// StdioTransport is built without a ClosedErr.
var errStdioTransportClosed = errors.New("llm: stdio transport closed")

// StdioTransport is a newline-delimited JSON transport over a subprocess
// stdin/stdout pair.
//
// Concurrent WriteJSON serialization is the owning client's responsibility
// (write mutex or write gate); ReadJSON must be called only from the single
// read-loop goroutine.
type StdioTransport struct {
	// Stdin is the subprocess stdin pipe written by WriteJSON and closed by
	// Close.
	Stdin io.WriteCloser

	// Stdout is the buffered subprocess stdout read by ReadJSON.
	Stdout *bufio.Reader

	// ClosedErr constructs the package-specific error returned when the
	// stream is closed or absent. A nil ClosedErr falls back to a generic
	// llm error.
	ClosedErr ErrorFunc

	// WrapWriteErr wraps stdin write errors into the owning package's error
	// type. A nil WrapWriteErr returns the underlying error unchanged.
	WrapWriteErr WrapErrorFunc
}

var _ Transport = (*StdioTransport)(nil)

// Close closes the subprocess stdin pipe, signaling EOF to the child process.
func (t *StdioTransport) Close() error {
	if t.Stdin == nil {
		return nil
	}
	return t.Stdin.Close()
}

// WriteJSON writes p followed by a newline to the subprocess stdin. The data
// is cloned so the caller may reuse the slice immediately.
func (t *StdioTransport) WriteJSON(_ context.Context, p []byte) error {
	return WriteJSONLine(t.Stdin, p, t.closedErrFunc(), t.wrapWriteErrFunc())
}

// ReadJSON reads the next newline-terminated JSON payload from the subprocess
// stdout, returning io.EOF when the stream ends cleanly.
//
// The read goroutine orphaned when ctx is cancelled exits naturally once the
// raw stdout pipe is closed, which makes its blocked read return an error.
func (t *StdioTransport) ReadJSON(ctx context.Context) ([]byte, error) {
	return ReadJSONLine(ctx, t.Stdout, t.closedErrFunc())
}

// closedErrFunc returns ClosedErr or the generic fallback.
func (t *StdioTransport) closedErrFunc() ErrorFunc {
	if t.ClosedErr != nil {
		return t.ClosedErr
	}
	return func() error { return errStdioTransportClosed }
}

// wrapWriteErrFunc returns WrapWriteErr or the identity fallback.
func (t *StdioTransport) wrapWriteErrFunc() WrapErrorFunc {
	if t.WrapWriteErr != nil {
		return t.WrapWriteErr
	}
	return func(err error) error { return err }
}
