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
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	gocmp "github.com/google/go-cmp/cmp"
)

// TestExecServerByteChunkRoundTrip verifies the base64 JSON wire format.
func TestExecServerByteChunkRoundTrip(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input       ByteChunk
		wantJSON    string
		wantDecoded []byte
	}{
		"success: ascii bytes": {
			input:       ByteChunk([]byte("hello, world")),
			wantJSON:    `"aGVsbG8sIHdvcmxk"`,
			wantDecoded: []byte("hello, world"),
		},
		"success: binary bytes": {
			input:       ByteChunk([]byte{0x00, 0x01, 0x02, 0xff}),
			wantJSON:    `"AAEC/w=="`,
			wantDecoded: []byte{0x00, 0x01, 0x02, 0xff},
		},
		"success: nil bytes decode to nil": {
			input:       nil,
			wantJSON:    `""`,
			wantDecoded: nil,
		},
		"success: empty bytes decode to nil": {
			input:       ByteChunk{},
			wantJSON:    `""`,
			wantDecoded: nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			raw, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if got := string(raw); got != tt.wantJSON {
				t.Fatalf("Marshal() = %s, want %s", got, tt.wantJSON)
			}

			var decoded ByteChunk
			err = json.Unmarshal(raw, &decoded)
			if err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if diff := gocmp.Diff(tt.wantDecoded, []byte(decoded)); diff != "" {
				t.Fatalf("round-trip mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestExecServerByteChunkRejectsInvalidBase64 verifies invalid chunks fail.
func TestExecServerByteChunkRejectsInvalidBase64(t *testing.T) {
	t.Parallel()

	var decoded ByteChunk
	err := json.Unmarshal([]byte(`"not-base64!"`), &decoded)
	if err == nil {
		t.Fatal("Unmarshal() error = nil, want base64 decode error")
	}
}

func TestExecServerProcessRequestWrappers(t *testing.T) {
	t.Parallel()

	tr := newScriptTransport()
	client := NewExecServerClient(tr)
	t.Cleanup(func() {
		_ = client.Close()
	})

	tr.onWrite = func(data []byte, tr *scriptTransport) error {
		var req rpcMessage
		if err := json.Unmarshal(data, &req); err != nil {
			return err
		}
		switch req.Method {
		case ExecServerInitializeMethod:
			return tr.enqueueJSON(Object{"id": req.ID, "result": Object{"sessionId": "session-1"}})
		case ExecServerInitializedMethod:
			return nil
		case ExecServerProcessStartMethod:
			var got ExecServerProcessStartParams
			if err := json.Unmarshal(req.Params, &got); err != nil {
				return err
			}
			if got.ProcessID != "proc-1" || got.Cwd != "/tmp" || !got.TTY || !got.PipeStdin || got.Argv[0] != "/bin/sh" || got.Arg0 == nil || *got.Arg0 != "bash" {
				return errors.New("unexpected process/start params")
			}
			if got.Env["FOO"] != "bar" || got.EnvPolicy == nil || got.EnvPolicy.Inherit != "inherit" || !got.EnvPolicy.IgnoreDefaultExcludes {
				return errors.New("unexpected process/start env policy")
			}
			return tr.enqueueJSON(Object{"id": req.ID, "result": Object{"processId": "proc-1"}})
		case ExecServerProcessReadMethod:
			return tr.enqueueJSON(Object{"id": req.ID, "result": Object{
				"chunks": []Object{
					{"seq": 1, "stream": "stdout", "chunk": "aGVsbG8="},
					{"seq": 2, "stream": "stderr", "chunk": "d29ybGQ="},
				},
				"nextSeq":  3,
				"exited":   true,
				"exitCode": 7,
				"closed":   false,
				"failure":  nil,
			}})
		case ExecServerProcessWriteMethod:
			var got ExecServerProcessWriteParams
			if err := json.Unmarshal(req.Params, &got); err != nil {
				return err
			}
			if got.ProcessID != "proc-1" || string(got.Chunk) != "input" {
				return errors.New("unexpected process/write params")
			}
			return tr.enqueueJSON(Object{"id": req.ID, "result": Object{"status": ExecServerProcessWriteStatusAccepted}})
		case ExecServerProcessTerminateMethod:
			var got ExecServerProcessTerminateParams
			if err := json.Unmarshal(req.Params, &got); err != nil {
				return err
			}
			if got.ProcessID != "proc-1" {
				return errors.New("unexpected process/terminate params")
			}
			return tr.enqueueJSON(Object{"id": req.ID, "result": Object{"running": false}})
		default:
			return errors.New("unexpected request method: " + req.Method)
		}
	}

	initResp, err := client.Initialize(t.Context(), &ExecServerInitializeParams{ClientName: "codex-test"})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if initResp.SessionID != "session-1" {
		t.Fatalf("Initialize() sessionId = %q, want session-1", initResp.SessionID)
	}

	handle, err := client.ProcessStart(t.Context(), &ExecServerProcessStartParams{
		ProcessID: "proc-1",
		Argv:      []string{"/bin/sh", "-lc", "echo hello"},
		Cwd:       "/tmp",
		EnvPolicy: &ExecServerEnvPolicy{
			Inherit:               "inherit",
			IgnoreDefaultExcludes: true,
			Exclude:               []string{"PATH"},
			Set:                   map[string]string{"X": "1"},
			IncludeOnly:           []string{"HOME"},
		},
		Env:       map[string]string{"FOO": "bar"},
		TTY:       true,
		PipeStdin: true,
		Arg0:      func() *string { s := "bash"; return &s }(),
	})
	if err != nil {
		t.Fatalf("ProcessStart() error = %v", err)
	}
	if got, want := handle.ID(), ProcessID("proc-1"); got != want {
		t.Fatalf("ProcessStart() handle id = %q, want %q", got, want)
	}

	readResp, err := handle.Read(t.Context(), &ExecServerProcessReadParams{AfterSeq: func() *uint64 { v := uint64(0); return &v }()})
	if err != nil {
		t.Fatalf("ProcessRead() error = %v", err)
	}
	if got, want := len(readResp.Chunks), 2; got != want {
		t.Fatalf("ProcessRead() chunks = %d, want %d", got, want)
	}
	if diff := gocmp.Diff(uint64(3), readResp.NextSeq); diff != "" {
		t.Fatalf("ProcessRead() nextSeq mismatch (-want +got):\n%s", diff)
	}
	if !readResp.Exited || readResp.ExitCode == nil || *readResp.ExitCode != 7 {
		t.Fatalf("ProcessRead() exit state = %#v, want exited with exit code 7", readResp)
	}

	writeResp, err := handle.Write(t.Context(), ByteChunk([]byte("input")))
	if err != nil {
		t.Fatalf("ProcessWrite() error = %v", err)
	}
	if writeResp.Status != ExecServerProcessWriteStatusAccepted {
		t.Fatalf("ProcessWrite() status = %q, want accepted", writeResp.Status)
	}

	termResp, err := handle.Terminate(t.Context())
	if err != nil {
		t.Fatalf("ProcessTerminate() error = %v", err)
	}
	if termResp.Running {
		t.Fatalf("ProcessTerminate() running = true, want false")
	}

	if got, want := len(tr.writes), 6; got != want {
		t.Fatalf("transport writes = %d, want %d", got, want)
	}
}

func TestExecServerProcessNotificationsAreOrderedBySeq(t *testing.T) {
	t.Parallel()

	client := NewExecServerClient(nil)
	handle := &ExecServerProcessHandle{client: client, processID: "proc-ordered"}

	routeErr := client.routeNotification(Notification{
		Method: ExecServerProcessOutputMethod,
		Params: mustJSON(t, ExecServerProcessOutputNotification{
			ProcessID: "proc-ordered",
			Seq:       2,
			Stream:    ExecServerProcessOutputStreamStdout,
			Chunk:     ByteChunk([]byte("two")),
		}),
	})
	if routeErr != nil {
		t.Fatalf("routeNotification(output) error = %v", routeErr)
	}
	routeErr = client.routeNotification(Notification{
		Method: ExecServerProcessClosedMethod,
		Params: mustJSON(t, ExecServerProcessClosedNotification{
			ProcessID: "proc-ordered",
			Seq:       3,
		}),
	})
	if routeErr != nil {
		t.Fatalf("routeNotification(closed) error = %v", routeErr)
	}
	routeErr = client.routeNotification(Notification{
		Method: ExecServerProcessExitedMethod,
		Params: mustJSON(t, ExecServerProcessExitedNotification{
			ProcessID: "proc-ordered",
			Seq:       1,
			ExitCode:  9,
		}),
	})
	if routeErr != nil {
		t.Fatalf("routeNotification(exited) error = %v", routeErr)
	}

	first, err := handle.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification() first error = %v", err)
	}
	if got, ok := first.(ExecServerProcessExitedNotification); !ok || got.Seq != 1 || got.ExitCode != 9 {
		t.Fatalf("first notification = %#v, want exited seq 1", first)
	}

	second, err := handle.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification() second error = %v", err)
	}
	if got, ok := second.(ExecServerProcessOutputNotification); !ok || got.Seq != 2 || string(got.Chunk) != "two" {
		t.Fatalf("second notification = %#v, want output seq 2", second)
	}

	third, err := handle.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification() third error = %v", err)
	}
	if got, ok := third.(ExecServerProcessClosedNotification); !ok || got.Seq != 3 {
		t.Fatalf("third notification = %#v, want closed seq 3", third)
	}
}

func TestExecServerProcessClosedNotificationRemovesQueue(t *testing.T) {
	t.Parallel()

	client := NewExecServerClient(nil)
	queue := client.ensureProcessQueue("proc-closed")
	handle := &ExecServerProcessHandle{client: client, processID: "proc-closed", processQueue: queue}

	if err := client.routeNotification(Notification{
		Method: ExecServerProcessClosedMethod,
		Params: mustJSON(t, ExecServerProcessClosedNotification{
			ProcessID: "proc-closed",
			Seq:       2,
		}),
	}); err != nil {
		t.Fatalf("routeNotification(closed) error = %v", err)
	}
	if err := client.routeNotification(Notification{
		Method: ExecServerProcessExitedMethod,
		Params: mustJSON(t, ExecServerProcessExitedNotification{
			ProcessID: "proc-closed",
			Seq:       1,
			ExitCode:  0,
		}),
	}); err != nil {
		t.Fatalf("routeNotification(exited) error = %v", err)
	}
	assertExecServerProcessQueueCount(t, client, 1)

	first, err := handle.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification() first error = %v", err)
	}
	if got, ok := first.(ExecServerProcessExitedNotification); !ok || got.Seq != 1 {
		t.Fatalf("first notification = %#v, want exited seq 1", first)
	}
	assertExecServerProcessQueueCount(t, client, 1)

	second, err := handle.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification() second error = %v", err)
	}
	if got, ok := second.(ExecServerProcessClosedNotification); !ok || got.Seq != 2 {
		t.Fatalf("second notification = %#v, want closed seq 2", second)
	}
	assertExecServerProcessQueueCount(t, client, 0)

	_, err = handle.NextNotification(t.Context())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("NextNotification() after closed error = %v, want io.EOF", err)
	}
	assertExecServerProcessQueueCount(t, client, 0)
}

