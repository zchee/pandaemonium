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
	"slices"
	"testing"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	gocmp "github.com/google/go-cmp/cmp"
)

func TestDecodeNotificationHelpers(t *testing.T) {
	t.Parallel()

	itemParams, err := json.Marshal(ItemCompletedNotification{
		ThreadID: "thr-1",
		TurnID:   "turn-1",
		Item:     RawThreadItem(`{"type":"agentMessage","text":"hello"}`),
	})
	if err != nil {
		t.Fatalf("json.Marshal() item params error = %v", err)
	}
	turnParams, err := json.Marshal(TurnCompletedNotification{
		Turn: Turn{
			ID:     "turn-1",
			Status: TurnStatusCompleted,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() turn params error = %v", err)
	}

	tests := map[string]struct {
		notification Notification
	}{
		"success: decode typed item completed helper": {
			notification: Notification{Method: NotificationMethodItemCompleted, Params: itemParams},
		},
		"success: decode typed turn completed helper": {
			notification: Notification{Method: NotificationMethodTurnCompleted, Params: turnParams},
		},
		"success: preserve raw unknown notification": {
			notification: Notification{Method: "thread/custom", Params: jsontext.Value([]byte(`{"hello":"world"}`))},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			switch tt.notification.Method {
			case NotificationMethodItemCompleted:
				got, ok, err := DecodeItemCompletedNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeItemCompletedNotification() error = %v", err)
				}
				if !ok {
					t.Fatalf("DecodeItemCompletedNotification() ok = false, want true")
				}
				if got.ThreadID != "thr-1" || got.TurnID != "turn-1" {
					t.Fatalf("decoded item notification = %#v, want thr-1/turn-1", got)
				}
				value, ok, err := tt.notification.ItemCompleted()
				if err != nil || !ok {
					t.Fatalf("Notification.ItemCompleted() = (%#v, %v, %v), want success", value, ok, err)
				}
				if diff := gocmp.Diff(got, value); diff != "" {
					t.Fatalf("wrapper mismatch (-want +got):\n%s", diff)
				}
				asValue, ok, err := tt.notification.As[ItemCompletedNotification](NotificationMethodItemCompleted)
				if err != nil || !ok {
					t.Fatalf("Notification.As() = (%#v, %v, %v), want success", asValue, ok, err)
				}
				if diff := gocmp.Diff(got, asValue); diff != "" {
					t.Fatalf("Notification.As() mismatch (-want +got):\n%s", diff)
				}
				known, matched, err := DecodeKnownNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeKnownNotification() error = %v", err)
				}
				if !matched {
					t.Fatalf("DecodeKnownNotification() matched = false, want true")
				}
				if known.Method != NotificationMethodItemCompleted {
					t.Fatalf("known.Method = %q, want %q", known.Method, NotificationMethodItemCompleted)
				}
				if diff := gocmp.Diff(got, known.Value); diff != "" {
					t.Fatalf("known.Value mismatch (-want +got):\n%s", diff)
				}
				if diff := gocmp.Diff(tt.notification, known.Raw); diff != "" {
					t.Fatalf("known.Raw mismatch (-want +got):\n%s", diff)
				}
			case NotificationMethodTurnCompleted:
				got, ok, err := DecodeTurnCompletedNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeTurnCompletedNotification() error = %v", err)
				}
				if !ok {
					t.Fatalf("DecodeTurnCompletedNotification() ok = false, want true")
				}
				if got.Turn.ID != "turn-1" || got.Turn.Status != TurnStatusCompleted {
					t.Fatalf("decoded turn notification = %#v, want completed turn-1", got)
				}
				value, ok, err := tt.notification.TurnCompleted()
				if err != nil || !ok {
					t.Fatalf("Notification.TurnCompleted() = (%#v, %v, %v), want success", value, ok, err)
				}
				if diff := gocmp.Diff(got, value); diff != "" {
					t.Fatalf("wrapper mismatch (-want +got):\n%s", diff)
				}
				asValue, ok, err := tt.notification.As[TurnCompletedNotification](NotificationMethodTurnCompleted)
				if err != nil || !ok {
					t.Fatalf("Notification.As() = (%#v, %v, %v), want success", asValue, ok, err)
				}
				if diff := gocmp.Diff(got, asValue); diff != "" {
					t.Fatalf("Notification.As() mismatch (-want +got):\n%s", diff)
				}
				known, matched, err := DecodeKnownNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeKnownNotification() error = %v", err)
				}
				if !matched {
					t.Fatalf("DecodeKnownNotification() matched = false, want true")
				}
				if diff := gocmp.Diff(got, known.Value); diff != "" {
					t.Fatalf("known.Value mismatch (-want +got):\n%s", diff)
				}
			default:
				known, matched, err := DecodeKnownNotification(tt.notification)
				if err != nil {
					t.Fatalf("DecodeKnownNotification() error = %v", err)
				}
				if matched {
					t.Fatalf("DecodeKnownNotification() matched = true, want false")
				}
				if diff := gocmp.Diff(tt.notification, known.Raw); diff != "" {
					t.Fatalf("unknown raw mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestDecodeProcessNotifications(t *testing.T) {
	t.Parallel()

	exitedParams := mustJSON(t, ProcessExitedNotification{
		ProcessHandle:    "proc-1",
		ExitCode:         7,
		Stdout:           "stdout",
		Stderr:           "stderr",
		StdoutCapReached: true,
	})
	outputDeltaParams := mustJSON(t, ProcessOutputDeltaNotification{
		ProcessHandle: "proc-1",
		Stream:        ProcessOutputStreamValueStdout,
		DeltaBase64:   "aGVsbG8=",
		CapReached:    true,
	})

	tests := map[string]struct {
		notification Notification
		assert       func(*testing.T, Notification, KnownNotification)
	}{
		"process exited": {
			notification: Notification{Method: NotificationMethodProcessExited, Params: exitedParams},
			assert: func(t *testing.T, notification Notification, known KnownNotification) {
				t.Helper()

				got, ok, err := DecodeProcessExitedNotification(notification)
				if err != nil {
					t.Fatalf("DecodeProcessExitedNotification() error = %v", err)
				}
				if !ok {
					t.Fatalf("DecodeProcessExitedNotification() ok = false, want true")
				}
				if got.ProcessHandle != "proc-1" || got.ExitCode != 7 || !got.StdoutCapReached {
					t.Fatalf("decoded process exited notification = %#v, want proc-1 exit 7 stdout cap", got)
				}
				value, ok, err := notification.ProcessExited()
				if err != nil || !ok {
					t.Fatalf("Notification.ProcessExited() = (%#v, %v, %v), want success", value, ok, err)
				}
				if diff := gocmp.Diff(got, value); diff != "" {
					t.Fatalf("wrapper mismatch (-want +got):\n%s", diff)
				}
				if diff := gocmp.Diff(got, known.Value); diff != "" {
					t.Fatalf("known.Value mismatch (-want +got):\n%s", diff)
				}
			},
		},
		"process output delta": {
			notification: Notification{Method: NotificationMethodProcessOutputDelta, Params: outputDeltaParams},
			assert: func(t *testing.T, notification Notification, known KnownNotification) {
				t.Helper()

				got, ok, err := DecodeProcessOutputDeltaNotification(notification)
				if err != nil {
					t.Fatalf("DecodeProcessOutputDeltaNotification() error = %v", err)
				}
				if !ok {
					t.Fatalf("DecodeProcessOutputDeltaNotification() ok = false, want true")
				}
				if got.ProcessHandle != "proc-1" || got.DeltaBase64 != "aGVsbG8=" || !got.CapReached {
					t.Fatalf("decoded process output delta notification = %#v, want proc-1 capped hello chunk", got)
				}
				stream, ok := got.Stream.(ProcessOutputStreamValue)
				if !ok || stream != ProcessOutputStreamValueStdout {
					t.Fatalf("decoded process output stream = %#v (%T), want stdout", got.Stream, got.Stream)
				}
				value, ok, err := notification.ProcessOutputDelta()
				if err != nil || !ok {
					t.Fatalf("Notification.ProcessOutputDelta() = (%#v, %v, %v), want success", value, ok, err)
				}
				if diff := gocmp.Diff(got, value); diff != "" {
					t.Fatalf("wrapper mismatch (-want +got):\n%s", diff)
				}
				if diff := gocmp.Diff(got, known.Value); diff != "" {
					t.Fatalf("known.Value mismatch (-want +got):\n%s", diff)
				}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			known, matched, err := DecodeKnownNotification(tt.notification)
			if err != nil {
				t.Fatalf("DecodeKnownNotification() error = %v", err)
			}
			if !matched {
				t.Fatalf("DecodeKnownNotification() matched = false, want true")
			}
			if known.Method != tt.notification.Method {
				t.Fatalf("known.Method = %q, want %q", known.Method, tt.notification.Method)
			}
			if diff := gocmp.Diff(tt.notification, known.Raw); diff != "" {
				t.Fatalf("known.Raw mismatch (-want +got):\n%s", diff)
			}
			tt.assert(t, tt.notification, known)
		})
	}
}

func TestDecodeKnownProcessNotificationMalformedParamsPreservesRaw(t *testing.T) {
	t.Parallel()

	notification := Notification{
		Method: NotificationMethodProcessExited,
		Params: jsontext.Value([]byte(`{"processHandle":true}`)),
	}

	known, matched, err := DecodeKnownNotification(notification)
	if !matched {
		t.Fatalf("DecodeKnownNotification() matched = false, want true")
	}
	if err == nil {
		t.Fatalf("DecodeKnownNotification() err = nil, want malformed params error")
	}
	if known.Method != NotificationMethodProcessExited {
		t.Fatalf("known.Method = %q, want %q", known.Method, NotificationMethodProcessExited)
	}
	if diff := gocmp.Diff(notification, known.Raw); diff != "" {
		t.Fatalf("DecodeKnownNotification() raw mismatch (-want +got):\n%s", diff)
	}
}

func TestNotificationConvenienceHelpersCoverKnownMethods(t *testing.T) {
	t.Parallel()

	tests := map[string]notificationConvenienceCase{
		"account/login/completed":                   newNotificationConvenienceCase(t, NotificationMethodAccountLoginCompleted, AccountLoginCompletedNotification{}, DecodeAccountLoginCompletedNotification, Notification.AccountLoginCompleted),
		"account/rateLimits/updated":                newNotificationConvenienceCase(t, NotificationMethodAccountRateLimitsUpdated, AccountRateLimitsUpdatedNotification{}, DecodeAccountRateLimitsUpdatedNotification, Notification.AccountRateLimitsUpdated),
		"account/updated":                           newNotificationConvenienceCase(t, NotificationMethodAccountUpdated, AccountUpdatedNotification{}, DecodeAccountUpdatedNotification, Notification.AccountUpdated),
		"app/list/updated":                          newNotificationConvenienceCase(t, NotificationMethodAppListUpdated, AppListUpdatedNotification{}, DecodeAppListUpdatedNotification, Notification.AppListUpdated),
		"command/exec/outputDelta":                  newNotificationConvenienceCase(t, NotificationMethodCommandExecOutputDelta, CommandExecOutputDeltaNotification{Stream: CommandExecOutputStreamValueStdout}, DecodeCommandExecOutputDeltaNotification, Notification.CommandExecOutputDelta),
		"configWarning":                             newNotificationConvenienceCase(t, NotificationMethodConfigWarning, ConfigWarningNotification{}, DecodeConfigWarningNotification, Notification.ConfigWarning),
		"deprecationNotice":                         newNotificationConvenienceCase(t, NotificationMethodDeprecationNotice, DeprecationNoticeNotification{}, DecodeDeprecationNoticeNotification, Notification.DeprecationNotice),
		"error":                                     newNotificationConvenienceCase(t, NotificationMethodError, ErrorNotification{Error: TurnError{Message: "boom"}}, DecodeErrorNotification, Notification.ErrorNotification),
		"externalAgentConfig/import/completed":      newNotificationConvenienceCase(t, NotificationMethodExternalAgentConfigImportCompleted, ExternalAgentConfigImportCompletedNotification{}, DecodeExternalAgentConfigImportCompletedNotification, Notification.ExternalAgentConfigImportCompleted),
		"fs/changed":                                newNotificationConvenienceCase(t, NotificationMethodFSChanged, FSChangedNotification{}, DecodeFSChangedNotification, Notification.FSChanged),
		"fuzzyFileSearch/sessionCompleted":          newNotificationConvenienceCase(t, NotificationMethodFuzzyFileSearchSessionCompleted, FuzzyFileSearchSessionCompletedNotification{}, DecodeFuzzyFileSearchSessionCompletedNotification, Notification.FuzzyFileSearchSessionCompleted),
		"fuzzyFileSearch/sessionUpdated":            newNotificationConvenienceCase(t, NotificationMethodFuzzyFileSearchSessionUpdated, FuzzyFileSearchSessionUpdatedNotification{}, DecodeFuzzyFileSearchSessionUpdatedNotification, Notification.FuzzyFileSearchSessionUpdated),
		"guardianWarning":                           newNotificationConvenienceCase(t, NotificationMethodGuardianWarning, GuardianWarningNotification{}, DecodeGuardianWarningNotification, Notification.GuardianWarning),
		"hook/completed":                            newNotificationConvenienceCase(t, NotificationMethodHookCompleted, HookCompletedNotification{}, DecodeHookCompletedNotification, Notification.HookCompleted),
		"hook/started":                              newNotificationConvenienceCase(t, NotificationMethodHookStarted, HookStartedNotification{}, DecodeHookStartedNotification, Notification.HookStarted),
		"item/agentMessage/delta":                   newNotificationConvenienceCase(t, NotificationMethodItemAgentMessageDelta, AgentMessageDeltaNotification{}, DecodeItemAgentMessageDeltaNotification, Notification.ItemAgentMessageDelta),
		"item/autoApprovalReview/completed":         newNotificationConvenienceCase(t, NotificationMethodItemAutoApprovalReviewCompleted, ItemGuardianApprovalReviewCompletedNotification{}, DecodeItemAutoApprovalReviewCompletedNotification, Notification.ItemAutoApprovalReviewCompleted),
		"item/autoApprovalReview/started":           newNotificationConvenienceCase(t, NotificationMethodItemAutoApprovalReviewStarted, ItemGuardianApprovalReviewStartedNotification{}, DecodeItemAutoApprovalReviewStartedNotification, Notification.ItemAutoApprovalReviewStarted),
		"item/commandExecution/outputDelta":         newNotificationConvenienceCase(t, NotificationMethodItemCommandExecutionOutputDelta, ItemCommandExecutionOutputDeltaNotification{}, DecodeItemCommandExecutionOutputDeltaNotification, Notification.ItemCommandExecutionOutputDelta),
		"item/commandExecution/terminalInteraction": newNotificationConvenienceCase(t, NotificationMethodItemCommandExecutionTerminalInteraction, ItemCommandExecutionTerminalInteractionNotification{}, DecodeItemCommandExecutionTerminalInteractionNotification, Notification.ItemCommandExecutionTerminalInteraction),
		"item/completed":                            newNotificationConvenienceCase(t, NotificationMethodItemCompleted, ItemCompletedNotification{Item: RawThreadItem(`{"type":"agentMessage","text":"hello"}`)}, DecodeItemCompletedNotification, Notification.ItemCompleted),
		"item/fileChange/outputDelta":               newNotificationConvenienceCase(t, NotificationMethodItemFileChangeOutputDelta, ItemFileChangeOutputDeltaNotification{}, DecodeItemFileChangeOutputDeltaNotification, Notification.ItemFileChangeOutputDelta),
		"item/fileChange/patchUpdated":              newNotificationConvenienceCase(t, NotificationMethodItemFileChangePatchUpdated, ItemFileChangePatchUpdatedNotification{}, DecodeItemFileChangePatchUpdatedNotification, Notification.ItemFileChangePatchUpdated),
		"item/mcpToolCall/progress":                 newNotificationConvenienceCase(t, NotificationMethodItemMCPToolCallProgress, MCPToolCallProgressNotification{}, DecodeItemMCPToolCallProgressNotification, Notification.ItemMCPToolCallProgress),
		"item/plan/delta":                           newNotificationConvenienceCase(t, NotificationMethodItemPlanDelta, PlanDeltaNotification{}, DecodeItemPlanDeltaNotification, Notification.ItemPlanDelta),
		"item/reasoning/summaryPartAdded":           newNotificationConvenienceCase(t, NotificationMethodItemReasoningSummaryPartAdded, ReasoningSummaryPartAddedNotification{}, DecodeItemReasoningSummaryPartAddedNotification, Notification.ItemReasoningSummaryPartAdded),
		"item/reasoning/summaryTextDelta":           newNotificationConvenienceCase(t, NotificationMethodItemReasoningSummaryTextDelta, ReasoningSummaryTextDeltaNotification{}, DecodeItemReasoningSummaryTextDeltaNotification, Notification.ItemReasoningSummaryTextDelta),
		"item/reasoning/textDelta":                  newNotificationConvenienceCase(t, NotificationMethodItemReasoningTextDelta, ReasoningTextDeltaNotification{}, DecodeItemReasoningTextDeltaNotification, Notification.ItemReasoningTextDelta),
		"item/started":                              newNotificationConvenienceCase(t, NotificationMethodItemStarted, ItemStartedNotification{Item: RawThreadItem(`{"type":"agentMessage","text":"hello"}`)}, DecodeItemStartedNotification, Notification.ItemStarted),
		"mcpServer/oauthLogin/completed":            newNotificationConvenienceCase(t, NotificationMethodMCPServerOAuthLoginCompleted, MCPServerOAuthLoginCompletedNotification{}, DecodeMCPServerOAuthLoginCompletedNotification, Notification.MCPServerOAuthLoginCompleted),
		"mcpServer/startupStatus/updated":           newNotificationConvenienceCase(t, NotificationMethodMCPServerStartupStatusUpdated, MCPServerStartupStatusUpdatedNotification{}, DecodeMCPServerStartupStatusUpdatedNotification, Notification.MCPServerStartupStatusUpdated),
		"model/rerouted":                            newNotificationConvenienceCase(t, NotificationMethodModelRerouted, ModelReroutedNotification{}, DecodeModelReroutedNotification, Notification.ModelRerouted),
		"model/verification":                        newNotificationConvenienceCase(t, NotificationMethodModelVerification, ModelVerificationNotification{}, DecodeModelVerificationNotification, Notification.ModelVerification),
		"process/exited":                            newNotificationConvenienceCase(t, NotificationMethodProcessExited, ProcessExitedNotification{}, DecodeProcessExitedNotification, Notification.ProcessExited),
		"process/outputDelta":                       newNotificationConvenienceCase(t, NotificationMethodProcessOutputDelta, ProcessOutputDeltaNotification{Stream: ProcessOutputStreamValueStdout}, DecodeProcessOutputDeltaNotification, Notification.ProcessOutputDelta),
		"remoteControl/status/changed":              newNotificationConvenienceCase(t, NotificationMethodRemoteControlStatusChanged, RemoteControlStatusChangedNotification{}, DecodeRemoteControlStatusChangedNotification, Notification.RemoteControlStatusChanged),
		"serverRequest/resolved":                    newNotificationConvenienceCase(t, NotificationMethodServerRequestResolved, ServerRequestResolvedNotification{}, DecodeServerRequestResolvedNotification, Notification.ServerRequestResolved),
		"skills/changed":                            newNotificationConvenienceCase(t, NotificationMethodSkillsChanged, SkillsChangedNotification{}, DecodeSkillsChangedNotification, Notification.SkillsChanged),
		"thread/archived":                           newNotificationConvenienceCase(t, NotificationMethodThreadArchived, ThreadArchivedNotification{}, DecodeThreadArchivedNotification, Notification.ThreadArchived),
		"thread/closed":                             newNotificationConvenienceCase(t, NotificationMethodThreadClosed, ThreadClosedNotification{}, DecodeThreadClosedNotification, Notification.ThreadClosed),
		"thread/compacted":                          newNotificationConvenienceCase(t, NotificationMethodThreadCompacted, ContextCompactedNotification{}, DecodeThreadCompactedNotification, Notification.ThreadCompacted),
		"thread/goal/cleared":                       newNotificationConvenienceCase(t, NotificationMethodThreadGoalCleared, ThreadGoalClearedNotification{}, DecodeThreadGoalClearedNotification, Notification.ThreadGoalCleared),
		"thread/goal/updated":                       newNotificationConvenienceCase(t, NotificationMethodThreadGoalUpdated, ThreadGoalUpdatedNotification{}, DecodeThreadGoalUpdatedNotification, Notification.ThreadGoalUpdated),
		"thread/name/updated":                       newNotificationConvenienceCase(t, NotificationMethodThreadNameUpdated, ThreadNameUpdatedNotification{}, DecodeThreadNameUpdatedNotification, Notification.ThreadNameUpdated),
		"thread/realtime/closed":                    newNotificationConvenienceCase(t, NotificationMethodThreadRealtimeClosed, ThreadRealtimeClosedNotification{}, DecodeThreadRealtimeClosedNotification, Notification.ThreadRealtimeClosed),
		"thread/realtime/error":                     newNotificationConvenienceCase(t, NotificationMethodThreadRealtimeError, ThreadRealtimeErrorNotification{}, DecodeThreadRealtimeErrorNotification, Notification.ThreadRealtimeError),
		"thread/realtime/itemAdded":                 newNotificationConvenienceCase(t, NotificationMethodThreadRealtimeItemAdded, ThreadRealtimeItemAddedNotification{}, DecodeThreadRealtimeItemAddedNotification, Notification.ThreadRealtimeItemAdded),
		"thread/realtime/outputAudio/delta":         newNotificationConvenienceCase(t, NotificationMethodThreadRealtimeOutputAudioDelta, ThreadRealtimeOutputAudioDeltaNotification{}, DecodeThreadRealtimeOutputAudioDeltaNotification, Notification.ThreadRealtimeOutputAudioDelta),
		"thread/realtime/sdp":                       newNotificationConvenienceCase(t, NotificationMethodThreadRealtimeSDP, ThreadRealtimeSDPNotification{}, DecodeThreadRealtimeSDPNotification, Notification.ThreadRealtimeSDP),
		"thread/realtime/started":                   newNotificationConvenienceCase(t, NotificationMethodThreadRealtimeStarted, ThreadRealtimeStartedNotification{}, DecodeThreadRealtimeStartedNotification, Notification.ThreadRealtimeStarted),
		"thread/realtime/transcript/delta":          newNotificationConvenienceCase(t, NotificationMethodThreadRealtimeTranscriptDelta, ThreadRealtimeTranscriptDeltaNotification{}, DecodeThreadRealtimeTranscriptDeltaNotification, Notification.ThreadRealtimeTranscriptDelta),
		"thread/realtime/transcript/done":           newNotificationConvenienceCase(t, NotificationMethodThreadRealtimeTranscriptDone, ThreadRealtimeTranscriptDoneNotification{}, DecodeThreadRealtimeTranscriptDoneNotification, Notification.ThreadRealtimeTranscriptDone),
		"thread/started":                            newNotificationConvenienceCase(t, NotificationMethodThreadStarted, ThreadStartedNotification{}, DecodeThreadStartedNotification, Notification.ThreadStarted),
		"thread/status/changed":                     newNotificationConvenienceCase(t, NotificationMethodThreadStatusChanged, ThreadStatusChangedNotification{Status: IDleThreadStatus{Type: "idle"}}, DecodeThreadStatusChangedNotification, Notification.ThreadStatusChanged),
		"thread/tokenUsage/updated":                 newNotificationConvenienceCase(t, NotificationMethodThreadTokenUsageUpdated, ThreadTokenUsageUpdatedNotification{}, DecodeThreadTokenUsageUpdatedNotification, Notification.ThreadTokenUsageUpdated),
		"thread/unarchived":                         newNotificationConvenienceCase(t, NotificationMethodThreadUnarchived, ThreadUnarchivedNotification{}, DecodeThreadUnarchivedNotification, Notification.ThreadUnarchived),
		"turn/completed":                            newNotificationConvenienceCase(t, NotificationMethodTurnCompleted, TurnCompletedNotification{}, DecodeTurnCompletedNotification, Notification.TurnCompleted),
		"turn/diff/updated":                         newNotificationConvenienceCase(t, NotificationMethodTurnDiffUpdated, TurnDiffUpdatedNotification{}, DecodeTurnDiffUpdatedNotification, Notification.TurnDiffUpdated),
		"turn/plan/updated":                         newNotificationConvenienceCase(t, NotificationMethodTurnPlanUpdated, TurnPlanUpdatedNotification{}, DecodeTurnPlanUpdatedNotification, Notification.TurnPlanUpdated),
		"turn/started":                              newNotificationConvenienceCase(t, NotificationMethodTurnStarted, TurnStartedNotification{}, DecodeTurnStartedNotification, Notification.TurnStarted),
		"warning":                                   newNotificationConvenienceCase(t, NotificationMethodWarning, WarningNotification{}, DecodeWarningNotification, Notification.Warning),
		"windows/worldWritableWarning":              newNotificationConvenienceCase(t, NotificationMethodWindowsWorldWritableWarning, WindowsWorldWritableWarningNotification{}, DecodeWindowsWorldWritableWarningNotification, Notification.WindowsWorldWritableWarning),
		"windowsSandbox/setupCompleted":             newNotificationConvenienceCase(t, NotificationMethodWindowsSandboxSetupCompleted, WindowsSandboxSetupCompletedNotification{}, DecodeWindowsSandboxSetupCompletedNotification, Notification.WindowsSandboxSetupCompleted),
	}

	coveredMethods := make([]string, 0, len(tests))
	for name, tt := range tests {
		coveredMethods = append(coveredMethods, tt.method)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			notification := Notification{Method: tt.method, Params: tt.params}
			decoded, ok, err := tt.decode(notification)
			if err != nil {
				t.Fatalf("decode helper error = %v", err)
			}
			if !ok {
				t.Fatalf("decode helper ok = false, want true")
			}
			received, ok, err := tt.receiver(notification)
			if err != nil {
				t.Fatalf("receiver helper error = %v", err)
			}
			if !ok {
				t.Fatalf("receiver helper ok = false, want true")
			}
			if diff := gocmp.Diff(decoded, received); diff != "" {
				t.Fatalf("receiver helper mismatch (-want +got):\n%s", diff)
			}
		})
	}

	if diff := gocmp.Diff(KnownNotificationMethods(), sortedStrings(coveredMethods)); diff != "" {
		t.Fatalf("notification helper coverage mismatch (-want +got):\n%s", diff)
	}
}

func TestDeprecatedAgentMessageDeltaNotificationHelpersRemainCompatible(t *testing.T) {
	t.Parallel()

	notification := Notification{
		Method: NotificationMethodItemAgentMessageDelta,
		Params: mustJSON(t, AgentMessageDeltaNotification{}),
	}

	decoded, ok, err := DecodeAgentMessageDeltaNotification(notification)
	if err != nil {
		t.Fatalf("DecodeAgentMessageDeltaNotification() error = %v", err)
	}
	if !ok {
		t.Fatalf("DecodeAgentMessageDeltaNotification() ok = false, want true")
	}

	received, ok, err := notification.AgentMessageDelta()
	if err != nil {
		t.Fatalf("Notification.AgentMessageDelta() error = %v", err)
	}
	if !ok {
		t.Fatalf("Notification.AgentMessageDelta() ok = false, want true")
	}
	if diff := gocmp.Diff(decoded, received); diff != "" {
		t.Fatalf("deprecated helper mismatch (-want +got):\n%s", diff)
	}
}

func TestKnownNotificationMethodsMatchesExpectedInventory(t *testing.T) {
	t.Parallel()

	if diff := gocmp.Diff(notificationMethodList, KnownNotificationMethods()); diff != "" {
		t.Fatalf("KnownNotificationMethods() mismatch (-want +got):\n%s", diff)
	}
}

func TestDecodeNotificationMethodMismatchAndMalformedParams(t *testing.T) {
	t.Parallel()

	mismatchedNotification := Notification{Method: NotificationMethodTurnCompleted, Params: jsontext.Value([]byte(`{"message":"nope"}`))}
	mismatch, ok, err := mismatchedNotification.As[ErrorNotification](NotificationMethodError)
	if err != nil {
		t.Fatalf("Notification.As() mismatch error = %v", err)
	}
	if ok {
		t.Fatalf("Notification.As() mismatch ok = true, want false")
	}
	if diff := gocmp.Diff(ErrorNotification{}, mismatch); diff != "" {
		t.Fatalf("Notification.As() mismatch value (-want +got):\n%s", diff)
	}

	mismatch, ok, err = DecodeNotificationAs[ErrorNotification](
		mismatchedNotification,
		NotificationMethodError,
	)
	if err != nil {
		t.Fatalf("DecodeNotification() mismatch error = %v", err)
	}
	if ok {
		t.Fatalf("DecodeNotification() mismatch ok = true, want false")
	}
	if diff := gocmp.Diff(ErrorNotification{}, mismatch); diff != "" {
		t.Fatalf("DecodeNotification() mismatch value (-want +got):\n%s", diff)
	}

	malformedNotification := Notification{
		Method: NotificationMethodError,
		Params: jsontext.Value([]byte(`{"missing":"fields"`)),
	}
	_, ok, err = malformedNotification.As[ErrorNotification](NotificationMethodError)
	if !ok {
		t.Fatalf("Notification.As() malformed ok = false, want true")
	}
	if err == nil {
		t.Fatalf("Notification.As() malformed err = nil, want error")
	}

	_, ok, err = DecodeErrorNotification(malformedNotification)
	if !ok {
		t.Fatalf("DecodeErrorNotification() malformed ok = false, want true")
	}
	if err == nil {
		t.Fatalf("DecodeErrorNotification() malformed err = nil, want error")
	}
}

func TestDecodeKnownNotificationUnknownMethodPreservesNestedRaw(t *testing.T) {
	t.Parallel()

	notification := Notification{
		Method: "item/custom",
		Params: jsontext.Value([]byte(`{"items":[{"id":"one"},{"id":"two"}],"nested":{"values":[1,null,2]}}`)),
	}

	known, matched, err := DecodeKnownNotification(notification)
	if err != nil {
		t.Fatalf("DecodeKnownNotification() error = %v", err)
	}
	if matched {
		t.Fatalf("DecodeKnownNotification() matched = true, want false")
	}
	if diff := gocmp.Diff(notification, known.Raw); diff != "" {
		t.Fatalf("DecodeKnownNotification() raw mismatch (-want +got):\n%s", diff)
	}
}

func TestTurnCompletedNotificationDecodesKnownThreadItemsAndPreservesUnknownRaw(t *testing.T) {
	t.Parallel()

	var decoded TurnCompletedNotification
	input := []byte(`{
		"threadId":"thr-1",
		"turn":{
			"id":"turn-1",
			"status":"completed",
			"items":[
				{"id":"item-1","type":"agentMessage","text":"hello"},
				["nested",{"kind":"union"}]
			]
		}
	}`)
	if err := json.Unmarshal(input, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded.ThreadID != "thr-1" || decoded.Turn.ID != "turn-1" || decoded.Turn.Status != TurnStatusCompleted {
		t.Fatalf("decoded turn completed header = %#v, want thread/turn/status", decoded)
	}
	if len(decoded.Turn.Items) != 2 {
		t.Fatalf("decoded item count = %d, want 2", len(decoded.Turn.Items))
	}
	agentMessage, ok := decoded.Turn.Items[0].(AgentMessageThreadItem)
	if !ok {
		t.Fatalf("first item = %#v (%T), want AgentMessageThreadItem", decoded.Turn.Items[0], decoded.Turn.Items[0])
	}
	if agentMessage.ID != "item-1" || agentMessage.Text != "hello" || agentMessage.Type != "agentMessage" {
		t.Fatalf("agent message item = %#v, want typed hello item", agentMessage)
	}
	nestedRaw, err := json.Marshal(decoded.Turn.Items[1])
	if err != nil {
		t.Fatalf("json.Marshal() nested item error = %v", err)
	}
	if got := string(nestedRaw); got != `["nested",{"kind":"union"}]` {
		t.Fatalf("nested slice item = %s, want preserved raw json", got)
	}
}

func mustJSON(t *testing.T, value any) jsontext.Value {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return jsontext.Value(raw)
}

type notificationConvenienceCase struct {
	method   string
	params   jsontext.Value
	decode   func(Notification) (any, bool, error)
	receiver func(Notification) (any, bool, error)
}

func newNotificationConvenienceCase[T any](
	t *testing.T,
	method string,
	value T,
	decode func(Notification) (T, bool, error),
	receiver func(Notification) (T, bool, error),
) notificationConvenienceCase {
	t.Helper()
	params := mustJSON(t, value)
	return notificationConvenienceCase{
		method: method,
		params: params,
		decode: func(notification Notification) (any, bool, error) {
			return decode(notification)
		},
		receiver: func(notification Notification) (any, bool, error) {
			return receiver(notification)
		},
	}
}

func sortedStrings(values []string) []string {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return sorted
}
