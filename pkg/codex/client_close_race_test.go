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

package codex

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// TestCloseDuringConcurrentWrite verifies that concurrent RequestRaw calls
// racing with Close() produce only nil, *TransportClosedError, or context
// errors — never a panic or unexpected error type.
func TestCloseDuringConcurrentWrite(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)

	client := NewClient(&Config{}, nil)
	client.storeTransport(&stdioTransport{stdin: stdinW, stdout: bufio.NewReader(stdoutR)})
	client.stdoutCloser = stdoutR
	client.responses = map[string]chan responseWait{}
	client.turnRouter = newTurnNotificationRouter()
	client.readDone = make(chan struct{})
	client.stderrDone = make(chan struct{})
	close(client.stderrDone)
	go client.readLoop(ctx, client.loadTransport(), client.readDone)

	// Responder: echo each request back as a successful response.
	go func() {
		defer stdinR.Close()
		scanner := bufio.NewScanner(stdinR)
		for scanner.Scan() {
			var msg rpcMessage
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				return
			}
			if msg.ID == "" {
				continue
			}
			resp := rpcMessage{ID: msg.ID, Result: jsontext.Value(`{"ok":true}`)}
			raw, err := json.Marshal(resp)
			if err != nil {
				return
			}
			if _, err := stdoutW.Write(append(raw, '\n')); err != nil {
				return
			}
		}
	}()

	const N = 64
	var wg sync.WaitGroup
	errs := make([]error, N)
	wg.Add(N)
	for i := range N {
		go func(i int) {
			defer wg.Done()
			_, err := client.RequestRaw(ctx, "helper/echo", Object{"n": i})
			errs[i] = err
		}(i)
	}

	// Let goroutines queue writes, then close mid-flight.
	time.Sleep(5 * time.Millisecond)
	_ = client.Close()
	// Drain stdoutW so any writes after stdoutCloser.Close() don't block responder.
	_ = stdoutW.Close()
	wg.Wait()

	for i, err := range errs {
		if err == nil {
			continue
		}
		if _, ok := errors.AsType[*TransportClosedError](err); ok {
			continue
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			continue
		}
		t.Errorf("goroutine %d: unexpected error type %T: %v", i, err, err)
	}
}

// TestCloseDuringConcurrentRead verifies that goroutines blocked in
// NextNotification all unblock and return *TransportClosedError after Close().
func TestCloseDuringConcurrentRead(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	t.Cleanup(func() {
		_ = stdinR.Close()
		_ = stdinW.Close()
		_ = stdoutW.Close()
	})

	client := NewClient(&Config{}, nil)
	client.storeTransport(&stdioTransport{stdin: stdinW, stdout: bufio.NewReader(stdoutR)})
	client.stdoutCloser = stdoutR
	client.responses = map[string]chan responseWait{}
	client.turnRouter = newTurnNotificationRouter()
	client.readDone = make(chan struct{})
	client.stderrDone = make(chan struct{})
	close(client.stderrDone)
	go client.readLoop(ctx, client.loadTransport(), client.readDone)

	const N = 64
	errs := make(chan error, N)
	for range N {
		go func() {
			_, err := client.NextNotification(ctx)
			errs <- err
		}()
	}

	time.Sleep(10 * time.Millisecond)
	_ = client.Close()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for range N {
		select {
		case err := <-errs:
			if err == nil {
				t.Error("NextNotification() returned nil, want error after Close")
				continue
			}
			var tcErr *TransportClosedError
			if !errors.As(err, &tcErr) {
				t.Errorf("NextNotification() error = %v (%T), want *TransportClosedError", err, err)
			}
		case <-deadline.C:
			t.Fatal("timed out waiting for NextNotification to unblock after Close")
		}
	}
}

// TestStdioReadCancellable verifies that stdioTransport.ReadJSON honours context
// cancellation even when the underlying bufio.Reader is blocked in ReadBytes.
func TestStdioReadCancellable(t *testing.T) {
	stdoutR, stdoutW := io.Pipe()
	// Close write end in cleanup to unblock the orphan ReadBytes goroutine
	// that exits after ctx cancellation returns to the caller.
	t.Cleanup(func() { _ = stdoutW.Close() })

	tr := &stdioTransport{stdout: bufio.NewReader(stdoutR)}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := tr.ReadJSON(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReadJSON() error = %v (%T), want context.DeadlineExceeded", err, err)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("ReadJSON() took %v to return after ctx cancel, want < 200ms", elapsed)
	}
}

// TestCloseGoroutineLeak starts and closes a real subprocess 100 times and
// verifies that goroutine count does not grow beyond baseline + 2.
func TestCloseGoroutineLeak(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	shimPath := writeCloseRaceHelperShim(t, exe)

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)

	// Warm-up iteration: allows the Go runtime's internal goroutines to settle.
	{
		c := NewClient(&Config{
			LaunchArgsOverride: []string{shimPath},
			Env:                map[string]string{"CODEX_CLOSE_RACE_HELPER": "1"},
		}, nil)
		if err := c.Start(ctx); err != nil {
			t.Fatalf("warm-up Start() error = %v", err)
		}
		_ = c.Close()
	}
	runtime.Gosched()
	time.Sleep(50 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	const iterations = 100
	for i := range iterations {
		c := NewClient(&Config{
			LaunchArgsOverride: []string{shimPath},
			Env:                map[string]string{"CODEX_CLOSE_RACE_HELPER": "1"},
		}, nil)
		if err := c.Start(ctx); err != nil {
			t.Fatalf("iteration %d: Start() error = %v", i, err)
		}
		if err := c.Close(); err != nil {
			t.Fatalf("iteration %d: Close() error = %v", i, err)
		}
	}

	runtime.Gosched()
	time.Sleep(100 * time.Millisecond)
	final := runtime.NumGoroutine()
	delta := final - baseline
	if delta > 2 {
		t.Errorf("goroutine leak after %d Start/Close cycles: baseline=%d final=%d delta=%d (want ≤ 2)",
			iterations, baseline, final, delta)
	}
}

// TestCodexCloseRaceHelperProcess is the subprocess entry point for
// TestCloseGoroutineLeak. It guards itself with an env var so the normal test
// run ignores it.
func TestCodexCloseRaceHelperProcess(t *testing.T) {
	if os.Getenv("CODEX_CLOSE_RACE_HELPER") != "1" {
		return
	}
	runTransportHelperStdio()
}

// writeCloseRaceHelperShim writes a shell script that re-executes the test
// binary as the close-race subprocess helper.
func writeCloseRaceHelperShim(t *testing.T, exe string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex-close-race-helper")
	body := "#!/bin/sh\nexec " + strconv.Quote(exe) + " -test.run=TestCodexCloseRaceHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", path, err)
	}
	return path
}
