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
	"io"
	"net"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type connectionTracker struct {
	t           testing.TB
	mu          sync.Mutex
	conns       []net.Conn
	workers     int
	completions chan struct{}
	closing     bool
}

func newConnectionTracker(t testing.TB) *connectionTracker {
	t.Helper()
	tracker := &connectionTracker{t: t, completions: make(chan struct{}, 128)}
	t.Cleanup(tracker.closeAndJoin)
	return tracker
}

func cleanupTrackedTestServer(t testing.TB, server *httptest.Server) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Config.Shutdown(ctx)
		server.CloseClientConnections()
		server.Close()
	})
}

func (t *connectionTracker) track(conn net.Conn) {
	t.mu.Lock()
	if t.closing {
		t.mu.Unlock()
		_ = conn.SetDeadline(time.Now())
		_ = conn.Close()
		return
	}
	t.conns = append(t.conns, conn)
	t.mu.Unlock()
}

func (t *connectionTracker) goCopy(dst, src net.Conn) {
	t.goWorker(func() { _, _ = io.Copy(dst, src) })
}

func (t *connectionTracker) goWorker(fn func()) {
	done, ok := t.registerWorker()
	if !ok {
		return
	}
	go func() { defer done(); fn() }()
}

func (t *connectionTracker) beginWorker() func() {
	done, ok := t.registerWorker()
	if !ok {
		return func() {}
	}
	return done
}

func (t *connectionTracker) registerWorker() (func(), bool) {
	t.mu.Lock()
	if t.closing {
		t.mu.Unlock()
		return nil, false
	}
	t.workers++
	t.mu.Unlock()
	return func() {
		t.mu.Lock()
		t.workers--
		closing := t.closing
		t.mu.Unlock()
		if closing {
			t.completions <- struct{}{}
		}
	}, true
}

func (t *connectionTracker) closeAndJoin() {
	t.mu.Lock()
	t.closing = true
	conns := append([]net.Conn(nil), t.conns...)
	workers := t.workers
	t.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for _, conn := range conns {
		_ = conn.SetDeadline(deadline)
		_ = conn.Close()
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	for range workers {
		select {
		case <-t.completions:
		case <-timer.C:
			t.t.Error("timed out joining tracked connection workers")
			return
		}
	}
	t.mu.Lock()
	remaining := t.workers
	t.mu.Unlock()
	if remaining != 0 {
		t.t.Errorf("tracked connection workers still outstanding = %d, want 0", remaining)
	}
}
