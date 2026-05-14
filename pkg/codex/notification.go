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

func init() {
	slices.Sort(notificationMethodList)
}

const (
	// Deprecated aliases kept for compatibility.
	NotificationMethodAgentMessageDelta             = NotificationMethodItemAgentMessageDelta
	NotificationMethodThreadTokenUsageUpdatedLegacy = NotificationMethodThreadTokenUsageUpdated
)

var notificationMethodList = []string{
	NotificationMethodAccountLoginCompleted,
	NotificationMethodAccountRateLimitsUpdated,
	NotificationMethodAccountUpdated,
	NotificationMethodAppListUpdated,
	NotificationMethodCommandExecOutputDelta,
	NotificationMethodConfigWarning,
	NotificationMethodDeprecationNotice,
	NotificationMethodError,
	NotificationMethodExternalAgentConfigImportCompleted,
	NotificationMethodFsChanged,
	NotificationMethodFuzzyFileSearchSessionCompleted,
	NotificationMethodFuzzyFileSearchSessionUpdated,
	NotificationMethodGuardianWarning,
	NotificationMethodHookCompleted,
	NotificationMethodHookStarted,
	NotificationMethodItemAgentMessageDelta,
	NotificationMethodItemAutoApprovalReviewCompleted,
	NotificationMethodItemAutoApprovalReviewStarted,
	NotificationMethodItemCommandExecutionOutputDelta,
	NotificationMethodItemCommandExecutionTerminalInteraction,
	NotificationMethodItemCompleted,
	NotificationMethodItemFileChangeOutputDelta,
	NotificationMethodItemFileChangePatchUpdated,
	NotificationMethodItemMCPToolCallProgress,
	NotificationMethodItemPlanDelta,
	NotificationMethodItemReasoningSummaryPartAdded,
	NotificationMethodItemReasoningSummaryTextDelta,
	NotificationMethodItemReasoningTextDelta,
	NotificationMethodItemStarted,
	NotificationMethodMCPServerOAuthLoginCompleted,
	NotificationMethodMCPServerStartupStatusUpdated,
	NotificationMethodModelRerouted,
	NotificationMethodModelVerification,
	NotificationMethodProcessExited,
	NotificationMethodProcessOutputDelta,
	NotificationMethodRemoteControlStatusChanged,
	NotificationMethodServerRequestResolved,
	NotificationMethodSkillsChanged,
	NotificationMethodThreadArchived,
	NotificationMethodThreadClosed,
	NotificationMethodThreadCompacted,
	NotificationMethodThreadGoalCleared,
	NotificationMethodThreadGoalUpdated,
	NotificationMethodThreadNameUpdated,
	NotificationMethodThreadRealtimeClosed,
	NotificationMethodThreadRealtimeError,
	NotificationMethodThreadRealtimeItemAdded,
	NotificationMethodThreadRealtimeOutputAudioDelta,
	NotificationMethodThreadRealtimeSDP,
	NotificationMethodThreadRealtimeStarted,
	NotificationMethodThreadRealtimeTranscriptDelta,
	NotificationMethodThreadRealtimeTranscriptDone,
	NotificationMethodThreadStarted,
	NotificationMethodThreadStatusChanged,
	NotificationMethodThreadTokenUsageUpdated,
	NotificationMethodThreadUnarchived,
	NotificationMethodTurnCompleted,
	NotificationMethodTurnDiffUpdated,
	NotificationMethodTurnPlanUpdated,
	NotificationMethodTurnStarted,
	NotificationMethodWarning,
	NotificationMethodWindowsWorldWritableWarning,
	NotificationMethodWindowsSandboxSetupCompleted,
}

// Notification is a server notification with its method and raw params.
type Notification struct {
	Method string         `json:"method"`
	Params jsontext.Value `json:"params,omitzero"`
}

