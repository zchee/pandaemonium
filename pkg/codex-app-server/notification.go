// Copyright 2026 The omxx Authors.
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

	json "github.com/go-json-experiment/json"

	"github.com/zchee/omxx/pkg/codex-app-server/protocol"
)

const (
	// NotificationMethodAgentMessageDelta is emitted as assistant text streams.
	NotificationMethodAgentMessageDelta = "item/agentMessage/delta"
	// NotificationMethodError is emitted when a turn reports a recoverable or final error.
	NotificationMethodError = "error"
	// NotificationMethodItemCompleted is emitted when a thread item is complete.
	NotificationMethodItemCompleted = "item/completed"
	// NotificationMethodThreadTokenUsageUpdated is emitted when turn token accounting changes.
	NotificationMethodThreadTokenUsageUpdated = "thread/tokenUsage/updated"
	// NotificationMethodTurnCompleted is emitted when a turn reaches a terminal state.
	NotificationMethodTurnCompleted = "turn/completed"
)

// KnownNotification is a decoded high-value server notification.
//
// Raw always preserves the original method and params so callers can log,
// forward, or re-decode the notification without losing future protocol fields.
type KnownNotification struct {
	Method string
	Value  any
	Raw    Notification
}

// DecodeNotification decodes notification params when the method matches.
//
// The boolean return is false when notification.Method does not match method.
// In that case the zero value is returned and params are not decoded.
func DecodeNotification[T any](notification Notification, method string) (T, bool, error) {
	var zero T
	if notification.Method != method {
		return zero, false, nil
	}
	if err := json.Unmarshal(notification.Params, &zero); err != nil {
		return zero, true, fmt.Errorf("decode %s notification: %w", method, err)
	}
	return zero, true, nil
}

// DecodeAgentMessageDeltaNotification decodes an item/agentMessage/delta notification.
func DecodeAgentMessageDeltaNotification(notification Notification) (protocol.AgentMessageDeltaNotification, bool, error) {
	return DecodeNotification[protocol.AgentMessageDeltaNotification](notification, NotificationMethodAgentMessageDelta)
}

// DecodeErrorNotification decodes an error notification.
func DecodeErrorNotification(notification Notification) (protocol.ErrorNotification, bool, error) {
	return DecodeNotification[protocol.ErrorNotification](notification, NotificationMethodError)
}

// DecodeItemCompletedNotification decodes an item/completed notification.
func DecodeItemCompletedNotification(notification Notification) (protocol.ItemCompletedNotification, bool, error) {
	return DecodeNotification[protocol.ItemCompletedNotification](notification, NotificationMethodItemCompleted)
}

// DecodeThreadTokenUsageUpdatedNotification decodes a thread/tokenUsage/updated notification.
func DecodeThreadTokenUsageUpdatedNotification(notification Notification) (protocol.ThreadTokenUsageUpdatedNotification, bool, error) {
	return DecodeNotification[protocol.ThreadTokenUsageUpdatedNotification](notification, NotificationMethodThreadTokenUsageUpdated)
}

// DecodeTurnCompletedNotification decodes a turn/completed notification.
func DecodeTurnCompletedNotification(notification Notification) (protocol.TurnCompletedNotification, bool, error) {
	return DecodeNotification[protocol.TurnCompletedNotification](notification, NotificationMethodTurnCompleted)
}

// DecodeKnownNotification decodes the bounded set of high-value notifications
// that the Go SDK exposes ergonomically while preserving raw access for every
// unknown or future notification method.
func DecodeKnownNotification(notification Notification) (KnownNotification, bool, error) {
	switch notification.Method {
	case NotificationMethodAgentMessageDelta:
		value, _, err := DecodeAgentMessageDeltaNotification(notification)
		return KnownNotification{Method: notification.Method, Value: value, Raw: notification}, true, err
	case NotificationMethodError:
		value, _, err := DecodeErrorNotification(notification)
		return KnownNotification{Method: notification.Method, Value: value, Raw: notification}, true, err
	case NotificationMethodItemCompleted:
		value, _, err := DecodeItemCompletedNotification(notification)
		return KnownNotification{Method: notification.Method, Value: value, Raw: notification}, true, err
	case NotificationMethodThreadTokenUsageUpdated:
		value, _, err := DecodeThreadTokenUsageUpdatedNotification(notification)
		return KnownNotification{Method: notification.Method, Value: value, Raw: notification}, true, err
	case NotificationMethodTurnCompleted:
		value, _, err := DecodeTurnCompletedNotification(notification)
		return KnownNotification{Method: notification.Method, Value: value, Raw: notification}, true, err
	default:
		return KnownNotification{Raw: notification}, false, nil
	}
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
