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
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	gocmp "github.com/google/go-cmp/cmp"
)

func TestNormalizeInput(t *testing.T) {
	tests := map[string]struct {
		input any
		want  []Object
	}{
		"success: string becomes text input": {
			input: "hello",
			want:  []Object{{"type": "text", "text": "hello"}},
		},
		"success: typed inputs preserve order": {
			input: []any{
				TextInput{Text: "describe"},
				ImageInput{URL: "https://example.com/a.png"},
				LocalImageInput{Path: "/tmp/a.png"},
				SkillInput{Name: "go", Path: "/skills/go"},
				MentionInput{Name: "README", Path: "README.md"},
			},
			want: []Object{
				{"type": "text", "text": "describe"},
				{"type": "image", "url": "https://example.com/a.png"},
				{"type": "localImage", "path": "/tmp/a.png"},
				{"type": "skill", "name": "go", "path": "/skills/go"},
				{"type": "mention", "name": "README", "path": "README.md"},
			},
		},
		"success: raw object is accepted": {
			input: Object{"type": "text", "text": "raw"},
			want:  []Object{{"type": "text", "text": "raw"}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := normalizeInput(tt.input)
			if err != nil {
				t.Fatalf("normalizeInput() error = %v", err)
			}
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("normalizeInput() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDefaultCodexHome(t *testing.T) {
	got := DefaultCodexHome()
	if got != filepath.Clean(got) {
		t.Fatalf("DefaultCodexHome() = %q, want clean path", got)
	}
	if filepath.Base(got) != ".codex" {
		t.Fatalf("DefaultCodexHome() = %q, want .codex basename", got)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("DefaultCodexHome() = %q, want absolute path", got)
	}
}

func TestJSONRPCErrorMapping(t *testing.T) {
	tests := map[string]struct {
		code       int64
		message    string
		data       string
		kind       string
		retryable  bool
		retryLimit bool
		errorType  string
	}{
		"success: invalid params is not retryable": {
			code:      -32602,
			message:   "bad params",
			data:      `null`,
			kind:      "invalid_params",
			errorType: "*codex.InvalidParamsError",
		},
		"success: nested server overloaded is retryable": {
			code:      -32000,
			message:   "busy",
			data:      `{"errorInfo":{"reason":"server_overloaded"}}`,
			kind:      "server_busy",
			retryable: true,
			errorType: "*codex.ServerBusyError",
		},
		"success: nested codex_error_info is retryable": {
			code:      -32000,
			message:   "busy",
			data:      `{"codex_error_info":{"reason":"server_overloaded"}}`,
			kind:      "server_busy",
			retryable: true,
			errorType: "*codex.ServerBusyError",
		},
		"success: nested codexErrorInfo is retryable": {
			code:      -32000,
			message:   "busy",
			data:      `{"codexErrorInfo":{"reason":"server_overloaded"}}`,
			kind:      "server_busy",
			retryable: true,
			errorType: "*codex.ServerBusyError",
		},
		"success: retry limit with overload marker is classified": {
			code:       -32000,
			message:    "retry limit reached",
			data:       `{"codex_error_info":{"status":"server_overloaded"}}`,
			kind:       "retry_limit_exceeded",
			retryable:  true,
			retryLimit: true,
			errorType:  "*codex.RetryLimitExceededError",
		},
		"success: retry limit text is retryable but classified": {
			code:       -32000,
			message:    "too many failed attempts",
			data:       `{"codexErrorInfo":{"reason":"server_overloaded"}}`,
			kind:       "retry_limit_exceeded",
			retryable:  true,
			retryLimit: true,
			errorType:  "*codex.RetryLimitExceededError",
		},
		"success: parse error maps to ParseError": {
			code:      -32700,
			message:   "parse",
			data:      `null`,
			kind:      "parse_error",
			errorType: "*codex.ParseError",
		},
		"success: invalid request maps to InvalidRequestError": {
			code:      -32600,
			message:   "bad request",
			data:      `null`,
			kind:      "invalid_request",
			errorType: "*codex.InvalidRequestError",
		},
		"success: method not found maps to MethodNotFoundError": {
			code:      -32601,
			message:   "not found",
			data:      `null`,
			kind:      "method_not_found",
			errorType: "*codex.MethodNotFoundError",
		},
		"success: internal rpc maps to InternalRpcError": {
			code:      -32603,
			message:   "internal",
			data:      `null`,
			kind:      "internal_error",
			errorType: "*codex.InternalRPCError",
		},
		"success: app-server json-rpc range uses app-server error": {
			code:      -32010,
			message:   "app_server_error",
			data:      `{"reason":"ok"}`,
			kind:      "app_server_rpc",
			errorType: "*codex.AppServerRPCError",
		},
		"success: unknown code uses raw JsonRpcError": {
			code:      -1234,
			message:   "unknown",
			data:      `null`,
			kind:      "jsonrpc",
			errorType: "*codex.JSONRPCError",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := mapJSONRPCError(tt.code, tt.message, jsontext.Value(tt.data))
			if got := err.Error(); got == "" {
				t.Fatalf("mapped error message is empty")
			}
			rpcErr := asJSONRPCError(err)
			if rpcErr == nil {
				t.Fatalf("error type = %T, want json-rpc-compatible error", err)
			}
			if rpcErr.Kind != tt.kind {
				t.Fatalf("kind = %q, want %q", rpcErr.Kind, tt.kind)
			}
			if got := IsRetryableError(err); got != tt.retryable {
				t.Fatalf("IsRetryableError() = %v, want %v", got, tt.retryable)
			}
			if got, want := IsServerBusy(err), tt.retryable; got != want {
				t.Fatalf("IsServerBusy() = %v, want %v", got, want)
			}
			gotType := fmt.Sprintf("%T", err)
			if gotType != tt.errorType {
				t.Fatalf("error type = %s, want %s", gotType, tt.errorType)
			}
			if got, want := IsRetryLimitExceeded(err), tt.retryLimit; got != want {
				t.Fatalf("IsRetryLimitExceeded() = %v, want %v", got, want)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"success: typed server-busy is retryable": {
			err:  mapJSONRPCError(-32000, "server overloaded", jsontext.Value(`{"errorInfo":{"reason":"server_overloaded"}}`)),
			want: true,
		},
		"success: invalid request with overload marker is retryable": {
			err:  mapJSONRPCError(-32600, "bad request", jsontext.Value(`{"codexErrorInfo":"server_overloaded"}`)),
			want: true,
		},
		"success: plain non-overload JSON-RPC error is not retryable": {
			err:  mapJSONRPCError(-32602, "bad params", jsontext.Value(`null`)),
			want: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := IsRetryableError(tt.err); got != tt.want {
				t.Fatalf("IsRetryableError(%T) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsServerBusyAlias(t *testing.T) {
	err := mapJSONRPCError(-32000, "too many failed attempts", jsontext.Value(`{}`))
	if got, want := IsServerBusy(err), true; got != want {
		t.Fatalf("IsServerBusy() = %v, want %v", got, want)
	}
}

func TestRetryOnOverload(t *testing.T) {
	tests := map[string]struct {
		errs      []error
		wantCalls int
		wantErr   bool
		maxRetry  int
	}{
		"success: retries retryable overload": {
			errs: []error{
				mapJSONRPCError(-32000, "busy", jsontext.Value(`{"codexErrorInfo":"server_overloaded"}`)),
				nil,
			},
			wantCalls: 2,
			maxRetry:  3,
		},
		"success: does not retry invalid params": {
			errs:      []error{mapJSONRPCError(-32602, "bad", jsontext.Value(`null`))},
			wantCalls: 1,
			wantErr:   true,
			maxRetry:  3,
		},
		"success: stops after max attempts": {
			errs: []error{
				mapJSONRPCError(-32000, "busy", jsontext.Value(`{"errorInfo":{"reason":"server_overloaded"}}`)),
				mapJSONRPCError(-32000, "busy", jsontext.Value(`{"errorInfo":{"reason":"server_overloaded"}}`)),
				mapJSONRPCError(-32000, "busy", jsontext.Value(`{"errorInfo":{"reason":"server_overloaded"}}`)),
				nil,
			},
			wantCalls: 2,
			wantErr:   true,
			maxRetry:  2,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			calls := 0
			_, err := RetryOnOverload(t.Context(), RetryConfig{MaxAttempts: tt.maxRetry, InitialDelay: time.Nanosecond, MaxDelay: time.Nanosecond}, func() (string, error) {
				err := tt.errs[calls]
				calls++
				return "ok", err
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("RetryOnOverload() error = %v, wantErr %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Fatalf("calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

func TestClientRequestWithRetryOnOverloadRetriesAndReturnsResult(t *testing.T) {
	client := newHelperClient(t, "retry_on_overload")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	got, err := RequestWithRetryOnOverload[string](
		t.Context(), client,
		"ping",
		nil,
		RetryConfig{
			MaxAttempts:  3,
			InitialDelay: time.Nanosecond,
			MaxDelay:     time.Nanosecond,
		},
	)
	if err != nil {
		t.Fatalf("RequestWithRetryOnOverload() error = %v", err)
	}
	if got != "ok" {
		t.Fatalf("result = %q, want ok", got)
	}
}

func TestPackageRequestReturnsTypedResult(t *testing.T) {
	client := newHelperClient(t, "stream_thread_lifecycle")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	params := ThreadReadParams{ThreadID: "thr_stream_start"}
	got, err := Request[ThreadReadResponse](t.Context(), client, RequestMethodThreadRead, params)
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if got.Thread.ID != "thr_stream_start" {
		t.Fatalf("Request() thread id = %q, want thr_stream_start", got.Thread.ID)
	}

	wrapper, err := Request[ThreadReadResponse](t.Context(), client, RequestMethodThreadRead, params)
	if err != nil {
		t.Fatalf("Request() wrapper error = %v", err)
	}
	if wrapper.Thread.ID != got.Thread.ID {
		t.Fatalf("Request() wrapper thread id = %q, want %q", wrapper.Thread.ID, got.Thread.ID)
	}
}

func TestPackageRequestWithRetryOnOverloadRetriesAndReturnsResult(t *testing.T) {
	client := newHelperClient(t, "retry_on_overload")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	got, err := RequestWithRetryOnOverload[string](
		t.Context(), client,
		"ping",
		nil,
		RetryConfig{
			MaxAttempts:  3,
			InitialDelay: time.Nanosecond,
			MaxDelay:     time.Nanosecond,
		},
	)
	if err != nil {
		t.Fatalf("RequestWithRetryOnOverload() error = %v", err)
	}
	if got != "ok" {
		t.Fatalf("result = %q, want ok", got)
	}
}

func TestClientStreamUntilMethods(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)

		out := make(chan []Notification, 1)
		go func() {
			out <- mustCollectNotifications(t, client, "thread/started", "item/completed")
		}()
		synctest.Wait()

		if err := client.routeNotification(Notification{Method: "thread/started", Params: mustJSON(t, Object{"threadId": "thr-1"})}); err != nil {
			t.Fatalf("routeNotification() error = %v", err)
		}
		if err := client.routeNotification(Notification{Method: "item/completed", Params: mustJSON(t, Object{"threadId": "thr-1"})}); err != nil {
			t.Fatalf("routeNotification() error = %v", err)
		}

		notifications := <-out
		methods := notificationMethods(notifications)
		want := []string{"thread/started"}
		if diff := gocmp.Diff(want, methods); diff != "" {
			t.Fatalf("notification methods mismatch (-want +got):\n%s", diff)
		}
	})
}

func mustCollectNotifications(t *testing.T, client *Client, methods ...string) []Notification {
	t.Helper()
	notifications, err := client.StreamUntilMethods(t.Context(), methods...)
	if err != nil {
		t.Fatalf("StreamUntilMethods() error = %v", err)
	}
	return notifications
}

func TestClientWaitForTurnCompletedSkipsUnmatchedTurns(t *testing.T) {
	client := NewClient(nil, nil)
	if err := client.routeNotification(Notification{Method: NotificationMethodTurnCompleted, Params: mustJSON(t, TurnCompletedNotification{
		ThreadID: "thr-other",
		Turn: Turn{
			ID:     "turn-other",
			Status: TurnStatusCompleted,
		},
	})}); err != nil {
		t.Fatalf("routeNotification() error = %v", err)
	}
	if err := client.routeNotification(Notification{Method: NotificationMethodTurnCompleted, Params: mustJSON(t, TurnCompletedNotification{
		ThreadID: "thr-one",
		Turn: Turn{
			ID:     "turn-target",
			Status: TurnStatusCompleted,
		},
	})}); err != nil {
		t.Fatalf("routeNotification() error = %v", err)
	}
	completed, err := client.WaitForTurnCompleted(t.Context(), "turn-target")
	if err != nil {
		t.Fatalf("WaitForTurnCompleted() error = %v", err)
	}
	if completed.Turn.ID != "turn-target" {
		t.Fatalf("completed.Turn.ID = %q, want turn-target", completed.Turn.ID)
	}
	if pending := pendingTurnNotifications(client, "turn-other"); len(pending) != 1 {
		t.Fatalf("turn-other pending len = %d, want 1", len(pending))
	}
}

func TestClientStreamTextYieldsAgentMessageDeltasForTargetTurn(t *testing.T) {
	client := newHelperClient(t, "stream_text")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	model := "gpt-5.4"
	deltas := collectAgentMessageDeltas(t, client.StreamText(t.Context(), "thr_stream_text", "hello", &TurnStartParams{Model: &model}))
	if diff := gocmp.Diff([]string{"alpha", "beta"}, deltas); diff != "" {
		t.Fatalf("stream text deltas mismatch (-want +got):\n%s", diff)
	}
}

func TestClientCloseIsIdempotentAndUnblocksPendingRequest(t *testing.T) {
	client := newHelperClient(t, "pending_request")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := client.RequestRaw(t.Context(), "wait", nil)
		result <- err
	}()

	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	select {
	case err := <-result:
		var closed *TransportClosedError
		if !errors.As(err, &closed) {
			t.Fatalf("RequestRaw() error = %T %v, want TransportClosedError", err, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pending request to unblock after Close")
	}
	if _, err := client.NextNotification(t.Context()); err == nil {
		t.Fatal("NextNotification() error = nil after Close, want closed notification stream")
	}
}

func TestClientCloseReleasesActiveTurnConsumer(t *testing.T) {
	client := newHelperClient(t, "stream_cancel")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	thread := &Thread{client: client, id: "thr_stream_cancel"}
	handle, err := thread.Turn(t.Context(), "start", nil)
	if err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	streamResult := collectStreamAsync(handle.Stream(t.Context()))
	waitForActiveTurnConsumer(t, client, handle.ID())

	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if activeTurnConsumer(client) != "" {
		t.Fatalf("activeTurnID = %q, want cleared by Close", activeTurnConsumer(client))
	}
	select {
	case result := <-streamResult:
		var closed *TransportClosedError
		if !errors.As(result.err, &closed) {
			t.Fatalf("stream error = %T %v, want TransportClosedError", result.err, result.err)
		}
		if len(result.notifications) != 0 {
			t.Fatalf("stream notifications len = %d, want 0 after Close", len(result.notifications))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for active stream to stop after Close")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestClientRequestContextCancelUnregistersPendingResponse(t *testing.T) {
	client := newHelperClient(t, "pending_request")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := client.RequestRaw(ctx, "wait", nil)
		result <- err
	}()

	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RequestRaw() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for context-canceled request")
	}
}

func TestClientRoutesConcurrentResponsesWithoutRaces(t *testing.T) {
	client := newHelperClient(t, "concurrent_requests")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	const requestCount = 16
	var wg sync.WaitGroup
	errs := make(chan error, requestCount)
	results := make(chan string, requestCount)
	for i := range requestCount {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw, err := client.RequestRaw(t.Context(), "echo", Object{"index": i})
			if err != nil {
				errs <- fmt.Errorf("request %d: %w", i, err)
				return
			}
			results <- string(raw)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	close(results)
	got := map[string]int{}
	for raw := range results {
		got[raw]++
	}
	if len(got) != requestCount {
		t.Fatalf("unique response count = %d, want %d", len(got), requestCount)
	}
	for i := range requestCount {
		want := fmt.Sprintf(`"go-sdk-%d"`, i+1)
		if got[want] != 1 {
			t.Fatalf("response %s count = %d, want 1", want, got[want])
		}
	}
	assertPendingResponses(t, client, 0)
}

func TestClientDrainsNotificationOverflowWhileWaitingForResponse(t *testing.T) {
	client := newHelperClient(t, "notification_overflow")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	const notificationCount = 160
	drained := make(chan int, 1)
	go func() {
		count := 0
		for count < notificationCount {
			if _, err := client.NextNotification(t.Context()); err != nil {
				drained <- -1
				return
			}
			count++
		}
		drained <- count
	}()

	started, err := client.ThreadStart(t.Context(), nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	if started.Thread.ID != "thr_overflow" {
		t.Fatalf("thread id = %q, want thr_overflow", started.Thread.ID)
	}
	select {
	case got := <-drained:
		if got != notificationCount {
			t.Fatalf("drained notifications = %d, want %d", got, notificationCount)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out draining %d notifications", notificationCount)
	}
}

func TestClientDrainsStderrTailOverflow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)

		var stderr strings.Builder
		for i := range 401 {
			fmt.Fprintf(&stderr, "line-%03d\n", i)
		}
		done := make(chan struct{})
		go client.drainStderr(strings.NewReader(stderr.String()), done)
		<-done

		client.stderrMu.Lock()
		if got := len(client.stderrLines); got != 400 {
			client.stderrMu.Unlock()
			t.Fatalf("stderr line count = %d, want 400", got)
		}
		client.stderrMu.Unlock()

		got := strings.Split(client.stderrTail(5), "\n")
		want := []string{"line-396", "line-397", "line-398", "line-399", "line-400"}
		if diff := gocmp.Diff(want, got); diff != "" {
			t.Fatalf("stderr tail mismatch (-want +got):\n%s", diff)
		}
	})
}

func collectAgentMessageDeltas(t *testing.T, stream iter.Seq2[AgentMessageDeltaNotification, error]) []string {
	t.Helper()
	var deltas []string
	for delta, err := range stream {
		if err != nil {
			t.Fatalf("stream error = %v", err)
		}
		deltas = append(deltas, delta.Delta)
	}
	return deltas
}

func TestValidateInitialize(t *testing.T) {
	tests := map[string]struct {
		payload InitializeResponse
		want    InitializeResponse
		wantErr bool
	}{
		"success: explicit server info": {
			payload: InitializeResponse{UserAgent: "codex/1.2.3", ServerInfo: &ServerInfo{Name: "codex", Version: "1.2.3"}},
			want:    InitializeResponse{UserAgent: "codex/1.2.3", ServerInfo: &ServerInfo{Name: "codex", Version: "1.2.3"}},
		},
		"success: fills server info from user agent": {
			payload: InitializeResponse{UserAgent: "codex/1.2.3"},
			want:    InitializeResponse{UserAgent: "codex/1.2.3", ServerInfo: &ServerInfo{Name: "codex", Version: "1.2.3"}},
		},
		"error: missing metadata": {
			payload: InitializeResponse{},
			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := validateInitialize(tt.payload)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateInitialize() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("validateInitialize() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClientProtocolQueuesNotificationsAndHandlesServerRequests(t *testing.T) {
	client := newHelperClient(t, "protocol")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	model := "gpt-5.4"
	started, err := client.ThreadStart(t.Context(), &ThreadStartParams{Model: &model})
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	if started.Thread.ID != "thr_protocol" {
		t.Fatalf("thread id = %q, want thr_protocol", started.Thread.ID)
	}
	notification, err := client.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification() error = %v", err)
	}
	if notification.Method != "thread/started" {
		t.Fatalf("notification method = %q, want thread/started", notification.Method)
	}

	models, err := client.ModelList(t.Context(), &ModelListParams{IncludeHidden: ptr(true)})
	if err != nil {
		t.Fatalf("ModelList() error = %v", err)
	}
	if len(models.Data) != 1 || models.Data[0].ID != "gpt-test" {
		t.Fatalf("models = %#v, want one gpt-test model", models.Data)
	}
}

func TestClientPreservesUnknownNotificationPayloads(t *testing.T) {
	client := newHelperClient(t, "notification_passthrough")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	model := "gpt-5.4"
	started, err := client.ThreadStart(t.Context(), &ThreadStartParams{Model: &model})
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	if started.Thread.ID != "thr_notifications" {
		t.Fatalf("thread id = %q, want thr_notifications", started.Thread.ID)
	}
	notification, err := client.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification() error = %v", err)
	}
	if notification.Method != "custom/event" {
		t.Fatalf("notification method = %q, want custom/event", notification.Method)
	}
	if got := string(notification.Params); got != `{"details":{"answer":42},"kind":"raw"}` {
		t.Fatalf("notification params = %s, want raw payload preserved", got)
	}
	if _, ok, err := DecodeItemCompletedNotification(notification); err != nil || ok {
		t.Fatalf("DecodeItemCompletedNotification() mismatch = (%v, %v), want raw mismatch", err, ok)
	}
}

func TestReleaseTurnConsumerDoesNotFailPendingRequests(t *testing.T) {
	client := NewClient(nil, nil)
	if _, err := client.openTurnConsumer("turn-1"); err != nil {
		t.Fatalf("openTurnConsumer() error = %v", err)
	}
	response := make(chan responseWait, 1)
	client.registerResponse("request-1", response)

	client.releaseTurnConsumer("turn-1")

	if got := activeTurnConsumers(client); len(got) != 0 {
		t.Fatalf("active turn consumers = %v, want cleared", got)
	}
	select {
	case got := <-response:
		t.Fatalf("releaseTurnConsumer delivered responseWait %#v, want pending request left untouched", got)
	default:
	}
	client.responseMu.Lock()
	_, stillRegistered := client.responses["request-1"]
	client.responseMu.Unlock()
	if !stillRegistered {
		t.Fatalf("pending response was unregistered; releaseTurnConsumer must not fail unrelated requests")
	}
}

type streamResult struct {
	notifications []Notification
	err           error
}

type streamMethodsResult struct {
	methods []string
	err     error
}

func collectStream(stream iter.Seq2[Notification, error]) ([]Notification, error) {
	var notifications []Notification
	for notification, err := range stream {
		if err != nil {
			return notifications, err
		}
		notifications = append(notifications, notification)
	}
	return notifications, nil
}

func collectStreamAsync(stream iter.Seq2[Notification, error]) <-chan streamResult {
	result := make(chan streamResult, 1)
	go func() {
		notifications, err := collectStream(stream)
		result <- streamResult{notifications: notifications, err: err}
	}()
	return result
}

func notificationMethods(notifications []Notification) []string {
	methods := make([]string, 0, len(notifications))
	for _, notification := range notifications {
		methods = append(methods, notification.Method)
	}
	return methods
}

func pendingTurnNotifications(client *Client, turnID string) []Notification {
	if client == nil || client.turnRouter == nil {
		return nil
	}
	client.turnRouter.mu.Lock()
	defer client.turnRouter.mu.Unlock()
	return append([]Notification(nil), client.turnRouter.pending[turnID]...)
}

func activeTurnConsumers(client *Client) []string {
	if client == nil || client.turnRouter == nil {
		return nil
	}
	client.turnRouter.mu.Lock()
	defer client.turnRouter.mu.Unlock()
	got := make([]string, 0, len(client.turnRouter.queues))
	for turnID := range client.turnRouter.queues {
		got = append(got, turnID)
	}
	slices.Sort(got)
	return got
}

func activeTurnConsumer(client *Client) string {
	got := activeTurnConsumers(client)
	if len(got) == 0 {
		return ""
	}
	return got[0]
}

func assertActiveTurnConsumer(t *testing.T, client *Client, turnID string) {
	t.Helper()
	got := activeTurnConsumers(client)
	want := []string{turnID}
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Fatalf("active turn consumers mismatch (-want +got):\n%s", diff)
	}
}

func waitForActiveTurnConsumer(t *testing.T, client *Client, turnID string) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if got := activeTurnConsumers(client); len(got) == 1 && got[0] == turnID {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("activeTurnID = %q, want %q", activeTurnConsumers(client), turnID)
		case <-ticker.C:
		}
	}
}

func assertPendingResponses(t *testing.T, client *Client, want int) {
	t.Helper()
	client.responseMu.Lock()
	got := len(client.responses)
	client.responseMu.Unlock()
	if got != want {
		t.Fatalf("pending responses = %d, want %d", got, want)
	}
}

func TestTurnStreamReleasesConsumerAfterCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)
		handle := &TurnHandle{client: client, threadID: "thr_stream_done", turnID: "turn_stream_done"}

		completed := Notification{
			Method: NotificationMethodTurnCompleted,
			Params: mustJSON(t, Object{"threadId": "thr_stream_done", "turn": Object{"id": "turn_stream_done", "status": "completed"}}),
		}
		streamResult := collectStreamAsync(handle.Stream(t.Context()))
		synctest.Wait()
		assertActiveTurnConsumer(t, client, handle.ID())

		client.routeNotification(completed)
		synctest.Wait()

		result := <-streamResult
		if result.err != nil {
			t.Fatalf("stream error = %v", result.err)
		}
		if diff := gocmp.Diff([]string{NotificationMethodTurnCompleted}, notificationMethods(result.notifications)); diff != "" {
			t.Fatalf("stream methods mismatch (-want +got):\n%s", diff)
		}
		if _, err := client.openTurnConsumer("turn_after_completion"); err != nil {
			t.Fatalf("openTurnConsumer() after stream completion error = %v", err)
		}
		client.releaseTurnConsumer("turn_after_completion")
	})
}

func TestTurnStreamReleasesConsumerAfterEarlyStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)
		handle := &TurnHandle{client: client, threadID: "thr_stream_stop", turnID: "turn_stream_stop"}

		firstNotification := Notification{
			Method: "custom/event",
			Params: mustJSON(t, Object{"phase": "first", "turnId": "turn_stream_stop"}),
		}
		result := make(chan streamMethodsResult, 1)
		go func() {
			var methods []string
			for notification, err := range handle.Stream(t.Context()) {
				if err != nil {
					result <- streamMethodsResult{err: err}
					return
				}
				methods = append(methods, notification.Method)
				break
			}
			result <- streamMethodsResult{methods: methods}
		}()
		synctest.Wait()
		assertActiveTurnConsumer(t, client, handle.ID())

		client.routeNotification(firstNotification)
		synctest.Wait()

		got := <-result
		if got.err != nil {
			t.Fatalf("stream error = %v", got.err)
		}
		if diff := gocmp.Diff([]string{"custom/event"}, got.methods); diff != "" {
			t.Fatalf("stream methods mismatch (-want +got):\n%s", diff)
		}
		if _, err := client.openTurnConsumer("turn_after_early_stop"); err != nil {
			t.Fatalf("openTurnConsumer() after early stream stop error = %v", err)
		}
		client.releaseTurnConsumer("turn_after_early_stop")
	})
}

func TestTurnStreamReleasesConsumerOnContextCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)
		handle := &TurnHandle{client: client, threadID: "thr_stream_cancel", turnID: "turn_stream_cancel"}
		ctx, cancel := context.WithCancel(t.Context())

		streamResult := collectStreamAsync(handle.Stream(ctx))
		synctest.Wait()
		assertActiveTurnConsumer(t, client, handle.ID())

		cancel()
		synctest.Wait()

		result := <-streamResult
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("stream error = %v, want context.Canceled", result.err)
		}
		if len(result.notifications) != 0 {
			t.Fatalf("notifications len = %d, want 0 after cancellation", len(result.notifications))
		}
		if _, err := client.openTurnConsumer("turn_after_cancel"); err != nil {
			t.Fatalf("openTurnConsumer() after stream cancellation error = %v", err)
		}
		client.releaseTurnConsumer("turn_after_cancel")
	})
}

