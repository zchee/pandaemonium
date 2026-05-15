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
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestPublicAPISignaturePortRootExports(t *testing.T) {
	t.Parallel()

	var (
		_ *codex.Config
		_ codex.ListenConfig
		_ codex.WebSocketConfig
		_ codex.WebSocketAuthMode
		_ *codex.Codex
		_ *codex.Client
		_ codex.ApprovalMode
		_ *codex.Thread
		_ *codex.StreamThread
		_ *codex.TurnHandle
		_ *codex.StreamTurnHandle
		_ codex.RunResult
		_ codex.InputItem = codex.TextInput{}
		_ codex.InputItem = codex.ImageInput{}
		_ codex.InputItem = codex.LocalImageInput{}
		_ codex.InputItem = codex.SkillInput{}
		_ codex.InputItem = codex.MentionInput{}
		_ codex.RetryConfig
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
	)

	types := map[string]reflect.Type{
		"Config":                  reflect.TypeFor[codex.Config](),
		"ListenConfig":            reflect.TypeFor[codex.ListenConfig](),
		"WebSocketConfig":         reflect.TypeFor[codex.WebSocketConfig](),
		"WebSocketAuthMode":       reflect.TypeFor[codex.WebSocketAuthMode](),
		"Codex":                   reflect.TypeFor[codex.Codex](),
		"Client":                  reflect.TypeFor[codex.Client](),
		"ApprovalMode":            reflect.TypeFor[codex.ApprovalMode](),
		"Thread":                  reflect.TypeFor[codex.Thread](),
		"StreamThread":            reflect.TypeFor[codex.StreamThread](),
		"TurnHandle":              reflect.TypeFor[codex.TurnHandle](),
		"StreamTurnHandle":        reflect.TypeFor[codex.StreamTurnHandle](),
		"RunResult":               reflect.TypeFor[codex.RunResult](),
		"InputItem":               reflect.TypeFor[codex.InputItem](),
		"TextInput":               reflect.TypeFor[codex.TextInput](),
		"ImageInput":              reflect.TypeFor[codex.ImageInput](),
		"LocalImageInput":         reflect.TypeFor[codex.LocalImageInput](),
		"SkillInput":              reflect.TypeFor[codex.SkillInput](),
		"MentionInput":            reflect.TypeFor[codex.MentionInput](),
		"RetryConfig":             reflect.TypeFor[codex.RetryConfig](),
		"AppServerError":          reflect.TypeFor[codex.AppServerError](),
		"TransportClosedError":    reflect.TypeFor[codex.TransportClosedError](),
		"JSONRPCError":            reflect.TypeFor[codex.JSONRPCError](),
		"AppServerRPCError":       reflect.TypeFor[codex.AppServerRPCError](),
		"ParseError":              reflect.TypeFor[codex.ParseError](),
		"InvalidRequestError":     reflect.TypeFor[codex.InvalidRequestError](),
		"MethodNotFoundError":     reflect.TypeFor[codex.MethodNotFoundError](),
		"InvalidParamsError":      reflect.TypeFor[codex.InvalidParamsError](),
		"InternalRPCError":        reflect.TypeFor[codex.InternalRPCError](),
		"ServerBusyError":         reflect.TypeFor[codex.ServerBusyError](),
		"RetryLimitExceededError": reflect.TypeFor[codex.RetryLimitExceededError](),
	}
	assertNamedPublicTypes(t, types)

	if got := string(codex.ApprovalModeDenyAll); got != "deny_all" {
		t.Fatalf("ApprovalModeDenyAll = %q, want deny_all", got)
	}
	if got := string(codex.ApprovalModeAutoReview); got != "auto_review" {
		t.Fatalf("ApprovalModeAutoReview = %q, want auto_review", got)
	}
}

