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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zchee/gows"
)

func TestDialWebSocketPipelinedUpgradeBytesArePreserved(t *testing.T) {
	t.Parallel()
	tracker := newConnectionTracker(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		coalesced := &coalescingResponseWriter{ResponseWriter: w}
		raw, hs, err := gows.UpgradeHTTP(coalesced, r)
		if err != nil {
			t.Errorf("UpgradeHTTP() error = %v", err)
			return
		}
		tracker.track(raw)
		conn := gows.NewServerConn(raw, gows.WithBuffered(hs.Buffered))
		if err := conn.WriteMessage(gows.OpcodeText, []byte(`{"pipelined":true}`)); err != nil {
			t.Errorf("WriteMessage() error = %v", err)
		}
	}))
	t.Cleanup(server.Close)

	transport, err := dialWebSocket(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http"), nil, nil, "")
	if err != nil {
		t.Fatalf("dialWebSocket() error = %v", err)
	}
	got, err := transport.ReadJSON(t.Context())
	if err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	if string(got) != "{\"pipelined\":true}\n" {
		t.Fatalf("ReadJSON() = %q, want pipelined frame", got)
	}
	_ = transport.closeRaw()
}

func TestDialWebSocketRedirectAuthorizationPolicyAndCap(t *testing.T) {
	t.Parallel()
	const token = "redirect-secret"
	tracker := newConnectionTracker(t)
	var crossAuth string
	cross := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		crossAuth = r.Header.Get("Authorization")
		closeUpgradedConnection(t, tracker, w, r)
	}))
	t.Cleanup(cross.Close)

	var initialAuth, inheritedAuth string
	mux := http.NewServeMux()
	same := httptest.NewServer(mux)
	t.Cleanup(same.Close)
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		initialAuth = r.Header.Get("Authorization")
		http.Redirect(w, r, "/same", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/same", func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		inheritedAuth = r.Header.Get("Authorization")
		http.Redirect(w, r, cross.URL, http.StatusTemporaryRedirect)
	})

	u, _ := url.Parse("ws" + strings.TrimPrefix(same.URL, "http") + "/start")
	transport, err := dialWebSocketURL(t.Context(), u, token, nil, nil)
	if err != nil {
		t.Fatalf("dialWebSocketURL() redirect error = %v", err)
	}
	_ = transport.closeRaw()
	if initialAuth != "Bearer "+token || inheritedAuth != "Bearer "+token {
		t.Fatalf("same-origin Authorization = initial %q inherited %q", initialAuth, inheritedAuth)
	}
	if crossAuth != "" {
		t.Fatalf("cross-origin Authorization = %q, want stripped", crossAuth)
	}

	loop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		http.Redirect(w, r, r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(loop.Close)
	loopURL, _ := url.Parse("ws" + strings.TrimPrefix(loop.URL, "http") + "/loop")
	_, err = dialWebSocketURL(t.Context(), loopURL, token, nil, nil)
	if !errors.Is(err, gows.ErrTooManyRedirects) {
		t.Fatalf("redirect loop error = %v, want ErrTooManyRedirects", err)
	}
}

func TestDialWebSocketWSSUsesConfiguredTrust(t *testing.T) {
	t.Parallel()
	tracker := newConnectionTracker(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		closeUpgradedConnection(t, tracker, w, r)
	}))
	t.Cleanup(server.Close)
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	u, _ := url.Parse("wss" + strings.TrimPrefix(server.URL, "https"))
	if _, err := dialWebSocket(t.Context(), u.String(), nil, nil, ""); err == nil {
		t.Fatal("public dialWebSocket() trusted an untrusted test certificate")
	}
	transport, err := dialWebSocketURL(t.Context(), u, "", nil, &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("trusted wss dial error = %v", err)
	}
	_ = transport.closeRaw()
}

