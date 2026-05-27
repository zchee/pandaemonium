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

package tmux

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// NotificationKind is the leading `%name` token for a tmux notification.
type NotificationKind string

const (
	// NotificationOutput is a `%output` pane-output notification.
	NotificationOutput NotificationKind = "%output"
	// NotificationExtendedOutput is a `%extended-output` flow-control output notification.
	NotificationExtendedOutput NotificationKind = "%extended-output"
	// NotificationSubscriptionChanged is a `%subscription-changed` notification.
	NotificationSubscriptionChanged NotificationKind = "%subscription-changed"
	// NotificationExit is a `%exit` control-client-exit notification.
	NotificationExit NotificationKind = "%exit"
	// NotificationPause is a `%pause` flow-control notification.
	NotificationPause NotificationKind = "%pause"
	// NotificationContinue is a `%continue` flow-control notification.
	NotificationContinue NotificationKind = "%continue"
	// NotificationMessage is a `%message` tmux message notification.
	NotificationMessage NotificationKind = "%message"
	// NotificationPaneModeChanged is a `%pane-mode-changed` notification.
	NotificationPaneModeChanged NotificationKind = "%pane-mode-changed"
	// NotificationWindowPaneChanged is a `%window-pane-changed` notification.
	NotificationWindowPaneChanged NotificationKind = "%window-pane-changed"
	// NotificationWindowClose is a `%window-close` notification.
	NotificationWindowClose NotificationKind = "%window-close"
	// NotificationUnlinkedWindowClose is a `%unlinked-window-close` notification.
	NotificationUnlinkedWindowClose NotificationKind = "%unlinked-window-close"
	// NotificationWindowAdd is a `%window-add` notification.
	NotificationWindowAdd NotificationKind = "%window-add"
	// NotificationUnlinkedWindowAdd is a `%unlinked-window-add` notification.
	NotificationUnlinkedWindowAdd NotificationKind = "%unlinked-window-add"
	// NotificationWindowRenamed is a `%window-renamed` notification.
	NotificationWindowRenamed NotificationKind = "%window-renamed"
	// NotificationUnlinkedWindowRenamed is a `%unlinked-window-renamed` notification.
	NotificationUnlinkedWindowRenamed NotificationKind = "%unlinked-window-renamed"
	// NotificationSessionChanged is a `%session-changed` notification.
	NotificationSessionChanged NotificationKind = "%session-changed"
	// NotificationClientSessionChanged is a `%client-session-changed` notification.
	NotificationClientSessionChanged NotificationKind = "%client-session-changed"
	// NotificationSessionRenamed is a `%session-renamed` notification.
	NotificationSessionRenamed NotificationKind = "%session-renamed"
	// NotificationSessionsChanged is a `%sessions-changed` notification.
	NotificationSessionsChanged NotificationKind = "%sessions-changed"
	// NotificationSessionWindowChanged is a `%session-window-changed` notification.
	NotificationSessionWindowChanged NotificationKind = "%session-window-changed"
)

// Notification is one raw asynchronous tmux control-mode notification.
type Notification struct {
	// Kind is the notification token, such as `%output`.
	Kind NotificationKind
	// Raw is the complete notification line without its line ending.
	Raw string
	// Args are whitespace-split generic arguments for forward-compatible callers.
	Args []string
}

// ParseNotification parses a `%`-prefixed control-mode notification line.
func ParseNotification(line string) (Notification, error) {
	if !strings.HasPrefix(line, "%") {
		return Notification{}, &ProtocolError{Line: line, Err: fmt.Errorf("notification must start with %%")}
	}
	kind, rest, _ := strings.Cut(line, " ")
	if kind == "%" {
		return Notification{}, &ProtocolError{Line: line, Err: fmt.Errorf("notification kind is empty")}
	}
	var args []string
	if rest != "" {
		args = strings.Fields(rest)
	}
	return Notification{Kind: NotificationKind(kind), Raw: line, Args: args}, nil
}

