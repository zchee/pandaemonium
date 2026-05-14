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

package codex_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/google/go-cmp/cmp"

	codex "github.com/zchee/pandaemonium/pkg/codex"
)

func TestPublicAPISurfaceMatchesPythonSDKIntent(t *testing.T) {
	t.Parallel()

	var (
		_ *codex.Config
		_ *codex.Codex
		_ *codex.Client
		_ *codex.Thread
		_ *codex.TurnHandle
		_ *codex.StreamThread
		_ *codex.StreamTurnHandle
		_ codex.InitializeResponse
		_ codex.RunResult
		_ codex.InputItem = codex.TextInput{}
		_ codex.InputItem = codex.ImageInput{}
		_ codex.InputItem = codex.LocalImageInput{}
		_ codex.InputItem = codex.SkillInput{}
		_ codex.InputItem = codex.MentionInput{}
		_ *codex.AppServerError
		_ *codex.TransportClosedError
		_ *codex.JSONRPCError
		_ *codex.AppServerRPCError
		_ *codex.ParseError
		_ *codex.InvalidRequestError
		_ *codex.MethodNotFoundError
		_ *codex.InvalidParamsError
		_ *codex.InternalRPCError
		_ *codex.ServerBusyError
		_ *codex.RetryLimitExceededError
		_ codex.Notification
		_ codex.KnownNotification
		_ codex.ThreadTokenUsageUpdatedNotification
		_ codex.TurnCompletedNotification
		_ codex.ModelListResponse
		_ codex.ThreadListResponse
		_ codex.ThreadReadResponse
		_ codex.ThreadArchiveResponse
		_ codex.ThreadCompactStartResponse
		_ codex.ThreadSetNameResponse
		_ codex.TurnInterruptResponse
		_ codex.TurnSteerResponse
	)

	rootExports := map[string]reflect.Type{
		"Config":                    reflect.TypeFor[codex.Config](),
		"Codex":                     reflect.TypeFor[codex.Codex](),
		"Client":                    reflect.TypeFor[codex.Client](),
		"ApprovalMode":              reflect.TypeFor[codex.ApprovalMode](),
		"Thread":                    reflect.TypeFor[codex.Thread](),
		"TurnHandle":                reflect.TypeFor[codex.TurnHandle](),
		"RunResult":                 reflect.TypeFor[codex.RunResult](),
		"InputItem":                 reflect.TypeFor[codex.InputItem](),
		"TextInput":                 reflect.TypeFor[codex.TextInput](),
		"ImageInput":                reflect.TypeFor[codex.ImageInput](),
		"LocalImageInput":           reflect.TypeFor[codex.LocalImageInput](),
		"SkillInput":                reflect.TypeFor[codex.SkillInput](),
		"MentionInput":              reflect.TypeFor[codex.MentionInput](),
		"RetryConfig":               reflect.TypeFor[codex.RetryConfig](),
		"Notification":              reflect.TypeFor[codex.Notification](),
		"InitializeResponse":        reflect.TypeFor[codex.InitializeResponse](),
		"ThreadStartParams":         reflect.TypeFor[codex.ThreadStartParams](),
		"TurnStartParams":           reflect.TypeFor[codex.TurnStartParams](),
		"TurnCompletedNotification": reflect.TypeFor[codex.TurnCompletedNotification](),
		"TurnStatus":                reflect.TypeFor[codex.TurnStatus](),
	}
	for name, typ := range rootExports {
		if typ.Name() == "" && typ.Kind() != reflect.Interface {
			t.Fatalf("%s has empty exported type name: %v", name, typ)
		}
	}
}

