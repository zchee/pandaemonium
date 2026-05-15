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
	"fmt"
	"slices"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// Notification is a server notification with its method and raw params.
type Notification struct {
	Method string         `json:"method"`
	Params jsontext.Value `json:"params,omitzero"`
}

var notificationDecoders = map[string]func(Notification) (any, bool, error){
	NotificationMethodAccountLoginCompleted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[AccountLoginCompletedNotification](notif, NotificationMethodAccountLoginCompleted)
	},
	NotificationMethodAccountRateLimitsUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[AccountRateLimitsUpdatedNotification](notif, NotificationMethodAccountRateLimitsUpdated)
	},
	NotificationMethodAccountUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[AccountUpdatedNotification](notif, NotificationMethodAccountUpdated)
	},
	NotificationMethodAppListUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[AppListUpdatedNotification](notif, NotificationMethodAppListUpdated)
	},
	NotificationMethodCommandExecOutputDelta: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[CommandExecOutputDeltaNotification](notif, NotificationMethodCommandExecOutputDelta)
	},
	NotificationMethodConfigWarning: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ConfigWarningNotification](notif, NotificationMethodConfigWarning)
	},
	NotificationMethodDeprecationNotice: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[DeprecationNoticeNotification](notif, NotificationMethodDeprecationNotice)
	},
	NotificationMethodItemAgentMessageDelta: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[AgentMessageDeltaNotification](notif, NotificationMethodItemAgentMessageDelta)
	},
	NotificationMethodError: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ErrorNotification](notif, NotificationMethodError)
	},
	NotificationMethodExternalAgentConfigImportCompleted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ExternalAgentConfigImportCompletedNotification](notif, NotificationMethodExternalAgentConfigImportCompleted)
	},
	NotificationMethodFSChanged: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[FSChangedNotification](notif, NotificationMethodFSChanged)
	},
	NotificationMethodFuzzyFileSearchSessionCompleted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[FuzzyFileSearchSessionCompletedNotification](notif, NotificationMethodFuzzyFileSearchSessionCompleted)
	},
	NotificationMethodFuzzyFileSearchSessionUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[FuzzyFileSearchSessionUpdatedNotification](notif, NotificationMethodFuzzyFileSearchSessionUpdated)
	},
	NotificationMethodGuardianWarning: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[GuardianWarningNotification](notif, NotificationMethodGuardianWarning)
	},
	NotificationMethodHookCompleted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[HookCompletedNotification](notif, NotificationMethodHookCompleted)
	},
	NotificationMethodHookStarted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[HookStartedNotification](notif, NotificationMethodHookStarted)
	},
	NotificationMethodItemAutoApprovalReviewCompleted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemGuardianApprovalReviewCompletedNotification](notif, NotificationMethodItemAutoApprovalReviewCompleted)
	},
	NotificationMethodItemAutoApprovalReviewStarted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemGuardianApprovalReviewStartedNotification](notif, NotificationMethodItemAutoApprovalReviewStarted)
	},
	NotificationMethodItemCommandExecutionOutputDelta: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemCommandExecutionOutputDeltaNotification](notif, NotificationMethodItemCommandExecutionOutputDelta)
	},
	NotificationMethodItemCommandExecutionTerminalInteraction: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemCommandExecutionTerminalInteractionNotification](notif, NotificationMethodItemCommandExecutionTerminalInteraction)
	},
	NotificationMethodItemCompleted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemCompletedNotification](notif, NotificationMethodItemCompleted)
	},
	NotificationMethodItemFileChangeOutputDelta: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemFileChangeOutputDeltaNotification](notif, NotificationMethodItemFileChangeOutputDelta)
	},
	NotificationMethodItemFileChangePatchUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemFileChangePatchUpdatedNotification](notif, NotificationMethodItemFileChangePatchUpdated)
	},
	NotificationMethodItemMCPToolCallProgress: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[MCPToolCallProgressNotification](notif, NotificationMethodItemMCPToolCallProgress)
	},
	NotificationMethodItemPlanDelta: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[PlanDeltaNotification](notif, NotificationMethodItemPlanDelta)
	},
	NotificationMethodItemReasoningSummaryPartAdded: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ReasoningSummaryPartAddedNotification](notif, NotificationMethodItemReasoningSummaryPartAdded)
	},
	NotificationMethodItemReasoningSummaryTextDelta: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ReasoningSummaryTextDeltaNotification](notif, NotificationMethodItemReasoningSummaryTextDelta)
	},
	NotificationMethodItemReasoningTextDelta: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ReasoningTextDeltaNotification](notif, NotificationMethodItemReasoningTextDelta)
	},
	NotificationMethodItemStarted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemStartedNotification](notif, NotificationMethodItemStarted)
	},
	NotificationMethodMCPServerOAuthLoginCompleted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[MCPServerOAuthLoginCompletedNotification](notif, NotificationMethodMCPServerOAuthLoginCompleted)
	},
	NotificationMethodMCPServerStartupStatusUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[MCPServerStartupStatusUpdatedNotification](notif, NotificationMethodMCPServerStartupStatusUpdated)
	},
	NotificationMethodModelRerouted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ModelReroutedNotification](notif, NotificationMethodModelRerouted)
	},
	NotificationMethodModelVerification: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ModelVerificationNotification](notif, NotificationMethodModelVerification)
	},
	NotificationMethodProcessExited: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ProcessExitedNotification](notif, NotificationMethodProcessExited)
	},
	NotificationMethodProcessOutputDelta: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ProcessOutputDeltaNotification](notif, NotificationMethodProcessOutputDelta)
	},
	NotificationMethodRemoteControlStatusChanged: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[RemoteControlStatusChangedNotification](notif, NotificationMethodRemoteControlStatusChanged)
	},
	NotificationMethodServerRequestResolved: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ServerRequestResolvedNotification](notif, NotificationMethodServerRequestResolved)
	},
	NotificationMethodSkillsChanged: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[SkillsChangedNotification](notif, NotificationMethodSkillsChanged)
	},
	NotificationMethodThreadArchived: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadArchivedNotification](notif, NotificationMethodThreadArchived)
	},
	NotificationMethodThreadClosed: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadClosedNotification](notif, NotificationMethodThreadClosed)
	},
	NotificationMethodThreadCompacted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ContextCompactedNotification](notif, NotificationMethodThreadCompacted)
	},
	NotificationMethodThreadGoalCleared: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadGoalClearedNotification](notif, NotificationMethodThreadGoalCleared)
	},
	NotificationMethodThreadGoalUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadGoalUpdatedNotification](notif, NotificationMethodThreadGoalUpdated)
	},
	NotificationMethodThreadNameUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadNameUpdatedNotification](notif, NotificationMethodThreadNameUpdated)
	},
	NotificationMethodThreadRealtimeClosed: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeClosedNotification](notif, NotificationMethodThreadRealtimeClosed)
	},
	NotificationMethodThreadRealtimeError: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeErrorNotification](notif, NotificationMethodThreadRealtimeError)
	},
	NotificationMethodThreadRealtimeItemAdded: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeItemAddedNotification](notif, NotificationMethodThreadRealtimeItemAdded)
	},
	NotificationMethodThreadRealtimeOutputAudioDelta: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeOutputAudioDeltaNotification](notif, NotificationMethodThreadRealtimeOutputAudioDelta)
	},
	NotificationMethodThreadRealtimeSDP: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeSDPNotification](notif, NotificationMethodThreadRealtimeSDP)
	},
	NotificationMethodThreadRealtimeStarted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeStartedNotification](notif, NotificationMethodThreadRealtimeStarted)
	},
	NotificationMethodThreadRealtimeTranscriptDelta: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeTranscriptDeltaNotification](notif, NotificationMethodThreadRealtimeTranscriptDelta)
	},
	NotificationMethodThreadRealtimeTranscriptDone: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeTranscriptDoneNotification](notif, NotificationMethodThreadRealtimeTranscriptDone)
	},
	NotificationMethodThreadStarted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadStartedNotification](notif, NotificationMethodThreadStarted)
	},
	NotificationMethodThreadStatusChanged: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadStatusChangedNotification](notif, NotificationMethodThreadStatusChanged)
	},
	NotificationMethodThreadTokenUsageUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadTokenUsageUpdatedNotification](notif, NotificationMethodThreadTokenUsageUpdated)
	},
	NotificationMethodThreadUnarchived: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadUnarchivedNotification](notif, NotificationMethodThreadUnarchived)
	},
	NotificationMethodTurnCompleted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[TurnCompletedNotification](notif, NotificationMethodTurnCompleted)
	},
	NotificationMethodTurnDiffUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[TurnDiffUpdatedNotification](notif, NotificationMethodTurnDiffUpdated)
	},
	NotificationMethodTurnPlanUpdated: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[TurnPlanUpdatedNotification](notif, NotificationMethodTurnPlanUpdated)
	},
	NotificationMethodTurnStarted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[TurnStartedNotification](notif, NotificationMethodTurnStarted)
	},
	NotificationMethodWarning: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[WarningNotification](notif, NotificationMethodWarning)
	},
	NotificationMethodWindowsWorldWritableWarning: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[WindowsWorldWritableWarningNotification](notif, NotificationMethodWindowsWorldWritableWarning)
	},
	NotificationMethodWindowsSandboxSetupCompleted: func(notif Notification) (any, bool, error) {
		return DecodeNotificationAs[WindowsSandboxSetupCompletedNotification](notif, NotificationMethodWindowsSandboxSetupCompleted)
	},
}