// Output returns the typed form of a `%output` notification.
func (n Notification) Output() (OutputNotification, bool, error) {
	if n.Kind != NotificationOutput {
		return OutputNotification{}, false, nil
	}
	_, rest, ok := strings.Cut(n.Raw, " ")
	if !ok {
		return OutputNotification{}, true, fmt.Errorf("%%output missing pane id")
	}
	pane, value, ok := strings.Cut(rest, " ")
	if !ok {
		value = ""
	}
	if err := validatePaneID(PaneID(pane)); err != nil {
		return OutputNotification{}, true, err
	}
	return OutputNotification{Pane: PaneID(pane), Value: value}, true, nil
}

// ExtendedOutput returns the typed form of a `%extended-output` notification.
func (n Notification) ExtendedOutput() (ExtendedOutputNotification, bool, error) {
	if n.Kind != NotificationExtendedOutput {
		return ExtendedOutputNotification{}, false, nil
	}
	_, rest, ok := strings.Cut(n.Raw, " ")
	if !ok {
		return ExtendedOutputNotification{}, true, fmt.Errorf("%%extended-output missing fields")
	}
	fields, value, err := splitFieldsBeforeValue(rest)
	if err != nil {
		return ExtendedOutputNotification{}, true, err
	}
	if len(fields) < 2 {
		return ExtendedOutputNotification{}, true, fmt.Errorf("%%extended-output requires pane id and age")
	}
	pane := PaneID(fields[0])
	if err := validatePaneID(pane); err != nil {
		return ExtendedOutputNotification{}, true, err
	}
	ageMillis, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || ageMillis < 0 {
		return ExtendedOutputNotification{}, true, fmt.Errorf("invalid %%extended-output age %q", fields[1])
	}
	return ExtendedOutputNotification{
		Pane:            pane,
		Age:             time.Duration(ageMillis) * time.Millisecond,
		ExtensionFields: slices.Clone(fields[2:]),
		Value:           value,
	}, true, nil
}

// SubscriptionChanged returns the typed form of a `%subscription-changed` notification.
func (n Notification) SubscriptionChanged() (SubscriptionChangedNotification, bool, error) {
	if n.Kind != NotificationSubscriptionChanged {
		return SubscriptionChangedNotification{}, false, nil
	}
	_, rest, ok := strings.Cut(n.Raw, " ")
	if !ok {
		return SubscriptionChangedNotification{}, true, fmt.Errorf("%%subscription-changed missing fields")
	}
	fields, value, err := splitFieldsBeforeValue(rest)
	if err != nil {
		return SubscriptionChangedNotification{}, true, err
	}
	if len(fields) < 5 {
		return SubscriptionChangedNotification{}, true, fmt.Errorf("%%subscription-changed requires at least five fields before value")
	}
	session := SessionID(fields[1])
	if err := validateSessionID(session); err != nil {
		return SubscriptionChangedNotification{}, true, err
	}
	window := WindowID(fields[2])
	if err := validateWindowID(window); err != nil {
		return SubscriptionChangedNotification{}, true, err
	}
	pane := PaneID(fields[4])
	if err := validatePaneID(pane); err != nil {
		return SubscriptionChangedNotification{}, true, err
	}
	return SubscriptionChangedNotification{
		Name:            fields[0],
		Session:         session,
		Window:          window,
		WindowIndex:     fields[3],
		Pane:            pane,
		ExtensionFields: slices.Clone(fields[5:]),
		Value:           value,
	}, true, nil
}

// Exit returns the typed form of a `%exit` notification.
func (n Notification) Exit() (ExitNotification, bool) {
	if n.Kind != NotificationExit {
		return ExitNotification{}, false
	}
	_, reason, ok := strings.Cut(n.Raw, " ")
	if !ok {
		reason = ""
	}
	return ExitNotification{Reason: reason}, true
}

// Pause returns the pane ID from a `%pause` notification.
func (n Notification) Pause() (PaneID, bool, error) {
	return paneNotification(n, NotificationPause)
}

// Continue returns the pane ID from a `%continue` notification.
func (n Notification) Continue() (PaneID, bool, error) {
	return paneNotification(n, NotificationContinue)
}

// Message returns the tmux message from a `%message` notification.
func (n Notification) Message() (string, bool) {
	if n.Kind != NotificationMessage {
		return "", false
	}
	_, msg, ok := strings.Cut(n.Raw, " ")
	if !ok {
		return "", true
	}
	return msg, true
}

