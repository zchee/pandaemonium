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
	"sync"
	"testing"

	"github.com/zchee/pandaemonium/pkg/claude/internal/fakecli"
)

// TestStdioTransport_WriteJSON verifies that WriteJSON appends a newline and
// writes the exact bytes to stdin.
func TestStdioTransport_WriteJSON(t *testing.T) {
	t.Parallel()

	t.Run("success: writes payload with trailing newline", func(t *testing.T) {
		t.Parallel()

		stdinR, stdinW := io.Pipe()
		t.Cleanup(func() { stdinR.Close(); stdinW.Close() })

		tr := &stdioTransport{stdin: stdinW}
		payload := []byte(`{"type":"ping"}`)
		want := append(payload, '\n')

		errCh := make(chan error, 1)
		go func() { errCh <- tr.WriteJSON(t.Context(), payload) }()

		got, err := bufio.NewReader(stdinR).ReadBytes('\n')
		if err != nil {
			t.Fatalf("read from stdin pipe: %v", err)
		}
		if err := <-errCh; err != nil {
			t.Fatalf("WriteJSON() = %v, want nil", err)
		}
		if string(got) != string(want) {
			t.Errorf("WriteJSON() wrote %q, want %q", got, want)
		}
	})

	t.Run("success: caller slice not aliased by transport", func(t *testing.T) {
		t.Parallel()

		stdinR, stdinW := io.Pipe()
		t.Cleanup(func() { stdinR.Close(); stdinW.Close() })

		tr := &stdioTransport{stdin: stdinW}
		payload := []byte(`{"k":"v"}`)

		errCh := make(chan error, 1)
		go func() { errCh <- tr.WriteJSON(t.Context(), payload) }()

		got, err := bufio.NewReader(stdinR).ReadBytes('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if err := <-errCh; err != nil {
			t.Fatalf("WriteJSON() = %v, want nil", err)
		}

		// Mutate original slice; transport must have cloned it.
		payload[0] = 'X'
		if got[0] != '{' {
			t.Errorf("transport aliased caller slice: got[0] = %q, want '{'", got[0])
		}
	})

	t.Run("error: nil stdin returns CLIConnectionError", func(t *testing.T) {
		t.Parallel()

		tr := &stdioTransport{}
		err := tr.WriteJSON(t.Context(), []byte(`{}`))
		var connErr *CLIConnectionError
		if !errors.As(err, &connErr) {
			t.Fatalf("WriteJSON() error type = %T, want *CLIConnectionError", err)
		}
	})
}

// TestStdioTransport_ReadJSON verifies that ReadJSON returns newline-terminated
// lines from stdout and propagates io.EOF when the stream ends.
func TestStdioTransport_ReadJSON(t *testing.T) {
	t.Parallel()

	t.Run("success: reads line from stdout", func(t *testing.T) {
		t.Parallel()

		stdoutR, stdoutW := io.Pipe()
		t.Cleanup(func() { stdoutR.Close(); stdoutW.Close() })

		tr := &stdioTransport{stdout: bufio.NewReader(stdoutR)}
		line := `{"type":"pong"}` + "\n"

		go func() {
			stdoutW.Write([]byte(line)) //nolint:errcheck
		}()

		got, err := tr.ReadJSON(t.Context())
		if err != nil {
			t.Fatalf("ReadJSON() = %v, want nil", err)
		}
		if string(got) != line {
			t.Errorf("ReadJSON() = %q, want %q", got, line)
		}
	})

	t.Run("success: returns io.EOF when stdout is closed", func(t *testing.T) {
		t.Parallel()

		stdoutR, stdoutW := io.Pipe()
		t.Cleanup(func() { stdoutR.Close() })

		tr := &stdioTransport{stdout: bufio.NewReader(stdoutR)}

		// Close the write end immediately → ReadJSON should return io.EOF.
		stdoutW.Close()

		_, err := tr.ReadJSON(t.Context())
		if !errors.Is(err, io.EOF) {
			t.Fatalf("ReadJSON() = %v, want io.EOF", err)
		}
	})

	t.Run("error: nil stdout returns CLIConnectionError", func(t *testing.T) {
		t.Parallel()

		tr := &stdioTransport{}
		_, err := tr.ReadJSON(t.Context())
		var connErr *CLIConnectionError
		if !errors.As(err, &connErr) {
			t.Fatalf("ReadJSON() error type = %T, want *CLIConnectionError", err)
		}
	})
}

// TestStdioTransport_Close verifies that Close signals EOF to the pipe reader
// and that calling Close on a nil stdin is a no-op.
func TestStdioTransport_Close(t *testing.T) {
	t.Parallel()

	t.Run("success: close signals EOF to stdin reader", func(t *testing.T) {
		t.Parallel()

		stdinR, stdinW := io.Pipe()
		t.Cleanup(func() { stdinR.Close() })

		tr := &stdioTransport{stdin: stdinW}
		if err := tr.Close(); err != nil {
			t.Fatalf("Close() = %v, want nil", err)
		}

		// Write end is closed; reading from read end must return io.EOF.
		buf := make([]byte, 1)
		_, err := stdinR.Read(buf)
		if !errors.Is(err, io.EOF) {
			t.Fatalf("after Close(), stdinR.Read() = %v, want io.EOF", err)
		}
	})

	t.Run("success: close nil stdin is idempotent", func(t *testing.T) {
		t.Parallel()

		tr := &stdioTransport{}
		if err := tr.Close(); err != nil {
			t.Fatalf("Close() nil stdin = %v, want nil", err)
		}
	})
}

// TestTransport_CloseDuringReadWrite is a data-race stress test.
//
// It concurrently calls Close and writeMessage on a ClaudeSDKClient backed by a
// FakeCLI transport and verifies that the race detector (go test -race) reports
// no data races. The race-safety relies on the write-symmetric clear from
// pkg/codex commit 8c16376: both writeMessage and Close access c.transport
// exclusively under writeMu.
//
// Recommended invocation for full stress coverage:
//
//	go test -race -run TestTransport_CloseDuringReadWrite -count=1000 ./pkg/claude
func TestTransport_CloseDuringReadWrite(t *testing.T) {
	t.Parallel()

	const iters = 200

	for range iters {
		ctx := context.Background()

		f := fakecli.New(t, nil) // empty script; ReadJSON blocks until Close
		c := &ClaudeSDKClient{}

		// start MUST be called with closeMu held — snapshot-as-arg discipline.
		c.closeMu.Lock()
		c.start(ctx, f, nil, nil, nil)
		c.closeMu.Unlock()

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine A: tear down the client.
		go func() {
			defer wg.Done()
			_ = c.Close()
		}()

		// Goroutine B: write a message (may lose the race with Close — that's fine).
		go func() {
			defer wg.Done()
			_ = c.writeMessage(ctx, []byte(`{"type":"ping"}`))
		}()

		wg.Wait()
	}
}