// KnownNotification is a decoded high-value server notification.
//
// Raw always preserves the original method and params so callers can log,
// forward, or re-decode the notification without losing future protocol fields.
type KnownNotification struct {
	Method string
	Value  any
	Raw    Notification
}

// KnownNotificationMethods returns the list of known notification methods
// that can be decoded by this package.
func KnownNotificationMethods() []string {
	methods := make([]string, 0, len(notificationDecoders))
	for method := range notificationDecoders {
		methods = append(methods, method)
	}
	slices.Sort(methods)
	return methods
}

// DecodeNotificationAs decodes notification params when the method matches.
//
// The boolean return is false when notification.Method does not match method.
// In that case the zero value is returned and params are not decoded.
func DecodeNotificationAs[T any](notif Notification, method string) (T, bool, error) {
	var zero T
	if notif.Method != method {
		return zero, false, nil
	}
	var got T
	if err := json.Unmarshal(notif.Params, &got); err != nil {
		return zero, false, fmt.Errorf("decode %s notification: %w", method, err)
	}
	return got, true, nil
}

// DecodeNotification decodes known server notifications from the upstream
// Python notification registry.
//
// The registry is intentionally explicit: only methods listed in
// notificationDecoders are decoded. New upstream methods stay visible through
// the raw Notification returned by Client.NextNotification instead of being
// dropped or guessed.
//
// If the method is known and params are malformed, a non-nil error is
// returned alongside matched=true and the raw notification.
func DecodeNotification(notif Notification) (KnownNotification, bool, error) {
	decode, ok := notificationDecoders[notif.Method]
	if !ok {
		return KnownNotification{Raw: notif}, false, nil
	}
	value, _, err := decode(notif)
	// The method is known regardless of whether params decoded cleanly.
	return KnownNotification{Method: notif.Method, Value: value, Raw: notif}, true, err
}

// DecodeAccountLoginCompletedNotification decodes the account/login/completed notification.
func DecodeAccountLoginCompletedNotification(notif Notification) (AccountLoginCompletedNotification, bool, error) {
	return DecodeNotificationAs[AccountLoginCompletedNotification](notif, NotificationMethodAccountLoginCompleted)
}

// DecodeAccountRateLimitsUpdatedNotification decodes the account/rateLimits/updated notification.
func DecodeAccountRateLimitsUpdatedNotification(notif Notification) (AccountRateLimitsUpdatedNotification, bool, error) {
	return DecodeNotificationAs[AccountRateLimitsUpdatedNotification](notif, NotificationMethodAccountRateLimitsUpdated)
}

