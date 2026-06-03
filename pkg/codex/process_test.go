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
	"strings"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

func TestAppServerProcessHandleRoutesRegisteredNotifications(t *testing.T) {
	client, _ := newProcessHandleTestClient(t)

	handle, err := client.SpawnProcess(t.Context(), &ProcessSpawnParams{
		ProcessHandle:      "proc-registered",
		Command:            []string{"/bin/echo", "ok"},
		Cwd:                "/tmp",
		StreamStdoutStderr: true,
	})
	if err != nil {
		t.Fatalf("SpawnProcess() error = %v", err)
	}
	if got, want := handle.ID(), "proc-registered"; got != want {
		t.Fatalf("ID() = %q, want %q", got, want)
	}

	if err := client.routeNotification(processOutputNotification(t, "proc-unregistered", "ignored")); err != nil {
		t.Fatalf("routeNotification(unregistered process) error = %v", err)
	}
	global, err := client.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification() unregistered process error = %v", err)
	}
	if global.Method != NotificationMethodProcessOutputDelta {
		t.Fatalf("NextNotification().Method = %q, want %s", global.Method, NotificationMethodProcessOutputDelta)
	}

	if err := client.routeNotification(processOutputNotification(t, "proc-registered", "aGVsbG8=")); err != nil {
		t.Fatalf("routeNotification(registered output) error = %v", err)
	}
	output, err := handle.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification(output) error = %v", err)
	}
	if output.OutputDelta == nil || output.OutputDelta.DeltaBase64 != "aGVsbG8=" || output.Exited != nil {
		t.Fatalf("NextNotification(output) = %#v, want output delta", output)
	}

	if err := client.routeNotification(processExitedNotification(t, "proc-registered", 0)); err != nil {
		t.Fatalf("routeNotification(exit) error = %v", err)
	}
	exited, err := handle.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification(exit) error = %v", err)
	}
	if exited.Exited == nil || exited.Exited.ExitCode != 0 || exited.OutputDelta != nil {
		t.Fatalf("NextNotification(exit) = %#v, want exit code 0", exited)
	}

	if err := client.routeNotification(processOutputNotification(t, "proc-registered", "Z2xvYmFs")); err != nil {
		t.Fatalf("routeNotification(after exit) error = %v", err)
	}
	afterExit, err := client.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification(after exit) error = %v", err)
	}
	if afterExit.Method != NotificationMethodProcessOutputDelta {
		t.Fatalf("NextNotification(after exit).Method = %q, want process output", afterExit.Method)
	}
}

func TestAppServerProcessHandleMethodsUseProcessHandle(t *testing.T) {
	client, methods := newProcessHandleTestClient(t)

	handle, err := client.SpawnProcess(t.Context(), &ProcessSpawnParams{
		ProcessHandle: "proc-methods",
		Command:       []string{"/bin/cat"},
		Cwd:           "/tmp",
		StreamStdin:   true,
	})
	if err != nil {
		t.Fatalf("SpawnProcess() error = %v", err)
	}

	stdin := "aGVsbG8="
	if _, err := handle.WriteStdin(t.Context(), &stdin, false); err != nil {
		t.Fatalf("WriteStdin() error = %v", err)
	}
	if _, err := handle.ResizePty(t.Context(), ProcessTerminalSize{Cols: 80, Rows: 24}); err != nil {
		t.Fatalf("ResizePty() error = %v", err)
	}
	if _, err := handle.Kill(t.Context()); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}

	want := []string{
		RequestMethodProcessSpawn,
		RequestMethodProcessWriteStdin,
		RequestMethodProcessResizePty,
		RequestMethodProcessKill,
	}
	if got := *methods; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("methods = %v, want %v", got, want)
	}
}

