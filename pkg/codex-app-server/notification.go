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
	"fmt"
	"sort"

	"github.com/go-json-experiment/json"

	"github.com/zchee/pandaemonium/pkg/codex-app-server/protocol"
)

const (
	NotificationMethodAccountLoginCompleted    = "account/login/completed"
	NotificationMethodAccountRateLimitsUpdated = "account/rateLimits/updated"
	NotificationMethodAccountUpdated           = "account/updated"
	NotificationMethodAppListUpdated           = "app/list/updated"
	NotificationMethodCommandExecOutputDelta   = "command/exec/outputDelta"
	NotificationMethodConfigWarning            = "configWarning"
	NotificationMethodDeprecationNotice        = "deprecationNotice"
	// NotificationMethodItemAgentMessageDelta is emitted as assistant text streams.
	NotificationMethodItemAgentMessageDelta = "item/agentMessage/delta"
	// NotificationMethodError is emitted when a turn reports a recoverable or final error.
	NotificationMethodError                                   = "error"
	NotificationMethodExternalAgentConfigImportCompleted      = "externalAgentConfig/import/completed"
	NotificationMethodFsChanged                               = "fs/changed"
	NotificationMethodFuzzyFileSearchSessionCompleted         = "fuzzyFileSearch/sessionCompleted"
	NotificationMethodFuzzyFileSearchSessionUpdated           = "fuzzyFileSearch/sessionUpdated"
	NotificationMethodGuardianWarning                         = "guardianWarning"
	NotificationMethodHookCompleted                           = "hook/completed"
	NotificationMethodHookStarted                             = "hook/started"
	NotificationMethodItemAutoApprovalReviewCompleted         = "item/autoApprovalReview/completed"
	NotificationMethodItemAutoApprovalReviewStarted           = "item/autoApprovalReview/started"
	NotificationMethodItemCommandExecutionOutputDelta         = "item/commandExecution/outputDelta"
	NotificationMethodItemCommandExecutionTerminalInteraction = "item/commandExecution/terminalInteraction"
	// NotificationMethodItemCompleted is emitted when a thread item is complete.
	NotificationMethodItemCompleted                 = "item/completed"
	NotificationMethodItemFileChangeOutputDelta     = "item/fileChange/outputDelta"
	NotificationMethodItemFileChangePatchUpdated    = "item/fileChange/patchUpdated"
	NotificationMethodItemMCPToolCallProgress       = "item/mcpToolCall/progress"
	NotificationMethodItemPlanDelta                 = "item/plan/delta"
	NotificationMethodItemReasoningSummaryPartAdded = "item/reasoning/summaryPartAdded"
	NotificationMethodItemReasoningSummaryTextDelta = "item/reasoning/summaryTextDelta"
	NotificationMethodItemReasoningTextDelta        = "item/reasoning/textDelta"
	// NotificationMethodItemStarted is emitted when an item is created.
	NotificationMethodItemStarted                    = "item/started"
	NotificationMethodMCPServerOAuthLoginCompleted   = "mcpServer/oauthLogin/completed"
	NotificationMethodMCPServerStartupStatusUpdated  = "mcpServer/startupStatus/updated"
	NotificationMethodModelRerouted                  = "model/rerouted"
	NotificationMethodModelVerification              = "model/verification"
	NotificationMethodRemoteControlStatusChanged     = "remoteControl/status/changed"
	NotificationMethodServerRequestResolved          = "serverRequest/resolved"
	NotificationMethodSkillsChanged                  = "skills/changed"
	NotificationMethodThreadArchived                 = "thread/archived"
	NotificationMethodThreadClosed                   = "thread/closed"
	NotificationMethodThreadCompacted                = "thread/compacted"
	NotificationMethodThreadGoalCleared              = "thread/goal/cleared"
	NotificationMethodThreadGoalUpdated              = "thread/goal/updated"
	NotificationMethodThreadNameUpdated              = "thread/name/updated"
	NotificationMethodThreadRealtimeClosed           = "thread/realtime/closed"
	NotificationMethodThreadRealtimeError            = "thread/realtime/error"
	NotificationMethodThreadRealtimeItemAdded        = "thread/realtime/itemAdded"
	NotificationMethodThreadRealtimeOutputAudioDelta = "thread/realtime/outputAudio/delta"
	NotificationMethodThreadRealtimeSDP              = "thread/realtime/sdp"
	NotificationMethodThreadRealtimeStarted          = "thread/realtime/started"
	NotificationMethodThreadRealtimeTranscriptDelta  = "thread/realtime/transcript/delta"
	NotificationMethodThreadRealtimeTranscriptDone   = "thread/realtime/transcript/done"
	NotificationMethodThreadStarted                  = "thread/started"
	// NotificationMethodThreadStatusChanged indicates thread lifecycle state changes.
	NotificationMethodThreadStatusChanged = "thread/status/changed"
	// NotificationMethodThreadTokenUsageUpdated is emitted when turn token accounting changes.
	NotificationMethodThreadTokenUsageUpdated = "thread/tokenUsage/updated"
	NotificationMethodThreadUnarchived        = "thread/unarchived"
	// NotificationMethodTurnCompleted is emitted when a turn reaches a terminal state.
	NotificationMethodTurnCompleted                = "turn/completed"
	NotificationMethodTurnDiffUpdated              = "turn/diff/updated"
	NotificationMethodTurnPlanUpdated              = "turn/plan/updated"
	NotificationMethodTurnStarted                  = "turn/started"
	NotificationMethodWarning                      = "warning"
	NotificationMethodWindowsWorldWritableWarning  = "windows/worldWritableWarning"
	NotificationMethodWindowsSandboxSetupCompleted = "windowsSandbox/setupCompleted"

	// Deprecated aliases kept for compatibility.
	NotificationMethodAgentMessageDelta             = NotificationMethodItemAgentMessageDelta
	NotificationMethodThreadTokenUsageUpdatedLegacy = NotificationMethodThreadTokenUsageUpdated
)

