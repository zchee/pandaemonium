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
	"strings"
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"
)

func TestParseNotification(t *testing.T) {
	t.Parallel()
	n, err := ParseNotification("%window-renamed @1 new name")
	if err != nil {
		t.Fatalf("ParseNotification() error = %v", err)
	}
	if n.Kind != "%window-renamed" || n.Raw != "%window-renamed @1 new name" {
		t.Fatalf("ParseNotification() = %#v", n)
	}
	if diff := gocmp.Diff([]string{"@1", "new", "name"}, n.Args); diff != "" {
		t.Fatalf("Args mismatch (-want +got):\n%s", diff)
	}
	if _, err := ParseNotification("window-renamed @1"); err == nil || !strings.Contains(err.Error(), "must start") {
		t.Fatalf("ParseNotification(non-control) error = %v, want must start", err)
	}
}

func TestNotificationTypedHelpers(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		line  string
		check func(*testing.T, Notification)
	}{
		"success: output": {
			line: `%output %1 hello\015\012`,
			check: func(t *testing.T, n Notification) {
				out, ok, err := n.Output()
				if err != nil || !ok {
					t.Fatalf("Output() = %#v, %v, %v", out, ok, err)
				}
				if out.Pane != "%1" || out.Value != `hello\015\012` {
					t.Fatalf("Output() = %#v", out)
				}
			},
		},
		"success: extended output with future fields": {
			line: `%extended-output %2 1234 future : data\012`,
			check: func(t *testing.T, n Notification) {
				out, ok, err := n.ExtendedOutput()
				if err != nil || !ok {
					t.Fatalf("ExtendedOutput() = %#v, %v, %v", out, ok, err)
				}
				if out.Pane != "%2" || out.Age != 1234*time.Millisecond || out.Value != `data\012` {
					t.Fatalf("ExtendedOutput() = %#v", out)
				}
				if diff := gocmp.Diff([]string{"future"}, out.ExtensionFields); diff != "" {
					t.Fatalf("ExtensionFields mismatch (-want +got):\n%s", diff)
				}
			},
		},
		"success: subscription changed with future fields": {
			line: `%subscription-changed sub $1 @2 3 %4 future : value with spaces`,
			check: func(t *testing.T, n Notification) {
				sub, ok, err := n.SubscriptionChanged()
				if err != nil || !ok {
					t.Fatalf("SubscriptionChanged() = %#v, %v, %v", sub, ok, err)
				}
				want := SubscriptionChangedNotification{Name: "sub", Session: "$1", Window: "@2", WindowIndex: "3", Pane: "%4", ExtensionFields: []string{"future"}, Value: "value with spaces"}
				if diff := gocmp.Diff(want, sub); diff != "" {
					t.Fatalf("SubscriptionChanged() mismatch (-want +got):\n%s", diff)
				}
			},
		},
		"success: exit": {
			line: `%exit detached`,
			check: func(t *testing.T, n Notification) {
				exit, ok := n.Exit()
				if !ok || exit.Reason != "detached" {
					t.Fatalf("Exit() = %#v, %v", exit, ok)
				}
			},
		},
		"success: pause continue and message": {
			line: `%message hello world`,
			check: func(t *testing.T, n Notification) {
				msg, ok := n.Message()
				if !ok || msg != "hello world" {
					t.Fatalf("Message() = %q, %v", msg, ok)
				}
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n, err := ParseNotification(tt.line)
			if err != nil {
				t.Fatalf("ParseNotification() error = %v", err)
			}
			tt.check(t, n)
		})
	}
}

func TestPaneNotifications(t *testing.T) {
	t.Parallel()
	pause, err := ParseNotification("%pause %1")
	if err != nil {
		t.Fatalf("ParseNotification() error = %v", err)
	}
	pane, ok, err := pause.Pause()
	if err != nil || !ok || pane != "%1" {
		t.Fatalf("Pause() = %q, %v, %v", pane, ok, err)
	}
	cont, err := ParseNotification("%continue bad")
	if err != nil {
		t.Fatalf("ParseNotification() error = %v", err)
	}
	if _, ok, err := cont.Continue(); !ok || err == nil || !strings.Contains(err.Error(), "pane ID") {
		t.Fatalf("Continue() ok=%v err=%v, want pane ID error", ok, err)
	}
}

func TestNotificationTypedHelperErrors(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		line    string
		call    func(Notification) error
		wantErr string
	}{
		"error: output invalid pane": {
			line:    `%output bad value`,
			call:    func(n Notification) error { _, _, err := n.Output(); return err },
			wantErr: "pane ID",
		},
		"error: extended output missing colon": {
			line:    `%extended-output %1 10 value`,
			call:    func(n Notification) error { _, _, err := n.ExtendedOutput(); return err },
			wantErr: "missing : value separator",
		},
		"error: subscription changed too few fields": {
			line:    `%subscription-changed name : value`,
			call:    func(n Notification) error { _, _, err := n.SubscriptionChanged(); return err },
			wantErr: "requires at least five fields",
		},
		"error: subscription changed invalid pane": {
			line:    `%subscription-changed name $1 @2 3 bad : value`,
			call:    func(n Notification) error { _, _, err := n.SubscriptionChanged(); return err },
			wantErr: "pane ID",
		},
		"error: subscription changed invalid session": {
			line:    `%subscription-changed name bad @2 3 %4 : value`,
			call:    func(n Notification) error { _, _, err := n.SubscriptionChanged(); return err },
			wantErr: "session ID",
		},
		"error: subscription changed invalid window": {
			line:    `%subscription-changed name $1 bad 3 %4 : value`,
			call:    func(n Notification) error { _, _, err := n.SubscriptionChanged(); return err },
			wantErr: "window ID",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n, err := ParseNotification(tt.line)
			if err != nil {
				t.Fatalf("ParseNotification() error = %v", err)
			}
			if err := tt.call(n); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("typed helper error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