// DecodeAccountUpdatedNotification decodes the account/updated notification.
func DecodeAccountUpdatedNotification(notif Notification) (AccountUpdatedNotification, bool, error) {
	return DecodeNotificationAs[AccountUpdatedNotification](notif, NotificationMethodAccountUpdated)
}

// DecodeAppListUpdatedNotification decodes the app/list/updated notification.
func DecodeAppListUpdatedNotification(notif Notification) (AppListUpdatedNotification, bool, error) {
	return DecodeNotificationAs[AppListUpdatedNotification](notif, NotificationMethodAppListUpdated)
}

// DecodeCommandExecOutputDeltaNotification decodes the command/exec/outputDelta notification.
func DecodeCommandExecOutputDeltaNotification(notif Notification) (CommandExecOutputDeltaNotification, bool, error) {
	return DecodeNotificationAs[CommandExecOutputDeltaNotification](notif, NotificationMethodCommandExecOutputDelta)
}

// DecodeConfigWarningNotification decodes the configWarning notification.
func DecodeConfigWarningNotification(notif Notification) (ConfigWarningNotification, bool, error) {
	return DecodeNotificationAs[ConfigWarningNotification](notif, NotificationMethodConfigWarning)
}

// DecodeDeprecationNoticeNotification decodes the deprecationNotice notification.
func DecodeDeprecationNoticeNotification(notif Notification) (DeprecationNoticeNotification, bool, error) {
	return DecodeNotificationAs[DeprecationNoticeNotification](notif, NotificationMethodDeprecationNotice)
}

// DecodeErrorNotification decodes the error notification.
func DecodeErrorNotification(notif Notification) (ErrorNotification, bool, error) {
	return DecodeNotificationAs[ErrorNotification](notif, NotificationMethodError)
}

// DecodeExternalAgentConfigImportCompletedNotification decodes the externalAgentConfig/import/completed notification.
func DecodeExternalAgentConfigImportCompletedNotification(notif Notification) (ExternalAgentConfigImportCompletedNotification, bool, error) {
	return DecodeNotificationAs[ExternalAgentConfigImportCompletedNotification](notif, NotificationMethodExternalAgentConfigImportCompleted)
}

// DecodeFSChangedNotification decodes the fs/changed notification.
func DecodeFSChangedNotification(notif Notification) (FSChangedNotification, bool, error) {
	return DecodeNotificationAs[FSChangedNotification](notif, NotificationMethodFSChanged)
}

// DecodeFuzzyFileSearchSessionCompletedNotification decodes the fuzzyFileSearch/sessionCompleted notification.
func DecodeFuzzyFileSearchSessionCompletedNotification(notif Notification) (FuzzyFileSearchSessionCompletedNotification, bool, error) {
	return DecodeNotificationAs[FuzzyFileSearchSessionCompletedNotification](notif, NotificationMethodFuzzyFileSearchSessionCompleted)
}

// DecodeFuzzyFileSearchSessionUpdatedNotification decodes the fuzzyFileSearch/sessionUpdated notification.
func DecodeFuzzyFileSearchSessionUpdatedNotification(notif Notification) (FuzzyFileSearchSessionUpdatedNotification, bool, error) {
	return DecodeNotificationAs[FuzzyFileSearchSessionUpdatedNotification](notif, NotificationMethodFuzzyFileSearchSessionUpdated)
}

// DecodeGuardianWarningNotification decodes the guardianWarning notification.
func DecodeGuardianWarningNotification(notif Notification) (GuardianWarningNotification, bool, error) {
	return DecodeNotificationAs[GuardianWarningNotification](notif, NotificationMethodGuardianWarning)
}

// DecodeHookCompletedNotification decodes the hook/completed notification.
func DecodeHookCompletedNotification(notif Notification) (HookCompletedNotification, bool, error) {
	return DecodeNotificationAs[HookCompletedNotification](notif, NotificationMethodHookCompleted)
}

// DecodeHookStartedNotification decodes the hook/started notification.
func DecodeHookStartedNotification(notif Notification) (HookStartedNotification, bool, error) {
	return DecodeNotificationAs[HookStartedNotification](notif, NotificationMethodHookStarted)
}

// DecodeItemAgentMessageDeltaNotification decodes the item/agentMessage/delta notification.
func DecodeItemAgentMessageDeltaNotification(notif Notification) (AgentMessageDeltaNotification, bool, error) {
	return DecodeNotificationAs[AgentMessageDeltaNotification](notif, NotificationMethodItemAgentMessageDelta)
}

// DecodeItemAutoApprovalReviewCompletedNotification decodes the item/autoApprovalReview/completed notification.
func DecodeItemAutoApprovalReviewCompletedNotification(notif Notification) (ItemGuardianApprovalReviewCompletedNotification, bool, error) {
	return DecodeNotificationAs[ItemGuardianApprovalReviewCompletedNotification](notif, NotificationMethodItemAutoApprovalReviewCompleted)
}

// DecodeItemAutoApprovalReviewStartedNotification decodes the item/autoApprovalReview/started notification.
func DecodeItemAutoApprovalReviewStartedNotification(notif Notification) (ItemGuardianApprovalReviewStartedNotification, bool, error) {
	return DecodeNotificationAs[ItemGuardianApprovalReviewStartedNotification](notif, NotificationMethodItemAutoApprovalReviewStarted)
}

// DecodeItemCommandExecutionOutputDeltaNotification decodes the item/commandExecution/outputDelta notification.
func DecodeItemCommandExecutionOutputDeltaNotification(notif Notification) (ItemCommandExecutionOutputDeltaNotification, bool, error) {
	return DecodeNotificationAs[ItemCommandExecutionOutputDeltaNotification](notif, NotificationMethodItemCommandExecutionOutputDelta)
}

// DecodeItemCommandExecutionTerminalInteractionNotification decodes the item/commandExecution/terminalInteraction notification.
func DecodeItemCommandExecutionTerminalInteractionNotification(notif Notification) (ItemCommandExecutionTerminalInteractionNotification, bool, error) {
	return DecodeNotificationAs[ItemCommandExecutionTerminalInteractionNotification](notif, NotificationMethodItemCommandExecutionTerminalInteraction)
}