var notificationDecoders = map[string]func(Notification) (any, bool, error){
	NotificationMethodAccountLoginCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.AccountLoginCompletedNotification](notification, NotificationMethodAccountLoginCompleted)
	},
	NotificationMethodAccountRateLimitsUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.AccountRateLimitsUpdatedNotification](notification, NotificationMethodAccountRateLimitsUpdated)
	},
	NotificationMethodAccountUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.AccountUpdatedNotification](notification, NotificationMethodAccountUpdated)
	},
	NotificationMethodAppListUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.AppListUpdatedNotification](notification, NotificationMethodAppListUpdated)
	},
	NotificationMethodCommandExecOutputDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.CommandExecOutputDeltaNotification](notification, NotificationMethodCommandExecOutputDelta)
	},
	NotificationMethodConfigWarning: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ConfigWarningNotification](notification, NotificationMethodConfigWarning)
	},
	NotificationMethodDeprecationNotice: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.DeprecationNoticeNotification](notification, NotificationMethodDeprecationNotice)
	},
	NotificationMethodItemAgentMessageDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.AgentMessageDeltaNotification](notification, NotificationMethodItemAgentMessageDelta)
	},
	NotificationMethodError: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ErrorNotification](notification, NotificationMethodError)
	},
	NotificationMethodExternalAgentConfigImportCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ExternalAgentConfigImportCompletedNotification](notification, NotificationMethodExternalAgentConfigImportCompleted)
	},
	NotificationMethodFsChanged: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.FsChangedNotification](notification, NotificationMethodFsChanged)
	},
	NotificationMethodFuzzyFileSearchSessionCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.FuzzyFileSearchSessionCompletedNotification](notification, NotificationMethodFuzzyFileSearchSessionCompleted)
	},
	NotificationMethodFuzzyFileSearchSessionUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.FuzzyFileSearchSessionUpdatedNotification](notification, NotificationMethodFuzzyFileSearchSessionUpdated)
	},
	NotificationMethodGuardianWarning: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.GuardianWarningNotification](notification, NotificationMethodGuardianWarning)
	},
	NotificationMethodHookCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.HookCompletedNotification](notification, NotificationMethodHookCompleted)
	},
	NotificationMethodHookStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.HookStartedNotification](notification, NotificationMethodHookStarted)
	},
	NotificationMethodItemAutoApprovalReviewCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ItemGuardianApprovalReviewCompletedNotification](notification, NotificationMethodItemAutoApprovalReviewCompleted)
	},
	NotificationMethodItemAutoApprovalReviewStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ItemGuardianApprovalReviewStartedNotification](notification, NotificationMethodItemAutoApprovalReviewStarted)
	},
	NotificationMethodItemCommandExecutionOutputDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ItemCommandExecutionOutputDeltaNotification](notification, NotificationMethodItemCommandExecutionOutputDelta)
	},
	NotificationMethodItemCommandExecutionTerminalInteraction: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ItemCommandExecutionTerminalInteractionNotification](notification, NotificationMethodItemCommandExecutionTerminalInteraction)
	},
	NotificationMethodItemCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ItemCompletedNotification](notification, NotificationMethodItemCompleted)
	},
	NotificationMethodItemFileChangeOutputDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ItemFileChangeOutputDeltaNotification](notification, NotificationMethodItemFileChangeOutputDelta)
	},
	NotificationMethodItemFileChangePatchUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ItemFileChangePatchUpdatedNotification](notification, NotificationMethodItemFileChangePatchUpdated)
	},
	NotificationMethodItemMCPToolCallProgress: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.MCPToolCallProgressNotification](notification, NotificationMethodItemMCPToolCallProgress)
	},
	NotificationMethodItemPlanDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.PlanDeltaNotification](notification, NotificationMethodItemPlanDelta)
	},
	NotificationMethodItemReasoningSummaryPartAdded: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ReasoningSummaryPartAddedNotification](notification, NotificationMethodItemReasoningSummaryPartAdded)
	},
	NotificationMethodItemReasoningSummaryTextDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ReasoningSummaryTextDeltaNotification](notification, NotificationMethodItemReasoningSummaryTextDelta)
	},
	NotificationMethodItemReasoningTextDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ReasoningTextDeltaNotification](notification, NotificationMethodItemReasoningTextDelta)
	},
	NotificationMethodItemStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ItemStartedNotification](notification, NotificationMethodItemStarted)
	},
	NotificationMethodMCPServerOAuthLoginCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.MCPServerOAuthLoginCompletedNotification](notification, NotificationMethodMCPServerOAuthLoginCompleted)
	},
	NotificationMethodMCPServerStartupStatusUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.MCPServerStartupStatusUpdatedNotification](notification, NotificationMethodMCPServerStartupStatusUpdated)
	},
	NotificationMethodModelRerouted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ModelReroutedNotification](notification, NotificationMethodModelRerouted)
	},
	NotificationMethodModelVerification: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ModelVerificationNotification](notification, NotificationMethodModelVerification)
	},
	NotificationMethodRemoteControlStatusChanged: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.RemoteControlStatusChangedNotification](notification, NotificationMethodRemoteControlStatusChanged)
	},
	NotificationMethodServerRequestResolved: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ServerRequestResolvedNotification](notification, NotificationMethodServerRequestResolved)
	},
	NotificationMethodSkillsChanged: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.SkillsChangedNotification](notification, NotificationMethodSkillsChanged)
	},
	NotificationMethodThreadArchived: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadArchivedNotification](notification, NotificationMethodThreadArchived)
	},
	NotificationMethodThreadClosed: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadClosedNotification](notification, NotificationMethodThreadClosed)
	},
	NotificationMethodThreadCompacted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ContextCompactedNotification](notification, NotificationMethodThreadCompacted)
	},
	NotificationMethodThreadGoalCleared: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadGoalClearedNotification](notification, NotificationMethodThreadGoalCleared)
	},
	NotificationMethodThreadGoalUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadGoalUpdatedNotification](notification, NotificationMethodThreadGoalUpdated)
	},
	NotificationMethodThreadNameUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadNameUpdatedNotification](notification, NotificationMethodThreadNameUpdated)
	},
	NotificationMethodThreadRealtimeClosed: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadRealtimeClosedNotification](notification, NotificationMethodThreadRealtimeClosed)
	},
	NotificationMethodThreadRealtimeError: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadRealtimeErrorNotification](notification, NotificationMethodThreadRealtimeError)
	},
	NotificationMethodThreadRealtimeItemAdded: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadRealtimeItemAddedNotification](notification, NotificationMethodThreadRealtimeItemAdded)
	},
	NotificationMethodThreadRealtimeOutputAudioDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadRealtimeOutputAudioDeltaNotification](notification, NotificationMethodThreadRealtimeOutputAudioDelta)
	},
	NotificationMethodThreadRealtimeSDP: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadRealtimeSDPNotification](notification, NotificationMethodThreadRealtimeSDP)
	},
	NotificationMethodThreadRealtimeStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadRealtimeStartedNotification](notification, NotificationMethodThreadRealtimeStarted)
	},
	NotificationMethodThreadRealtimeTranscriptDelta: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadRealtimeTranscriptDeltaNotification](notification, NotificationMethodThreadRealtimeTranscriptDelta)
	},
	NotificationMethodThreadRealtimeTranscriptDone: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadRealtimeTranscriptDoneNotification](notification, NotificationMethodThreadRealtimeTranscriptDone)
	},
	NotificationMethodThreadStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadStartedNotification](notification, NotificationMethodThreadStarted)
	},
	NotificationMethodThreadStatusChanged: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadStatusChangedNotification](notification, NotificationMethodThreadStatusChanged)
	},
	NotificationMethodThreadTokenUsageUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadTokenUsageUpdatedNotification](notification, NotificationMethodThreadTokenUsageUpdated)
	},
	NotificationMethodThreadUnarchived: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.ThreadUnarchivedNotification](notification, NotificationMethodThreadUnarchived)
	},
	NotificationMethodTurnCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.TurnCompletedNotification](notification, NotificationMethodTurnCompleted)
	},
	NotificationMethodTurnDiffUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.TurnDiffUpdatedNotification](notification, NotificationMethodTurnDiffUpdated)
	},
	NotificationMethodTurnPlanUpdated: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.TurnPlanUpdatedNotification](notification, NotificationMethodTurnPlanUpdated)
	},
	NotificationMethodTurnStarted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.TurnStartedNotification](notification, NotificationMethodTurnStarted)
	},
	NotificationMethodWarning: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.WarningNotification](notification, NotificationMethodWarning)
	},
	NotificationMethodWindowsWorldWritableWarning: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.WindowsWorldWritableWarningNotification](notification, NotificationMethodWindowsWorldWritableWarning)
	},
	NotificationMethodWindowsSandboxSetupCompleted: func(notification Notification) (any, bool, error) {
		return DecodeNotificationAs[protocol.WindowsSandboxSetupCompletedNotification](notification, NotificationMethodWindowsSandboxSetupCompleted)
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
	sort.Strings(methods)
	return methods
}