func TestTurnStreamReleasesConsumerOnContextDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)
		handle := &TurnHandle{client: client, threadID: "thr_stream_deadline", turnID: "turn_stream_deadline"}
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		streamResult := collectStreamAsync(handle.Stream(ctx))
		synctest.Wait()
		assertActiveTurnConsumer(t, client, handle.ID())

		time.Sleep(5 * time.Second)
		synctest.Wait()

		result := <-streamResult
		if !errors.Is(result.err, context.DeadlineExceeded) {
			t.Fatalf("stream error = %v, want context.DeadlineExceeded", result.err)
		}
		if len(result.notifications) != 0 {
			t.Fatalf("notifications len = %d, want 0 after deadline", len(result.notifications))
		}
		if _, err := client.openTurnConsumer("turn_after_deadline"); err != nil {
			t.Fatalf("openTurnConsumer() after stream deadline error = %v", err)
		}
		client.releaseTurnConsumer("turn_after_deadline")
	})
}

func TestDuplicateTurnConsumerFailsPredictably(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)
		if _, err := client.openTurnConsumer("turn_busy"); err != nil {
			t.Fatalf("initial openTurnConsumer() error = %v", err)
		}
		defer client.releaseTurnConsumer("turn_busy")

		handle := &TurnHandle{client: client, threadID: "thr_busy", turnID: "turn_busy"}
		notifications, err := collectStream(handle.Stream(t.Context()))
		if err == nil {
			t.Fatalf("Stream() error = nil, want duplicate consumer rejection")
		}
		if len(notifications) != 0 {
			t.Fatalf("Stream() notifications len = %d, want 0 on acquire failure", len(notifications))
		}
		if !strings.Contains(err.Error(), "turn consumer already active for turn_busy") {
			t.Fatalf("Stream() error = %q, want active consumer message", err)
		}
		if _, err := handle.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "turn consumer already active for turn_busy") {
			t.Fatalf("Run() error = %v, want active consumer message", err)
		}
	})
}

