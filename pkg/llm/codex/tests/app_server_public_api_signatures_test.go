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
	"reflect"
	"testing"

	"github.com/zchee/pandaemonium/pkg/llm/codex"
)

func TestPublicAPISignaturePortRootExports(t *testing.T) {
	t.Parallel()

	var (
		_ *codex.Config
		_ codex.ListenConfig
		_ codex.WebSocketConfig
		_ codex.WebSocketAuthMode
		_ codex.ServerMode
		_ *codex.Codex
		_ *codex.ExecServer
		_ *codex.Client
		_ codex.ApprovalMode
		_ *codex.Thread
		_ *codex.StreamThread
		_ *codex.TurnHandle
		_ *codex.StreamTurnHandle
		_ *codex.ChatGPTLoginHandle
		_ *codex.DeviceCodeLoginHandle
		_ codex.RunResult
		_ codex.RunInput  = "plain text"
		_ codex.InputItem = codex.TextInput{}
		_ codex.InputItem = codex.ImageInput{}
		_ codex.InputItem = codex.LocalImageInput{}
		_ codex.InputItem = codex.SkillInput{}
		_ codex.InputItem = codex.MentionInput{}
		_ codex.RetryConfig
		_ *codex.AppServerError
		_ *codex.TransportClosedError
		_ *codex.LoginNotificationDroppedError
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
		"Config":                        reflect.TypeFor[codex.Config](),
		"ListenConfig":                  reflect.TypeFor[codex.ListenConfig](),
		"WebSocketConfig":               reflect.TypeFor[codex.WebSocketConfig](),
		"WebSocketAuthMode":             reflect.TypeFor[codex.WebSocketAuthMode](),
		"ServerMode":                    reflect.TypeFor[codex.ServerMode](),
		"Codex":                         reflect.TypeFor[codex.Codex](),
		"ExecServer":                    reflect.TypeFor[codex.ExecServer](),
		"Client":                        reflect.TypeFor[codex.Client](),
		"ApprovalMode":                  reflect.TypeFor[codex.ApprovalMode](),
		"Thread":                        reflect.TypeFor[codex.Thread](),
		"StreamThread":                  reflect.TypeFor[codex.StreamThread](),
		"TurnHandle":                    reflect.TypeFor[codex.TurnHandle](),
		"StreamTurnHandle":              reflect.TypeFor[codex.StreamTurnHandle](),
		"ChatGPTLoginHandle":            reflect.TypeFor[codex.ChatGPTLoginHandle](),
		"DeviceCodeLoginHandle":         reflect.TypeFor[codex.DeviceCodeLoginHandle](),
		"RunResult":                     reflect.TypeFor[codex.RunResult](),
		"RunInput":                      reflect.TypeFor[codex.RunInput](),
		"InputItem":                     reflect.TypeFor[codex.InputItem](),
		"TextInput":                     reflect.TypeFor[codex.TextInput](),
		"ImageInput":                    reflect.TypeFor[codex.ImageInput](),
		"LocalImageInput":               reflect.TypeFor[codex.LocalImageInput](),
		"SkillInput":                    reflect.TypeFor[codex.SkillInput](),
		"MentionInput":                  reflect.TypeFor[codex.MentionInput](),
		"RetryConfig":                   reflect.TypeFor[codex.RetryConfig](),
		"AppServerError":                reflect.TypeFor[codex.AppServerError](),
		"TransportClosedError":          reflect.TypeFor[codex.TransportClosedError](),
		"LoginNotificationDroppedError": reflect.TypeFor[codex.LoginNotificationDroppedError](),
		"JSONRPCError":                  reflect.TypeFor[codex.JSONRPCError](),
		"AppServerRPCError":             reflect.TypeFor[codex.AppServerRPCError](),
		"ParseError":                    reflect.TypeFor[codex.ParseError](),
		"InvalidRequestError":           reflect.TypeFor[codex.InvalidRequestError](),
		"MethodNotFoundError":           reflect.TypeFor[codex.MethodNotFoundError](),
		"InvalidParamsError":            reflect.TypeFor[codex.InvalidParamsError](),
		"InternalRPCError":              reflect.TypeFor[codex.InternalRPCError](),
		"ServerBusyError":               reflect.TypeFor[codex.ServerBusyError](),
		"RetryLimitExceededError":       reflect.TypeFor[codex.RetryLimitExceededError](),
	}
	assertNamedPublicTypes(t, types)

	if got := string(codex.ApprovalModeDenyAll); got != "deny_all" {
		t.Fatalf("ApprovalModeDenyAll = %q, want deny_all", got)
	}
	if got := string(codex.ApprovalModeAutoReview); got != "auto_review" {
		t.Fatalf("ApprovalModeAutoReview = %q, want auto_review", got)
	}
	if got := string(codex.ServerModeAppServer); got != "app-server" {
		t.Fatalf("ServerModeAppServer = %q, want app-server", got)
	}
	if got := string(codex.ServerModeExecServer); got != "exec-server" {
		t.Fatalf("ServerModeExecServer = %q, want exec-server", got)
	}
	if got := string(codex.NotificationMethodInitialized); got != "initialized" {
		t.Fatalf("NotificationMethodInitialized = %q, want initialized", got)
	}
}

