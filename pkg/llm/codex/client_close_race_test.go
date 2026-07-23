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
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"

	llm "github.com/zchee/pandaemonium/pkg/llm"
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
	client.storeTransport(newStdioTransport(stdinW, bufio.NewReader(stdoutR)))
	client.stdoutCloser = stdoutR
	client.rpcState = newJSONRPCClientState()
	client.turnRouter = newTurnNotificationRouter()
	client.readDone = make(chan struct{})
	client.stderrDone = make(chan struct{})
	close(client.stderrDone)
	go client.readLoop(ctx, client.loadTransport(), client.readDone)

	// Responder: echo each request back as a successful response.
	responderErr := make(chan error, 1)
	responderDone := make(chan struct{})
	go func() {
		defer close(responderDone)
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
		if err := scanner.Err(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			responderErr <- err
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
	responderExit := time.NewTimer(5 * time.Second)
	defer responderExit.Stop()
	select {
	case <-responderDone:
	case <-responderExit.C:
		t.Fatal("timed out waiting for responder goroutine to exit")
	}
	select {
	case err := <-responderErr:
		t.Errorf("responder scanner error: %v", err)
	default:
	}

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
	client.storeTransport(newStdioTransport(stdinW, bufio.NewReader(stdoutR)))
	client.stdoutCloser = stdoutR
	client.rpcState = newJSONRPCClientState()
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
			if _, ok := errors.AsType[*TransportClosedError](err); !ok {
				t.Errorf("NextNotification() error = %v (%T), want *TransportClosedError", err, err)
			}
		case <-deadline.C:
			t.Fatal("timed out waiting for NextNotification to unblock after Close")
		}
	}
}

func TestClientCloseUsesOneAbsoluteDeadlineBeforeWriteGate(t *testing.T) {
	t.Parallel()
	client := NewClient(&Config{}, nil)
	client.closeBudget = clientCloseBudget{
		transport: 50 * time.Millisecond,
		process:   20 * time.Millisecond,
		killReap:  5 * time.Millisecond,
		read:      10 * time.Millisecond,
		stderr:    5 * time.Millisecond,
	}
	base := time.Now()
	client.closeNow = func() time.Time { return base }
	transport := &recordingDeadlineTransport{closed: make(chan struct{})}
	client.storeTransport(transport)
	stdinR, stdinW := io.Pipe()
	t.Cleanup(func() { _ = stdinR.Close() })
	client.stdin = stdinW
	client.stdout = bufio.NewReader(strings.NewReader(""))
	client.stdoutCloser = io.NopCloser(strings.NewReader(""))
	client.stderr = io.NopCloser(strings.NewReader(""))
	client.readDone = make(chan struct{})
	close(client.readDone)
	client.stderrDone = make(chan struct{})
	close(client.stderrDone)

	client.rpcState.lockWrite()
	closeDone := make(chan error, 1)
	go func() { closeDone <- client.Close() }()
	select {
	case <-transport.closed:
	case <-time.After(time.Second):
		t.Fatal("Client.Close() waited for write gate before closing transport")
	}
	wantDeadline := base.Add(50 * time.Millisecond)
	if !transport.deadline.Equal(wantDeadline) {
		t.Fatalf("transport deadline = %v, want %v", transport.deadline, wantDeadline)
	}
	client.rpcState.unlockWrite()
	if err := <-closeDone; err != nil {
		t.Fatalf("Client.Close() error = %v", err)
	}
	if got := client.loadTransport(); got != nil {
		t.Fatalf("Client.Close() retained transport = %T, want nil", got)
	}
	if client.stdin != nil || client.stdout != nil || client.stdoutCloser != nil || client.stderr != nil || client.readDone != nil || client.stderrDone != nil {
		t.Fatalf("Client.Close() retained detached process state: stdin=%v stdout=%v stdoutCloser=%v stderr=%v readDone=%v stderrDone=%v", client.stdin, client.stdout, client.stdoutCloser, client.stderr, client.readDone, client.stderrDone)
	}
}