func TestApprovalModesSerializeToExpectedStartParams(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mode    codex.ApprovalMode
		want    map[string]any
		wantErr bool
	}{
		"success: deny all maps to never": {
			mode: codex.ApprovalModeDenyAll,
			want: map[string]any{"approvalPolicy": "never"},
		},
		"success: auto review maps reviewer": {
			mode: codex.ApprovalModeAutoReview,
			want: map[string]any{"approvalPolicy": "on-request", "approvalsReviewer": "auto_review"},
		},
		"error: unknown approval mode": {
			mode:    codex.ApprovalMode("allow_all"),
			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			approval, reviewer, err := codex.ApprovalModeSettings(tt.mode)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ApprovalModeSettings() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ApprovalModeSettings() error = %v", err)
			}
			params := codex.TurnStartParams{ApprovalPolicy: &approval, ApprovalsReviewer: reviewer}
			encoded, err := json.Marshal(params)
			if err != nil {
				t.Fatalf("json.Marshal(TurnStartParams) error = %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatalf("json.Unmarshal(TurnStartParams) error = %v", err)
			}
			for key := range got {
				if _, ok := tt.want[key]; !ok {
					delete(got, key)
				}
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("approval settings mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGeneratedProtocolPublicBehaviorPort(t *testing.T) {
	t.Parallel()

	searchTerm := "needle"
	limit := int32(5)
	params := codex.ThreadListParams{SearchTerm: &searchTerm, Limit: &limit}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal(ThreadListParams) error = %v", err)
	}
	var dumped map[string]any
	if err := json.Unmarshal(encoded, &dumped); err != nil {
		t.Fatalf("json.Unmarshal(ThreadListParams) error = %v", err)
	}
	if diff := cmp.Diff(map[string]any{"searchTerm": "needle", "limit": float64(5)}, dumped); diff != "" {
		t.Fatalf("ThreadListParams JSON mismatch (-want +got):\n%s", diff)
	}

	var resumed codex.ThreadResumeResponse
	if err := json.Unmarshal([]byte(`{
		"approvalPolicy":"on-request",
		"approvalsReviewer":"auto_review",
		"cwd":"/tmp",
		"model":"gpt-test",
		"modelProvider":"openai",
		"sandbox":{"type":"dangerFullAccess"},
		"thread":{
			"id":"thread-1",
			"sessionId":"session-1",
			"createdAt":1,
			"updatedAt":1,
			"cwd":"/tmp",
			"modelProvider":"openai",
			"preview":"",
			"source":"cli",
			"status":{"type":"idle"},
			"turns":[],
			"ephemeral":false,
			"cliVersion":"1.2.3"
		}
	}`), &resumed); err != nil {
		t.Fatalf("json.Unmarshal(ThreadResumeResponse) error = %v", err)
	}
	if resumed.ApprovalsReviewer != codex.ApprovalsReviewerAutoReview {
		t.Fatalf("ApprovalsReviewer = %q, want auto_review", resumed.ApprovalsReviewer)
	}

	event := codex.Notification{
		Method: codex.NotificationMethodThreadTokenUsageUpdated,
		Params: mustJSONValue(t, map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"tokenUsage": map[string]any{
				"last":  map[string]any{"cachedInputTokens": 0, "inputTokens": 1, "outputTokens": 2, "reasoningOutputTokens": 0, "totalTokens": 3},
				"total": map[string]any{"cachedInputTokens": 0, "inputTokens": 1, "outputTokens": 2, "reasoningOutputTokens": 0, "totalTokens": 3},
			},
		}),
	}
	usage, ok, err := event.ThreadTokenUsageUpdated()
	if err != nil {
		t.Fatalf("ThreadTokenUsageUpdated() error = %v", err)
	}
	if !ok || usage.TurnID != "turn-1" || usage.TokenUsage.Last.TotalTokens != 3 {
		t.Fatalf("ThreadTokenUsageUpdated() = (%#v, %v), want typed usage", usage, ok)
	}

	unknown := codex.Notification{Method: "unknown/notification", Params: mustJSONValue(t, map[string]any{"msg": map[string]any{"type": "turn_aborted"}})}
	if got, matched, err := codex.DecodeNotification(unknown); err != nil || matched || got.Raw.Method != unknown.Method {
		t.Fatalf("DecodeNotification(unknown) = (%#v, %v, %v), want raw unmatched payload", got, matched, err)
	}
}

func TestPublicClientRoutingAndRetryPort(t *testing.T) {
	client := newHelperClient(t, "client_routing_retry")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	stream := client.StreamText(ctx, "thread-route", "route this", nil)
	var deltas []string
	for delta, err := range stream {
		if err != nil {
			t.Fatalf("StreamText() error = %v", err)
		}
		deltas = append(deltas, delta.Delta)
	}
	if diff := cmp.Diff([]string{"first"}, deltas); diff != "" {
		t.Fatalf("StreamText deltas mismatch (-want +got):\n%s", diff)
	}

	got, err := codex.RequestWithRetryOnOverload[string](ctx, client, "ping", nil, codex.RetryConfig{
		MaxAttempts:  2,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Millisecond,
		JitterRatio:  -1,
	})
	if err != nil {
		t.Fatalf("RequestWithRetryOnOverload() error = %v", err)
	}
	if got != "ok" {
		t.Fatalf("RequestWithRetryOnOverload() = %q, want ok", got)
	}
}

func TestRetryableErrorClassificationPort(t *testing.T) {
	t.Parallel()

	_, err := codex.RetryOnOverload[string](t.Context(), codex.RetryConfig{MaxAttempts: 1}, func() (string, error) {
		return "", context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RetryOnOverload non-retryable error = %v, want context.Canceled", err)
	}
}

func mustJSONValue(t *testing.T, value any) jsontext.Value {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%T) error = %v", value, err)
	}
	return jsontext.Value(encoded)
}