func TestPublicAPISignaturePortTypesExports(t *testing.T) {
	t.Parallel()

	var (
		_ codex.ApprovalsReviewer
		_ codex.Account
		_ codex.AccountLoginCompletedNotification
		_ codex.AskForApproval
		_ codex.CancelLoginAccountResponse
		_ codex.CancelLoginAccountStatus
		_ codex.GetAccountResponse
		_ codex.GetAccountTokenUsageResponse
		_ codex.RemoteControlPairingStatusResponse
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
		_ codex.TurnModerationMetadataNotification
		_ codex.TurnStatus
		_ codex.TurnSteerResponse
	)

	types := map[string]reflect.Type{
		"ApprovalsReviewer":                   reflect.TypeFor[codex.ApprovalsReviewer](),
		"Account":                             reflect.TypeFor[codex.Account](),
		"AccountLoginCompletedNotification":   reflect.TypeFor[codex.AccountLoginCompletedNotification](),
		"AskForApproval":                      reflect.TypeFor[codex.AskForApproval](),
		"CancelLoginAccountResponse":          reflect.TypeFor[codex.CancelLoginAccountResponse](),
		"CancelLoginAccountStatus":            reflect.TypeFor[codex.CancelLoginAccountStatus](),
		"GetAccountResponse":                  reflect.TypeFor[codex.GetAccountResponse](),
		"GetAccountTokenUsageResponse":        reflect.TypeFor[codex.GetAccountTokenUsageResponse](),
		"RemoteControlPairingStatusResponse":  reflect.TypeFor[codex.RemoteControlPairingStatusResponse](),
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
		"TurnModerationMetadataNotification":  reflect.TypeFor[codex.TurnModerationMetadataNotification](),
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

	contextType := reflect.TypeFor[context.Context]()
	errorType := reflect.TypeFor[error]()
	runInputType := reflect.TypeFor[codex.RunInput]()
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
		"success: Codex LoginAPIKey": {
			typ:  reflect.TypeFor[*codex.Codex](),
			name: "LoginAPIKey",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[string](),
			},
			out: []reflect.Type{
				errorType,
			},
		},
		"success: Codex LoginChatGPT": {
			typ:  reflect.TypeFor[*codex.Codex](),
			name: "LoginChatGPT",
			in: []reflect.Type{
				contextType,
			},
			out: []reflect.Type{
				reflect.TypeFor[*codex.ChatGPTLoginHandle](),
				errorType,
			},
		},
		"success: Codex LoginChatGPTDeviceCode": {
			typ:  reflect.TypeFor[*codex.Codex](),
			name: "LoginChatGPTDeviceCode",
			in: []reflect.Type{
				contextType,
			},
			out: []reflect.Type{
				reflect.TypeFor[*codex.DeviceCodeLoginHandle](),
				errorType,
			},
		},
		"success: Codex Account": {
			typ:  reflect.TypeFor[*codex.Codex](),
			name: "Account",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[*codex.GetAccountParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.GetAccountResponse](),
				errorType,
			},
		},
		"success: ExecServer CommandExec": {
			typ:  reflect.TypeFor[*codex.ExecServer](),
			name: "CommandExec",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[*codex.CommandExecParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.CommandExecResponse](),
				errorType,
			},
		},
		"success: ExecServer CommandExecWrite": {
			typ:  reflect.TypeFor[*codex.ExecServer](),
			name: "CommandExecWrite",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[*codex.CommandExecWriteParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.CommandExecWriteResponse](),
				errorType,
			},
		},
		"success: ExecServer CommandExecTerminate": {
			typ:  reflect.TypeFor[*codex.ExecServer](),
			name: "CommandExecTerminate",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[*codex.CommandExecTerminateParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.CommandExecTerminateResponse](),
				errorType,
			},
		},
		"success: ExecServer CommandExecResize": {
			typ:  reflect.TypeFor[*codex.ExecServer](),
			name: "CommandExecResize",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[*codex.CommandExecResizeParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.CommandExecResizeResponse](),
				errorType,
			},
		},
		"success: ExecServer Metadata": {
			typ:  reflect.TypeFor[*codex.ExecServer](),
			name: "Metadata",
			out: []reflect.Type{
				reflect.TypeFor[codex.InitializeResponse](),
			},
		},
		"success: ExecServer SessionID": {
			typ:  reflect.TypeFor[*codex.ExecServer](),
			name: "SessionID",
			out: []reflect.Type{
				reflect.TypeFor[string](),
			},
		},
		"success: ExecServer Client": {
			typ:  reflect.TypeFor[*codex.ExecServer](),
			name: "Client",
			out: []reflect.Type{
				reflect.TypeFor[*codex.Client](),
			},
		},
		"success: ExecServer Close": {
			typ:  reflect.TypeFor[*codex.ExecServer](),
			name: "Close",
			out: []reflect.Type{
				errorType,
			},
		},
		"success: Codex Logout": {
			typ:  reflect.TypeFor[*codex.Codex](),
			name: "Logout",
			in: []reflect.Type{
				contextType,
			},
			out: []reflect.Type{
				errorType,
			},
		},
		"success: Client WaitForLoginCompleted": {
			typ:  reflect.TypeFor[*codex.Client](),
			name: "WaitForLoginCompleted",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[string](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.AccountLoginCompletedNotification](),
				errorType,
			},
		},
		"success: Client AccountUsageRead": {
			typ:  reflect.TypeFor[*codex.Client](),
			name: "AccountUsageRead",
			in: []reflect.Type{
				contextType,
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.GetAccountTokenUsageResponse](),
				errorType,
			},
		},
		"success: Client RemoteControlPairingStatus": {
			typ:  reflect.TypeFor[*codex.Client](),
			name: "RemoteControlPairingStatus",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[*codex.RemoteControlPairingStatusParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.RemoteControlPairingStatusResponse](),
				errorType,
			},
		},
		"success: Client TurnStart": {
			typ:  reflect.TypeFor[*codex.Client](),
			name: "TurnStart",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[string](),
				runInputType,
				reflect.TypeFor[*codex.TurnStartParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.TurnStartResponse](),
				errorType,
			},
		},
		"success: Client TurnSteer": {
			typ:  reflect.TypeFor[*codex.Client](),
			name: "TurnSteer",
			in: []reflect.Type{
				contextType,
				reflect.TypeFor[string](),
				reflect.TypeFor[string](),
				runInputType,
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.TurnSteerResponse](),
				errorType,
			},
		},
		"success: ChatGPTLoginHandle Wait": {
			typ:  reflect.TypeFor[*codex.ChatGPTLoginHandle](),
			name: "Wait",
			in: []reflect.Type{
				contextType,
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.AccountLoginCompletedNotification](),
				errorType,
			},
		},
		"success: ChatGPTLoginHandle Cancel": {
			typ:  reflect.TypeFor[*codex.ChatGPTLoginHandle](),
			name: "Cancel",
			in: []reflect.Type{
				contextType,
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.CancelLoginAccountResponse](),
				errorType,
			},
		},
		"success: ChatGPTLoginHandle LoginID": {
			typ:  reflect.TypeFor[*codex.ChatGPTLoginHandle](),
			name: "LoginID",
			out: []reflect.Type{
				reflect.TypeFor[string](),
			},
		},
		"success: ChatGPTLoginHandle AuthURL": {
			typ:  reflect.TypeFor[*codex.ChatGPTLoginHandle](),
			name: "AuthURL",
			out: []reflect.Type{
				reflect.TypeFor[string](),
			},
		},
		"success: DeviceCodeLoginHandle Wait": {
			typ:  reflect.TypeFor[*codex.DeviceCodeLoginHandle](),
			name: "Wait",
			in: []reflect.Type{
				contextType,
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.AccountLoginCompletedNotification](),
				errorType,
			},
		},
		"success: DeviceCodeLoginHandle LoginID": {
			typ:  reflect.TypeFor[*codex.DeviceCodeLoginHandle](),
			name: "LoginID",
			out: []reflect.Type{
				reflect.TypeFor[string](),
			},
		},
		"success: DeviceCodeLoginHandle VerificationURL": {
			typ:  reflect.TypeFor[*codex.DeviceCodeLoginHandle](),
			name: "VerificationURL",
			out: []reflect.Type{
				reflect.TypeFor[string](),
			},
		},
		"success: DeviceCodeLoginHandle UserCode": {
			typ:  reflect.TypeFor[*codex.DeviceCodeLoginHandle](),
			name: "UserCode",
			out: []reflect.Type{
				reflect.TypeFor[string](),
			},
		},
		"success: DeviceCodeLoginHandle Cancel": {
			typ:  reflect.TypeFor[*codex.DeviceCodeLoginHandle](),
			name: "Cancel",
			in: []reflect.Type{
				contextType,
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.CancelLoginAccountResponse](),
				errorType,
			},
		},
		"success: Thread Turn": {
			typ:  reflect.TypeFor[*codex.Thread](),
			name: "Turn",
			in: []reflect.Type{
				contextType,
				runInputType,
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
				runInputType,
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
		"success: TurnHandle Steer": {
			typ:  reflect.TypeFor[*codex.TurnHandle](),
			name: "Steer",
			in: []reflect.Type{
				contextType,
				runInputType,
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.TurnSteerResponse](),
				errorType,
			},
		},
		"success: StreamThread Turn": {
			typ:  reflect.TypeFor[*codex.StreamThread](),
			name: "Turn",
			in: []reflect.Type{
				contextType,
				runInputType,
				reflect.TypeFor[*codex.TurnStartParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[*codex.StreamTurnHandle](),
				errorType,
			},
		},
		"success: StreamThread Run": {
			typ:  reflect.TypeFor[*codex.StreamThread](),
			name: "Run",
			in: []reflect.Type{
				contextType,
				runInputType,
				reflect.TypeFor[*codex.TurnStartParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.RunResult](),
				errorType,
			},
		},
		"success: StreamThread RunStream": {
			typ:  reflect.TypeFor[*codex.StreamThread](),
			name: "RunStream",
			in: []reflect.Type{
				contextType,
				runInputType,
				reflect.TypeFor[*codex.TurnStartParams](),
			},
			out: []reflect.Type{
				reflect.TypeFor[iter.Seq2[codex.Notification, error]](),
			},
		},
		"success: StreamTurnHandle Steer": {
			typ:  reflect.TypeFor[*codex.StreamTurnHandle](),
			name: "Steer",
			in: []reflect.Type{
				contextType,
				runInputType,
			},
			out: []reflect.Type{
				reflect.TypeFor[codex.TurnSteerResponse](),
				errorType,
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
		if typ.PkgPath() != "github.com/zchee/pandaemonium/pkg/llm/codex" {
			t.Fatalf("%s package path = %q, want pkg/codex", name, typ.PkgPath())
		}
	}
}