func TestPublicAPISignaturePortTypesExports(t *testing.T) {
	t.Parallel()

	var (
		_ codex.ApprovalsReviewer
		_ codex.AskForApproval
		_ codex.InitializeResponse
		_ codex.Object
		_ codex.ModelListResponse
		_ codex.Notification
		_ codex.Personality
		_ codex.PlanType
		_ codex.ReasoningEffort
		_ codex.ReasoningSummary
		_ codex.SandboxMode
		_ codex.SandboxPolicy
		_ codex.SortDirection
		_ codex.ThreadArchiveResponse
		_ codex.ThreadCompactStartResponse
		_ codex.ThreadItem
		_ codex.ThreadListCwdFilter
		_ codex.ThreadListResponse
		_ codex.ThreadReadResponse
		_ codex.ThreadSetNameResponse
		_ codex.ThreadSortKey
		_ codex.ThreadSource
		_ codex.ThreadSourceKind
		_ codex.ThreadStartSource
		_ codex.ThreadTokenUsage
		_ codex.ThreadTokenUsageUpdatedNotification
		_ codex.Turn
		_ codex.TurnCompletedNotification
		_ codex.TurnInterruptResponse
		_ codex.TurnStatus
		_ codex.TurnSteerResponse
	)

	types := map[string]reflect.Type{
		"ApprovalsReviewer":                   reflect.TypeFor[codex.ApprovalsReviewer](),
		"AskForApproval":                      reflect.TypeFor[codex.AskForApproval](),
		"InitializeResponse":                  reflect.TypeFor[codex.InitializeResponse](),
		"ModelListResponse":                   reflect.TypeFor[codex.ModelListResponse](),
		"Notification":                        reflect.TypeFor[codex.Notification](),
		"Personality":                         reflect.TypeFor[codex.Personality](),
		"PlanType":                            reflect.TypeFor[codex.PlanType](),
		"ReasoningEffort":                     reflect.TypeFor[codex.ReasoningEffort](),
		"ReasoningSummary":                    reflect.TypeFor[codex.ReasoningSummary](),
		"SandboxMode":                         reflect.TypeFor[codex.SandboxMode](),
		"SandboxPolicy":                       reflect.TypeFor[codex.SandboxPolicy](),
		"SortDirection":                       reflect.TypeFor[codex.SortDirection](),
		"ThreadArchiveResponse":               reflect.TypeFor[codex.ThreadArchiveResponse](),
		"ThreadCompactStartResponse":          reflect.TypeFor[codex.ThreadCompactStartResponse](),
		"ThreadItem":                          reflect.TypeFor[codex.ThreadItem](),
		"ThreadListCwdFilter":                 reflect.TypeFor[codex.ThreadListCwdFilter](),
		"ThreadListResponse":                  reflect.TypeFor[codex.ThreadListResponse](),
		"ThreadReadResponse":                  reflect.TypeFor[codex.ThreadReadResponse](),
		"ThreadSetNameResponse":               reflect.TypeFor[codex.ThreadSetNameResponse](),
		"ThreadSortKey":                       reflect.TypeFor[codex.ThreadSortKey](),
		"ThreadSource":                        reflect.TypeFor[codex.ThreadSource](),
		"ThreadSourceKind":                    reflect.TypeFor[codex.ThreadSourceKind](),
		"ThreadStartSource":                   reflect.TypeFor[codex.ThreadStartSource](),
		"ThreadTokenUsage":                    reflect.TypeFor[codex.ThreadTokenUsage](),
		"ThreadTokenUsageUpdatedNotification": reflect.TypeFor[codex.ThreadTokenUsageUpdatedNotification](),
		"Turn":                                reflect.TypeFor[codex.Turn](),
		"TurnCompletedNotification":           reflect.TypeFor[codex.TurnCompletedNotification](),
		"TurnInterruptResponse":               reflect.TypeFor[codex.TurnInterruptResponse](),
		"TurnStatus":                          reflect.TypeFor[codex.TurnStatus](),
		"TurnSteerResponse":                   reflect.TypeFor[codex.TurnSteerResponse](),
	}
	assertNamedPublicTypes(t, types)

	objectType := reflect.TypeFor[codex.Object]()
	if objectType.Kind() != reflect.Map || objectType.Key().Kind() != reflect.String {
		t.Fatalf("Object kind = %v[%v], want map[string]any", objectType.Kind(), objectType.Key())
	}
}