func TestThreadRunCollectsFinalResponseAndUsage(t *testing.T) {
	client := newHelperClient(t, "run")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()
	model := "gpt-5.4"

	thread := &Thread{client: client, id: "thr_run"}
	result, err := thread.Run(t.Context(), "hello", &TurnStartParams{Model: &model})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.FinalResponse != "final text" {
		t.Fatalf("FinalResponse = %q, want final text", result.FinalResponse)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(result.Items))
	}
	if result.Usage == nil || result.Usage.Total.TotalTokens != 6 {
		t.Fatalf("usage = %#v, want total tokens 6", result.Usage)
	}
	if result.Turn.ID != "turn_run" || result.Turn.Status != TurnStatusCompleted {
		t.Fatalf("turn = %#v, want completed turn_run", result.Turn)
	}
	if got := activeTurnConsumers(client); len(got) != 0 {
		t.Fatalf("active turn consumers = %v, want released after successful Run", got)
	}

	nextHandle, err := thread.Turn(t.Context(), "follow-up", &TurnStartParams{Model: &model})
	if err != nil {
		t.Fatalf("Turn() after successful Run error = %v", err)
	}
	if _, err := collectStream(nextHandle.Stream(t.Context())); err != nil {
		t.Fatalf("stream error after successful Run = %v", err)
	}
}