// TestExecServerProcessMalformedNotificationFailsQueue verifies malformed
// process notifications fail waiting handles.
func TestExecServerProcessMalformedNotificationFailsQueue(t *testing.T) {
	t.Parallel()

	tr := newScriptTransport()
	client := NewExecServerClient(tr)
	t.Cleanup(func() {
		_ = client.Close()
	})

	handle := &ExecServerProcessHandle{client: client, processID: "proc-invalid"}
	client.ensureProcessQueue("proc-invalid")
	err := tr.enqueueJSON(rpcMessage{
		Method: ExecServerProcessOutputMethod,
		Params: mustJSON(t, map[string]any{
			"processId": "proc-invalid",
			"seq":       1,
			"stream":    "stdout",
			"chunk":     123,
		}),
	})
	if err != nil {
		t.Fatalf("enqueue malformed process notification error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	_, err = handle.NextNotification(ctx)
	if err == nil {
		t.Fatal("NextNotification() error = nil, want malformed notification failure")
	}
	if !strings.Contains(err.Error(), "decode process/output notification") {
		t.Fatalf("NextNotification() error = %v, want decode process/output notification context", err)
	}
}

// TestExecServerProcessNotificationsRequireRoutingFields verifies
// required routing metadata.
func TestExecServerProcessNotificationsRequireRoutingFields(t *testing.T) {
	t.Parallel()

	client := NewExecServerClient(nil)
	tests := map[string]struct {
		wantErr      string
		notification Notification
	}{
		"error: missing process id": {
			wantErr: "missing processId",
			notification: Notification{
				Method: ExecServerProcessOutputMethod,
				Params: mustJSON(t, map[string]any{
					"seq":    1,
					"stream": "stdout",
					"chunk":  "b2s=",
				}),
			},
		},
		"error: missing seq": {
			wantErr: "missing seq",
			notification: Notification{
				Method: ExecServerProcessExitedMethod,
				Params: mustJSON(t, map[string]any{
					"processId": "proc-invalid",
					"exitCode":  7,
				}),
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := client.routeNotification(tt.notification)
			if err == nil {
				t.Fatal("routeNotification() error = nil, want validation failure")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("routeNotification() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func assertExecServerProcessQueueCount(t *testing.T, client *ExecServerClient, want int) {
	t.Helper()

	client.processMu.Lock()
	got := len(client.processQueues)
	client.processMu.Unlock()
	if got != want {
		t.Fatalf("process queue count = %d, want %d", got, want)
	}
}

type scriptTransport struct {
	mu      sync.Mutex
	reads   chan []byte
	writes  [][]byte
	closed  bool
	onWrite func([]byte, *scriptTransport) error
}

func newScriptTransport() *scriptTransport {
	return &scriptTransport{reads: make(chan []byte, 16)}
}

func (t *scriptTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	close(t.reads)
	return nil
}

func (t *scriptTransport) WriteJSON(_ context.Context, data []byte) error {
	t.mu.Lock()
	t.writes = append(t.writes, append([]byte(nil), data...))
	hook := t.onWrite
	t.mu.Unlock()
	if hook != nil {
		return hook(data, t)
	}
	return nil
}

func (t *scriptTransport) ReadJSON(ctx context.Context) ([]byte, error) {
	select {
	case data, ok := <-t.reads:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *scriptTransport) enqueueJSON(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return io.EOF
	}
	t.reads <- raw
	return nil
}