func TestClientCloseBudgetDeadlinesAreCumulative(t *testing.T) {
	t.Parallel()
	start := time.Unix(123, 456)
	budget := clientCloseBudget{
		transport: 10 * time.Second,
		process:   2 * time.Second,
		killReap:  250 * time.Millisecond,
		read:      500 * time.Millisecond,
		stderr:    500 * time.Millisecond,
	}
	got := budget.deadlines(start)
	if want := start.Add(10 * time.Second); !got.transport.Equal(want) {
		t.Fatalf("transport deadline = %v, want %v", got.transport, want)
	}
	if want := start.Add(12 * time.Second); !got.process.Equal(want) {
		t.Fatalf("process deadline = %v, want %v", got.process, want)
	}
	if want := start.Add(11750 * time.Millisecond); !got.interrupt.Equal(want) {
		t.Fatalf("interrupt deadline = %v, want %v", got.interrupt, want)
	}
	if want := start.Add(12500 * time.Millisecond); !got.read.Equal(want) {
		t.Fatalf("read deadline = %v, want %v", got.read, want)
	}
	if want := start.Add(13 * time.Second); !got.stderr.Equal(want) {
		t.Fatalf("stderr deadline = %v, want %v", got.stderr, want)
	}
}

func TestClientCloseKillsAndReapsInterruptIgnoringProcessWithinBudget(t *testing.T) {
	if os.Getenv("CODEX_CLOSE_IGNORE_INTERRUPT_HELPER") == "1" {
		signal.Ignore(os.Interrupt)
		fmt.Fprintln(os.Stdout, "ready")
		select {}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	cmd := exec.Command(exe, "-test.run=TestClientCloseKillsAndReapsInterruptIgnoringProcessWithinBudget")
	cmd.Env = append(os.Environ(), "CODEX_CLOSE_IGNORE_INTERRUPT_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start() error = %v", err)
	}
	ready := bufio.NewScanner(stdout)
	if !ready.Scan() || ready.Text() != "ready" {
		_ = cmd.Process.Kill()
		t.Fatalf("helper readiness = %q err=%v", ready.Text(), ready.Err())
	}
	done := llm.WaitForCommand(cmd)
	client := NewClient(&Config{}, nil)
	client.closeBudget = clientCloseBudget{
		transport: 5 * time.Millisecond,
		process:   100 * time.Millisecond,
		killReap:  25 * time.Millisecond,
		read:      5 * time.Millisecond,
		stderr:    5 * time.Millisecond,
	}
	client.cmd = cmd
	client.cmdDone = done
	client.storeTransport(&recordingDeadlineTransport{closed: make(chan struct{})})
	client.readDone = make(chan struct{})
	close(client.readDone)
	client.stderrDone = make(chan struct{})
	close(client.stderrDone)
	start := time.Now()
	if err := client.Close(); err != nil {
		t.Fatalf("Client.Close() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Client.Close() elapsed = %v, want bounded process schedule", elapsed)
	}
	select {
	case _, ok := <-done:
		if ok {
			select {
			case _, ok = <-done:
				if ok {
					t.Fatal("cmdDone remained open after kill/reap")
				}
			default:
				t.Fatal("cmdDone remained open after kill/reap")
			}
		}
	default:
		t.Fatal("Client.Close() returned before interrupt-ignoring process was reaped")
	}
}

type recordingDeadlineTransport struct {
	deadline time.Time
	closed   chan struct{}
}

func (t *recordingDeadlineTransport) closeByDeadline(deadline time.Time) error {
	t.deadline = deadline
	close(t.closed)
	return nil
}

func (t *recordingDeadlineTransport) Close() error { return t.closeByDeadline(time.Time{}) }

func (*recordingDeadlineTransport) WriteJSON(context.Context, []byte) error { return nil }

func (*recordingDeadlineTransport) ReadJSON(context.Context) ([]byte, error) {
	return nil, io.EOF
}

// TestStdioReadCancellable verifies that stdioTransport.ReadJSON honors context
// cancellation even when the underlying bufio.Reader is blocked in ReadBytes.
func TestStdioReadCancellable(t *testing.T) {
	stdoutR, stdoutW := io.Pipe()
	// Close write end in cleanup to unblock the orphan ReadBytes goroutine
	// that exits after ctx cancellation returns to the caller.
	t.Cleanup(func() { _ = stdoutW.Close() })

	tr := newStdioTransport(nil, bufio.NewReader(stdoutR))
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