func TestStreamThreadRunCollectsFinalResponseAndUsage(t *testing.T) {
	client := newHelperClient(t, "run")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()
	model := "gpt-5.4"

	thread := newStreamThread(client, "thr_run")
	if thread.ID() != "thr_run" {
		t.Fatalf("StreamThread.ID() = %q, want thr_run", thread.ID())
	}
	result, err := thread.Run(t.Context(), "hello", &TurnStartParams{Model: &model})
	if err != nil {
		t.Fatalf("StreamThread.Run() error = %v", err)
	}
	if result.FinalResponse != "final text" {
		t.Fatalf("FinalResponse = %q, want final text", result.FinalResponse)
	}
	if result.Usage == nil || result.Usage.Total.TotalTokens != 6 {
		t.Fatalf("usage = %#v, want total tokens 6", result.Usage)
	}
	if result.Turn.ID != "turn_run" || result.Turn.Status != TurnStatusCompleted {
		t.Fatalf("turn = %#v, want completed turn_run", result.Turn)
	}
	if got := activeTurnConsumers(client); len(got) != 0 {
		t.Fatalf("active turn consumers = %v, want released after StreamThread.Run", got)
	}
}

func TestStreamThreadRunStreamYieldsNotificationsAndReleasesConsumer(t *testing.T) {
	client := newHelperClient(t, "run")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()
	model := "gpt-5.4"

	thread := newStreamThread(client, "thr_run")
	notifications, err := collectStream(thread.RunStream(t.Context(), "hello", &TurnStartParams{Model: &model}))
	if err != nil {
		t.Fatalf("StreamThread.RunStream() error = %v", err)
	}
	want := []string{
		NotificationMethodItemCompleted,
		NotificationMethodThreadTokenUsageUpdated,
		NotificationMethodItemCompleted,
		NotificationMethodTurnCompleted,
	}
	if diff := gocmp.Diff(want, notificationMethods(notifications)); diff != "" {
		t.Fatalf("stream methods mismatch (-want +got):\n%s", diff)
	}
	if got := activeTurnConsumers(client); len(got) != 0 {
		t.Fatalf("active turn consumers = %v, want released after StreamThread.RunStream", got)
	}
}