// DecodeNotificationAs decodes notification params when the method matches.
//
// The boolean return is false when notification.Method does not match method.
// In that case the zero value is returned and params are not decoded.
func DecodeNotificationAs[T any](notification Notification, method string) (T, bool, error) {
	var zero T
	if notification.Method != method {
		return zero, false, nil
	}
	if err := json.Unmarshal(notification.Params, &zero); err != nil {
		return zero, true, fmt.Errorf("decode %s notification: %w", method, err)
	}
	return zero, true, nil
}

// DecodeNotification decodes known server notifications from the upstream
// Python notification registry.
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

// DecodeAgentMessageDeltaNotification decodes an item/agentMessage/delta notification.
func DecodeAgentMessageDeltaNotification(notification Notification) (protocol.AgentMessageDeltaNotification, bool, error) {
	return DecodeNotificationAs[protocol.AgentMessageDeltaNotification](notification, NotificationMethodItemAgentMessageDelta)
}

// DecodeErrorNotification decodes an error notification.
func DecodeErrorNotification(notification Notification) (protocol.ErrorNotification, bool, error) {
	return DecodeNotificationAs[protocol.ErrorNotification](notification, NotificationMethodError)
}

// DecodeItemCompletedNotification decodes an item/completed notification.
func DecodeItemCompletedNotification(notification Notification) (protocol.ItemCompletedNotification, bool, error) {
	return DecodeNotificationAs[protocol.ItemCompletedNotification](notification, NotificationMethodItemCompleted)
}