// DecodeItemCompletedNotification decodes the item/completed notification.
func DecodeItemCompletedNotification(notif Notification) (ItemCompletedNotification, bool, error) {
	return DecodeNotificationAs[ItemCompletedNotification](notif, NotificationMethodItemCompleted)
}

// DecodeItemFileChangeOutputDeltaNotification decodes the item/fileChange/outputDelta notification.
func DecodeItemFileChangeOutputDeltaNotification(notif Notification) (ItemFileChangeOutputDeltaNotification, bool, error) {
	return DecodeNotificationAs[ItemFileChangeOutputDeltaNotification](notif, NotificationMethodItemFileChangeOutputDelta)
}

// DecodeItemFileChangePatchUpdatedNotification decodes the item/fileChange/patchUpdated notification.
func DecodeItemFileChangePatchUpdatedNotification(notif Notification) (ItemFileChangePatchUpdatedNotification, bool, error) {
	return DecodeNotificationAs[ItemFileChangePatchUpdatedNotification](notif, NotificationMethodItemFileChangePatchUpdated)
}

// DecodeItemMCPToolCallProgressNotification decodes the item/mcpToolCall/progress notification.
func DecodeItemMCPToolCallProgressNotification(notif Notification) (MCPToolCallProgressNotification, bool, error) {
	return DecodeNotificationAs[MCPToolCallProgressNotification](notif, NotificationMethodItemMCPToolCallProgress)
}

// DecodeItemPlanDeltaNotification decodes the item/plan/delta notification.
func DecodeItemPlanDeltaNotification(notif Notification) (PlanDeltaNotification, bool, error) {
	return DecodeNotificationAs[PlanDeltaNotification](notif, NotificationMethodItemPlanDelta)
}

// DecodeItemReasoningSummaryPartAddedNotification decodes the item/reasoning/summaryPartAdded notification.
func DecodeItemReasoningSummaryPartAddedNotification(notif Notification) (ReasoningSummaryPartAddedNotification, bool, error) {
	return DecodeNotificationAs[ReasoningSummaryPartAddedNotification](notif, NotificationMethodItemReasoningSummaryPartAdded)
}

// DecodeItemReasoningSummaryTextDeltaNotification decodes the item/reasoning/summaryTextDelta notification.
func DecodeItemReasoningSummaryTextDeltaNotification(notif Notification) (ReasoningSummaryTextDeltaNotification, bool, error) {
	return DecodeNotificationAs[ReasoningSummaryTextDeltaNotification](notif, NotificationMethodItemReasoningSummaryTextDelta)
}

// DecodeItemReasoningTextDeltaNotification decodes the item/reasoning/textDelta notification.
func DecodeItemReasoningTextDeltaNotification(notif Notification) (ReasoningTextDeltaNotification, bool, error) {
	return DecodeNotificationAs[ReasoningTextDeltaNotification](notif, NotificationMethodItemReasoningTextDelta)
}

// DecodeItemStartedNotification decodes the item/started notification.
func DecodeItemStartedNotification(notif Notification) (ItemStartedNotification, bool, error) {
	return DecodeNotificationAs[ItemStartedNotification](notif, NotificationMethodItemStarted)
}

// DecodeMCPServerOAuthLoginCompletedNotification decodes the mcpServer/oauthLogin/completed notification.
func DecodeMCPServerOAuthLoginCompletedNotification(notif Notification) (MCPServerOAuthLoginCompletedNotification, bool, error) {
	return DecodeNotificationAs[MCPServerOAuthLoginCompletedNotification](notif, NotificationMethodMCPServerOAuthLoginCompleted)
}

// DecodeMCPServerStartupStatusUpdatedNotification decodes the mcpServer/startupStatus/updated notification.
func DecodeMCPServerStartupStatusUpdatedNotification(notif Notification) (MCPServerStartupStatusUpdatedNotification, bool, error) {
	return DecodeNotificationAs[MCPServerStartupStatusUpdatedNotification](notif, NotificationMethodMCPServerStartupStatusUpdated)
}

// DecodeModelReroutedNotification decodes the model/rerouted notification.
func DecodeModelReroutedNotification(notif Notification) (ModelReroutedNotification, bool, error) {
	return DecodeNotificationAs[ModelReroutedNotification](notif, NotificationMethodModelRerouted)
}

// DecodeModelVerificationNotification decodes the model/verification notification.
func DecodeModelVerificationNotification(notif Notification) (ModelVerificationNotification, bool, error) {
	return DecodeNotificationAs[ModelVerificationNotification](notif, NotificationMethodModelVerification)
}

// DecodeProcessExitedNotification decodes the process/exited notification.
func DecodeProcessExitedNotification(notif Notification) (ProcessExitedNotification, bool, error) {
	return DecodeNotificationAs[ProcessExitedNotification](notif, NotificationMethodProcessExited)
}

// DecodeProcessOutputDeltaNotification decodes the process/outputDelta notification.
func DecodeProcessOutputDeltaNotification(notif Notification) (ProcessOutputDeltaNotification, bool, error) {
	return DecodeNotificationAs[ProcessOutputDeltaNotification](notif, NotificationMethodProcessOutputDelta)
}

// DecodeRemoteControlStatusChangedNotification decodes the remoteControl/status/changed notification.
func DecodeRemoteControlStatusChangedNotification(notif Notification) (RemoteControlStatusChangedNotification, bool, error) {
	return DecodeNotificationAs[RemoteControlStatusChangedNotification](notif, NotificationMethodRemoteControlStatusChanged)
}

// DecodeServerRequestResolvedNotification decodes the serverRequest/resolved notification.
func DecodeServerRequestResolvedNotification(notif Notification) (ServerRequestResolvedNotification, bool, error) {
	return DecodeNotificationAs[ServerRequestResolvedNotification](notif, NotificationMethodServerRequestResolved)
}

