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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/google/go-cmp/cmp"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestPublicAPIRuntimeBehaviorPortNewCodexInitFailureClosesClient(t *testing.T) {
	closeMarker := filepath.Join(t.TempDir(), "helper-closed")
	config := helperCodexConfig(t, "initialize_invalid_metadata")
	config.Env["CODEX_PORT_HELPER_CLOSE_MARKER"] = closeMarker

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	client, err := codex.NewCodex(ctx, config)
	if err == nil {
		t.Fatal("NewCodex() error = nil, want missing metadata validation error")
	}
	if client != nil {
		t.Fatalf("NewCodex() client = %#v, want nil on initialization failure", client)
	}
	if !strings.Contains(err.Error(), "missing required metadata") {
		t.Fatalf("NewCodex() error = %v, want missing required metadata", err)
	}

	got, readErr := os.ReadFile(closeMarker)
	if readErr != nil {
		t.Fatalf("os.ReadFile(close marker) error = %v; NewCodex must close the started helper process on init failure", readErr)
	}
	if string(got) != "closed\n" {
		t.Fatalf("close marker = %q, want closed marker", got)
	}
}

func TestPublicAPIRuntimeBehaviorPortConcurrentPublicCallsReuseInitializedClient(t *testing.T) {
	sdk := newHelperCodex(t, "async_client_behavior")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	const callCount = 2
	results := make([]codex.ModelListResponse, callCount)
	errs := make([]error, callCount)

	var ready sync.WaitGroup
	ready.Add(callCount)
	start := make(chan struct{})
	var done sync.WaitGroup
	done.Add(callCount)
	for i := range callCount {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			results[i], errs[i] = sdk.Models(ctx, false)
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Models() concurrent call %d error = %v", i, err)
		}
	}
	for i, result := range results {
		if len(result.Data) != 1 {
			t.Fatalf("Models() concurrent call %d returned %d models, want 1", i, len(result.Data))
		}
		if result.Data[0].ID != "gpt-overlap-2" {
			t.Fatalf("Models() concurrent call %d model id = %q, want gpt-overlap-2", i, result.Data[0].ID)
		}
	}
}

func TestPublicAPIRuntimeBehaviorPortApprovalModesSerializeToStartParams(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mode    codex.ApprovalMode
		want    map[string]any
		wantErr string
	}{
		"success: deny all maps to never": {
			mode: codex.ApprovalModeDenyAll,
			want: map[string]any{"approvalPolicy": "never"},
		},
		"success: auto review maps reviewer": {
			mode: codex.ApprovalModeAutoReview,
			want: map[string]any{"approvalPolicy": "on-request", "approvalsReviewer": "auto_review"},
		},
		"error: unknown approval mode is rejected before params": {
			mode:    codex.ApprovalMode("allow_all"),
			wantErr: "unsupported ApprovalMode",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			approval, reviewer, err := codex.ApprovalModeSettings(tt.mode)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("ApprovalModeSettings() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ApprovalModeSettings() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ApprovalModeSettings() error = %v", err)
			}

			params := codex.TurnStartParams{
				ThreadID:          "thread-1",
				Input:             []codex.UserInput{},
				ApprovalPolicy:    &approval,
				ApprovalsReviewer: reviewer,
			}
			got, err := publicAPIRuntimeBehaviorApprovalSettings(params)
			if err != nil {
				t.Fatalf("approval settings from TurnStartParams error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("approval settings mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPublicAPIRuntimeBehaviorPortRetryExampleComparesStatusWithEnum(t *testing.T) {
	t.Parallel()

	sourcePath := filepath.Join(artifactWorkflowRepoRoot(t), "pkg", "codex", "examples", "10_error_handling_and_retry", "main.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", sourcePath, err)
	}
	if strings.Contains(string(source), `== "failed"`) {
		t.Fatalf("%s compares turn status with raw string \"failed\"; want codex.TurnStatusFailed", sourcePath)
	}
	if !strings.Contains(string(source), "codex.TurnStatusFailed") {
		t.Fatalf("%s missing codex.TurnStatusFailed enum comparison", sourcePath)
	}
}

func publicAPIRuntimeBehaviorApprovalSettings(params codex.TurnStartParams) (map[string]any, error) {
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var dumped map[string]any
	if err := json.Unmarshal(encoded, &dumped); err != nil {
		return nil, err
	}
	for key := range dumped {
		if key != "approvalPolicy" && key != "approvalsReviewer" {
			delete(dumped, key)
		}
	}
	return dumped, nil
}