// OutputNotification is a typed `%output` notification.
type OutputNotification struct {
	// Pane is the tmux pane ID that produced the output.
	Pane PaneID
	// Value is the tmux octal-escaped output value.
	Value string
}

// Bytes decodes the output value to terminal bytes.
func (o OutputNotification) Bytes() ([]byte, error) { return DecodeOutputValue(o.Value) }

// Text returns the decoded output as UTF-8 text, failing on invalid UTF-8.
func (o OutputNotification) Text() (string, error) { return decodeOutputText(o.Value) }

// TextLossy returns decoded output with invalid UTF-8 replaced.
func (o OutputNotification) TextLossy() string { return decodeOutputTextLossy(o.Value) }

// ExtendedOutputNotification is a typed `%extended-output` notification.
type ExtendedOutputNotification struct {
	// Pane is the tmux pane ID that produced the output.
	Pane PaneID
	// Age is how long tmux buffered the pane output before delivery.
	Age time.Duration
	// ExtensionFields are currently reserved fields before the `:` separator.
	ExtensionFields []string
	// Value is the tmux octal-escaped output value.
	Value string
}

// Bytes decodes the extended output value to terminal bytes.
func (o ExtendedOutputNotification) Bytes() ([]byte, error) { return DecodeOutputValue(o.Value) }

// Text returns the decoded extended output as UTF-8 text, failing on invalid UTF-8.
func (o ExtendedOutputNotification) Text() (string, error) { return decodeOutputText(o.Value) }

// TextLossy returns decoded extended output with invalid UTF-8 replaced.
func (o ExtendedOutputNotification) TextLossy() string { return decodeOutputTextLossy(o.Value) }

// SubscriptionChangedNotification is a typed `%subscription-changed` notification.
type SubscriptionChangedNotification struct {
	// Name is the caller-provided subscription name.
	Name string
	// Session is the session ID reported by tmux.
	Session SessionID
	// Window is the window ID reported by tmux.
	Window WindowID
	// WindowIndex is the window index reported by tmux.
	WindowIndex string
	// Pane is the pane ID reported by tmux.
	Pane PaneID
	// ExtensionFields are currently reserved fields before the `:` separator.
	ExtensionFields []string
	// Value is the expanded format value.
	Value string
}

// ExitNotification is a typed `%exit` notification.
type ExitNotification struct {
	// Reason is the optional tmux exit reason.
	Reason string
}

func paneNotification(n Notification, kind NotificationKind) (PaneID, bool, error) {
	if n.Kind != kind {
		return "", false, nil
	}
	if len(n.Args) != 1 {
		return "", true, fmt.Errorf("%s requires one pane id", kind)
	}
	pane := PaneID(n.Args[0])
	if err := validatePaneID(pane); err != nil {
		return "", true, err
	}
	return pane, true, nil
}

func validateSessionID(session SessionID) error {
	value := string(session)
	if len(value) < 2 || value[0] != '$' {
		return fmt.Errorf("tmux: session ID %q must have $ prefix", value)
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return fmt.Errorf("tmux: session ID %q must contain decimal digits after $", value)
		}
	}
	return nil
}

func validateWindowID(window WindowID) error {
	value := string(window)
	if len(value) < 2 || value[0] != '@' {
		return fmt.Errorf("tmux: window ID %q must have @ prefix", value)
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return fmt.Errorf("tmux: window ID %q must contain decimal digits after @", value)
		}
	}
	return nil
}

func splitFieldsBeforeValue(s string) ([]string, string, error) {
	if s == "" {
		return nil, "", fmt.Errorf("missing fields")
	}
	fields := make([]string, 0, 8)
	for {
		if s == "" {
			return nil, "", fmt.Errorf("missing : value separator")
		}
		if s == ":" {
			return fields, "", nil
		}
		if after, ok := strings.CutPrefix(s, ": "); ok {
			return fields, after, nil
		}
		field, rest, ok := strings.Cut(s, " ")
		if !ok {
			return nil, "", fmt.Errorf("missing : value separator")
		}
		if field == ":" {
			return fields, rest, nil
		}
		fields = append(fields, field)
		s = strings.TrimLeft(rest, " ")
	}
}
