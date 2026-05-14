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
	"reflect"
	"strings"
	"testing"

	"github.com/go-json-experiment/json"
)

var (
	_ AskForApproval       = AskForApprovalValueNever
	_ AskForApproval       = GranularAskForApproval{}
	_ AskForApproval       = RawAskForApproval{}
	_ json.MarshalerTo     = RawAskForApproval{}
	_ json.UnmarshalerFrom = (*RawAskForApproval)(nil)
)

// TestPublicGeneratedTypeRenamePolicy keeps the documented collision-safe
// generated names stable. The package must continue to expose payload Go names
// (`ConfigPayload` and `ThreadPayload`) rather than the upstream Python schema
// names that would collide with the higher-level SDK surface.
func TestPublicGeneratedTypeRenamePolicy(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		typ reflect.Type
	}{
		"success: AskForApproval": {
			typ: reflect.TypeFor[AskForApproval](),
		},
		"success: ConfigPayload": {
			typ: reflect.TypeFor[ConfigPayload](),
		},
		"success: ThreadPayload": {
			typ: reflect.TypeFor[ThreadPayload](),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			want := strings.TrimPrefix(name, "success: ")
			if got := tt.typ.Name(); got != want {
				t.Fatalf("type name = %q, want %q", got, want)
			}
			if got := tt.typ.PkgPath(); got != "github.com/zchee/pandaemonium/pkg/codex" {
				t.Fatalf("type package path = %q, want pkg/codex", got)
			}
		})
	}
}

func TestPublicGeneratedInterfaceUnionParityTypes(t *testing.T) {
	t.Parallel()

	var (
		_ CodexErrorInfo   = CodexErrorInfoValueOther
		_ CodexErrorInfo   = HTTPConnectionFailedCodexErrorInfo{}
		_ CodexErrorInfo   = ResponseStreamConnectionFailedCodexErrorInfo{}
		_ CodexErrorInfo   = ResponseStreamDisconnectedCodexErrorInfo{}
		_ CodexErrorInfo   = ResponseTooManyFailedAttemptsCodexErrorInfo{}
		_ CodexErrorInfo   = ActiveTurnNotSteerableCodexErrorInfo{}
		_ CodexErrorInfo   = RawCodexErrorInfo{}
		_ ReasoningSummary = ReasoningSummaryValueNone
		_ ReasoningSummary = RawReasoningSummary{}
		_ SessionSource    = SessionSourceValueCli
		_ SessionSource    = CustomSessionSource{}
		_ SessionSource    = SubAgentSessionSource{}
		_ SessionSource    = RawSessionSource{}
		_ SubAgentSource   = SubAgentSourceValueReview
		_ SubAgentSource   = ThreadSpawnSubAgentSource{}
		_ SubAgentSource   = OtherSubAgentSource{}
		_ SubAgentSource   = RawSubAgentSource{}
	)

	tests := []struct {
		name string
		typ  reflect.Type
	}{
		{name: "CodexErrorInfoValue", typ: reflect.TypeFor[CodexErrorInfoValue]()},
		{name: "HTTPConnectionFailed", typ: reflect.TypeFor[HTTPConnectionFailed]()},
		{name: "HTTPConnectionFailedCodexErrorInfo", typ: reflect.TypeFor[HTTPConnectionFailedCodexErrorInfo]()},
		{name: "ResponseStreamConnectionFailed", typ: reflect.TypeFor[ResponseStreamConnectionFailed]()},
		{name: "ResponseStreamConnectionFailedCodexErrorInfo", typ: reflect.TypeFor[ResponseStreamConnectionFailedCodexErrorInfo]()},
		{name: "ResponseStreamDisconnected", typ: reflect.TypeFor[ResponseStreamDisconnected]()},
		{name: "ResponseStreamDisconnectedCodexErrorInfo", typ: reflect.TypeFor[ResponseStreamDisconnectedCodexErrorInfo]()},
		{name: "ResponseTooManyFailedAttempts", typ: reflect.TypeFor[ResponseTooManyFailedAttempts]()},
		{name: "ResponseTooManyFailedAttemptsCodexErrorInfo", typ: reflect.TypeFor[ResponseTooManyFailedAttemptsCodexErrorInfo]()},
		{name: "ActiveTurnNotSteerable", typ: reflect.TypeFor[ActiveTurnNotSteerable]()},
		{name: "ActiveTurnNotSteerableCodexErrorInfo", typ: reflect.TypeFor[ActiveTurnNotSteerableCodexErrorInfo]()},
		{name: "ReasoningSummaryValue", typ: reflect.TypeFor[ReasoningSummaryValue]()},
		{name: "SessionSourceValue", typ: reflect.TypeFor[SessionSourceValue]()},
		{name: "CustomSessionSource", typ: reflect.TypeFor[CustomSessionSource]()},
		{name: "SubAgentSessionSource", typ: reflect.TypeFor[SubAgentSessionSource]()},
		{name: "SubAgentSourceValue", typ: reflect.TypeFor[SubAgentSourceValue]()},
		{name: "ThreadSpawn", typ: reflect.TypeFor[ThreadSpawn]()},
		{name: "ThreadSpawnSubAgentSource", typ: reflect.TypeFor[ThreadSpawnSubAgentSource]()},
		{name: "OtherSubAgentSource", typ: reflect.TypeFor[OtherSubAgentSource]()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.typ.Name(); got != tt.name {
				t.Fatalf("type name = %q, want %q", got, tt.name)
			}
		})
	}
}

