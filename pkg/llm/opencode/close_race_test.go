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

package opencode

import (
	"errors"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

// waitForGoroutines polls until the live goroutine count settles at or below
// baseline+slack, dumping stacks on timeout.
func waitForGoroutines(t *testing.T, baseline, slack int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		runtime.GC()
		if count := runtime.NumGoroutine(); count <= baseline+slack {
			return
		}
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			t.Fatalf("goroutines never settled (baseline %d, slack %d, now %d):\n%s",
				baseline, slack, runtime.NumGoroutine(), buf[:n])
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestCloseDuringInFlightTurn is AC11: Close during an in-flight turn
// unblocks Stream/Run callers, refuses new turns, and reaps every goroutine
// the wrapper spawned (asserted via goroutine-count delta under -race).
//
// Deliberately not parallel: it accounts for global goroutine counts.
func TestCloseDuringInFlightTurn(t *testing.T) { //nolint:paralleltest // goroutine accounting requires isolation
	baseline := runtime.NumGoroutine()

	fake := newFakeOpencode()
	fake.promptBlock = make(chan struct{})
	server := httptest.NewServer(fake.handler())

	oc, err := NewRemoteOpencode(t.Context(), &RemoteConfig{
		BaseURL:     server.URL,
		DialTimeout: 10 * time.Second,
		DrainWindow: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewRemoteOpencode: %v", err)
	}

	session, err := oc.SessionStart(t.Context(), nil)
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	handle, err := session.Turn(t.Context(), "long task", nil)
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	select {
	case <-fake.promptStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("prompt never started")
	}

	runResult := make(chan error, 1)
	go func() {
		_, err := handle.Run(t.Context())
		runResult <- err
	}()
	streamResult := make(chan error, 1)
	go func() {
		// The concurrent-consumer contract makes this second consumer fail
		// fast — asserting Close does not deadlock even with contending
		// consumers.
		_, err := handle.Run(t.Context())
		streamResult <- err
	}()

	closeDone := make(chan error, 1)
	go func() { closeDone <- oc.Close() }()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Close hung during in-flight turn")
	}

	for _, ch := range []chan error{runResult, streamResult} {
		select {
		case err := <-ch:
			if err == nil {
				t.Error("consumer resolved without error despite Close mid-turn")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("turn consumer still blocked after Close")
		}
	}

	// New work is refused after Close.
	if _, err := session.Turn(t.Context(), "again", nil); !errors.Is(err, errClientClosed) {
		t.Errorf("Turn after Close = %v, want errClientClosed", err)
	}
	if _, err := session.Run(t.Context(), "again", nil); !errors.Is(err, errClientClosed) {
		t.Errorf("Run after Close = %v, want errClientClosed", err)
	}

	server.Close()
	waitForGoroutines(t, baseline, 4)
}

// TestAbandonedTurnHandleLeaksNothing is the AC11 abandonment half: a turn
// whose handle is never consumed leaves no goroutine behind after the turn
// completes and the client closes.
func TestAbandonedTurnHandleLeaksNothing(t *testing.T) { //nolint:paralleltest // goroutine accounting requires isolation
	baseline := runtime.NumGoroutine()

	fake := newFakeOpencode()
	server := httptest.NewServer(fake.handler())

	oc, err := NewRemoteOpencode(t.Context(), &RemoteConfig{
		BaseURL:     server.URL,
		DialTimeout: 10 * time.Second,
		DrainWindow: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewRemoteOpencode: %v", err)
	}

	session, err := oc.SessionStart(t.Context(), nil)
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if _, err := session.Turn(t.Context(), "fire and forget", nil); err != nil {
		t.Fatalf("Turn: %v", err)
	}

	// The abandoned turn completes server-side and must release the session's
	// one-active-turn slot on its own (the buffered outcome channel means the
	// prompt goroutine never blocks on the missing consumer).
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := session.Run(t.Context(), "next turn works", nil); err == nil {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("session slot never released by abandoned turn: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := oc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	server.Close()
	waitForGoroutines(t, baseline, 4)
}
