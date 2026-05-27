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
	"context"
	"errors"
	"io"
	"iter"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"
)

type scriptedTransport struct {
	mu        sync.Mutex
	writes    []string
	readCh    chan readResult
	closed    bool
	closeOnce sync.Once
	onWrite   func(string)
}

type readResult struct {
	line string
	err  error
}

func newScriptedTransport() *scriptedTransport {
	return &scriptedTransport{readCh: make(chan readResult, 64)}
}

func (t *scriptedTransport) ReadLine(context.Context) (string, error) {
	result, ok := <-t.readCh
	if !ok {
		return "", io.EOF
	}
	return result.line, result.err
}

func (t *scriptedTransport) WriteLine(_ context.Context, line string) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrClosed
	}
	t.writes = append(t.writes, line)
	onWrite := t.onWrite
	t.mu.Unlock()
	if onWrite != nil {
		onWrite(line)
	}
	return nil
}

func (t *scriptedTransport) Close() error {
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	t.closeOnce.Do(func() { close(t.readCh) })
	return nil
}

func (t *scriptedTransport) writeLines(lines ...string) {
	for _, line := range lines {
		t.readCh <- readResult{line: line}
	}
}

func (t *scriptedTransport) written() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.writes)
}

func (t *scriptedTransport) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

func nextNotification(events iter.Seq[Notification]) (Notification, bool) {
	for notification := range events {
		return notification, true
	}
	return Notification{}, false
}

func newScriptedClient(t *testing.T, buffer int) (*Client, *scriptedTransport) {
	t.Helper()
	cfg, err := applyOptions([]Option{WithInitialCommand("new-session", "-A", "-s", "test"), WithEventBuffer(buffer), WithShutdownTimeout(50 * time.Millisecond)})
	if err != nil {
		t.Fatalf("applyOptions() error = %v", err)
	}
	tr := newScriptedTransport()
	client := newClient(cfg, tr, nil, nil)
	close(client.stderrDone)
	go client.readLoop(t.Context(), tr, client.readDone)
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	return client, tr
}

func TestClientExecRoutesResponses(t *testing.T) {
	t.Parallel()
	client, tr := newScriptedClient(t, 8)
	tr.onWrite = func(line string) {
		tr.writeLines("%begin 1 2 1", "hello", "%end 1 2 1")
	}
	resp, err := client.Exec(t.Context(), DisplayMessage, RawArg("-p"), StringArg("hello"))
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if diff := gocmp.Diff([]string{"hello"}, resp.Lines); diff != "" {
		t.Fatalf("response lines mismatch (-want +got):\n%s", diff)
	}
	if diff := gocmp.Diff([]string{"display-message -p hello"}, tr.written()); diff != "" {
		t.Fatalf("writes mismatch (-want +got):\n%s", diff)
	}
}

func TestClientNilExecReturnsErrClosed(t *testing.T) {
	t.Parallel()
	var client *Client
	if _, err := client.ExecRaw(t.Context(), "display-message -p ok"); !errors.Is(err, ErrClosed) {
		t.Fatalf("nil ExecRaw() error = %v, want ErrClosed", err)
	}
}

func TestClientExecCommandError(t *testing.T) {
	t.Parallel()
	client, tr := newScriptedClient(t, 8)
	tr.onWrite = func(line string) {
		tr.writeLines("%begin 1 3 1", "parse error", "%error 1 3 1")
	}
	_, err := client.ExecRaw(t.Context(), "bad-command")
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("ExecRaw() error = %v (%T), want *CommandError", err, err)
	}
	if commandErr.Line != "bad-command" || strings.Join(commandErr.Response.Lines, "\n") != "parse error" {
		t.Fatalf("CommandError = %#v", commandErr)
	}
}

func TestClientExecSerializesConcurrentCommands(t *testing.T) {
	t.Parallel()
	client, tr := newScriptedClient(t, 16)
	var mu sync.Mutex
	count := 0
	tr.onWrite = func(line string) {
		mu.Lock()
		count++
		id := count
		mu.Unlock()
		tr.writeLines("%begin 1 "+strconv.Itoa(id)+" 1", line, "%end 1 "+strconv.Itoa(id)+" 1")
	}
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := client.ExecRaw(t.Context(), "display-message -p "+strconv.Itoa(i))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ExecRaw() concurrent error = %v", err)
		}
	}
	if got := len(tr.written()); got != n {
		t.Fatalf("written command count = %d, want %d", got, n)
	}
}

func TestClientNotificationOverflowDropsOldest(t *testing.T) {
	t.Parallel()
	client, tr := newScriptedClient(t, 1)
	tr.writeLines("%message first", "%message second", "%message third")
	deadline := time.Now().Add(time.Second)
	for client.DroppedNotifications() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if client.DroppedNotifications() != 2 {
		t.Fatalf("DroppedNotifications() = %d, want 2", client.DroppedNotifications())
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	got, ok := nextNotification(client.Events(ctx))
	if !ok {
		t.Fatal("timed out waiting for notification")
	}
	if got.Raw != "%message third" {
		t.Fatalf("event Raw = %q, want newest third", got.Raw)
	}
}

func TestClientEventsNilClientStops(t *testing.T) {
	t.Parallel()
	var client *Client
	for notification := range client.Events(t.Context()) {
		t.Fatalf("nil Events yielded unexpected notification %#v", notification)
	}
}

func TestClientEventsStopsOnContextCancel(t *testing.T) {
	t.Parallel()
	client, _ := newScriptedClient(t, 8)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for notification := range client.Events(ctx) {
		t.Fatalf("canceled Events yielded unexpected notification %#v", notification)
	}
}

func TestClientCloseUnblocksPendingExec(t *testing.T) {
	t.Parallel()
	client, tr := newScriptedClient(t, 8)
	started := make(chan struct{})
	var startedOnce sync.Once
	tr.onWrite = func(line string) { startedOnce.Do(func() { close(started) }) }
	errCh := make(chan error, 1)
	go func() {
		_, err := client.ExecRaw(t.Context(), "display-message -p wait")
		errCh <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for write")
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("ExecRaw() error = %v, want ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ExecRaw to unblock")
	}
}

func TestClientExecTimeoutClosesClient(t *testing.T) {
	t.Parallel()
	client, tr := newScriptedClient(t, 8)
	started := make(chan struct{})
	var startedOnce sync.Once
	tr.onWrite = func(string) { startedOnce.Do(func() { close(started) }) }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := client.ExecRaw(ctx, "display-message -p wait")
		errCh <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for command write")
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ExecRaw() timeout error = %v, want DeadlineExceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ExecRaw timeout")
	}
	if _, err := client.ExecRaw(t.Context(), "display-message -p after-timeout"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecRaw() after timeout error = %v, want DeadlineExceeded", err)
	}
}

func TestClientCloseAfterReadEOFStillClosesTransport(t *testing.T) {
	t.Parallel()
	client, tr := newScriptedClient(t, 8)
	tr.readCh <- readResult{err: io.EOF}
	select {
	case <-client.readDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for read loop EOF")
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatalf("Close() after read EOF error = %v", err)
	}
	if !tr.isClosed() {
		t.Fatal("Close() after read EOF did not close transport")
	}
}