// DecodeSkillsChangedNotification decodes the skills/changed notification.
func DecodeSkillsChangedNotification(notif Notification) (SkillsChangedNotification, bool, error) {
	return DecodeNotificationAs[SkillsChangedNotification](notif, NotificationMethodSkillsChanged)
}

// DecodeThreadArchivedNotification decodes the thread/archived notification.
func DecodeThreadArchivedNotification(notif Notification) (ThreadArchivedNotification, bool, error) {
	return DecodeNotificationAs[ThreadArchivedNotification](notif, NotificationMethodThreadArchived)
}

// DecodeThreadClosedNotification decodes the thread/closed notification.
func DecodeThreadClosedNotification(notif Notification) (ThreadClosedNotification, bool, error) {
	return DecodeNotificationAs[ThreadClosedNotification](notif, NotificationMethodThreadClosed)
}

// DecodeThreadCompactedNotification decodes the thread/compacted notification.
func DecodeThreadCompactedNotification(notif Notification) (ContextCompactedNotification, bool, error) {
	return DecodeNotificationAs[ContextCompactedNotification](notif, NotificationMethodThreadCompacted)
}

// DecodeThreadGoalClearedNotification decodes the thread/goal/cleared notification.
func DecodeThreadGoalClearedNotification(notif Notification) (ThreadGoalClearedNotification, bool, error) {
	return DecodeNotificationAs[ThreadGoalClearedNotification](notif, NotificationMethodThreadGoalCleared)
}

// DecodeThreadGoalUpdatedNotification decodes the thread/goal/updated notification.
func DecodeThreadGoalUpdatedNotification(notif Notification) (ThreadGoalUpdatedNotification, bool, error) {
	return DecodeNotificationAs[ThreadGoalUpdatedNotification](notif, NotificationMethodThreadGoalUpdated)
}

// DecodeThreadNameUpdatedNotification decodes the thread/name/updated notification.
func DecodeThreadNameUpdatedNotification(notif Notification) (ThreadNameUpdatedNotification, bool, error) {
	return DecodeNotificationAs[ThreadNameUpdatedNotification](notif, NotificationMethodThreadNameUpdated)
}

// DecodeThreadRealtimeClosedNotification decodes the thread/realtime/closed notification.
func DecodeThreadRealtimeClosedNotification(notif Notification) (ThreadRealtimeClosedNotification, bool, error) {
	return DecodeNotificationAs[ThreadRealtimeClosedNotification](notif, NotificationMethodThreadRealtimeClosed)
}

// DecodeThreadRealtimeErrorNotification decodes the thread/realtime/error notification.
func DecodeThreadRealtimeErrorNotification(notif Notification) (ThreadRealtimeErrorNotification, bool, error) {
	return DecodeNotificationAs[ThreadRealtimeErrorNotification](notif, NotificationMethodThreadRealtimeError)
}

// DecodeThreadRealtimeItemAddedNotification decodes the thread/realtime/itemAdded notification.
func DecodeThreadRealtimeItemAddedNotification(notif Notification) (ThreadRealtimeItemAddedNotification, bool, error) {
	return DecodeNotificationAs[ThreadRealtimeItemAddedNotification](notif, NotificationMethodThreadRealtimeItemAdded)
}

// DecodeThreadRealtimeOutputAudioDeltaNotification decodes the thread/realtime/outputAudio/delta notification.
func DecodeThreadRealtimeOutputAudioDeltaNotification(notif Notification) (ThreadRealtimeOutputAudioDeltaNotification, bool, error) {
	return DecodeNotificationAs[ThreadRealtimeOutputAudioDeltaNotification](notif, NotificationMethodThreadRealtimeOutputAudioDelta)
}

// DecodeThreadRealtimeSDPNotification decodes the thread/realtime/sdp notification.
func DecodeThreadRealtimeSDPNotification(notif Notification) (ThreadRealtimeSDPNotification, bool, error) {
	return DecodeNotificationAs[ThreadRealtimeSDPNotification](notif, NotificationMethodThreadRealtimeSDP)
}

// DecodeThreadRealtimeStartedNotification decodes the thread/realtime/started notification.
func DecodeThreadRealtimeStartedNotification(notif Notification) (ThreadRealtimeStartedNotification, bool, error) {
	return DecodeNotificationAs[ThreadRealtimeStartedNotification](notif, NotificationMethodThreadRealtimeStarted)
}

// DecodeThreadRealtimeTranscriptDeltaNotification decodes the thread/realtime/transcript/delta notification.
func DecodeThreadRealtimeTranscriptDeltaNotification(notif Notification) (ThreadRealtimeTranscriptDeltaNotification, bool, error) {
	return DecodeNotificationAs[ThreadRealtimeTranscriptDeltaNotification](notif, NotificationMethodThreadRealtimeTranscriptDelta)
}

// DecodeThreadRealtimeTranscriptDoneNotification decodes the thread/realtime/transcript/done notification.
func DecodeThreadRealtimeTranscriptDoneNotification(notif Notification) (ThreadRealtimeTranscriptDoneNotification, bool, error) {
	return DecodeNotificationAs[ThreadRealtimeTranscriptDoneNotification](notif, NotificationMethodThreadRealtimeTranscriptDone)
}

// DecodeThreadStartedNotification decodes the thread/started notification.
func DecodeThreadStartedNotification(notif Notification) (ThreadStartedNotification, bool, error) {
	return DecodeNotificationAs[ThreadStartedNotification](notif, NotificationMethodThreadStarted)
}

// DecodeThreadStatusChangedNotification decodes the thread/status/changed notification.
func DecodeThreadStatusChangedNotification(notif Notification) (ThreadStatusChangedNotification, bool, error) {
	return DecodeNotificationAs[ThreadStatusChangedNotification](notif, NotificationMethodThreadStatusChanged)
}

// DecodeThreadTokenUsageUpdatedNotification decodes the thread/tokenUsage/updated notification.
func DecodeThreadTokenUsageUpdatedNotification(notif Notification) (ThreadTokenUsageUpdatedNotification, bool, error) {
	return DecodeNotificationAs[ThreadTokenUsageUpdatedNotification](notif, NotificationMethodThreadTokenUsageUpdated)
}