func TestApprovalModeSettings(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mode         ApprovalMode
		wantApproval string
		wantReviewer *ApprovalsReviewer
		wantErr      bool
	}{
		"success: deny all": {
			mode:         ApprovalModeDenyAll,
			wantApproval: `"never"`,
		},
		"success: auto review": {
			mode:         ApprovalModeAutoReview,
			wantApproval: `"on-request"`,
			wantReviewer: ptr(ApprovalsReviewerAutoReview),
		},
		"error: unsupported approval mode": {
			mode:    ApprovalMode("future"),
			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotApproval, gotReviewer, err := ApprovalModeSettings(tt.mode)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ApprovalModeSettings() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ApprovalModeSettings() error = %v", err)
			}
			gotBytes, err := json.Marshal(gotApproval)
			if err != nil {
				t.Fatalf("json.Marshal(AskForApproval) error = %v", err)
			}
			if got := string(gotBytes); got != tt.wantApproval {
				t.Fatalf("approval JSON = %s, want %s", got, tt.wantApproval)
			}
			if tt.wantReviewer == nil {
				if gotReviewer != nil {
					t.Fatalf("reviewer = %q, want nil", *gotReviewer)
				}
				return
			}
			if gotReviewer == nil || *gotReviewer != *tt.wantReviewer {
				t.Fatalf("reviewer = %#v, want %#v", gotReviewer, tt.wantReviewer)
			}
		})
	}
}

func TestApprovalModeOverrideSettings(t *testing.T) {
	t.Parallel()

	gotApproval, gotReviewer, err := ApprovalModeOverrideSettings(nil)
	if err != nil {
		t.Fatalf("ApprovalModeOverrideSettings(nil) error = %v", err)
	}
	if gotApproval != nil || gotReviewer != nil {
		t.Fatalf("ApprovalModeOverrideSettings(nil) = (%#v, %#v), want nil pointers", gotApproval, gotReviewer)
	}

	mode := ApprovalModeAutoReview
	gotApproval, gotReviewer, err = ApprovalModeOverrideSettings(&mode)
	if err != nil {
		t.Fatalf("ApprovalModeOverrideSettings(auto_review) error = %v", err)
	}
	if gotApproval == nil {
		t.Fatal("approval pointer = nil, want value")
	}
	gotBytes, err := json.Marshal(*gotApproval)
	if err != nil {
		t.Fatalf("json.Marshal(AskForApproval) error = %v", err)
	}
	if got := string(gotBytes); got != `"on-request"` {
		t.Fatalf("approval JSON = %s, want \"on-request\"", got)
	}
	if gotReviewer == nil || *gotReviewer != ApprovalsReviewerAutoReview {
		t.Fatalf("reviewer = %#v, want auto_review", gotReviewer)
	}
}

func ptr[T any](value T) *T {
	return &value
}