var notificationDecoders = map[string]func(Notification) (any, bool, error){
	NotificationMethodAccountLoginCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[AccountLoginCompletedNotification](notification, NotificationMethodAccountLoginCompleted)
	},
	NotificationMethodAccountRateLimitsUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[AccountRateLimitsUpdatedNotification](notification, NotificationMethodAccountRateLimitsUpdated)
	},
	NotificationMethodAccountUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[AccountUpdatedNotification](notification, NotificationMethodAccountUpdated)
	},
	NotificationMethodAppListUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[AppListUpdatedNotification](notification, NotificationMethodAppListUpdated)
	},
	NotificationMethodCommandExecOutputDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[CommandExecOutputDeltaNotification](notification, NotificationMethodCommandExecOutputDelta)
	},
	NotificationMethodConfigWarning: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ConfigWarningNotification](notification, NotificationMethodConfigWarning)
	},
	NotificationMethodDeprecationNotice: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[DeprecationNoticeNotification](notification, NotificationMethodDeprecationNotice)
	},
	NotificationMethodItemAgentMessageDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[AgentMessageDeltaNotification](notification, NotificationMethodItemAgentMessageDelta)
	},
	NotificationMethodError: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ErrorNotification](notification, NotificationMethodError)
	},
	NotificationMethodExternalAgentConfigImportCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ExternalAgentConfigImportCompletedNotification](notification, NotificationMethodExternalAgentConfigImportCompleted)
	},
	NotificationMethodFsChanged: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[FsChangedNotification](notification, NotificationMethodFsChanged)
	},
	NotificationMethodFuzzyFileSearchSessionCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[FuzzyFileSearchSessionCompletedNotification](notification, NotificationMethodFuzzyFileSearchSessionCompleted)
	},
	NotificationMethodFuzzyFileSearchSessionUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[FuzzyFileSearchSessionUpdatedNotification](notification, NotificationMethodFuzzyFileSearchSessionUpdated)
	},
	NotificationMethodGuardianWarning: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[GuardianWarningNotification](notification, NotificationMethodGuardianWarning)
	},
	NotificationMethodHookCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[HookCompletedNotification](notification, NotificationMethodHookCompleted)
	},
	NotificationMethodHookStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[HookStartedNotification](notification, NotificationMethodHookStarted)
	},
	NotificationMethodItemAutoApprovalReviewCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemGuardianApprovalReviewCompletedNotification](notification, NotificationMethodItemAutoApprovalReviewCompleted)
	},
	NotificationMethodItemAutoApprovalReviewStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemGuardianApprovalReviewStartedNotification](notification, NotificationMethodItemAutoApprovalReviewStarted)
	},
	NotificationMethodItemCommandExecutionOutputDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemCommandExecutionOutputDeltaNotification](notification, NotificationMethodItemCommandExecutionOutputDelta)
	},
	NotificationMethodItemCommandExecutionTerminalInteraction: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemCommandExecutionTerminalInteractionNotification](notification, NotificationMethodItemCommandExecutionTerminalInteraction)
	},
	NotificationMethodItemCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemCompletedNotification](notification, NotificationMethodItemCompleted)
	},
	NotificationMethodItemFileChangeOutputDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemFileChangeOutputDeltaNotification](notification, NotificationMethodItemFileChangeOutputDelta)
	},
	NotificationMethodItemFileChangePatchUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemFileChangePatchUpdatedNotification](notification, NotificationMethodItemFileChangePatchUpdated)
	},
	NotificationMethodItemMCPToolCallProgress: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[MCPToolCallProgressNotification](notification, NotificationMethodItemMCPToolCallProgress)
	},
	NotificationMethodItemPlanDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[PlanDeltaNotification](notification, NotificationMethodItemPlanDelta)
	},
	NotificationMethodItemReasoningSummaryPartAdded: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ReasoningSummaryPartAddedNotification](notification, NotificationMethodItemReasoningSummaryPartAdded)
	},
	NotificationMethodItemReasoningSummaryTextDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ReasoningSummaryTextDeltaNotification](notification, NotificationMethodItemReasoningSummaryTextDelta)
	},
	NotificationMethodItemReasoningTextDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ReasoningTextDeltaNotification](notification, NotificationMethodItemReasoningTextDelta)
	},
	NotificationMethodItemStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ItemStartedNotification](notification, NotificationMethodItemStarted)
	},
	NotificationMethodMCPServerOAuthLoginCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[MCPServerOAuthLoginCompletedNotification](notification, NotificationMethodMCPServerOAuthLoginCompleted)
	},
	NotificationMethodMCPServerStartupStatusUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[MCPServerStartupStatusUpdatedNotification](notification, NotificationMethodMCPServerStartupStatusUpdated)
	},
	NotificationMethodModelRerouted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ModelReroutedNotification](notification, NotificationMethodModelRerouted)
	},
	NotificationMethodModelVerification: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ModelVerificationNotification](notification, NotificationMethodModelVerification)
	},
	NotificationMethodProcessExited: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ProcessExitedNotification](notification, NotificationMethodProcessExited)
	},
	NotificationMethodProcessOutputDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ProcessOutputDeltaNotification](notification, NotificationMethodProcessOutputDelta)
	},
	NotificationMethodRemoteControlStatusChanged: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[RemoteControlStatusChangedNotification](notification, NotificationMethodRemoteControlStatusChanged)
	},
	NotificationMethodServerRequestResolved: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ServerRequestResolvedNotification](notification, NotificationMethodServerRequestResolved)
	},
	NotificationMethodSkillsChanged: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[SkillsChangedNotification](notification, NotificationMethodSkillsChanged)
	},
	NotificationMethodThreadArchived: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadArchivedNotification](notification, NotificationMethodThreadArchived)
	},
	NotificationMethodThreadClosed: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadClosedNotification](notification, NotificationMethodThreadClosed)
	},
	NotificationMethodThreadCompacted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ContextCompactedNotification](notification, NotificationMethodThreadCompacted)
	},
	NotificationMethodThreadGoalCleared: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadGoalClearedNotification](notification, NotificationMethodThreadGoalCleared)
	},
	NotificationMethodThreadGoalUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadGoalUpdatedNotification](notification, NotificationMethodThreadGoalUpdated)
	},
	NotificationMethodThreadNameUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadNameUpdatedNotification](notification, NotificationMethodThreadNameUpdated)
	},
	NotificationMethodThreadRealtimeClosed: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeClosedNotification](notification, NotificationMethodThreadRealtimeClosed)
	},
	NotificationMethodThreadRealtimeError: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeErrorNotification](notification, NotificationMethodThreadRealtimeError)
	},
	NotificationMethodThreadRealtimeItemAdded: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeItemAddedNotification](notification, NotificationMethodThreadRealtimeItemAdded)
	},
	NotificationMethodThreadRealtimeOutputAudioDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeOutputAudioDeltaNotification](notification, NotificationMethodThreadRealtimeOutputAudioDelta)
	},
	NotificationMethodThreadRealtimeSDP: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeSDPNotification](notification, NotificationMethodThreadRealtimeSDP)
	},
	NotificationMethodThreadRealtimeStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeStartedNotification](notification, NotificationMethodThreadRealtimeStarted)
	},
	NotificationMethodThreadRealtimeTranscriptDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeTranscriptDeltaNotification](notification, NotificationMethodThreadRealtimeTranscriptDelta)
	},
	NotificationMethodThreadRealtimeTranscriptDone: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadRealtimeTranscriptDoneNotification](notification, NotificationMethodThreadRealtimeTranscriptDone)
	},
	NotificationMethodThreadStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadStartedNotification](notification, NotificationMethodThreadStarted)
	},
	NotificationMethodThreadStatusChanged: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadStatusChangedNotification](notification, NotificationMethodThreadStatusChanged)
	},
	NotificationMethodThreadTokenUsageUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadTokenUsageUpdatedNotification](notification, NotificationMethodThreadTokenUsageUpdated)
	},
	NotificationMethodThreadUnarchived: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[ThreadUnarchivedNotification](notification, NotificationMethodThreadUnarchived)
	},
	NotificationMethodTurnCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[TurnCompletedNotification](notification, NotificationMethodTurnCompleted)
	},
	NotificationMethodTurnDiffUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[TurnDiffUpdatedNotification](notification, NotificationMethodTurnDiffUpdated)
	},
	NotificationMethodTurnPlanUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[TurnPlanUpdatedNotification](notification, NotificationMethodTurnPlanUpdated)
	},
	NotificationMethodTurnStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[TurnStartedNotification](notification, NotificationMethodTurnStarted)
	},
	NotificationMethodWarning: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[WarningNotification](notification, NotificationMethodWarning)
	},
	NotificationMethodWindowsWorldWritableWarning: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[WindowsWorldWritableWarningNotification](notification, NotificationMethodWindowsWorldWritableWarning)
	},
	NotificationMethodWindowsSandboxSetupCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[WindowsSandboxSetupCompletedNotification](notification, NotificationMethodWindowsSandboxSetupCompleted)
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

