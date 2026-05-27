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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
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

type blockingTransport struct {
	readStarted chan struct{}
	release     chan struct{}
	closeOnce   sync.Once
	startOnce   sync.Once
}

func newBlockingTransport() *blockingTransport {
	return &blockingTransport{
		readStarted: make(chan struct{}),
		release:     make(chan struct{}),
	}
}

func (t *blockingTransport) ReadLine(ctx context.Context) (string, error) {
	t.startOnce.Do(func() { close(t.readStarted) })
	select {
	case <-t.release:
		return "", io.EOF
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (t *blockingTransport) WriteLine(context.Context, string) error {
	return nil
}

func (t *blockingTransport) Close() error {
	t.closeOnce.Do(func() { close(t.release) })
	return nil
}

type blockingWriteTransport struct {
	writeStarted chan struct{}
	writeRelease chan struct{}
	closeCalled  chan struct{}
	writeOnce    sync.Once
	releaseOnce  sync.Once
	closeOnce    sync.Once
}

func newBlockingWriteTransport() *blockingWriteTransport {
	return &blockingWriteTransport{
		writeStarted: make(chan struct{}),
		writeRelease: make(chan struct{}),
		closeCalled:  make(chan struct{}),
	}
}

func (t *blockingWriteTransport) ReadLine(context.Context) (string, error) {
	return "", io.EOF
}

func (t *blockingWriteTransport) WriteLine(context.Context, string) error {
	t.writeOnce.Do(func() { close(t.writeStarted) })
	<-t.writeRelease
	return ErrClosed
}

func (t *blockingWriteTransport) Close() error {
	t.closeOnce.Do(func() { close(t.closeCalled) })
	t.releaseWrite()
	return nil
}

func (t *blockingWriteTransport) releaseWrite() {
	t.releaseOnce.Do(func() { close(t.writeRelease) })
}

type blockingCloseTransport struct {
	closeStarted chan struct{}
	closeRelease chan struct{}
	closeOnce    sync.Once
	releaseOnce  sync.Once
}

var errBlockingTransportClose = errors.New("blocking transport close")

func newBlockingCloseTransport() *blockingCloseTransport {
	return &blockingCloseTransport{
		closeStarted: make(chan struct{}),
		closeRelease: make(chan struct{}),
	}
}

func (t *blockingCloseTransport) ReadLine(context.Context) (string, error) {
	return "", io.EOF
}

func (t *blockingCloseTransport) WriteLine(context.Context, string) error {
	return nil
}

func (t *blockingCloseTransport) Close() error {
	t.closeOnce.Do(func() { close(t.closeStarted) })
	<-t.closeRelease
	return errBlockingTransportClose
}

func (t *blockingCloseTransport) releaseClose() {
	t.releaseOnce.Do(func() { close(t.closeRelease) })
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
	client := newClient(nil, cfg, tr, nil)
	client.readDone = make(chan struct{})
	client.stderrDone = make(chan struct{})
	close(client.stderrDone)
	go client.readLoop(context.Background(), tr, client.readDone)
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	return client, tr
}

func writeTmuxTestHelper(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tmux-helper")
	execPath, err := os.Executable()
	if err != nil {
		t.Fatalf("get executable path: %v", err)
	}
	script := fmt.Sprintf("#!/bin/sh\nexec %s -test.run=TestTmuxHelperProcess -- \"$@\"\n", strconv.Quote(execPath))
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write tmux helper script: %v", err)
	}
	return path
}

func TestTmuxHelperProcess(t *testing.T) {
	if os.Getenv("PANDAEMONIUM_TMUX_HELPER") != "1" {
		return
	}
	_, _ = io.WriteString(os.Stdout, "%begin 1 1 1\n%end 1 1 1\n")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if scanner.Text() == string(DetachClient) {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
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

func TestClientDeliverEventDropsOldestWhenCalledDirectly(t *testing.T) {
	t.Parallel()
	cfg, err := applyOptions([]Option{WithInitialCommand("new-session", "-A", "-s", "test"), WithEventBuffer(1)})
	if err != nil {
		t.Fatalf("applyOptions() error = %v", err)
	}
	client := newClient(nil, cfg, nil, nil)
	client.events <- Notification{Kind: NotificationMessage, Raw: "%message existing"}
	client.deliverEvent(Notification{Kind: NotificationMessage, Raw: "%message replacement"})
	client.deliverEvent(Notification{Kind: NotificationMessage, Raw: "%message newest"})

	if got := client.DroppedNotifications(); got != 2 {
		t.Fatalf("DroppedNotifications() = %d, want 2", got)
	}
	got := <-client.events
	if got.Raw != "%message newest" {
		t.Fatalf("buffered event Raw = %q, want newest", got.Raw)
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

func TestClientCloseAllowsUnstartedLoopChannels(t *testing.T) {
	t.Parallel()
	cfg, err := applyOptions([]Option{WithInitialCommand("new-session", "-A", "-s", "test"), WithShutdownTimeout(10 * time.Millisecond)})
	if err != nil {
		t.Fatalf("applyOptions() error = %v", err)
	}
	tr := newScriptedTransport()
	client := newClient(nil, cfg, tr, nil)
	if client.readDone != nil || client.stderrDone != nil {
		t.Fatalf("newClient() readDone = %v, stderrDone = %v; want nil channels before goroutine launch", client.readDone, client.stderrDone)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Close(ctx); err != nil {
		t.Fatalf("Close() with unstarted loops error = %v", err)
	}
	if !tr.isClosed() {
		t.Fatal("Close() with unstarted loops did not close transport")
	}
}

func TestReadLoopCanOutliveStartupContextCancellation(t *testing.T) {
	t.Parallel()
	startupCtx, cancelStartup := context.WithCancel(t.Context())
	defer cancelStartup()
	cfg, err := applyOptions([]Option{WithInitialCommand("new-session", "-A", "-s", "test"), WithShutdownTimeout(10 * time.Millisecond)})
	if err != nil {
		t.Fatalf("applyOptions() error = %v", err)
	}
	tr := newBlockingTransport()
	client := newClient(nil, cfg, tr, nil)
	client.readDone = make(chan struct{})
	client.stderrDone = make(chan struct{})
	close(client.stderrDone)
	go client.readLoop(context.Background(), tr, client.readDone)

	select {
	case <-tr.readStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for read loop to start")
	}
	cancelStartup()
	if err := startupCtx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("startup context error = %v, want context.Canceled", err)
	}
	select {
	case <-client.readDone:
		t.Fatal("read loop stopped when unrelated startup context was canceled")
	case <-time.After(25 * time.Millisecond):
	}

	if err := tr.Close(); err != nil {
		t.Fatalf("blocking transport close error = %v", err)
	}
	select {
	case <-client.readDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for read loop to stop after transport close")
	}
}

func TestNewClientProcessOutlivesStartupContextCancellation(t *testing.T) {
	t.Parallel()
	helper := writeTmuxTestHelper(t)
	startupCtx, cancelStartup := context.WithCancel(context.Background())
	client, err := New(
		startupCtx,
		WithPath(helper),
		WithEnv("PANDAEMONIUM_TMUX_HELPER=1"),
		WithInitialCommand("new-session", "-A", "-s", "test"),
		WithShutdownTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("New() with helper error = %v", err)
	}
	cancelStartup()
	select {
	case <-client.readDone:
		t.Fatal("read loop stopped when startup context was canceled after New returned")
	case <-time.After(50 * time.Millisecond):
	}
	if err := client.closedError(); err != nil {
		t.Fatalf("client closed after startup context cancellation: %v", err)
	}

	closeCtx, cancelClose := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelClose()
	if err := client.Close(closeCtx); err != nil {
		t.Fatalf("Close() after startup context cancellation error = %v", err)
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

func TestClientCloseUnblocksExecRawStuckInTransportWrite(t *testing.T) {
	t.Parallel()
	cfg, err := applyOptions([]Option{WithInitialCommand("new-session", "-A", "-s", "test"), WithShutdownTimeout(time.Second)})
	if err != nil {
		t.Fatalf("applyOptions() error = %v", err)
	}
	tr := newBlockingWriteTransport()
	client := newClient(nil, cfg, tr, nil)

	execDone := make(chan error, 1)
	go func() {
		_, err := client.ExecRaw(context.Background(), "display-message -p stuck")
		execDone <- err
	}()
	select {
	case <-tr.writeStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ExecRaw to enter transport WriteLine")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- client.Close(context.Background()) }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		tr.releaseWrite()
		if err := <-closeDone; err != nil {
			t.Fatalf("Close() after forced release error = %v", err)
		}
		t.Fatal("Close() blocked behind ExecRaw's writeMu instead of closing transport to unblock WriteLine")
	}
	select {
	case <-tr.closeCalled:
	default:
		t.Fatal("Close() returned without closing the transport")
	}
	select {
	case err := <-execDone:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("ExecRaw() error = %v, want ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ExecRaw to return after Close")
	}
}

func TestClientConcurrentCloseWaitsForCleanupToFinish(t *testing.T) {
	t.Parallel()
	cfg, err := applyOptions([]Option{WithInitialCommand("new-session", "-A", "-s", "test"), WithShutdownTimeout(time.Second)})
	if err != nil {
		t.Fatalf("applyOptions() error = %v", err)
	}
	tr := newBlockingCloseTransport()
	client := newClient(nil, cfg, tr, nil)

	firstDone := make(chan error, 1)
	go func() { firstDone <- client.Close(context.Background()) }()
	select {
	case <-tr.closeStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first Close to start transport cleanup")
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- client.Close(context.Background()) }()
	select {
	case err := <-secondDone:
		tr.releaseClose()
		if firstErr := <-firstDone; !errors.Is(firstErr, errBlockingTransportClose) {
			t.Fatalf("first Close() after forced release error = %v, want errBlockingTransportClose", firstErr)
		}
		t.Fatalf("concurrent Close() returned before the in-flight cleanup finished: %v", err)
	case <-time.After(50 * time.Millisecond):
		// Expected: concurrent Close callers share the in-flight cleanup result.
	}

	tr.releaseClose()
	for name, ch := range map[string]<-chan error{"first": firstDone, "second": secondDone} {
		select {
		case err := <-ch:
			if !errors.Is(err, errBlockingTransportClose) {
				t.Fatalf("%s Close() error = %v, want errBlockingTransportClose", name, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s Close() after cleanup release", name)
		}
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