// DecodeThreadUnarchivedNotification decodes the thread/unarchived notification.
func DecodeThreadUnarchivedNotification(notif Notification) (ThreadUnarchivedNotification, bool, error) {
	return DecodeNotificationAs[ThreadUnarchivedNotification](notif, NotificationMethodThreadUnarchived)
}

// DecodeTurnCompletedNotification decodes the turn/completed notification.
func DecodeTurnCompletedNotification(notif Notification) (TurnCompletedNotification, bool, error) {
	return DecodeNotificationAs[TurnCompletedNotification](notif, NotificationMethodTurnCompleted)
}

// DecodeTurnDiffUpdatedNotification decodes the turn/diff/updated notification.
func DecodeTurnDiffUpdatedNotification(notif Notification) (TurnDiffUpdatedNotification, bool, error) {
	return DecodeNotificationAs[TurnDiffUpdatedNotification](notif, NotificationMethodTurnDiffUpdated)
}

// DecodeTurnPlanUpdatedNotification decodes the turn/plan/updated notification.
func DecodeTurnPlanUpdatedNotification(notif Notification) (TurnPlanUpdatedNotification, bool, error) {
	return DecodeNotificationAs[TurnPlanUpdatedNotification](notif, NotificationMethodTurnPlanUpdated)
}

// DecodeTurnStartedNotification decodes the turn/started notification.
func DecodeTurnStartedNotification(notif Notification) (TurnStartedNotification, bool, error) {
	return DecodeNotificationAs[TurnStartedNotification](notif, NotificationMethodTurnStarted)
}

// DecodeWarningNotification decodes the warning notification.
func DecodeWarningNotification(notif Notification) (WarningNotification, bool, error) {
	return DecodeNotificationAs[WarningNotification](notif, NotificationMethodWarning)
}

// DecodeWindowsWorldWritableWarningNotification decodes the windows/worldWritableWarning notification.
func DecodeWindowsWorldWritableWarningNotification(notif Notification) (WindowsWorldWritableWarningNotification, bool, error) {
	return DecodeNotificationAs[WindowsWorldWritableWarningNotification](notif, NotificationMethodWindowsWorldWritableWarning)
}

// DecodeWindowsSandboxSetupCompletedNotification decodes the windowsSandbox/setupCompleted notification.
func DecodeWindowsSandboxSetupCompletedNotification(notif Notification) (WindowsSandboxSetupCompletedNotification, bool, error) {
	return DecodeNotificationAs[WindowsSandboxSetupCompletedNotification](notif, NotificationMethodWindowsSandboxSetupCompleted)
}

// AccountLoginCompleted decodes the account/login/completed notification.
func (notif Notification) AccountLoginCompleted() (AccountLoginCompletedNotification, bool, error) {
	return DecodeAccountLoginCompletedNotification(notif)
}

// AccountRateLimitsUpdated decodes the account/rateLimits/updated notification.
func (notif Notification) AccountRateLimitsUpdated() (AccountRateLimitsUpdatedNotification, bool, error) {
	return DecodeAccountRateLimitsUpdatedNotification(notif)
}

// AccountUpdated decodes the account/updated notification.
func (notif Notification) AccountUpdated() (AccountUpdatedNotification, bool, error) {
	return DecodeAccountUpdatedNotification(notif)
}

// AppListUpdated decodes the app/list/updated notification.
func (notif Notification) AppListUpdated() (AppListUpdatedNotification, bool, error) {
	return DecodeAppListUpdatedNotification(notif)
}

// CommandExecOutputDelta decodes the command/exec/outputDelta notification.
func (notif Notification) CommandExecOutputDelta() (CommandExecOutputDeltaNotification, bool, error) {
	return DecodeCommandExecOutputDeltaNotification(notif)
}

// ConfigWarning decodes the configWarning notification.
func (notif Notification) ConfigWarning() (ConfigWarningNotification, bool, error) {
	return DecodeConfigWarningNotification(notif)
}

// DeprecationNotice decodes the deprecationNotice notification.
func (notif Notification) DeprecationNotice() (DeprecationNoticeNotification, bool, error) {
	return DecodeDeprecationNoticeNotification(notif)
}

// ErrorNotification decodes the error notification.
func (notif Notification) ErrorNotification() (ErrorNotification, bool, error) {
	return DecodeErrorNotification(notif)
}

// ExternalAgentConfigImportCompleted decodes the externalAgentConfig/import/completed notification.
func (notif Notification) ExternalAgentConfigImportCompleted() (ExternalAgentConfigImportCompletedNotification, bool, error) {
	return DecodeExternalAgentConfigImportCompletedNotification(notif)
}

// FSChanged decodes the fs/changed notification.
func (notif Notification) FSChanged() (FSChangedNotification, bool, error) {
	return DecodeFSChangedNotification(notif)
}

// FuzzyFileSearchSessionCompleted decodes the fuzzyFileSearch/sessionCompleted notification.
func (notif Notification) FuzzyFileSearchSessionCompleted() (FuzzyFileSearchSessionCompletedNotification, bool, error) {
	return DecodeFuzzyFileSearchSessionCompletedNotification(notif)
}

// FuzzyFileSearchSessionUpdated decodes the fuzzyFileSearch/sessionUpdated notification.
func (notif Notification) FuzzyFileSearchSessionUpdated() (FuzzyFileSearchSessionUpdatedNotification, bool, error) {
	return DecodeFuzzyFileSearchSessionUpdatedNotification(notif)
}

// GuardianWarning decodes the guardianWarning notification.
func (notif Notification) GuardianWarning() (GuardianWarningNotification, bool, error) {
	return DecodeGuardianWarningNotification(notif)
}

// HookCompleted decodes the hook/completed notification.
func (notif Notification) HookCompleted() (HookCompletedNotification, bool, error) {
	return DecodeHookCompletedNotification(notif)
}

// HookStarted decodes the hook/started notification.
func (notif Notification) HookStarted() (HookStartedNotification, bool, error) {
	return DecodeHookStartedNotification(notif)
}