func TestPublicAPISignaturePortHighLevelMethodSignatures(t *testing.T) {
	t.Parallel()

	anyType := reflect.TypeFor[any]()
	contextType := reflect.TypeFor[context.Context]()
	errorType := reflect.TypeFor[error]()
	tests := map[string]struct {
		typ  reflect.Type
		name string
		in   []reflect.Type
		out  []reflect.Type
	}{
		"success: Codex ThreadStart": {
			typ:  reflect.TypeFor[*codex.Codex](),
			name: "ThreadStart",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[*codex.ThreadStartParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[*codex.Thread](),
				errorType,
			},
		},
		"success: Codex ThreadList": {
			typ:  reflect.TypeFor[*codex.Codex](),
			name: "ThreadList",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[*codex.ThreadListParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.ThreadListResponse](),
				errorType,
			},
		},
		"success: Codex ThreadResume": {
			typ:  reflect.TypeFor[*codex.Codex](),
			name: "ThreadResume",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[string](),
				reflect.TypeFor[*codex.ThreadResumeParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[*codex.Thread](),
				errorType,
			},
		},
		"success: Codex ThreadFork": {
			typ:  reflect.TypeFor[*codex.Codex](),
			name: "ThreadFork",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[string](),
				reflect.TypeFor[*codex.ThreadForkParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[*codex.Thread](),
				errorType,
			},
		},
		"success: Thread Turn": {
			typ:  reflect.TypeFor[*codex.Thread](),
			name: "Turn",
			in: []reflect.Type{
				contextType,
				anyType,
				reflect.TypeFor[*codex.TurnStartParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[*codex.TurnHandle](),
				errorType,
			},
		},
		"success: Thread Run": {
			typ:  reflect.TypeFor[*codex.Thread](),
			name: "Run",
			in: []reflect.Type{
				contextType,
				anyType,
				reflect.TypeFor[*codex.TurnStartParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.RunResult](),
				errorType,
			},
		},
		"success: TurnHandle Stream": {
			typ:  reflect.TypeFor[*codex.TurnHandle](),
			name: "Stream",
			in: []reflect.Type{
				contextType,
			},
			out: []reflect.Type{
				reflect.TypeFor[iter.Seq2[codex.Notification, error]](),
			},
		},
		"success: StreamThread Turn": {
			typ:  reflect.TypeFor[*codex.StreamThread](),
			name: "Turn",
			in: []reflect.Type{
				contextType,
				anyType,
				reflect.TypeFor[*codex.TurnStartParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[*codex.StreamTurnHandle](),
				errorType,
			},
		},
		"success: StreamThread RunStream": {
			typ:  reflect.TypeFor[*codex.StreamThread](),
			name: "RunStream",
			in: []reflect.Type{
				contextType,
				anyType,
				reflect.TypeFor[*codex.TurnStartParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[iter.Seq2[codex.Notification, error]](),
			},
		},
		"success: StreamTurnHandle Stream": {
			typ:  reflect.TypeFor[*codex.StreamTurnHandle](),
			name: "Stream",
			in: []reflect.Type{
				contextType,
			},
			out: []reflect.Type{
				reflect.TypeFor[iter.Seq2[codex.Notification, error]](),
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			method, ok := tt.typ.MethodByName(tt.name)
			if !ok {
				t.Fatalf("%v.%s missing", tt.typ, tt.name)
			}
			assertMethodSignature(t, method, tt.in, tt.out)
		})
	}
}

func TestPublicAPISignaturePortLifecycleMethodOwnership(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"ThreadResume",
		"ThreadFork",
		"ThreadArchive",
		"ThreadUnarchive",
		"StreamThreadResume",
		"StreamThreadFork",
		"StreamThreadUnarchive",
	} {
		if _, ok := reflect.TypeFor[*codex.Codex]().MethodByName(name); !ok {
			t.Fatalf("Codex.%s missing", name)
		}
	}

	for _, typ := range []reflect.Type{
		reflect.TypeFor[*codex.Thread](),
		reflect.TypeFor[*codex.StreamThread](),
	} {
		for _, name := range []string{"Resume", "Fork", "Archive", "Unarchive"} {
			if _, ok := typ.MethodByName(name); ok {
				t.Fatalf("%v.%s exists; lifecycle operations must stay Codex-scoped", typ, name)
			}
		}
	}
}

func TestPublicAPISignaturePortExamplesUsePublicImports(t *testing.T) {
	t.Parallel()

	examplesRoot := filepath.Join("..", "examples")
	privateMarkers := []string{
		"github.com/zchee/pandaemonium/pkg/codex/internal",
		"github.com/zchee/pandaemonium/pkg/codex/client",
		"github.com/zchee/pandaemonium/pkg/codex/generated",
		"github.com/zchee/pandaemonium/pkg/codex/retry",
		"protocol_gen.go",
	}

	offenders := map[string][]string{}
	err := filepath.WalkDir(examplesRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if ext := filepath.Ext(path); ext != ".go" && ext != ".md" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, marker := range privateMarkers {
			if strings.Contains(string(content), marker) {
				rel, relErr := filepath.Rel(examplesRoot, path)
				if relErr != nil {
					return relErr
				}
				offenders[rel] = append(offenders[rel], marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk examples: %v", err)
	}
	if len(offenders) != 0 {
		t.Fatalf("examples import private surfaces: %#v", offenders)
	}
}

func assertMethodSignature(t *testing.T, method reflect.Method, wantIn, wantOut []reflect.Type) {
	t.Helper()

	methodType := method.Type
	if got := methodType.NumIn() - 1; got != len(wantIn) {
		t.Fatalf("%s input count = %d, want %d", method.Name, got, len(wantIn))
	}
	for index, want := range wantIn {
		if got := methodType.In(index + 1); got != want {
			t.Fatalf("%s input[%d] = %v, want %v", method.Name, index, got, want)
		}
	}
	if got := methodType.NumOut(); got != len(wantOut) {
		t.Fatalf("%s output count = %d, want %d", method.Name, got, len(wantOut))
	}
	for index, want := range wantOut {
		if got := methodType.Out(index); got != want {
			t.Fatalf("%s output[%d] = %v, want %v", method.Name, index, got, want)
		}
	}
}

func assertNamedPublicTypes(t *testing.T, types map[string]reflect.Type) {
	t.Helper()

	for name, typ := range types {
		if got := typ.Name(); got != name {
			t.Fatalf("%s reflected name = %q, want %q", name, got, name)
		}
		if typ.PkgPath() != "github.com/zchee/pandaemonium/pkg/codex" {
			t.Fatalf("%s package path = %q, want pkg/codex", name, typ.PkgPath())
		}
	}
}
