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

package codexappserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"
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
			errorType: "*codexappserver.InvalidParamsError",
		},
		"success: nested server overloaded is retryable": {
			code:      -32000,
			message:   "busy",
			data:      `{"errorInfo":{"reason":"server_overloaded"}}`,
			kind:      "server_busy",
			retryable: true,
			errorType: "*codexappserver.ServerBusyError",
		},
		"success: nested codex_error_info is retryable": {
			code:      -32000,
			message:   "busy",
			data:      `{"codex_error_info":{"reason":"server_overloaded"}}`,
			kind:      "server_busy",
			retryable: true,
			errorType: "*codexappserver.ServerBusyError",
		},
		"success: nested codexErrorInfo is retryable": {
			code:      -32000,
			message:   "busy",
			data:      `{"codexErrorInfo":{"reason":"server_overloaded"}}`,
			kind:      "server_busy",
			retryable: true,
			errorType: "*codexappserver.ServerBusyError",
		},
		"success: retry limit with overload marker is classified": {
			code:       -32000,
			message:    "retry limit reached",
			data:       `{"codex_error_info":{"status":"server_overloaded"}}`,
			kind:       "retry_limit_exceeded",
			retryable:  true,
			retryLimit: true,
			errorType:  "*codexappserver.RetryLimitExceededError",
		},
		"success: retry limit text is retryable but classified": {
			code:       -32000,
			message:    "too many failed attempts",
			data:       `{"codexErrorInfo":{"reason":"server_overloaded"}}`,
			kind:       "retry_limit_exceeded",
			retryable:  true,
			retryLimit: true,
			errorType:  "*codexappserver.RetryLimitExceededError",
		},
		"success: parse error maps to ParseError": {
			code:      -32700,
			message:   "parse",
			data:      `null`,
			kind:      "parse_error",
			errorType: "*codexappserver.ParseError",
		},
		"success: invalid request maps to InvalidRequestError": {
			code:      -32600,
			message:   "bad request",
			data:      `null`,
			kind:      "invalid_request",
			errorType: "*codexappserver.InvalidRequestError",
		},
		"success: method not found maps to MethodNotFoundError": {
			code:      -32601,
			message:   "not found",
			data:      `null`,
			kind:      "method_not_found",
			errorType: "*codexappserver.MethodNotFoundError",
		},
		"success: internal rpc maps to InternalRpcError": {
			code:      -32603,
			message:   "internal",
			data:      `null`,
			kind:      "internal_error",
			errorType: "*codexappserver.InternalRPCError",
		},
		"success: app-server json-rpc range uses app-server error": {
			code:      -32010,
			message:   "app_server_error",
			data:      `{"reason":"ok"}`,
			kind:      "app_server_rpc",
			errorType: "*codexappserver.AppServerRPCError",
		},
		"success: unknown code uses raw JsonRpcError": {
			code:      -1234,
			message:   "unknown",
			data:      `null`,
			kind:      "jsonrpc",
			errorType: "*codexappserver.JSONRPCError",
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
		t.Context(),
		client,
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
	client := NewClient(nil, nil)
	client.notifications = make(chan Notification, 4)

	out := make(chan []Notification, 1)
	go func() {
		out <- mustCollectNotifications(t, client, "thread/started", "item/completed")
	}()
	defer close(client.notifications)

	client.notifications <- Notification{Method: "thread/started", Params: mustJSON(t, Object{"threadId": "thr-1"})}
	client.notifications <- Notification{Method: "item/completed", Params: mustJSON(t, Object{"threadId": "thr-1"})}

	notifications := <-out
	methods := notificationMethods(notifications)
	want := []string{"thread/started"}
	if diff := gocmp.Diff(want, methods); diff != "" {
		t.Fatalf("notification methods mismatch (-want +got):\n%s", diff)
	}
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
	client.notifications = make(chan Notification, 4)

	go func() {
		client.notifications <- Notification{Method: NotificationMethodTurnCompleted, Params: mustJSON(t, TurnCompletedNotification{
			ThreadID: "thr-other",
			Turn: Turn{
				ID:     "turn-other",
				Status: TurnStatusCompleted,
			},
		})}
		client.notifications <- Notification{Method: NotificationMethodTurnCompleted, Params: mustJSON(t, TurnCompletedNotification{
			ThreadID: "thr-one",
			Turn: Turn{
				ID:     "turn-target",
				Status: TurnStatusCompleted,
			},
		})}
	}()
	completed, err := client.WaitForTurnCompleted(t.Context(), "turn-target")
	if err != nil {
		t.Fatalf("WaitForTurnCompleted() error = %v", err)
	}
	if completed.Turn.ID != "turn-target" {
		t.Fatalf("completed.Turn.ID = %q, want turn-target", completed.Turn.ID)
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

	models, err := client.ModelList(t.Context(), true)
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
	client.activeTurnID = "turn-1"
	response := make(chan responseWait, 1)
	client.registerResponse("request-1", response)

	client.releaseTurnConsumer("turn-1")

	if client.activeTurnID != "" {
		t.Fatalf("activeTurnID = %q, want cleared", client.activeTurnID)
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

func activeTurnConsumer(client *Client) string {
	client.turnConsumerMu.Lock()
	defer client.turnConsumerMu.Unlock()
	return client.activeTurnID
}

func assertActiveTurnConsumer(t *testing.T, client *Client, turnID string) {
	t.Helper()
	if activeTurnID := activeTurnConsumer(client); activeTurnID != turnID {
		t.Fatalf("activeTurnID = %q, want %q", activeTurnID, turnID)
	}
}

func waitForActiveTurnConsumer(t *testing.T, client *Client, turnID string) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if activeTurnID := activeTurnConsumer(client); activeTurnID == turnID {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("activeTurnID = %q, want %q", activeTurnConsumer(client), turnID)
		case <-ticker.C:
		}
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

		client.notifications <- completed
		synctest.Wait()

		result := <-streamResult
		if result.err != nil {
			t.Fatalf("stream error = %v", result.err)
		}
		if diff := gocmp.Diff([]string{NotificationMethodTurnCompleted}, notificationMethods(result.notifications)); diff != "" {
			t.Fatalf("stream methods mismatch (-want +got):\n%s", diff)
		}
		if err := client.acquireTurnConsumer("turn_after_completion"); err != nil {
			t.Fatalf("acquireTurnConsumer() after stream completion error = %v", err)
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
			Params: mustJSON(t, Object{"phase": "first"}),
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

		client.notifications <- firstNotification
		synctest.Wait()

		got := <-result
		if got.err != nil {
			t.Fatalf("stream error = %v", got.err)
		}
		if diff := gocmp.Diff([]string{"custom/event"}, got.methods); diff != "" {
			t.Fatalf("stream methods mismatch (-want +got):\n%s", diff)
		}
		if err := client.acquireTurnConsumer("turn_after_early_stop"); err != nil {
			t.Fatalf("acquireTurnConsumer() after early stream stop error = %v", err)
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
		if err := client.acquireTurnConsumer("turn_after_cancel"); err != nil {
			t.Fatalf("acquireTurnConsumer() after stream cancellation error = %v", err)
		}
		client.releaseTurnConsumer("turn_after_cancel")
	})
}

func TestSecondTurnConsumerFailsPredictably(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient(nil, nil)
		if err := client.acquireTurnConsumer("turn_busy"); err != nil {
			t.Fatalf("initial acquireTurnConsumer() error = %v", err)
		}
		defer client.releaseTurnConsumer("turn_busy")

		handle := &TurnHandle{client: client, threadID: "thr_busy", turnID: "turn_other"}
		notifications, err := collectStream(handle.Stream(t.Context()))
		if err == nil {
			t.Fatalf("Stream() error = nil, want second consumer rejection")
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
	if client.activeTurnID != "" {
		t.Fatalf("activeTurnID = %q, want released after successful Run", client.activeTurnID)
	}

	nextHandle, err := thread.Turn(t.Context(), "follow-up", &TurnStartParams{Model: &model})
	if err != nil {
		t.Fatalf("Turn() after successful Run error = %v", err)
	}
	if _, err := collectStream(nextHandle.Stream(t.Context())); err != nil {
		t.Fatalf("stream error after successful Run = %v", err)
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
	if client.activeTurnID != "" {
		t.Fatalf("activeTurnID = %q, want released after failed Run", client.activeTurnID)
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

func TestTurnStreamRejectsSecondConsumerUntilFirstCancels(t *testing.T) {
	client := newHelperClient(t, "stream_cancel")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	thread := &Thread{client: client, id: "thr_stream"}
	firstHandle, err := thread.Turn(t.Context(), "first", nil)
	if err != nil {
		t.Fatalf("first Turn() error = %v", err)
	}
	firstCtx, firstCancel := context.WithCancel(t.Context())
	firstResult := collectStreamAsync(firstHandle.Stream(firstCtx))
	waitForActiveTurnConsumer(t, client, firstHandle.ID())

	secondHandle, err := thread.Turn(t.Context(), "second", nil)
	if err != nil {
		t.Fatalf("second Turn() error = %v", err)
	}
	secondCtx, secondCancel := context.WithCancel(t.Context())
	defer secondCancel()
	notifications, err := collectStream(secondHandle.Stream(secondCtx))
	if err == nil || !strings.Contains(err.Error(), "turn consumer already active") {
		t.Fatalf("second Stream() error = %v, want active-consumer failure", err)
	}
	if len(notifications) != 0 {
		t.Fatalf("second Stream() notifications len = %d, want 0 on active-consumer failure", len(notifications))
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

	secondCancel()
	notifications, err = collectStream(secondHandle.Stream(secondCtx))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("second stream error = %v, want context canceled", err)
	}
	if len(notifications) != 0 {
		t.Fatalf("second stream notifications len = %d, want 0", len(notifications))
	}
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
	}
}

var retryOnOverloadAttempts = map[string]int{}

func handleInitializeScenario(writer *bufio.Writer, method, id string) {
	if method == "initialize" {
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
	case "thread/start":
		writeJSON(writer, Object{"method": "thread/started", "params": Object{"threadId": "thr_protocol"}})
		writeJSON(writer, Object{"id": id, "result": Object{"thread": Object{"id": "thr_protocol"}}})
	case "model/list":
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
	if method == "thread/start" {
		writeRawJSONLine(writer, `{"method":"custom/event","params":{"details":{"answer":42},"kind":"raw"}}`)
		writeJSON(writer, Object{"id": id, "result": Object{"thread": Object{"id": "thr_notifications"}}})
		return
	}
	writeJSON(writer, Object{"id": id, "result": Object{}})
}

func handleRunScenario(writer *bufio.Writer, method, id string) {
	if method != "turn/start" {
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
	if method != "turn/start" {
		writeJSON(writer, Object{"id": id, "result": Object{}})
		return
	}
	writeJSON(writer, Object{"id": id, "result": Object{"turn": Object{"id": "turn_failed", "status": "inProgress"}}})
	writeJSON(writer, Object{"method": "turn/completed", "params": Object{"threadId": "thr_run_failed", "turn": Object{"id": "turn_failed", "status": "failed", "error": Object{"message": "boom"}}}})
}

func handleStreamSteerScenario(writer *bufio.Writer, method, id string) {
	switch method {
	case "turn/start":
		writeJSON(writer, Object{"id": id, "result": Object{"turn": Object{"id": "turn_stream", "status": "inProgress"}}})
	case "turn/steer":
		writeJSON(writer, Object{"id": id, "result": Object{}})
		writeJSON(writer, Object{"method": "item/agentMessage/delta", "params": Object{"threadId": "thr_stream", "turnId": "turn_stream", "delta": "ok"}})
		writeJSON(writer, Object{"method": "turn/completed", "params": Object{"turn": Object{"id": "turn_stream", "status": "completed"}}})
	default:
		writeJSON(writer, Object{"id": id, "result": Object{}})
	}
}

func handleStreamCancelScenario(writer *bufio.Writer, method, id string) {
	if method != "turn/start" {
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
	case "turn/start":
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