// ItemAgentMessageDelta decodes the item/agentMessage/delta notification.
func (notif Notification) ItemAgentMessageDelta() (AgentMessageDeltaNotification, bool, error) {
	return DecodeItemAgentMessageDeltaNotification(notif)
}

// ItemAutoApprovalReviewCompleted decodes the item/autoApprovalReview/completed notification.
func (notif Notification) ItemAutoApprovalReviewCompleted() (ItemGuardianApprovalReviewCompletedNotification, bool, error) {
	return DecodeItemAutoApprovalReviewCompletedNotification(notif)
}

// ItemAutoApprovalReviewStarted decodes the item/autoApprovalReview/started notification.
func (notif Notification) ItemAutoApprovalReviewStarted() (ItemGuardianApprovalReviewStartedNotification, bool, error) {
	return DecodeItemAutoApprovalReviewStartedNotification(notif)
}

// ItemCommandExecutionOutputDelta decodes the item/commandExecution/outputDelta notification.
func (notif Notification) ItemCommandExecutionOutputDelta() (ItemCommandExecutionOutputDeltaNotification, bool, error) {
	return DecodeItemCommandExecutionOutputDeltaNotification(notif)
}

// ItemCommandExecutionTerminalInteraction decodes the item/commandExecution/terminalInteraction notification.
func (notif Notification) ItemCommandExecutionTerminalInteraction() (ItemCommandExecutionTerminalInteractionNotification, bool, error) {
	return DecodeItemCommandExecutionTerminalInteractionNotification(notif)
}

// ItemCompleted decodes the item/completed notification.
func (notif Notification) ItemCompleted() (ItemCompletedNotification, bool, error) {
	return DecodeItemCompletedNotification(notif)
}

// ItemFileChangeOutputDelta decodes the item/fileChange/outputDelta notification.
func (notif Notification) ItemFileChangeOutputDelta() (ItemFileChangeOutputDeltaNotification, bool, error) {
	return DecodeItemFileChangeOutputDeltaNotification(notif)
}

// ItemFileChangePatchUpdated decodes the item/fileChange/patchUpdated notification.
func (notif Notification) ItemFileChangePatchUpdated() (ItemFileChangePatchUpdatedNotification, bool, error) {
	return DecodeItemFileChangePatchUpdatedNotification(notif)
}

// ItemMCPToolCallProgress decodes the item/mcpToolCall/progress notification.
func (notif Notification) ItemMCPToolCallProgress() (MCPToolCallProgressNotification, bool, error) {
	return DecodeItemMCPToolCallProgressNotification(notif)
}

// ItemPlanDelta decodes the item/plan/delta notification.
func (notif Notification) ItemPlanDelta() (PlanDeltaNotification, bool, error) {
	return DecodeItemPlanDeltaNotification(notif)
}

// ItemReasoningSummaryPartAdded decodes the item/reasoning/summaryPartAdded notification.
func (notif Notification) ItemReasoningSummaryPartAdded() (ReasoningSummaryPartAddedNotification, bool, error) {
	return DecodeItemReasoningSummaryPartAddedNotification(notif)
}

// ItemReasoningSummaryTextDelta decodes the item/reasoning/summaryTextDelta notification.
func (notif Notification) ItemReasoningSummaryTextDelta() (ReasoningSummaryTextDeltaNotification, bool, error) {
	return DecodeItemReasoningSummaryTextDeltaNotification(notif)
}

// ItemReasoningTextDelta decodes the item/reasoning/textDelta notification.
func (notif Notification) ItemReasoningTextDelta() (ReasoningTextDeltaNotification, bool, error) {
	return DecodeItemReasoningTextDeltaNotification(notif)
}

// ItemStarted decodes the item/started notification.
func (notif Notification) ItemStarted() (ItemStartedNotification, bool, error) {
	return DecodeItemStartedNotification(notif)
}

// MCPServerOAuthLoginCompleted decodes the mcpServer/oauthLogin/completed notification.
func (notif Notification) MCPServerOAuthLoginCompleted() (MCPServerOAuthLoginCompletedNotification, bool, error) {
	return DecodeMCPServerOAuthLoginCompletedNotification(notif)
}

// MCPServerStartupStatusUpdated decodes the mcpServer/startupStatus/updated notification.
func (notif Notification) MCPServerStartupStatusUpdated() (MCPServerStartupStatusUpdatedNotification, bool, error) {
	return DecodeMCPServerStartupStatusUpdatedNotification(notif)
}

// ModelRerouted decodes the model/rerouted notification.
func (notif Notification) ModelRerouted() (ModelReroutedNotification, bool, error) {
	return DecodeModelReroutedNotification(notif)
}

// ModelVerification decodes the model/verification notification.
func (notif Notification) ModelVerification() (ModelVerificationNotification, bool, error) {
	return DecodeModelVerificationNotification(notif)
}

// ProcessExited decodes the process/exited notification.
func (notif Notification) ProcessExited() (ProcessExitedNotification, bool, error) {
	return DecodeProcessExitedNotification(notif)
}

// ProcessOutputDelta decodes the process/outputDelta notification.
func (notif Notification) ProcessOutputDelta() (ProcessOutputDeltaNotification, bool, error) {
	return DecodeProcessOutputDeltaNotification(notif)
}

// RemoteControlStatusChanged decodes the remoteControl/status/changed notification.
func (notif Notification) RemoteControlStatusChanged() (RemoteControlStatusChangedNotification, bool, error) {
	return DecodeRemoteControlStatusChangedNotification(notif)
}

// ServerRequestResolved decodes the serverRequest/resolved notification.
func (notif Notification) ServerRequestResolved() (ServerRequestResolvedNotification, bool, error) {
	return DecodeServerRequestResolvedNotification(notif)
}

// SkillsChanged decodes the skills/changed notification.
func (notif Notification) SkillsChanged() (SkillsChangedNotification, bool, error) {
	return DecodeSkillsChangedNotification(notif)
}