func TestAppServerProcessHandleMalformedNotificationSurfacesError(t *testing.T) {
	client, _ := newProcessHandleTestClient(t)
	handle, err := client.SpawnProcess(t.Context(), &ProcessSpawnParams{
		ProcessHandle: "proc-malformed",
		Command:       []string{"/bin/true"},
		Cwd:           "/tmp",
	})
	if err != nil {
		t.Fatalf("SpawnProcess() error = %v", err)
	}

	if err := client.routeNotification(Notification{
		Method: NotificationMethodProcessExited,
		Params: jsontext.Value(`{
			"processHandle":"proc-malformed",
			"exitCode":"not-an-int",
			"stdout":"",
			"stderr":"",
			"stdoutCapReached":false,
			"stderrCapReached":false
		}`),
	}); err != nil {
		t.Fatalf("routeNotification(malformed) error = %v", err)
	}
	if _, err := handle.NextNotification(t.Context()); err == nil {
		t.Fatal("NextNotification(malformed) error = nil, want decode error")
	}
}

func TestAppServerProcessHandleCloseUnblocksWaiter(t *testing.T) {
	client, _ := newProcessHandleTestClient(t)
	handle, err := client.SpawnProcess(t.Context(), &ProcessSpawnParams{
		ProcessHandle: "proc-close",
		Command:       []string{"/bin/sleep", "10"},
		Cwd:           "/tmp",
	})
	if err != nil {
		t.Fatalf("SpawnProcess() error = %v", err)
	}

	waiter := make(chan error, 1)
	go func() {
		_, err := handle.NextNotification(context.Background())
		waiter <- err
	}()

	if err := handle.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case err := <-waiter:
		if err == nil || (!strings.Contains(err.Error(), "process consumer closed") && !strings.Contains(err.Error(), "process consumer is not active")) {
			t.Fatalf("NextNotification() after Close error = %v, want process consumer closed/not active", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for process handle Close to unblock waiter")
	}
}

func newProcessHandleTestClient(t *testing.T) (*Client, *[]string) {
	t.Helper()

	tr := newScriptTransport()
	methods := []string{}
	tr.onWrite = func(data []byte, tr *scriptTransport) error {
		var req rpcMessage
		if err := json.Unmarshal(data, &req); err != nil {
			return err
		}
		if req.ID == "" {
			return nil
		}
		methods = append(methods, req.Method)
		switch req.Method {
		case RequestMethodProcessSpawn,
			RequestMethodProcessWriteStdin,
			RequestMethodProcessResizePty,
			RequestMethodProcessKill:
			return tr.enqueueJSON(Object{"id": req.ID, "result": Object{}})
		default:
			return errors.New("unexpected request method: " + req.Method)
		}
	}

	return newScriptedClient(t, tr), &methods
}

// newScriptedClient wires a Client onto a scripted transport and starts its
// read loop, mirroring the runtime setup that NewClient performs for a live
// process. The stderr stream is pre-closed because scripted transports carry no
// child process.
func newScriptedClient(t *testing.T, tr *scriptTransport) *Client {
	t.Helper()

	client := NewClient(&Config{}, nil)
	client.storeTransport(tr)
	client.rpcState = newJSONRPCClientState()
	client.turnRouter = newTurnNotificationRouter()
	client.readDone = make(chan struct{})
	client.stderrDone = make(chan struct{})
	close(client.stderrDone)
	go client.readLoop(t.Context(), client.loadTransport(), client.readDone)
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client
}

func processOutputNotification(t *testing.T, processHandle, deltaBase64 string) Notification {
	t.Helper()
	return Notification{
		Method: NotificationMethodProcessOutputDelta,
		Params: mustJSON(t, ProcessOutputDeltaNotification{
			ProcessHandle: processHandle,
			DeltaBase64:   deltaBase64,
			Stream:        ProcessOutputStreamValueStdout,
		}),
	}
}

func processExitedNotification(t *testing.T, processHandle string, exitCode int32) Notification {
	t.Helper()
	return Notification{
		Method: NotificationMethodProcessExited,
		Params: mustJSON(t, ProcessExitedNotification{
			ProcessHandle: processHandle,
			ExitCode:      exitCode,
		}),
	}
}
