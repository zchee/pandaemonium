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
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/zchee/pandaemonium/pkg/llm"
)

// memListener is an in-memory net.Listener whose connections are net.Pipe
// pairs. Pipe reads block on channels rather than socket syscalls, so HTTP
// served over it is durably blocking inside a testing/synctest bubble —
// unlike httptest.Server, whose TCP reads never look idle to synctest.Wait.
type memListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newMemListener() *memListener {
	return &memListener{conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *memListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *memListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *memListener) Addr() net.Addr {
	return &net.UnixAddr{Name: "mem", Net: "mem"}
}

// dial hands the server half of a fresh pipe to Accept and returns the
// client half.
func (l *memListener) dial(ctx context.Context) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case l.conns <- server:
		return client, nil
	case <-l.done:
		client.Close()
		server.Close()
		return nil, net.ErrClosed
	case <-ctx.Done():
		client.Close()
		server.Close()
		return nil, ctx.Err()
	}
}

// startFakeOpencodeMem is startFakeOpencode over the in-memory transport,
// for tests running inside a testing/synctest bubble. Cleanups unwind in
// LIFO order — client, transport pool, then server — so every goroutine
// exits before the bubble does.
func startFakeOpencodeMem(t *testing.T, fake *fakeOpencode, mutate func(*RemoteConfig)) *Opencode {
	t.Helper()

	listener := newMemListener()
	server := &http.Server{Handler: fake.handler()}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return listener.dial(ctx)
		},
	}
	t.Cleanup(transport.CloseIdleConnections)

	cfg := &RemoteConfig{
		// Loopback satisfies the plain-http guard; the address is never
		// dialed — DialContext above routes everything through the pipe.
		BaseURL:     "http://127.0.0.1:1",
		HTTPClient:  &http.Client{Transport: transport},
		DialTimeout: 10 * time.Second,
		DrainWindow: 300 * time.Millisecond,
		Retry:       llm.RetryConfig{MaxAttempts: 2, InitialDelay: 10 * time.Millisecond, MaxDelay: 20 * time.Millisecond, JitterRatio: -1},
	}
	if mutate != nil {
		mutate(cfg)
	}
	oc, err := NewRemoteOpencode(t.Context(), cfg)
	if err != nil {
		t.Fatalf("NewRemoteOpencode: %v", err)
	}
	t.Cleanup(func() { _ = oc.Close() })
	return oc
}