// DecodeThreadTokenUsageUpdatedNotification decodes a thread/tokenUsage/updated notification.
func DecodeThreadTokenUsageUpdatedNotification(notification Notification) (protocol.ThreadTokenUsageUpdatedNotification, bool, error) {
	return DecodeNotificationAs[protocol.ThreadTokenUsageUpdatedNotification](notification, NotificationMethodThreadTokenUsageUpdated)
}

// DecodeTurnCompletedNotification decodes a turn/completed notification.
func DecodeTurnCompletedNotification(notification Notification) (protocol.TurnCompletedNotification, bool, error) {
	return DecodeNotificationAs[protocol.TurnCompletedNotification](notification, NotificationMethodTurnCompleted)
}

// AgentMessageDelta decodes an item/agentMessage/delta notification.
func (notification Notification) AgentMessageDelta() (protocol.AgentMessageDeltaNotification, bool, error) {
	return DecodeAgentMessageDeltaNotification(notification)
}

// ErrorNotification decodes an error notification.
func (notification Notification) ErrorNotification() (protocol.ErrorNotification, bool, error) {
	return DecodeErrorNotification(notification)
}

// ItemCompleted decodes an item/completed notification.
func (notification Notification) ItemCompleted() (protocol.ItemCompletedNotification, bool, error) {
	return DecodeItemCompletedNotification(notification)
}

// ThreadTokenUsageUpdated decodes a thread/tokenUsage/updated notification.
func (notification Notification) ThreadTokenUsageUpdated() (protocol.ThreadTokenUsageUpdatedNotification, bool, error) {
	return DecodeThreadTokenUsageUpdatedNotification(notification)
}

// TurnCompleted decodes a turn/completed notification.
func (notification Notification) TurnCompleted() (protocol.TurnCompletedNotification, bool, error) {
	return DecodeTurnCompletedNotification(notification)
}

var expectedNotificationMethods = []string{
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

func init() {
	sort.Strings(expectedNotificationMethods)
}