func KnownNotificationMethods() []string {
	methods := make([]string, 0, len(notificationDecoders))
	for method := range notificationDecoders {
		methods = append(methods, method)
	}
	slices.Sort(methods)
	return methods
}

// As decodes notification params when the method matches.
//
// The boolean return is false when notification.Method does not match method.
// In that case the zero value is returned and params are not decoded.
func (notification Notification) As[T any](method string) (T, bool, error) {
	var zero T
	if notification.Method != method {
		return zero, false, nil
	}
	var got T
	if err := json.Unmarshal(notification.Params, &got); err != nil {
		return zero, true, fmt.Errorf("decode %s notification: %w", method, err)
	}
	return got, true, nil
}

// DecodeNotificationAs decodes notification params when the method matches.
//
// The boolean return is false when notification.Method does not match method.
// In that case the zero value is returned and params are not decoded.
func DecodeNotificationAs[T any](notification Notification, method string) (T, bool, error) {
	return notification.As[T](method)
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
func DecodeNotification(notification Notification) (KnownNotification, bool, error) {
	decode, ok := notificationDecoders[notification.Method]
	if !ok {
		return KnownNotification{Raw: notification}, false, nil
	}
	value, matched, err := decode(notification)
	return KnownNotification{Method: notification.Method, Value: value, Raw: notification}, matched, err
}

// DecodeKnownNotification decodes the known notification set.
//
// Deprecated: use DecodeNotification.
func DecodeKnownNotification(notification Notification) (KnownNotification, bool, error) {
	return DecodeNotification(notification)
}

// DecodeAccountLoginCompletedNotification decodes the account/login/completed notification.
func DecodeAccountLoginCompletedNotification(notification Notification) (AccountLoginCompletedNotification, bool, error) {
	return notification.As[AccountLoginCompletedNotification](NotificationMethodAccountLoginCompleted)
}

// DecodeAccountRateLimitsUpdatedNotification decodes the account/rateLimits/updated notification.
func DecodeAccountRateLimitsUpdatedNotification(notification Notification) (AccountRateLimitsUpdatedNotification, bool, error) {
	return notification.As[AccountRateLimitsUpdatedNotification](NotificationMethodAccountRateLimitsUpdated)
}

// DecodeAccountUpdatedNotification decodes the account/updated notification.
func DecodeAccountUpdatedNotification(notification Notification) (AccountUpdatedNotification, bool, error) {
	return notification.As[AccountUpdatedNotification](NotificationMethodAccountUpdated)
}

// DecodeAppListUpdatedNotification decodes the app/list/updated notification.
func DecodeAppListUpdatedNotification(notification Notification) (AppListUpdatedNotification, bool, error) {
	return notification.As[AppListUpdatedNotification](NotificationMethodAppListUpdated)
}

// DecodeCommandExecOutputDeltaNotification decodes the command/exec/outputDelta notification.
func DecodeCommandExecOutputDeltaNotification(notification Notification) (CommandExecOutputDeltaNotification, bool, error) {
	return notification.As[CommandExecOutputDeltaNotification](NotificationMethodCommandExecOutputDelta)
}

// DecodeConfigWarningNotification decodes the configWarning notification.
func DecodeConfigWarningNotification(notification Notification) (ConfigWarningNotification, bool, error) {
	return notification.As[ConfigWarningNotification](NotificationMethodConfigWarning)
}

// DecodeDeprecationNoticeNotification decodes the deprecationNotice notification.
func DecodeDeprecationNoticeNotification(notification Notification) (DeprecationNoticeNotification, bool, error) {
	return notification.As[DeprecationNoticeNotification](NotificationMethodDeprecationNotice)
}

// DecodeErrorNotification decodes the error notification.
func DecodeErrorNotification(notification Notification) (ErrorNotification, bool, error) {
	return notification.As[ErrorNotification](NotificationMethodError)
}

// DecodeExternalAgentConfigImportCompletedNotification decodes the externalAgentConfig/import/completed notification.
func DecodeExternalAgentConfigImportCompletedNotification(notification Notification) (ExternalAgentConfigImportCompletedNotification, bool, error) {
	return notification.As[ExternalAgentConfigImportCompletedNotification](NotificationMethodExternalAgentConfigImportCompleted)
}

// DecodeFsChangedNotification decodes the fs/changed notification.
func DecodeFsChangedNotification(notification Notification) (FsChangedNotification, bool, error) {
	return notification.As[FsChangedNotification](NotificationMethodFsChanged)
}

// DecodeFuzzyFileSearchSessionCompletedNotification decodes the fuzzyFileSearch/sessionCompleted notification.
func DecodeFuzzyFileSearchSessionCompletedNotification(notification Notification) (FuzzyFileSearchSessionCompletedNotification, bool, error) {
	return notification.As[FuzzyFileSearchSessionCompletedNotification](NotificationMethodFuzzyFileSearchSessionCompleted)
}

// DecodeFuzzyFileSearchSessionUpdatedNotification decodes the fuzzyFileSearch/sessionUpdated notification.
func DecodeFuzzyFileSearchSessionUpdatedNotification(notification Notification) (FuzzyFileSearchSessionUpdatedNotification, bool, error) {
	return notification.As[FuzzyFileSearchSessionUpdatedNotification](NotificationMethodFuzzyFileSearchSessionUpdated)
}

// DecodeGuardianWarningNotification decodes the guardianWarning notification.
func DecodeGuardianWarningNotification(notification Notification) (GuardianWarningNotification, bool, error) {
	return notification.As[GuardianWarningNotification](NotificationMethodGuardianWarning)
}

// DecodeHookCompletedNotification decodes the hook/completed notification.
func DecodeHookCompletedNotification(notification Notification) (HookCompletedNotification, bool, error) {
	return notification.As[HookCompletedNotification](NotificationMethodHookCompleted)
}

// DecodeHookStartedNotification decodes the hook/started notification.
func DecodeHookStartedNotification(notification Notification) (HookStartedNotification, bool, error) {
	return notification.As[HookStartedNotification](NotificationMethodHookStarted)
}

// DecodeItemAgentMessageDeltaNotification decodes the item/agentMessage/delta notification.
func DecodeItemAgentMessageDeltaNotification(notification Notification) (AgentMessageDeltaNotification, bool, error) {
	return notification.As[AgentMessageDeltaNotification](NotificationMethodItemAgentMessageDelta)
}

// DecodeItemAutoApprovalReviewCompletedNotification decodes the item/autoApprovalReview/completed notification.
func DecodeItemAutoApprovalReviewCompletedNotification(notification Notification) (ItemGuardianApprovalReviewCompletedNotification, bool, error) {
	return notification.As[ItemGuardianApprovalReviewCompletedNotification](NotificationMethodItemAutoApprovalReviewCompleted)
}

// DecodeItemAutoApprovalReviewStartedNotification decodes the item/autoApprovalReview/started notification.
func DecodeItemAutoApprovalReviewStartedNotification(notification Notification) (ItemGuardianApprovalReviewStartedNotification, bool, error) {
	return notification.As[ItemGuardianApprovalReviewStartedNotification](NotificationMethodItemAutoApprovalReviewStarted)
}

// DecodeItemCommandExecutionOutputDeltaNotification decodes the item/commandExecution/outputDelta notification.
func DecodeItemCommandExecutionOutputDeltaNotification(notification Notification) (ItemCommandExecutionOutputDeltaNotification, bool, error) {
	return notification.As[ItemCommandExecutionOutputDeltaNotification](NotificationMethodItemCommandExecutionOutputDelta)
}

// DecodeItemCommandExecutionTerminalInteractionNotification decodes the item/commandExecution/terminalInteraction notification.
func DecodeItemCommandExecutionTerminalInteractionNotification(notification Notification) (ItemCommandExecutionTerminalInteractionNotification, bool, error) {
	return notification.As[ItemCommandExecutionTerminalInteractionNotification](NotificationMethodItemCommandExecutionTerminalInteraction)
}

// DecodeItemCompletedNotification decodes the item/completed notification.
func DecodeItemCompletedNotification(notification Notification) (ItemCompletedNotification, bool, error) {
	return notification.As[ItemCompletedNotification](NotificationMethodItemCompleted)
}

// DecodeItemFileChangeOutputDeltaNotification decodes the item/fileChange/outputDelta notification.
func DecodeItemFileChangeOutputDeltaNotification(notification Notification) (ItemFileChangeOutputDeltaNotification, bool, error) {
	return notification.As[ItemFileChangeOutputDeltaNotification](NotificationMethodItemFileChangeOutputDelta)
}

// DecodeItemFileChangePatchUpdatedNotification decodes the item/fileChange/patchUpdated notification.
func DecodeItemFileChangePatchUpdatedNotification(notification Notification) (ItemFileChangePatchUpdatedNotification, bool, error) {
	return notification.As[ItemFileChangePatchUpdatedNotification](NotificationMethodItemFileChangePatchUpdated)
}

// DecodeItemMCPToolCallProgressNotification decodes the item/mcpToolCall/progress notification.
func DecodeItemMCPToolCallProgressNotification(notification Notification) (MCPToolCallProgressNotification, bool, error) {
	return notification.As[MCPToolCallProgressNotification](NotificationMethodItemMCPToolCallProgress)
}

// DecodeItemPlanDeltaNotification decodes the item/plan/delta notification.
func DecodeItemPlanDeltaNotification(notification Notification) (PlanDeltaNotification, bool, error) {
	return notification.As[PlanDeltaNotification](NotificationMethodItemPlanDelta)
}

// DecodeItemReasoningSummaryPartAddedNotification decodes the item/reasoning/summaryPartAdded notification.
func DecodeItemReasoningSummaryPartAddedNotification(notification Notification) (ReasoningSummaryPartAddedNotification, bool, error) {
	return notification.As[ReasoningSummaryPartAddedNotification](NotificationMethodItemReasoningSummaryPartAdded)
}

// DecodeItemReasoningSummaryTextDeltaNotification decodes the item/reasoning/summaryTextDelta notification.
func DecodeItemReasoningSummaryTextDeltaNotification(notification Notification) (ReasoningSummaryTextDeltaNotification, bool, error) {
	return notification.As[ReasoningSummaryTextDeltaNotification](NotificationMethodItemReasoningSummaryTextDelta)
}

// DecodeItemReasoningTextDeltaNotification decodes the item/reasoning/textDelta notification.
func DecodeItemReasoningTextDeltaNotification(notification Notification) (ReasoningTextDeltaNotification, bool, error) {
	return notification.As[ReasoningTextDeltaNotification](NotificationMethodItemReasoningTextDelta)
}

// DecodeItemStartedNotification decodes the item/started notification.
func DecodeItemStartedNotification(notification Notification) (ItemStartedNotification, bool, error) {
	return notification.As[ItemStartedNotification](NotificationMethodItemStarted)
}

// DecodeMCPServerOAuthLoginCompletedNotification decodes the mcpServer/oauthLogin/completed notification.
func DecodeMCPServerOAuthLoginCompletedNotification(notification Notification) (MCPServerOAuthLoginCompletedNotification, bool, error) {
	return notification.As[MCPServerOAuthLoginCompletedNotification](NotificationMethodMCPServerOAuthLoginCompleted)
}

// DecodeMCPServerStartupStatusUpdatedNotification decodes the mcpServer/startupStatus/updated notification.
func DecodeMCPServerStartupStatusUpdatedNotification(notification Notification) (MCPServerStartupStatusUpdatedNotification, bool, error) {
	return notification.As[MCPServerStartupStatusUpdatedNotification](NotificationMethodMCPServerStartupStatusUpdated)
}

// DecodeModelReroutedNotification decodes the model/rerouted notification.
func DecodeModelReroutedNotification(notification Notification) (ModelReroutedNotification, bool, error) {
	return notification.As[ModelReroutedNotification](NotificationMethodModelRerouted)
}

// DecodeModelVerificationNotification decodes the model/verification notification.
func DecodeModelVerificationNotification(notification Notification) (ModelVerificationNotification, bool, error) {
	return notification.As[ModelVerificationNotification](NotificationMethodModelVerification)
}

// DecodeProcessExitedNotification decodes the process/exited notification.
func DecodeProcessExitedNotification(notification Notification) (ProcessExitedNotification, bool, error) {
	return notification.As[ProcessExitedNotification](NotificationMethodProcessExited)
}

// DecodeProcessOutputDeltaNotification decodes the process/outputDelta notification.
func DecodeProcessOutputDeltaNotification(notification Notification) (ProcessOutputDeltaNotification, bool, error) {
	return notification.As[ProcessOutputDeltaNotification](NotificationMethodProcessOutputDelta)
}

// DecodeRemoteControlStatusChangedNotification decodes the remoteControl/status/changed notification.
func DecodeRemoteControlStatusChangedNotification(notification Notification) (RemoteControlStatusChangedNotification, bool, error) {
	return notification.As[RemoteControlStatusChangedNotification](NotificationMethodRemoteControlStatusChanged)
}

// DecodeServerRequestResolvedNotification decodes the serverRequest/resolved notification.
func DecodeServerRequestResolvedNotification(notification Notification) (ServerRequestResolvedNotification, bool, error) {
	return notification.As[ServerRequestResolvedNotification](NotificationMethodServerRequestResolved)
}

// DecodeSkillsChangedNotification decodes the skills/changed notification.
func DecodeSkillsChangedNotification(notification Notification) (SkillsChangedNotification, bool, error) {
	return notification.As[SkillsChangedNotification](NotificationMethodSkillsChanged)
}

// DecodeThreadArchivedNotification decodes the thread/archived notification.
func DecodeThreadArchivedNotification(notification Notification) (ThreadArchivedNotification, bool, error) {
	return notification.As[ThreadArchivedNotification](NotificationMethodThreadArchived)
}

// DecodeThreadClosedNotification decodes the thread/closed notification.
func DecodeThreadClosedNotification(notification Notification) (ThreadClosedNotification, bool, error) {
	return notification.As[ThreadClosedNotification](NotificationMethodThreadClosed)
}

// DecodeThreadCompactedNotification decodes the thread/compacted notification.
func DecodeThreadCompactedNotification(notification Notification) (ContextCompactedNotification, bool, error) {
	return notification.As[ContextCompactedNotification](NotificationMethodThreadCompacted)
}

// DecodeThreadGoalClearedNotification decodes the thread/goal/cleared notification.
func DecodeThreadGoalClearedNotification(notification Notification) (ThreadGoalClearedNotification, bool, error) {
	return notification.As[ThreadGoalClearedNotification](NotificationMethodThreadGoalCleared)
}

// DecodeThreadGoalUpdatedNotification decodes the thread/goal/updated notification.
func DecodeThreadGoalUpdatedNotification(notification Notification) (ThreadGoalUpdatedNotification, bool, error) {
	return notification.As[ThreadGoalUpdatedNotification](NotificationMethodThreadGoalUpdated)
}

// DecodeThreadNameUpdatedNotification decodes the thread/name/updated notification.
func DecodeThreadNameUpdatedNotification(notification Notification) (ThreadNameUpdatedNotification, bool, error) {
	return notification.As[ThreadNameUpdatedNotification](NotificationMethodThreadNameUpdated)
}

// DecodeThreadRealtimeClosedNotification decodes the thread/realtime/closed notification.
func DecodeThreadRealtimeClosedNotification(notification Notification) (ThreadRealtimeClosedNotification, bool, error) {
	return notification.As[ThreadRealtimeClosedNotification](NotificationMethodThreadRealtimeClosed)
}

// DecodeThreadRealtimeErrorNotification decodes the thread/realtime/error notification.
func DecodeThreadRealtimeErrorNotification(notification Notification) (ThreadRealtimeErrorNotification, bool, error) {
	return notification.As[ThreadRealtimeErrorNotification](NotificationMethodThreadRealtimeError)
}

// DecodeThreadRealtimeItemAddedNotification decodes the thread/realtime/itemAdded notification.
func DecodeThreadRealtimeItemAddedNotification(notification Notification) (ThreadRealtimeItemAddedNotification, bool, error) {
	return notification.As[ThreadRealtimeItemAddedNotification](NotificationMethodThreadRealtimeItemAdded)
}

// DecodeThreadRealtimeOutputAudioDeltaNotification decodes the thread/realtime/outputAudio/delta notification.
func DecodeThreadRealtimeOutputAudioDeltaNotification(notification Notification) (ThreadRealtimeOutputAudioDeltaNotification, bool, error) {
	return notification.As[ThreadRealtimeOutputAudioDeltaNotification](NotificationMethodThreadRealtimeOutputAudioDelta)
}

// DecodeThreadRealtimeSDPNotification decodes the thread/realtime/sdp notification.
func DecodeThreadRealtimeSDPNotification(notification Notification) (ThreadRealtimeSDPNotification, bool, error) {
	return notification.As[ThreadRealtimeSDPNotification](NotificationMethodThreadRealtimeSDP)
}

// DecodeThreadRealtimeStartedNotification decodes the thread/realtime/started notification.
func DecodeThreadRealtimeStartedNotification(notification Notification) (ThreadRealtimeStartedNotification, bool, error) {
	return notification.As[ThreadRealtimeStartedNotification](NotificationMethodThreadRealtimeStarted)
}

// DecodeThreadRealtimeTranscriptDeltaNotification decodes the thread/realtime/transcript/delta notification.
func DecodeThreadRealtimeTranscriptDeltaNotification(notification Notification) (ThreadRealtimeTranscriptDeltaNotification, bool, error) {
	return notification.As[ThreadRealtimeTranscriptDeltaNotification](NotificationMethodThreadRealtimeTranscriptDelta)
}

// DecodeThreadRealtimeTranscriptDoneNotification decodes the thread/realtime/transcript/done notification.
func DecodeThreadRealtimeTranscriptDoneNotification(notification Notification) (ThreadRealtimeTranscriptDoneNotification, bool, error) {
	return notification.As[ThreadRealtimeTranscriptDoneNotification](NotificationMethodThreadRealtimeTranscriptDone)
}

// DecodeThreadStartedNotification decodes the thread/started notification.
func DecodeThreadStartedNotification(notification Notification) (ThreadStartedNotification, bool, error) {
	return notification.As[ThreadStartedNotification](NotificationMethodThreadStarted)
}

// DecodeThreadStatusChangedNotification decodes the thread/status/changed notification.
func DecodeThreadStatusChangedNotification(notification Notification) (ThreadStatusChangedNotification, bool, error) {
	return notification.As[ThreadStatusChangedNotification](NotificationMethodThreadStatusChanged)
}

// DecodeThreadTokenUsageUpdatedNotification decodes the thread/tokenUsage/updated notification.
func DecodeThreadTokenUsageUpdatedNotification(notification Notification) (ThreadTokenUsageUpdatedNotification, bool, error) {
	return notification.As[ThreadTokenUsageUpdatedNotification](NotificationMethodThreadTokenUsageUpdated)
}

// DecodeThreadUnarchivedNotification decodes the thread/unarchived notification.
func DecodeThreadUnarchivedNotification(notification Notification) (ThreadUnarchivedNotification, bool, error) {
	return notification.As[ThreadUnarchivedNotification](NotificationMethodThreadUnarchived)
}

// DecodeTurnCompletedNotification decodes the turn/completed notification.
func DecodeTurnCompletedNotification(notification Notification) (TurnCompletedNotification, bool, error) {
	return notification.As[TurnCompletedNotification](NotificationMethodTurnCompleted)
}

// DecodeTurnDiffUpdatedNotification decodes the turn/diff/updated notification.
func DecodeTurnDiffUpdatedNotification(notification Notification) (TurnDiffUpdatedNotification, bool, error) {
	return notification.As[TurnDiffUpdatedNotification](NotificationMethodTurnDiffUpdated)
}

// DecodeTurnPlanUpdatedNotification decodes the turn/plan/updated notification.
func DecodeTurnPlanUpdatedNotification(notification Notification) (TurnPlanUpdatedNotification, bool, error) {
	return notification.As[TurnPlanUpdatedNotification](NotificationMethodTurnPlanUpdated)
}

// DecodeTurnStartedNotification decodes the turn/started notification.
func DecodeTurnStartedNotification(notification Notification) (TurnStartedNotification, bool, error) {
	return notification.As[TurnStartedNotification](NotificationMethodTurnStarted)
}

// DecodeWarningNotification decodes the warning notification.
func DecodeWarningNotification(notification Notification) (WarningNotification, bool, error) {
	return notification.As[WarningNotification](NotificationMethodWarning)
}

// DecodeWindowsWorldWritableWarningNotification decodes the windows/worldWritableWarning notification.
func DecodeWindowsWorldWritableWarningNotification(notification Notification) (WindowsWorldWritableWarningNotification, bool, error) {
	return notification.As[WindowsWorldWritableWarningNotification](NotificationMethodWindowsWorldWritableWarning)
}

// DecodeWindowsSandboxSetupCompletedNotification decodes the windowsSandbox/setupCompleted notification.
func DecodeWindowsSandboxSetupCompletedNotification(notification Notification) (WindowsSandboxSetupCompletedNotification, bool, error) {
	return notification.As[WindowsSandboxSetupCompletedNotification](NotificationMethodWindowsSandboxSetupCompleted)
}

// DecodeAgentMessageDeltaNotification decodes the item/agentMessage/delta notification.
//
// Deprecated: use DecodeItemAgentMessageDeltaNotification.
func DecodeAgentMessageDeltaNotification(notification Notification) (AgentMessageDeltaNotification, bool, error) {
	return DecodeItemAgentMessageDeltaNotification(notification)
}

// AccountLoginCompleted decodes the account/login/completed notification.
func (notification Notification) AccountLoginCompleted() (AccountLoginCompletedNotification, bool, error) {
	return DecodeAccountLoginCompletedNotification(notification)
}

// AccountRateLimitsUpdated decodes the account/rateLimits/updated notification.
func (notification Notification) AccountRateLimitsUpdated() (AccountRateLimitsUpdatedNotification, bool, error) {
	return DecodeAccountRateLimitsUpdatedNotification(notification)
}

// AccountUpdated decodes the account/updated notification.
func (notification Notification) AccountUpdated() (AccountUpdatedNotification, bool, error) {
	return DecodeAccountUpdatedNotification(notification)
}

// AppListUpdated decodes the app/list/updated notification.
func (notification Notification) AppListUpdated() (AppListUpdatedNotification, bool, error) {
	return DecodeAppListUpdatedNotification(notification)
}

// CommandExecOutputDelta decodes the command/exec/outputDelta notification.
func (notification Notification) CommandExecOutputDelta() (CommandExecOutputDeltaNotification, bool, error) {
	return DecodeCommandExecOutputDeltaNotification(notification)
}

// ConfigWarning decodes the configWarning notification.
func (notification Notification) ConfigWarning() (ConfigWarningNotification, bool, error) {
	return DecodeConfigWarningNotification(notification)
}

// DeprecationNotice decodes the deprecationNotice notification.
func (notification Notification) DeprecationNotice() (DeprecationNoticeNotification, bool, error) {
	return DecodeDeprecationNoticeNotification(notification)
}

// ErrorNotification decodes the error notification.
func (notification Notification) ErrorNotification() (ErrorNotification, bool, error) {
	return DecodeErrorNotification(notification)
}

// ExternalAgentConfigImportCompleted decodes the externalAgentConfig/import/completed notification.
func (notification Notification) ExternalAgentConfigImportCompleted() (ExternalAgentConfigImportCompletedNotification, bool, error) {
	return DecodeExternalAgentConfigImportCompletedNotification(notification)
}

// FsChanged decodes the fs/changed notification.
func (notification Notification) FsChanged() (FsChangedNotification, bool, error) {
	return DecodeFsChangedNotification(notification)
}

// FuzzyFileSearchSessionCompleted decodes the fuzzyFileSearch/sessionCompleted notification.
func (notification Notification) FuzzyFileSearchSessionCompleted() (FuzzyFileSearchSessionCompletedNotification, bool, error) {
	return DecodeFuzzyFileSearchSessionCompletedNotification(notification)
}

// FuzzyFileSearchSessionUpdated decodes the fuzzyFileSearch/sessionUpdated notification.
func (notification Notification) FuzzyFileSearchSessionUpdated() (FuzzyFileSearchSessionUpdatedNotification, bool, error) {
	return DecodeFuzzyFileSearchSessionUpdatedNotification(notification)
}

// GuardianWarning decodes the guardianWarning notification.
func (notification Notification) GuardianWarning() (GuardianWarningNotification, bool, error) {
	return DecodeGuardianWarningNotification(notification)
}

// HookCompleted decodes the hook/completed notification.
func (notification Notification) HookCompleted() (HookCompletedNotification, bool, error) {
	return DecodeHookCompletedNotification(notification)
}

// HookStarted decodes the hook/started notification.
func (notification Notification) HookStarted() (HookStartedNotification, bool, error) {
	return DecodeHookStartedNotification(notification)
}

// ItemAgentMessageDelta decodes the item/agentMessage/delta notification.
func (notification Notification) ItemAgentMessageDelta() (AgentMessageDeltaNotification, bool, error) {
	return DecodeItemAgentMessageDeltaNotification(notification)
}

// ItemAutoApprovalReviewCompleted decodes the item/autoApprovalReview/completed notification.
func (notification Notification) ItemAutoApprovalReviewCompleted() (ItemGuardianApprovalReviewCompletedNotification, bool, error) {
	return DecodeItemAutoApprovalReviewCompletedNotification(notification)
}

// ItemAutoApprovalReviewStarted decodes the item/autoApprovalReview/started notification.
func (notification Notification) ItemAutoApprovalReviewStarted() (ItemGuardianApprovalReviewStartedNotification, bool, error) {
	return DecodeItemAutoApprovalReviewStartedNotification(notification)
}

// ItemCommandExecutionOutputDelta decodes the item/commandExecution/outputDelta notification.
func (notification Notification) ItemCommandExecutionOutputDelta() (ItemCommandExecutionOutputDeltaNotification, bool, error) {
	return DecodeItemCommandExecutionOutputDeltaNotification(notification)
}

// ItemCommandExecutionTerminalInteraction decodes the item/commandExecution/terminalInteraction notification.
func (notification Notification) ItemCommandExecutionTerminalInteraction() (ItemCommandExecutionTerminalInteractionNotification, bool, error) {
	return DecodeItemCommandExecutionTerminalInteractionNotification(notification)
}

// ItemCompleted decodes the item/completed notification.
func (notification Notification) ItemCompleted() (ItemCompletedNotification, bool, error) {
	return DecodeItemCompletedNotification(notification)
}

// ItemFileChangeOutputDelta decodes the item/fileChange/outputDelta notification.
func (notification Notification) ItemFileChangeOutputDelta() (ItemFileChangeOutputDeltaNotification, bool, error) {
	return DecodeItemFileChangeOutputDeltaNotification(notification)
}

// ItemFileChangePatchUpdated decodes the item/fileChange/patchUpdated notification.
func (notification Notification) ItemFileChangePatchUpdated() (ItemFileChangePatchUpdatedNotification, bool, error) {
	return DecodeItemFileChangePatchUpdatedNotification(notification)
}

// ItemMCPToolCallProgress decodes the item/mcpToolCall/progress notification.
func (notification Notification) ItemMCPToolCallProgress() (MCPToolCallProgressNotification, bool, error) {
	return DecodeItemMCPToolCallProgressNotification(notification)
}

// ItemPlanDelta decodes the item/plan/delta notification.
func (notification Notification) ItemPlanDelta() (PlanDeltaNotification, bool, error) {
	return DecodeItemPlanDeltaNotification(notification)
}

// ItemReasoningSummaryPartAdded decodes the item/reasoning/summaryPartAdded notification.
func (notification Notification) ItemReasoningSummaryPartAdded() (ReasoningSummaryPartAddedNotification, bool, error) {
	return DecodeItemReasoningSummaryPartAddedNotification(notification)
}

// ItemReasoningSummaryTextDelta decodes the item/reasoning/summaryTextDelta notification.
func (notification Notification) ItemReasoningSummaryTextDelta() (ReasoningSummaryTextDeltaNotification, bool, error) {
	return DecodeItemReasoningSummaryTextDeltaNotification(notification)
}

// ItemReasoningTextDelta decodes the item/reasoning/textDelta notification.
func (notification Notification) ItemReasoningTextDelta() (ReasoningTextDeltaNotification, bool, error) {
	return DecodeItemReasoningTextDeltaNotification(notification)
}

// ItemStarted decodes the item/started notification.
func (notification Notification) ItemStarted() (ItemStartedNotification, bool, error) {
	return DecodeItemStartedNotification(notification)
}

// MCPServerOAuthLoginCompleted decodes the mcpServer/oauthLogin/completed notification.
func (notification Notification) MCPServerOAuthLoginCompleted() (MCPServerOAuthLoginCompletedNotification, bool, error) {
	return DecodeMCPServerOAuthLoginCompletedNotification(notification)
}

// MCPServerStartupStatusUpdated decodes the mcpServer/startupStatus/updated notification.
func (notification Notification) MCPServerStartupStatusUpdated() (MCPServerStartupStatusUpdatedNotification, bool, error) {
	return DecodeMCPServerStartupStatusUpdatedNotification(notification)
}

// ModelRerouted decodes the model/rerouted notification.
func (notification Notification) ModelRerouted() (ModelReroutedNotification, bool, error) {
	return DecodeModelReroutedNotification(notification)
}

// ModelVerification decodes the model/verification notification.
func (notification Notification) ModelVerification() (ModelVerificationNotification, bool, error) {
	return DecodeModelVerificationNotification(notification)
}

// ProcessExited decodes the process/exited notification.
func (notification Notification) ProcessExited() (ProcessExitedNotification, bool, error) {
	return DecodeProcessExitedNotification(notification)
}

// ProcessOutputDelta decodes the process/outputDelta notification.
func (notification Notification) ProcessOutputDelta() (ProcessOutputDeltaNotification, bool, error) {
	return DecodeProcessOutputDeltaNotification(notification)
}

// RemoteControlStatusChanged decodes the remoteControl/status/changed notification.
func (notification Notification) RemoteControlStatusChanged() (RemoteControlStatusChangedNotification, bool, error) {
	return DecodeRemoteControlStatusChangedNotification(notification)
}

// ServerRequestResolved decodes the serverRequest/resolved notification.
func (notification Notification) ServerRequestResolved() (ServerRequestResolvedNotification, bool, error) {
	return DecodeServerRequestResolvedNotification(notification)
}

// SkillsChanged decodes the skills/changed notification.
func (notification Notification) SkillsChanged() (SkillsChangedNotification, bool, error) {
	return DecodeSkillsChangedNotification(notification)
}

// ThreadArchived decodes the thread/archived notification.
func (notification Notification) ThreadArchived() (ThreadArchivedNotification, bool, error) {
	return DecodeThreadArchivedNotification(notification)
}

// ThreadClosed decodes the thread/closed notification.
func (notification Notification) ThreadClosed() (ThreadClosedNotification, bool, error) {
	return DecodeThreadClosedNotification(notification)
}

// ThreadCompacted decodes the thread/compacted notification.
func (notification Notification) ThreadCompacted() (ContextCompactedNotification, bool, error) {
	return DecodeThreadCompactedNotification(notification)
}

// ThreadGoalCleared decodes the thread/goal/cleared notification.
func (notification Notification) ThreadGoalCleared() (ThreadGoalClearedNotification, bool, error) {
	return DecodeThreadGoalClearedNotification(notification)
}

// ThreadGoalUpdated decodes the thread/goal/updated notification.
func (notification Notification) ThreadGoalUpdated() (ThreadGoalUpdatedNotification, bool, error) {
	return DecodeThreadGoalUpdatedNotification(notification)
}

// ThreadNameUpdated decodes the thread/name/updated notification.
func (notification Notification) ThreadNameUpdated() (ThreadNameUpdatedNotification, bool, error) {
	return DecodeThreadNameUpdatedNotification(notification)
}

// ThreadRealtimeClosed decodes the thread/realtime/closed notification.
func (notification Notification) ThreadRealtimeClosed() (ThreadRealtimeClosedNotification, bool, error) {
	return DecodeThreadRealtimeClosedNotification(notification)
}

// ThreadRealtimeError decodes the thread/realtime/error notification.
func (notification Notification) ThreadRealtimeError() (ThreadRealtimeErrorNotification, bool, error) {
	return DecodeThreadRealtimeErrorNotification(notification)
}

// ThreadRealtimeItemAdded decodes the thread/realtime/itemAdded notification.
func (notification Notification) ThreadRealtimeItemAdded() (ThreadRealtimeItemAddedNotification, bool, error) {
	return DecodeThreadRealtimeItemAddedNotification(notification)
}

// ThreadRealtimeOutputAudioDelta decodes the thread/realtime/outputAudio/delta notification.
func (notification Notification) ThreadRealtimeOutputAudioDelta() (ThreadRealtimeOutputAudioDeltaNotification, bool, error) {
	return DecodeThreadRealtimeOutputAudioDeltaNotification(notification)
}

// ThreadRealtimeSDP decodes the thread/realtime/sdp notification.
func (notification Notification) ThreadRealtimeSDP() (ThreadRealtimeSDPNotification, bool, error) {
	return DecodeThreadRealtimeSDPNotification(notification)
}

// ThreadRealtimeStarted decodes the thread/realtime/started notification.
func (notification Notification) ThreadRealtimeStarted() (ThreadRealtimeStartedNotification, bool, error) {
	return DecodeThreadRealtimeStartedNotification(notification)
}

// ThreadRealtimeTranscriptDelta decodes the thread/realtime/transcript/delta notification.
func (notification Notification) ThreadRealtimeTranscriptDelta() (ThreadRealtimeTranscriptDeltaNotification, bool, error) {
	return DecodeThreadRealtimeTranscriptDeltaNotification(notification)
}

// ThreadRealtimeTranscriptDone decodes the thread/realtime/transcript/done notification.
func (notification Notification) ThreadRealtimeTranscriptDone() (ThreadRealtimeTranscriptDoneNotification, bool, error) {
	return DecodeThreadRealtimeTranscriptDoneNotification(notification)
}

// ThreadStarted decodes the thread/started notification.
func (notification Notification) ThreadStarted() (ThreadStartedNotification, bool, error) {
	return DecodeThreadStartedNotification(notification)
}

// ThreadStatusChanged decodes the thread/status/changed notification.
func (notification Notification) ThreadStatusChanged() (ThreadStatusChangedNotification, bool, error) {
	return DecodeThreadStatusChangedNotification(notification)
}

// ThreadTokenUsageUpdated decodes the thread/tokenUsage/updated notification.
func (notification Notification) ThreadTokenUsageUpdated() (ThreadTokenUsageUpdatedNotification, bool, error) {
	return DecodeThreadTokenUsageUpdatedNotification(notification)
}

// ThreadUnarchived decodes the thread/unarchived notification.
func (notification Notification) ThreadUnarchived() (ThreadUnarchivedNotification, bool, error) {
	return DecodeThreadUnarchivedNotification(notification)
}

// TurnCompleted decodes the turn/completed notification.
func (notification Notification) TurnCompleted() (TurnCompletedNotification, bool, error) {
	return DecodeTurnCompletedNotification(notification)
}

// TurnDiffUpdated decodes the turn/diff/updated notification.
func (notification Notification) TurnDiffUpdated() (TurnDiffUpdatedNotification, bool, error) {
	return DecodeTurnDiffUpdatedNotification(notification)
}

// TurnPlanUpdated decodes the turn/plan/updated notification.
func (notification Notification) TurnPlanUpdated() (TurnPlanUpdatedNotification, bool, error) {
	return DecodeTurnPlanUpdatedNotification(notification)
}

// TurnStarted decodes the turn/started notification.
func (notification Notification) TurnStarted() (TurnStartedNotification, bool, error) {
	return DecodeTurnStartedNotification(notification)
}

// Warning decodes the warning notification.
func (notification Notification) Warning() (WarningNotification, bool, error) {
	return DecodeWarningNotification(notification)
}

// WindowsWorldWritableWarning decodes the windows/worldWritableWarning notification.
func (notification Notification) WindowsWorldWritableWarning() (WindowsWorldWritableWarningNotification, bool, error) {
	return DecodeWindowsWorldWritableWarningNotification(notification)
}

// WindowsSandboxSetupCompleted decodes the windowsSandbox/setupCompleted notification.
func (notification Notification) WindowsSandboxSetupCompleted() (WindowsSandboxSetupCompletedNotification, bool, error) {
	return DecodeWindowsSandboxSetupCompletedNotification(notification)
}

// AgentMessageDelta decodes the item/agentMessage/delta notification.
//
// Deprecated: use ItemAgentMessageDelta.
func (notification Notification) AgentMessageDelta() (AgentMessageDeltaNotification, bool, error) {
	return notification.ItemAgentMessageDelta()
}
