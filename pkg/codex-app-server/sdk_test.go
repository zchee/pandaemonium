// Copyright 2026 The omxx Authors.
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
	"os"
	"testing"
	"time"

	json "github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/google/go-cmp/cmp"

	"github.com/zchee/omxx/pkg/codex-app-server/protocol"
)

func TestRootAPIUsesProtocolTypesDirectly(t *testing.T) {
	client := NewClient(nil, nil)
	var modelList func(context.Context, bool) (protocol.ModelListResponse, error) = client.ModelList
	var threadStart func(context.Context, *ThreadStartParams) (protocol.ThreadStartResponse, error) = client.ThreadStart

	var runResult RunResult
	var _ []protocol.ThreadItem = runResult.Items
	var _ *protocol.ThreadTokenUsage = runResult.Usage
	var _ protocol.Turn = runResult.Turn

	_ = modelList
	_ = threadStart
}

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
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("normalizeInput() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestJSONRPCErrorMapping(t *testing.T) {
	tests := map[string]struct {
		code      int64
		message   string
		data      string
		kind      string
		retryable bool
	}{
		"success: invalid params is not retryable": {
			code:    -32602,
			message: "bad params",
			data:    `null`,
			kind:    "invalid_params",
		},
		"success: nested server overloaded is retryable": {
			code:      -32000,
			message:   "busy",
			data:      `{"errorInfo":{"reason":"server_overloaded"}}`,
			kind:      "server_busy",
			retryable: true,
		},
		"success: retry limit text is retryable but classified": {
			code:      -32000,
			message:   "too many failed attempts",
			data:      `{}`,
			kind:      "retry_limit_exceeded",
			retryable: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := mapJSONRPCError(tt.code, tt.message, jsontext.Value(tt.data))
			var rpcErr *JSONRPCError
			if !errors.As(err, &rpcErr) {
				t.Fatalf("error type = %T, want *JSONRPCError", err)
			}
			if rpcErr.Kind != tt.kind {
				t.Fatalf("kind = %q, want %q", rpcErr.Kind, tt.kind)
			}
			if got := IsServerBusy(err); got != tt.retryable {
				t.Fatalf("IsServerBusy() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestRetryOnOverload(t *testing.T) {
	tests := map[string]struct {
		errs      []error
		wantCalls int
		wantErr   bool
	}{
		"success: retries retryable overload": {
			errs: []error{
				mapJSONRPCError(-32000, "busy", jsontext.Value(`{"codexErrorInfo":"server_overloaded"}`)),
				nil,
			},
			wantCalls: 2,
		},
		"success: does not retry invalid params": {
			errs:      []error{mapJSONRPCError(-32602, "bad", jsontext.Value(`null`))},
			wantCalls: 1,
			wantErr:   true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			calls := 0
			_, err := RetryOnOverload(t.Context(), RetryConfig{MaxAttempts: 3, InitialDelay: time.Nanosecond, MaxDelay: time.Nanosecond}, func() (string, error) {
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
			if diff := cmp.Diff(tt.want, got); diff != "" {
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

	started, err := client.ThreadStart(t.Context(), &ThreadStartParams{Model: "gpt-5.4"})
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

func TestThreadRunCollectsFinalResponseAndUsage(t *testing.T) {
	client := newHelperClient(t, "run")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	thread := &Thread{client: client, id: "thr_run"}
	result, err := thread.Run(t.Context(), "hello", &TurnStartParams{Model: "gpt-5.4"})
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
	if result.Turn.ID != "turn_run" || result.Turn.Status != protocol.TurnStatusCompleted {
		t.Fatalf("turn = %#v, want completed turn_run", result.Turn)
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
	notifications, errs, err := handle.Stream(t.Context())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	steerCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if _, err := handle.Steer(steerCtx, TextInput{Text: "continue"}); err != nil {
		t.Fatalf("Steer() while streaming error = %v", err)
	}
	var methods []string
	for notification := range notifications {
		methods = append(methods, notification.Method)
	}
	if err := <-errs; err != nil {
		t.Fatalf("stream error = %v", err)
	}
	want := []string{"item/agentMessage/delta", "turn/completed"}
	if diff := cmp.Diff(want, methods); diff != "" {
		t.Fatalf("stream methods mismatch (-want +got):\n%s", diff)
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
		if scenario == "run" {
			handleRunScenario(writer, method, id)
			continue
		}
		if scenario == "stream_steer" {
			handleStreamSteerScenario(writer, method, id)
			continue
		}
	}
}

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

func usage(total int64) Object {
	return Object{"cachedInputTokens": int64(0), "inputTokens": total, "outputTokens": int64(0), "reasoningOutputTokens": int64(0), "totalTokens": total}
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