func TestDialWebSocketWSSDowngradeStripsAuthorization(t *testing.T) {
	t.Parallel()
	tracker := newConnectionTracker(t)
	var downgradedAuth string
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		downgradedAuth = r.Header.Get("Authorization")
		closeUpgradedConnection(t, tracker, w, r)
	}))
	t.Cleanup(plain.Close)
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		http.Redirect(w, r, "ws"+strings.TrimPrefix(plain.URL, "http"), http.StatusTemporaryRedirect)
	}))
	t.Cleanup(secure.Close)
	pool := x509.NewCertPool()
	pool.AddCert(secure.Certificate())
	u, _ := url.Parse("wss" + strings.TrimPrefix(secure.URL, "https"))
	transport, err := dialWebSocketURL(t.Context(), u, "downgrade-secret", nil, &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("wss downgrade dial error = %v", err)
	}
	_ = transport.closeRaw()
	if downgradedAuth != "" {
		t.Fatalf("downgraded ws Authorization = %q, want stripped", downgradedAuth)
	}
}

func TestDialWebSocketProxyAndWSSConnectCredentialIsolation(t *testing.T) {
	t.Parallel()
	tracker := newConnectionTracker(t)
	const proxyAuthorization = "Basic cHJveHktdXNlcjpwcm94eS1wYXNz"
	wsProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		if got := r.Header.Get("Proxy-Authorization"); got != proxyAuthorization {
			t.Errorf("ws proxy authorization = %q, want %q", got, proxyAuthorization)
		}
		if !strings.HasPrefix(r.RequestURI, "http://origin.invalid/") {
			t.Errorf("ws proxy RequestURI = %q, want absolute form", r.RequestURI)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer origin-token" {
			t.Errorf("origin Authorization at proxy = %q, want independently preserved", got)
		}
		closeUpgradedConnection(t, tracker, w, r)
	}))
	t.Cleanup(wsProxy.Close)
	proxyURL, _ := url.Parse(wsProxy.URL)
	proxyURL.User = url.UserPassword("proxy-user", "proxy-pass")
	origin, _ := url.Parse("ws://origin.invalid/socket")
	transport, err := dialWebSocketURL(t.Context(), origin, "origin-token", func(*http.Request) (*url.URL, error) { return proxyURL, nil }, nil)
	if err != nil {
		t.Fatalf("ws proxy dial error = %v", err)
	}
	_ = transport.closeRaw()

	var originProxyAuth string
	tlsOrigin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		originProxyAuth = r.Header.Get("Proxy-Authorization")
		if got := r.Header.Get("Authorization"); got != "Bearer origin-token" {
			t.Errorf("wss origin Authorization = %q, want bearer", got)
		}
		closeUpgradedConnection(t, tracker, w, r)
	}))
	t.Cleanup(tlsOrigin.Close)
	connectProxy := newConnectProxy(t, tracker, proxyAuthorization)
	connectURL, _ := url.Parse(connectProxy.URL)
	connectURL.User = url.UserPassword("proxy-user", "proxy-pass")
	pool := x509.NewCertPool()
	pool.AddCert(tlsOrigin.Certificate())
	wssURL, _ := url.Parse("wss" + strings.TrimPrefix(tlsOrigin.URL, "https"))
	transport, err = dialWebSocketURL(t.Context(), wssURL, "origin-token", func(*http.Request) (*url.URL, error) { return connectURL, nil }, &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("wss CONNECT proxy dial error = %v", err)
	}
	_ = transport.closeRaw()
	if originProxyAuth != "" {
		t.Fatalf("origin received Proxy-Authorization %q, want isolated to proxy leg", originProxyAuth)
	}
}