func TestStreamTurnHandleDelegatesSteerInterruptAndStream(t *testing.T) {
	client := newHelperClient(t, "stream_steer")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	thread := newStreamThread(client, "thr_stream")
	handle, err := thread.Turn(t.Context(), "start", nil)
	if err != nil {
		t.Fatalf("StreamThread.Turn() error = %v", err)
	}
	if handle.ID() != "turn_stream" {
		t.Fatalf("StreamTurnHandle.ID() = %q, want turn_stream", handle.ID())
	}
	streamResult := collectStreamAsync(handle.Stream(t.Context()))
	waitForActiveTurnConsumer(t, client, handle.ID())

	controlCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if _, err := handle.Interrupt(controlCtx); err != nil {
		t.Fatalf("StreamTurnHandle.Interrupt() while streaming error = %v", err)
	}
	if _, err := handle.Steer(controlCtx, TextInput{Text: "continue"}); err != nil {
		t.Fatalf("StreamTurnHandle.Steer() while streaming error = %v", err)
	}
	select {
	case result := <-streamResult:
		if result.err != nil {
			t.Fatalf("stream error = %v", result.err)
		}
		want := []string{NotificationMethodItemAgentMessageDelta, NotificationMethodTurnCompleted}
		if diff := gocmp.Diff(want, notificationMethods(result.notifications)); diff != "" {
			t.Fatalf("stream methods mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream completion")
	}
}

func TestCodexStreamThreadLifecycleMethodsDelegateToThreadAPI(t *testing.T) {
	codex, err := NewCodex(t.Context(), helperConfig("stream_thread_lifecycle"))
	if err != nil {
		t.Fatalf("NewCodex() error = %v", err)
	}
	defer func() { _ = codex.Close() }()

	started, err := codex.StreamThreadStart(t.Context(), nil)
	if err != nil {
		t.Fatalf("StreamThreadStart() error = %v", err)
	}
	if started.ID() != "thr_stream_start" {
		t.Fatalf("started.ID() = %q, want thr_stream_start", started.ID())
	}

	resumed, err := codex.StreamThreadResume(t.Context(), "thr_existing", nil)
	if err != nil {
		t.Fatalf("StreamThreadResume() error = %v", err)
	}
	if resumed.ID() != "thr_stream_resume" {
		t.Fatalf("resumed.ID() = %q, want thr_stream_resume", resumed.ID())
	}

	forked, err := codex.StreamThreadFork(t.Context(), "thr_existing", nil)
	if err != nil {
		t.Fatalf("StreamThreadFork() error = %v", err)
	}
	if forked.ID() != "thr_stream_fork" {
		t.Fatalf("forked.ID() = %q, want thr_stream_fork", forked.ID())
	}

	unarchived, err := codex.StreamThreadUnarchive(t.Context(), "thr_archived")
	if err != nil {
		t.Fatalf("StreamThreadUnarchive() error = %v", err)
	}
	if unarchived.ID() != "thr_stream_unarchive" {
		t.Fatalf("unarchived.ID() = %q, want thr_stream_unarchive", unarchived.ID())
	}

	read, err := started.Read(t.Context(), &ThreadReadParams{IncludeTurns: ptr(true)})
	if err != nil {
		t.Fatalf("StreamThread.Read() error = %v", err)
	}
	if read.Thread.ID != "thr_stream_start" {
		t.Fatalf("read.Thread.ID = %q, want thr_stream_start", read.Thread.ID)
	}
	if _, err := started.SetName(t.Context(), "named"); err != nil {
		t.Fatalf("StreamThread.SetName() error = %v", err)
	}
	if _, err := started.Compact(t.Context()); err != nil {
		t.Fatalf("StreamThread.Compact() error = %v", err)
	}
}

func TestThreadRunReleasesConsumerAfterFailure(t *testing.T) {
	client := newHelperClient(t, "run_failed")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()
	model := "gpt-5.4"

	thread := &Thread{client: client, id: "thr_run_failed"}
	_, err := thread.Run(t.Context(), "hello", nil)
	if err == nil {
		t.Fatalf("Run() error = nil, want failed turn error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Run() error = %v, want boom", err)
	}
	if got := activeTurnConsumers(client); len(got) != 0 {
		t.Fatalf("active turn consumers = %v, want released after failed Run", got)
	}

	nextHandle, err := thread.Turn(t.Context(), "follow-up", &TurnStartParams{Model: &model})
	if err != nil {
		t.Fatalf("Turn() after failed Run error = %v", err)
	}
	if _, err := collectStream(nextHandle.Stream(t.Context())); err != nil {
		t.Fatalf("stream error after failed Run = %v", err)
	}
}

func TestTurnStreamAllowsConcurrentSteer(t *testing.T) {
	client := newHelperClient(t, "stream_steer")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	thread := &Thread{client: client, id: "thr_stream"}
	handle, err := thread.Turn(t.Context(), "start", nil)
	if err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	streamResult := collectStreamAsync(handle.Stream(t.Context()))
	waitForActiveTurnConsumer(t, client, handle.ID())

	steerCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if _, err := handle.Steer(steerCtx, TextInput{Text: "continue"}); err != nil {
		t.Fatalf("Steer() while streaming error = %v", err)
	}
	select {
	case result := <-streamResult:
		if result.err != nil {
			t.Fatalf("stream error = %v", result.err)
		}
		want := []string{"item/agentMessage/delta", "turn/completed"}
		if diff := gocmp.Diff(want, notificationMethods(result.notifications)); diff != "" {
			t.Fatalf("stream methods mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream completion")
	}
}

func TestTurnStreamRejectsDuplicateConsumerUntilFirstCancels(t *testing.T) {
	client := newHelperClient(t, "stream_cancel")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	firstHandle := &TurnHandle{client: client, threadID: "thr_stream", turnID: "turn_stream"}
	firstCtx, firstCancel := context.WithCancel(t.Context())
	firstResult := collectStreamAsync(firstHandle.Stream(firstCtx))
	waitForActiveTurnConsumer(t, client, firstHandle.ID())

	secondHandle := &TurnHandle{client: client, threadID: "thr_stream", turnID: "turn_stream"}
	secondCtx, secondCancel := context.WithCancel(t.Context())
	defer secondCancel()
	notifications, err := collectStream(secondHandle.Stream(secondCtx))
	if err == nil || !strings.Contains(err.Error(), "turn consumer already active") {
		t.Fatalf("duplicate Stream() error = %v, want active-consumer failure", err)
	}
	if len(notifications) != 0 {
		t.Fatalf("duplicate Stream() notifications len = %d, want 0 on active-consumer failure", len(notifications))
	}

	firstCancel()
	select {
	case result := <-firstResult:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("first stream error = %v, want context canceled", result.err)
		}
		if len(result.notifications) != 0 {
			t.Fatalf("first stream notifications len = %d, want 0", len(result.notifications))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first stream cancellation")
	}

	if err := client.acquireTurnConsumer(firstHandle.ID()); err != nil {
		t.Fatalf("acquireTurnConsumer() after first stream cancellation error = %v", err)
	}
	client.releaseTurnConsumer(firstHandle.ID())
}

func TestNewCodexInitializesAndCloses(t *testing.T) {
	codex, err := NewCodex(t.Context(), helperConfig("initialize"))
	if err != nil {
		t.Fatalf("NewCodex() error = %v", err)
	}
	defer func() { _ = codex.Close() }()
	metadata := codex.Metadata()
	if metadata.ServerInfo == nil || metadata.ServerInfo.Name != "codex-test" || metadata.ServerInfo.Version != "1.2.3" {
		t.Fatalf("metadata = %#v, want codex-test 1.2.3", metadata)
	}
}

func newHelperClient(t *testing.T, scenario string) *Client {
	t.Helper()
	return NewClient(helperConfig(scenario), nil)
}

func helperConfig(scenario string) *Config {
	return &Config{
		LaunchArgsOverride: []string{os.Args[0], "-test.run=TestHelperProcess", "--"},
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
			"CODEX_HELPER_SCENARIO":  scenario,
		},
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	scenario := os.Getenv("CODEX_HELPER_SCENARIO")
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer func() { _ = writer.Flush() }()
	methodWrapperIndex := 0
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		var req map[string]any
		if err := json.Unmarshal(line, &req); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		method, _ := req["method"].(string)
		id, _ := req["id"].(string)
		if scenario == "initialize" {
			handleInitializeScenario(writer, method, id)
			continue
		}
		if scenario == "protocol" {
			handleProtocolScenario(reader, writer, method, id)
			continue
		}
		if scenario == "notification_passthrough" {
			handleNotificationPassthroughScenario(writer, method, id)
			continue
		}
		if scenario == "run" {
			handleRunScenario(writer, method, id)
			continue
		}
		if scenario == "run_failed" {
			handleRunFailedScenario(writer, method, id)
			continue
		}
		if scenario == "stream_steer" {
			handleStreamSteerScenario(writer, method, id)
			continue
		}
		if scenario == "stream_thread_lifecycle" {
			handleStreamThreadLifecycleScenario(writer, method, id)
			continue
		}
		if scenario == "stream_cancel" {
			handleStreamCancelScenario(writer, method, id)
			continue
		}
		if scenario == "retry_on_overload" {
			handleRetryOnOverloadScenario(writer, method, id)
			continue
		}
		if scenario == "stream_text" {
			handleStreamTextScenario(writer, method, id)
			continue
		}
		if scenario == "pending_request" {
			handlePendingRequestScenario(writer, method, id)
			continue
		}
		if scenario == "concurrent_requests" {
			handleConcurrentRequestsScenario(writer, method, id)
			continue
		}
		if scenario == "notification_overflow" {
			handleNotificationOverflowScenario(writer, method, id)
			continue
		}
		if scenario == "method_wrappers" {
			handleMethodWrappersScenario(writer, req, method, id, &methodWrapperIndex)
			continue
		}
	}
}

var methodWrapperMethods = []string{
	RequestMethodThreadStart,
	RequestMethodThreadResume,
	RequestMethodThreadFork,
	RequestMethodThreadArchive,
	RequestMethodThreadUnsubscribe,
	RequestMethodThreadNameSet,
	RequestMethodThreadMetadataUpdate,
	RequestMethodThreadUnarchive,
	RequestMethodThreadCompactStart,
	RequestMethodThreadShellCommand,
	RequestMethodThreadApproveGuardianDeniedAction,
	RequestMethodThreadRollback,
	RequestMethodThreadList,
	RequestMethodThreadLoadedList,
	RequestMethodThreadRead,
	RequestMethodThreadInjectItems,
	RequestMethodSkillsList,
	RequestMethodHooksList,
	RequestMethodMarketplaceAdd,
	RequestMethodMarketplaceRemove,
	RequestMethodMarketplaceUpgrade,
	RequestMethodPluginList,
	RequestMethodPluginRead,
	RequestMethodPluginSkillRead,
	RequestMethodPluginShareSave,
	RequestMethodPluginShareUpdateTargets,
	RequestMethodPluginShareList,
	RequestMethodPluginShareDelete,
	RequestMethodAppList,
	RequestMethodFSReadFile,
	RequestMethodFSWriteFile,
	RequestMethodFSCreateDirectory,
	RequestMethodFSGetMetadata,
	RequestMethodFSReadDirectory,
	RequestMethodFSRemove,
	RequestMethodFSCopy,
	RequestMethodFSWatch,
	RequestMethodFSUnwatch,
	RequestMethodSkillsConfigWrite,
	RequestMethodPluginInstall,
	RequestMethodPluginUninstall,
	RequestMethodTurnStart,
	RequestMethodTurnSteer,
	RequestMethodTurnInterrupt,
	RequestMethodReviewStart,
	RequestMethodModelList,
	RequestMethodModelProviderCapabilitiesRead,
	RequestMethodExperimentalFeatureList,
	RequestMethodExperimentalFeatureEnablementSet,
	RequestMethodMCPServerOAuthLogin,
	RequestMethodConfigMCPServerReload,
	RequestMethodMCPServerStatusList,
	RequestMethodMCPServerResourceRead,
	RequestMethodMCPServerToolCall,
	RequestMethodWindowsSandboxSetupStart,
	RequestMethodWindowsSandboxReadiness,
	RequestMethodAccountLoginStart,
	RequestMethodAccountLoginCancel,
	RequestMethodAccountLogout,
	RequestMethodAccountRateLimitsRead,
	RequestMethodAccountSendAddCreditsNudgeEmail,
	RequestMethodFeedbackUpload,
	RequestMethodCommandExec,
	RequestMethodCommandExecWrite,
	RequestMethodCommandExecTerminate,
	RequestMethodCommandExecResize,
	RequestMethodConfigRead,
	RequestMethodExternalAgentConfigDetect,
	RequestMethodExternalAgentConfigImport,
	RequestMethodConfigValueWrite,
	RequestMethodConfigBatchWrite,
	RequestMethodConfigRequirementsRead,
	RequestMethodAccountRead,
	RequestMethodFuzzyFileSearch,
}

func handleMethodWrappersScenario(writer *bufio.Writer, req map[string]any, method, id string, index *int) {
	if *index >= len(methodWrapperMethods) {
		writeJSON(writer, Object{"id": id, "error": Object{"code": -32601, "message": "unexpected extra method " + method}})
		return
	}
	want := methodWrapperMethods[*index]
	(*index)++
	if method != want {
		writeJSON(writer, Object{"id": id, "error": Object{"code": -32601, "message": "unexpected method " + method + ", want " + want}})
		return
	}
	params, ok := req["params"].(map[string]any)
	if !ok {
		writeJSON(writer, Object{"id": id, "error": Object{"code": -32602, "message": "missing object params for " + method}})
		return
	}
	if methodWrapperExpectsEmptyParams(method) && len(params) != 0 {
		writeJSON(writer, Object{"id": id, "error": Object{"code": -32602, "message": "unexpected params for " + method}})
		return
	}
	switch method {
	case RequestMethodAccountLoginStart:
		writeJSON(writer, Object{"id": id, "result": Object{"type": "apiKey"}})
	case RequestMethodFuzzyFileSearch:
		writeJSON(writer, Object{"id": id, "result": Object{"sessionId": "fuzzy-session"}})
	default:
		writeJSON(writer, Object{"id": id, "result": nil})
	}
}

func methodWrapperExpectsEmptyParams(method string) bool {
	switch method {
	case RequestMethodConfigMCPServerReload,
		RequestMethodWindowsSandboxReadiness,
		RequestMethodAccountLogout,
		RequestMethodAccountRateLimitsRead,
		RequestMethodConfigRequirementsRead:
		return true
	default:
		return false
	}
}

var retryOnOverloadAttempts = map[string]int{}

func handleInitializeScenario(writer *bufio.Writer, method, id string) {
	if method == RequestMethodInitialize {
		writeJSON(writer, Object{"id": id, "result": Object{"userAgent": "codex-test/1.2.3"}})
		return
	}
	if method == "initialized" {
		return
	}
	writeJSON(writer, Object{"id": id, "result": Object{}})
}

func handleProtocolScenario(reader *bufio.Reader, writer *bufio.Writer, method, id string) {
	switch method {
	case RequestMethodThreadStart:
		writeJSON(writer, Object{"method": "thread/started", "params": Object{"threadId": "thr_protocol"}})
		writeJSON(writer, Object{"id": id, "result": Object{"thread": Object{"id": "thr_protocol"}}})
	case RequestMethodModelList:
		writeJSON(writer, Object{"id": "server-approval-1", "method": "item/commandExecution/requestApproval", "params": Object{"command": "echo ok"}})
		line, err := reader.ReadBytes('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		var approval map[string]any
		if err := json.Unmarshal(line, &approval); err != nil || approval["id"] != "server-approval-1" {
			fmt.Fprintf(os.Stderr, "bad approval response: %s %v\n", string(line), err)
			os.Exit(2)
		}
		writeJSON(writer, Object{"id": id, "result": Object{"data": []Object{{"id": "gpt-test", "model": "gpt-test", "displayName": "GPT Test", "description": "test", "hidden": false, "isDefault": true, "defaultReasoningEffort": "medium", "supportedReasoningEfforts": []Object{{"reasoningEffort": "medium"}}}}}})
	default:
		writeJSON(writer, Object{"id": id, "result": Object{}})
	}
}

func handleNotificationPassthroughScenario(writer *bufio.Writer, method, id string) {
	if method == RequestMethodThreadStart {
		writeRawJSONLine(writer, `{"method":"custom/event","params":{"details":{"answer":42},"kind":"raw"}}`)
		writeJSON(writer, Object{"id": id, "result": Object{"thread": Object{"id": "thr_notifications"}}})
		return
	}
	writeJSON(writer, Object{"id": id, "result": Object{}})
}

func handleRunScenario(writer *bufio.Writer, method, id string) {
	if method != RequestMethodTurnStart {
		writeJSON(writer, Object{"id": id, "result": Object{}})
		return
	}
	writeJSON(writer, Object{"id": id, "result": Object{"turn": Object{"id": "turn_run", "status": "inProgress"}}})
	writeJSON(writer, Object{"method": "item/completed", "params": Object{"threadId": "thr_run", "turnId": "turn_run", "item": Object{"type": "agentMessage", "text": "draft text"}}})
	writeJSON(writer, Object{"method": "thread/tokenUsage/updated", "params": Object{"threadId": "thr_run", "turnId": "turn_run", "tokenUsage": Object{"last": usage(1), "total": usage(6)}}})
	writeJSON(writer, Object{"method": "item/completed", "params": Object{"threadId": "thr_run", "turnId": "turn_run", "item": Object{"type": "agentMessage", "phase": "final_answer", "text": "final text"}}})
	writeJSON(writer, Object{"method": "turn/completed", "params": Object{"turn": Object{"id": "turn_run", "status": "completed"}}})
}

func handleRunFailedScenario(writer *bufio.Writer, method, id string) {
	if method != RequestMethodTurnStart {
		writeJSON(writer, Object{"id": id, "result": Object{}})
		return
	}
	writeJSON(writer, Object{"id": id, "result": Object{"turn": Object{"id": "turn_failed", "status": "inProgress"}}})
	writeJSON(writer, Object{"method": "turn/completed", "params": Object{"threadId": "thr_run_failed", "turn": Object{"id": "turn_failed", "status": "failed", "error": Object{"message": "boom"}}}})
}

func handleStreamSteerScenario(writer *bufio.Writer, method, id string) {
	switch method {
	case RequestMethodTurnStart:
		writeJSON(writer, Object{"id": id, "result": Object{"turn": Object{"id": "turn_stream", "status": "inProgress"}}})
	case RequestMethodTurnSteer:
		writeJSON(writer, Object{"id": id, "result": Object{}})
		writeJSON(writer, Object{"method": "item/agentMessage/delta", "params": Object{"threadId": "thr_stream", "turnId": "turn_stream", "delta": "ok"}})
		writeJSON(writer, Object{"method": "turn/completed", "params": Object{"turn": Object{"id": "turn_stream", "status": "completed"}}})
	default:
		writeJSON(writer, Object{"id": id, "result": Object{}})
	}
}

func handleStreamThreadLifecycleScenario(writer *bufio.Writer, method, id string) {
	switch method {
	case RequestMethodInitialize:
		writeJSON(writer, Object{"id": id, "result": Object{"userAgent": "codex-test/1.2.3"}})
	case "initialized":
	case RequestMethodThreadStart:
		writeJSON(writer, Object{"id": id, "result": Object{"thread": Object{"id": "thr_stream_start"}}})
	case RequestMethodThreadResume:
		writeJSON(writer, Object{"id": id, "result": Object{"thread": Object{"id": "thr_stream_resume"}}})
	case RequestMethodThreadFork:
		writeJSON(writer, Object{"id": id, "result": Object{"thread": Object{"id": "thr_stream_fork"}}})
	case RequestMethodThreadUnarchive:
		writeJSON(writer, Object{"id": id, "result": Object{"thread": Object{"id": "thr_stream_unarchive"}}})
	case RequestMethodThreadRead:
		writeJSON(writer, Object{"id": id, "result": Object{"thread": Object{"id": "thr_stream_start"}}})
	case RequestMethodThreadNameSet, RequestMethodThreadCompactStart:
		writeJSON(writer, Object{"id": id, "result": Object{}})
	default:
		writeJSON(writer, Object{"id": id, "result": Object{}})
	}
}

func handleStreamCancelScenario(writer *bufio.Writer, method, id string) {
	if method != RequestMethodTurnStart {
		writeJSON(writer, Object{"id": id, "result": Object{}})
		return
	}
	turnID := "turn_cancel_1"
	if id != "go-sdk-1" {
		turnID = "turn_cancel_2"
	}
	writeJSON(writer, Object{"id": id, "result": Object{"turn": Object{"id": turnID, "status": "inProgress"}}})
}

func handleRetryOnOverloadScenario(writer *bufio.Writer, method, id string) {
	if method != "ping" {
		writeJSON(writer, Object{"id": id, "result": Object{}})
		return
	}
	if retryOnOverloadAttempts[method] == 0 {
		retryOnOverloadAttempts[method]++
		writeJSON(writer, Object{"id": id, "error": Object{
			"code":    -32000,
			"message": "busy",
			"data":    Object{"codexErrorInfo": "server_overloaded"},
		}})
		return
	}
	delete(retryOnOverloadAttempts, method)
	writeJSON(writer, Object{"id": id, "result": "ok"})
}

func handleStreamTextScenario(writer *bufio.Writer, method, id string) {
	switch method {
	case RequestMethodTurnStart:
		writeJSON(writer, Object{"id": id, "result": Object{"turn": Object{"id": "turn-stream-text", "status": "inProgress"}}})
		writeJSON(writer, Object{"method": "item/agentMessage/delta", "params": Object{
			"threadId": "thr_stream_text",
			"turnId":   "turn-stream-text",
			"delta":    "alpha",
		}})
		writeJSON(writer, Object{"method": "item/agentMessage/delta", "params": Object{
			"threadId": "thr_stream_text",
			"turnId":   "turn-stream-text",
			"delta":    "beta",
		}})
		writeJSON(writer, Object{
			"method": "turn/completed",
			"params": Object{"turn": Object{"id": "turn-stream-text"}},
		})
	default:
		writeJSON(writer, Object{"id": id, "result": Object{}})
	}
}

func handlePendingRequestScenario(writer *bufio.Writer, method, id string) {
	if method == "wait" {
		select {}
	}
	writeJSON(writer, Object{"id": id, "result": Object{}})
}

func handleConcurrentRequestsScenario(writer *bufio.Writer, method, id string) {
	if method == "echo" {
		writeJSON(writer, Object{"id": id, "result": id})
		return
	}
	writeJSON(writer, Object{"id": id, "result": Object{}})
}

func handleNotificationOverflowScenario(writer *bufio.Writer, method, id string) {
	if method != RequestMethodThreadStart {
		writeJSON(writer, Object{"id": id, "result": Object{}})
		return
	}
	for i := range 160 {
		writeJSON(writer, Object{"method": "custom/overflow", "params": Object{"index": i}})
	}
	writeJSON(writer, Object{"id": id, "result": Object{"thread": Object{"id": "thr_overflow"}}})
}

func usage(total int64) Object {
	return Object{"cachedInputTokens": int64(0), "inputTokens": total, "outputTokens": int64(0), "reasoningOutputTokens": int64(0), "totalTokens": total}
}

func writeRawJSONLine(writer *bufio.Writer, line string) {
	_, _ = writer.WriteString(line)
	_, _ = writer.WriteString("\n")
	_ = writer.Flush()
}

func writeJSON(writer *bufio.Writer, payload any) {
	line, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	_, _ = writer.Write(line)
	_, _ = writer.WriteString("\n")
	_ = writer.Flush()
}
