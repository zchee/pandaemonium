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
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zchee/gows"
)

func TestWebSocketTransportMessageContracts(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		write    func(*gows.Conn) error
		wantEOF  bool
		wantCode gows.CloseCode
		wantApp  bool
		terminal bool
	}{
		"success: text payload is owned and newline terminated": {
			write: func(c *gows.Conn) error { return c.WriteMessage(gows.OpcodeText, []byte(`{"ok":true}`)) },
		},
		"error: binary payload is rejected": {
			write:    func(c *gows.Conn) error { return c.WriteMessage(gows.OpcodeBinary, []byte(`{"ok":true}`)) },
			wantApp:  true,
			terminal: true,
		},
		"success: normal close maps to EOF": {
			write:    func(c *gows.Conn) error { return writeCloseAndDrain(c, gows.CloseNormalClosure, "") },
			wantEOF:  true,
			terminal: true,
		},
		"error: abnormal close preserves sanitized typed code": {
			write:    func(c *gows.Conn) error { return writeCloseAndDrain(c, gows.CloseMessageTooBig, "secret peer detail") },
			wantCode: gows.CloseMessageTooBig,
			terminal: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			transport, server := newPipeWebSocketTransport(t)
			writeDone := make(chan error, 1)
			go func() { writeDone <- tt.write(server) }()

			got, err := transport.ReadJSON(t.Context())
			switch {
			case tt.wantEOF:
				if !errors.Is(err, io.EOF) {
					t.Fatalf("ReadJSON() error = %v, want EOF", err)
				}
			case tt.wantApp:
				if _, ok := errors.AsType[*AppServerError](err); !ok {
					t.Fatalf("ReadJSON() error = %v (%T), want *AppServerError", err, err)
				}
			case tt.wantCode != 0:
				closed, ok := errors.AsType[*TransportClosedError](err)
				if !ok {
					t.Fatalf("ReadJSON() error = %v (%T), want *TransportClosedError", err, err)
				}
				cause, ok := errors.AsType[*gows.CloseError](closed)
				if !ok || cause.Code != tt.wantCode || cause.Reason != "[redacted peer reason]" {
					t.Fatalf("typed cause = %#v, want code %d and sanitized reason", cause, tt.wantCode)
				}
				if strings.Contains(err.Error(), "secret peer detail") {
					t.Fatal("ReadJSON() error leaked peer close reason")
				}
			default:
				if err != nil {
					t.Fatalf("ReadJSON() error = %v", err)
				}
				if string(got) != "{\"ok\":true}\n" {
					t.Fatalf("ReadJSON() = %q, want owned newline-terminated JSON", got)
				}
			}
			if err := <-writeDone; err != nil && !errors.Is(err, net.ErrClosed) {
				t.Fatalf("server write error = %v", err)
			}
			if tt.terminal {
				first := transport.terminalErr
				if first == nil {
					t.Fatal("terminal path did not retain a stable TransportClosedError")
				}
				_, next := transport.ReadJSON(t.Context())
				if next != first {
					t.Fatalf("subsequent ReadJSON() error = %p %v, want stable %p %v", next, next, first, first)
				}
				if _, rawErr := transport.raw.Write([]byte("after terminal")); rawErr == nil {
					t.Fatal("raw connection remained writable after terminal path")
				}
			}
		})
	}
}

func writeCloseAndDrain(c *gows.Conn, code gows.CloseCode, reason string) error {
	go func() { _, _, _ = c.ReadMessage() }()
	return c.WriteClose(code, reason)
}