func TestDialWebSocketStatefulProxyIsCalledOnceAndErrorsAreRedacted(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	u, _ := url.Parse("ws://origin.invalid/socket?session=partial-query-secret")
	_, err := dialWebSocketURL(t.Context(), u, "bearer-secret", func(*http.Request) (*url.URL, error) {
		calls.Add(1)
		return nil, errors.New("proxy credentials proxy-user:proxy-password partial-query-secret bearer-secret")
	}, nil)
	if got := calls.Load(); got != 1 {
		t.Fatalf("proxy callback calls = %d, want exactly 1", got)
	}
	for _, secret := range []string{"proxy-user", "proxy-password", "partial-query-secret", "bearer-secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("dial error leaked %q: %v", secret, err)
		}
	}
	if errors.Unwrap(err) != nil {
		t.Fatalf("generic sanitized dial error unwrap = %v, want nil", errors.Unwrap(err))
	}

	redactor := newTransportRedactor(u, "bearer-secret")
	proxyURL, _ := url.Parse("http://proxy-user:proxy-password@proxy.invalid")
	redactor.addURL(proxyURL)
	status := &gows.UnexpectedStatusError{StatusCode: http.StatusForbidden, Reason: "partial-query-secret"}
	typedErr := websocketDialError("websocket dial failed", status, redactor)
	typed, ok := errors.AsType[*gows.UnexpectedStatusError](typedErr)
	if !ok || typed.Reason != "[redacted peer reason]" {
		t.Fatalf("partial query reflection typed cause = %#v, want redacted", typed)
	}
	status = &gows.UnexpectedStatusError{StatusCode: http.StatusProxyAuthRequired, Reason: "cHJveHktdXNlcjpwcm94eS1wYXNzd29yZA=="}
	typedErr = websocketDialError("websocket dial failed", fmt.Errorf("%w: %w", gows.ErrProxyConnectFailed, status), redactor)
	typed, ok = errors.AsType[*gows.UnexpectedStatusError](typedErr)
	if !ok || typed.Reason != "[redacted peer reason]" || !errors.Is(typedErr, gows.ErrProxyConnectFailed) {
		t.Fatalf("encoded proxy credential typed cause = %#v err=%v, want redacted with proxy sentinel", typed, typedErr)
	}
}

func closeUpgradedConnection(t *testing.T, tracker *connectionTracker, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	raw, _, err := gows.UpgradeHTTP(w, r)
	if err != nil {
		t.Errorf("UpgradeHTTP() error = %v", err)
		return
	}
	tracker.track(raw)
}

func newConnectProxy(t *testing.T, tracker *connectionTracker, wantAuthorization string) *httptest.Server {
	t.Helper()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer tracker.beginWorker()()
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		if got := r.Header.Get("Proxy-Authorization"); got != wantAuthorization {
			t.Errorf("CONNECT proxy authorization = %q, want %q", got, wantAuthorization)
		}
		upstream, err := net.DialTimeout("tcp", r.Host, time.Second)
		if err != nil {
			http.Error(w, "connect failed", http.StatusBadGateway)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			upstream.Close()
			t.Error("proxy ResponseWriter does not implement http.Hijacker")
			return
		}
		client, rw, err := hijacker.Hijack()
		if err != nil {
			upstream.Close()
			t.Errorf("Hijack() error = %v", err)
			return
		}
		tracker.track(client)
		tracker.track(upstream)
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		tracker.goCopy(upstream, client)
		tracker.goCopy(client, upstream)
	}))
	t.Cleanup(proxy.Close)
	return proxy
}

type coalescingResponseWriter struct {
	http.ResponseWriter
}

func (w *coalescingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	raw, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, err
	}
	coalesced := &coalescingConn{Conn: raw}
	return coalesced, bufio.NewReadWriter(rw.Reader, bufio.NewWriter(coalesced)), nil
}

type coalescingConn struct {
	net.Conn
	first []byte
}

func (c *coalescingConn) Write(p []byte) (int, error) {
	if c.first == nil {
		c.first = append([]byte(nil), p...)
		return len(p), nil
	}
	combined := append(c.first, p...)
	c.first = []byte{}
	n, err := c.Conn.Write(combined)
	if n >= len(combined) {
		return len(p), err
	}
	if n <= len(combined)-len(p) {
		return 0, err
	}
	return n - (len(combined) - len(p)), err
}

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
