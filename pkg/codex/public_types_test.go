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
	"reflect"
	"testing"

	"github.com/go-json-experiment/json"
)

var (
	_ json.MarshalerTo     = AskForApproval{}
	_ json.UnmarshalerFrom = (*AskForApproval)(nil)
)

// TestPublicGeneratedTypeRenamePolicy keeps the documented collision-safe
// generated names stable. The package must continue to expose payload Go names
// (`ConfigPayload` and `ThreadPayload`) rather than the upstream Python schema
// names that would collide with the higher-level SDK surface.
func TestPublicGeneratedTypeRenamePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
	}{
		{name: "AskForApproval", value: AskForApproval{}},
		{name: "ConfigPayload", value: ConfigPayload{}},
		{name: "ThreadPayload", value: ThreadPayload{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			typ := reflect.TypeOf(tt.value)
			if got := typ.Name(); got != tt.name {
				t.Fatalf("type name = %q, want %q", got, tt.name)
			}
			if got := typ.PkgPath(); got != "github.com/zchee/pandaemonium/pkg/codex" {
				t.Fatalf("type package path = %q, want pkg/codex", got)
			}
		})
	}
}

func TestPublicGeneratedInterfaceUnionParityTypes(t *testing.T) {
	t.Parallel()

	var (
		_ CodexErrorInfo   = CodexErrorInfoValueOther
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
		{name: "ActiveTurnNotSteerableCodexErrorInfo", typ: reflect.TypeFor[ActiveTurnNotSteerableCodexErrorInfo]()},
		{name: "ReasoningSummaryValue", typ: reflect.TypeFor[ReasoningSummaryValue]()},
		{name: "SessionSourceValue", typ: reflect.TypeFor[SessionSourceValue]()},
		{name: "CustomSessionSource", typ: reflect.TypeFor[CustomSessionSource]()},
		{name: "SubAgentSessionSource", typ: reflect.TypeFor[SubAgentSessionSource]()},
		{name: "SubAgentSourceValue", typ: reflect.TypeFor[SubAgentSourceValue]()},
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