func TestWebSocketTransportCanceledWriteIsTerminal(t *testing.T) {
	t.Parallel()
	transport, _ := newPipeWebSocketTransport(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := transport.WriteJSON(ctx, []byte(`{"id":1}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteJSON() error = %v, want errors.Is context.Canceled", err)
	}
	if _, err := transport.raw.Write([]byte("after cancellation")); err == nil {
		t.Fatal("raw connection remained writable after terminal cancellation")
	}
	if next := transport.WriteJSON(t.Context(), []byte(`{"id":2}`)); next != err {
		t.Fatalf("subsequent WriteJSON() error = %p %v, want stable %p %v", next, next, err, err)
	}
}

func TestWebSocketTransportGateWaitCancellationIsTerminal(t *testing.T) {
	t.Parallel()
	transport, _ := newPipeWebSocketTransport(t)
	<-transport.writeGate
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- transport.WriteJSON(ctx, []byte(`{"id":1}`)) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WriteJSON() gate-wait error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled writer remained blocked waiting for direction gate")
	}
	transport.writeGate <- struct{}{}
	if _, err := transport.raw.Write([]byte("after gate cancellation")); err == nil {
		t.Fatal("gate-wait cancellation did not terminal-close raw connection")
	}
}

func TestWebSocketTransportTerminalIdentityWinsOverCanceledContext(t *testing.T) {
	t.Parallel()
	transport, _ := newPipeWebSocketTransport(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	first := transport.WriteJSON(ctx, []byte(`{"id":1}`))
	if first == nil {
		t.Fatal("initial canceled WriteJSON() error = nil")
	}
	for range 100 {
		if got := transport.WriteJSON(ctx, []byte(`{"id":2}`)); got != first {
			t.Fatalf("terminal error identity = %p %v, want %p %v", got, got, first, first)
		}
	}
}

func TestWebSocketTransportCloseIsBoundedAndIdempotent(t *testing.T) {
	t.Parallel()
	clientRaw, serverRaw := net.Pipe()
	counted := &closeCountingConn{Conn: clientRaw}
	transport := newWebsocketTransport(counted, gows.Handshake{})
	transport.closeTimeout = 20 * time.Millisecond
	t.Cleanup(func() { _ = serverRaw.Close() })

	start := time.Now()
	done := make(chan error, 2)
	go func() { done <- transport.Close() }()
	go func() { done <- transport.Close() }()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Close() elapsed = %v, want bounded short test budget", elapsed)
	}
	if got := counted.closes.Load(); got != 1 {
		t.Fatalf("raw Close() calls = %d, want exactly 1", got)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("idempotent Close() error = %v", err)
	}
	if got := counted.closes.Load(); got != 1 {
		t.Fatalf("raw Close() calls after idempotent Close = %d, want exactly 1", got)
	}
}

func TestWebSocketTransportCloseUnblocksPendingRead(t *testing.T) {
	t.Parallel()
	clientRaw, serverRaw := net.Pipe()
	counted := &closeCountingConn{Conn: clientRaw}
	transport := newWebsocketTransport(counted, gows.Handshake{})
	transport.closeTimeout = 20 * time.Millisecond
	t.Cleanup(func() { _ = serverRaw.Close() })
	readDone := make(chan error, 1)
	go func() { _, err := transport.ReadJSON(t.Context()); readDone <- err }()
	if err := transport.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case err := <-readDone:
		if _, ok := errors.AsType[*TransportClosedError](err); !ok {
			t.Fatalf("pending ReadJSON() error = %v (%T), want *TransportClosedError", err, err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() did not unblock pending ReadJSON")
	}
	if got := counted.closes.Load(); got != 1 {
		t.Fatalf("raw Close() calls = %d, want exactly 1", got)
	}
}

func TestWebSocketTransportCloseUnblocksBlockedWrite(t *testing.T) {
	t.Parallel()
	clientRaw, serverRaw := net.Pipe()
	counted := &closeCountingConn{Conn: clientRaw}
	signaled := &writeSignalingConn{Conn: counted, started: make(chan struct{})}
	transport := newWebsocketTransport(signaled, gows.Handshake{})
	transport.closeTimeout = 20 * time.Millisecond
	t.Cleanup(func() { _ = serverRaw.Close() })
	writeDone := make(chan error, 1)
	go func() { writeDone <- transport.WriteJSON(t.Context(), make([]byte, 1<<20)) }()
	<-signaled.started
	if err := transport.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case err := <-writeDone:
		if _, ok := errors.AsType[*TransportClosedError](err); !ok {
			t.Fatalf("blocked WriteJSON() error = %v (%T), want *TransportClosedError", err, err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() did not unblock blocked WriteJSON")
	}
	if got := counted.closes.Load(); got != 1 {
		t.Fatalf("raw Close() calls = %d, want exactly 1", got)
	}
}

func TestWebSocketContextIOLateCallbackCannotClearTerminalDeadline(t *testing.T) {
	t.Parallel()
	transport, _ := newPipeWebSocketTransport(t)
	recorder := &deadlineRecorder{}
	entered := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- transport.contextIO(ctx, recorder.set, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	transport.markTerminal(newTransportClosedError("test terminal", net.ErrClosed))
	transport.deadlineMu.Lock()
	_ = recorder.set(time.Now().Add(time.Second))
	transport.deadlineMu.Unlock()
	cancel()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("contextIO() error = %v, want context.Canceled", err)
	}
	if got := recorder.last(); got.IsZero() {
		t.Fatal("late contextIO completion cleared deadline after terminal close began")
	}
}

func TestWebSocketTransportPayloadOwnershipAcrossReads(t *testing.T) {
	t.Parallel()
	transport, server := newPipeWebSocketTransport(t)
	go func() { _ = server.WriteMessage(gows.OpcodeText, []byte(`{"value":"first"}`)) }()
	first, err := transport.ReadJSON(t.Context())
	if err != nil {
		t.Fatalf("first ReadJSON() error = %v", err)
	}
	want := append([]byte(nil), first...)
	go func() { _ = server.WriteMessage(gows.OpcodeText, []byte(`{"value":"second-and-longer"}`)) }()
	if _, err := transport.ReadJSON(t.Context()); err != nil {
		t.Fatalf("second ReadJSON() error = %v", err)
	}
	if string(first) != string(want) {
		t.Fatalf("first payload mutated after second read: got %q want %q", first, want)
	}
}

func TestWebSocketTypedPeerErrorsAreSanitized(t *testing.T) {
	t.Parallel()
	const secret = "status-reflected-secret"
	redactor := transportRedactor{secrets: []string{secret}}
	status := &gows.UnexpectedStatusError{StatusCode: http.StatusUnauthorized, Reason: "Bearer " + secret}
	err := websocketDialError("websocket dial failed", status, redactor)
	if strings.Contains(err.Error(), secret) {
		t.Fatal("websocketDialError() text leaked peer-reflected secret")
	}
	typed, ok := errors.AsType[*gows.UnexpectedStatusError](err)
	if !ok || typed.StatusCode != http.StatusUnauthorized || typed.Reason != "[redacted peer reason]" {
		t.Fatalf("sanitized typed status = %#v, want code 401 and redacted reason", typed)
	}
	if !errors.Is(err, gows.ErrUnexpectedStatus) {
		t.Fatalf("websocketDialError() = %v, want ErrUnexpectedStatus identity", err)
	}

	proxyErr := fmt.Errorf("%w: %w", gows.ErrProxyConnectFailed, status)
	err = websocketDialError("websocket dial failed", proxyErr, redactor)
	if !errors.Is(err, gows.ErrProxyConnectFailed) {
		t.Fatalf("proxy websocketDialError() = %v, want ErrProxyConnectFailed identity", err)
	}
	typed, ok = errors.AsType[*gows.UnexpectedStatusError](err)
	if !ok || strings.Contains(typed.Reason, secret) {
		t.Fatalf("proxy sanitized typed status = %#v, want secret-free copy", typed)
	}

	if got := (transportRedactor{}).sanitize("maintenance restart"); got != "maintenance restart" {
		t.Fatalf("safe close reason = %q, want preserved safe text", got)
	}
	transport, _ := newPipeWebSocketTransport(t)
	transport.redactor = transportRedactor{}
	if got := transport.sanitizedCloseError(&gows.CloseError{Code: gows.CloseGoingAway, Reason: "maintenance restart"}); got.Reason != "maintenance restart" {
		t.Fatalf("sanitized harmless CloseError reason = %q, want preserved", got.Reason)
	}
}

func TestWebSocketTransportCancellationCloseRaceReturnsCanonicalError(t *testing.T) {
	t.Parallel()
	transport, _ := newPipeWebSocketTransport(t)
	transport.closeTimeout = 20 * time.Millisecond
	<-transport.writeGate
	ctx, cancel := context.WithCancel(t.Context())
	atBoundary := make(chan struct{})
	release := make(chan struct{})
	transport.beforeTerminate = func() {
		close(atBoundary)
		<-release
	}
	writeDone := make(chan error, 1)
	closeDone := make(chan error, 1)
	go func() { writeDone <- transport.WriteJSON(ctx, []byte(`{"id":1}`)) }()
	cancel()
	<-atBoundary
	go func() { closeDone <- transport.Close() }()
	<-transport.done
	close(release)
	writeErr := <-writeDone
	transport.writeGate <- struct{}{}
	if err := <-closeDone; err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if writeErr != transport.terminalErr {
		t.Fatalf("cancellation-vs-close error = %p %v, want canonical %p %v", writeErr, writeErr, transport.terminalErr, transport.terminalErr)
	}
}

func TestWebSocketTransportPreservesCoderReadLimit(t *testing.T) {
	t.Parallel()
	transport, server := newPipeWebSocketTransport(t)
	writeDone := make(chan error, 1)
	go func() { _, _, _ = server.ReadMessage() }()
	go func() {
		writeDone <- server.WriteMessage(gows.OpcodeText, make([]byte, (32<<10)+1))
	}()

	_, err := transport.ReadJSON(t.Context())
	closed, ok := errors.AsType[*TransportClosedError](err)
	if !ok {
		t.Fatalf("ReadJSON() error = %v (%T), want *TransportClosedError", err, err)
	}
	cause, ok := errors.AsType[*gows.CloseError](closed)
	if !ok || cause.Code != gows.CloseMessageTooBig {
		t.Fatalf("typed cause = %#v, want close code %d", cause, gows.CloseMessageTooBig)
	}
	if err := <-writeDone; err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("server write error = %v", err)
	}
}

func TestWebSocketTransportReadLimitBoundary(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		size    int
		wantErr bool
	}{
		"success: 32768 bytes": {size: 32 << 10},
		"error: 32769 bytes":   {size: (32 << 10) + 1, wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			transport, server := newPipeWebSocketTransport(t)
			payload := make([]byte, tt.size)
			writeDone := make(chan error, 1)
			if tt.wantErr {
				go func() { _, _, _ = server.ReadMessage() }()
			}
			go func() { writeDone <- server.WriteMessage(gows.OpcodeText, payload) }()
			got, err := transport.ReadJSON(t.Context())
			if tt.wantErr {
				if _, ok := errors.AsType[*TransportClosedError](err); !ok {
					t.Fatalf("ReadJSON() error = %v (%T), want *TransportClosedError", err, err)
				}
			} else {
				if err != nil {
					t.Fatalf("ReadJSON() error = %v", err)
				}
				if len(got) != tt.size+1 || got[len(got)-1] != '\n' {
					t.Fatalf("ReadJSON() length/suffix = %d/%q, want %d/newline", len(got), got[len(got)-1:], tt.size+1)
				}
			}
			if err := <-writeDone; err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("server write error = %v", err)
			}
		})
	}
}

type closeCountingConn struct {
	net.Conn
	closes atomic.Int32
}

type deadlineRecorder struct {
	mu        sync.Mutex
	deadlines []time.Time
}

type writeSignalingConn struct {
	net.Conn
	once    sync.Once
	started chan struct{}
}

func (c *writeSignalingConn) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	return c.Conn.Write(p)
}

func (r *deadlineRecorder) set(deadline time.Time) error {
	r.mu.Lock()
	r.deadlines = append(r.deadlines, deadline)
	r.mu.Unlock()
	return nil
}

func (r *deadlineRecorder) last() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.deadlines[len(r.deadlines)-1]
}

func (c *closeCountingConn) Close() error {
	c.closes.Add(1)
	return c.Conn.Close()
}

func newPipeWebSocketTransport(t *testing.T) (*websocketTransport, *gows.Conn) {
	t.Helper()
	clientRaw, serverRaw := net.Pipe()
	t.Cleanup(func() {
		_ = clientRaw.Close()
		_ = serverRaw.Close()
	})
	return newWebsocketTransport(clientRaw, gows.Handshake{}, transportRedactor{secrets: []string{"secret peer detail"}}), gows.NewServerConn(serverRaw)
}