// ThreadArchived decodes the thread/archived notification.
func (notif Notification) ThreadArchived() (ThreadArchivedNotification, bool, error) {
	return DecodeThreadArchivedNotification(notif)
}

// ThreadClosed decodes the thread/closed notification.
func (notif Notification) ThreadClosed() (ThreadClosedNotification, bool, error) {
	return DecodeThreadClosedNotification(notif)
}

// ThreadCompacted decodes the thread/compacted notification.
func (notif Notification) ThreadCompacted() (ContextCompactedNotification, bool, error) {
	return DecodeThreadCompactedNotification(notif)
}

// ThreadGoalCleared decodes the thread/goal/cleared notification.
func (notif Notification) ThreadGoalCleared() (ThreadGoalClearedNotification, bool, error) {
	return DecodeThreadGoalClearedNotification(notif)
}

// ThreadGoalUpdated decodes the thread/goal/updated notification.
func (notif Notification) ThreadGoalUpdated() (ThreadGoalUpdatedNotification, bool, error) {
	return DecodeThreadGoalUpdatedNotification(notif)
}

// ThreadNameUpdated decodes the thread/name/updated notification.
func (notif Notification) ThreadNameUpdated() (ThreadNameUpdatedNotification, bool, error) {
	return DecodeThreadNameUpdatedNotification(notif)
}

// ThreadRealtimeClosed decodes the thread/realtime/closed notification.
func (notif Notification) ThreadRealtimeClosed() (ThreadRealtimeClosedNotification, bool, error) {
	return DecodeThreadRealtimeClosedNotification(notif)
}

// ThreadRealtimeError decodes the thread/realtime/error notification.
func (notif Notification) ThreadRealtimeError() (ThreadRealtimeErrorNotification, bool, error) {
	return DecodeThreadRealtimeErrorNotification(notif)
}

// ThreadRealtimeItemAdded decodes the thread/realtime/itemAdded notification.
func (notif Notification) ThreadRealtimeItemAdded() (ThreadRealtimeItemAddedNotification, bool, error) {
	return DecodeThreadRealtimeItemAddedNotification(notif)
}

// ThreadRealtimeOutputAudioDelta decodes the thread/realtime/outputAudio/delta notification.
func (notif Notification) ThreadRealtimeOutputAudioDelta() (ThreadRealtimeOutputAudioDeltaNotification, bool, error) {
	return DecodeThreadRealtimeOutputAudioDeltaNotification(notif)
}

// ThreadRealtimeSDP decodes the thread/realtime/sdp notification.
func (notif Notification) ThreadRealtimeSDP() (ThreadRealtimeSDPNotification, bool, error) {
	return DecodeThreadRealtimeSDPNotification(notif)
}

// ThreadRealtimeStarted decodes the thread/realtime/started notification.
func (notif Notification) ThreadRealtimeStarted() (ThreadRealtimeStartedNotification, bool, error) {
	return DecodeThreadRealtimeStartedNotification(notif)
}

// ThreadRealtimeTranscriptDelta decodes the thread/realtime/transcript/delta notification.
func (notif Notification) ThreadRealtimeTranscriptDelta() (ThreadRealtimeTranscriptDeltaNotification, bool, error) {
	return DecodeThreadRealtimeTranscriptDeltaNotification(notif)
}

// ThreadRealtimeTranscriptDone decodes the thread/realtime/transcript/done notification.
func (notif Notification) ThreadRealtimeTranscriptDone() (ThreadRealtimeTranscriptDoneNotification, bool, error) {
	return DecodeThreadRealtimeTranscriptDoneNotification(notif)
}

// ThreadStarted decodes the thread/started notification.
func (notif Notification) ThreadStarted() (ThreadStartedNotification, bool, error) {
	return DecodeThreadStartedNotification(notif)
}

// ThreadStatusChanged decodes the thread/status/changed notification.
func (notif Notification) ThreadStatusChanged() (ThreadStatusChangedNotification, bool, error) {
	return DecodeThreadStatusChangedNotification(notif)
}

// ThreadTokenUsageUpdated decodes the thread/tokenUsage/updated notification.
func (notif Notification) ThreadTokenUsageUpdated() (ThreadTokenUsageUpdatedNotification, bool, error) {
	return DecodeThreadTokenUsageUpdatedNotification(notif)
}

// ThreadUnarchived decodes the thread/unarchived notification.
func (notif Notification) ThreadUnarchived() (ThreadUnarchivedNotification, bool, error) {
	return DecodeThreadUnarchivedNotification(notif)
}

// TurnCompleted decodes the turn/completed notification.
func (notif Notification) TurnCompleted() (TurnCompletedNotification, bool, error) {
	return DecodeTurnCompletedNotification(notif)
}

// TurnDiffUpdated decodes the turn/diff/updated notification.
func (notif Notification) TurnDiffUpdated() (TurnDiffUpdatedNotification, bool, error) {
	return DecodeTurnDiffUpdatedNotification(notif)
}

// TurnPlanUpdated decodes the turn/plan/updated notification.
func (notif Notification) TurnPlanUpdated() (TurnPlanUpdatedNotification, bool, error) {
	return DecodeTurnPlanUpdatedNotification(notif)
}

// TurnStarted decodes the turn/started notification.
func (notif Notification) TurnStarted() (TurnStartedNotification, bool, error) {
	return DecodeTurnStartedNotification(notif)
}

// Warning decodes the warning notification.
func (notif Notification) Warning() (WarningNotification, bool, error) {
	return DecodeWarningNotification(notif)
}

// WindowsWorldWritableWarning decodes the windows/worldWritableWarning notification.
func (notif Notification) WindowsWorldWritableWarning() (WindowsWorldWritableWarningNotification, bool, error) {
	return DecodeWindowsWorldWritableWarningNotification(notif)
}

// WindowsSandboxSetupCompleted decodes the windowsSandbox/setupCompleted notification.
func (notif Notification) WindowsSandboxSetupCompleted() (WindowsSandboxSetupCompletedNotification, bool, error) {
	return DecodeWindowsSandboxSetupCompletedNotification(notif)
}
