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

package opencode

import (
	"strings"
	"testing"
	"time"
)

// TestPermissionPolicySyncPath is AC10: the fake pauses a sync prompt on
// permission.asked exactly like the probed real server; the client-lifetime
// consumer must reply (once/reject per PermissionAuto) so the blocking
// Session.Run completes in both modes without hanging.
func TestPermissionPolicySyncPath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		permissionAuto bool
		wantResponse   PermissionResponse
	}{
		"success: PermissionAuto replies once": {
			permissionAuto: true,
			wantResponse:   PermissionOnce,
		},
		"success: default policy replies reject without stalling the turn": {
			permissionAuto: false,
			wantResponse:   PermissionReject,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeOpencode()
			fake.permissionGate = true
			oc := startFakeOpencode(t, fake, func(cfg *RemoteConfig) {
				cfg.PermissionAuto = tt.permissionAuto
			})

			session, err := oc.SessionStart(t.Context(), nil)
			if err != nil {
				t.Fatalf("SessionStart: %v", err)
			}

			done := make(chan error, 1)
			go func() {
				_, err := session.Run(t.Context(), "run a gated tool", nil)
				done <- err
			}()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("sync Run with permission gate: %v", err)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("sync Run hung on permission.asked — the client-lifetime consumer did not reply")
			}

			replies := fake.permissionRepliesSeen()
			if len(replies) != 1 {
				t.Fatalf("permission replies = %d, want 1 (%+v)", len(replies), replies)
			}
			reply := replies[0]
			if reply.Response != tt.wantResponse {
				t.Errorf("reply = %q, want %q", reply.Response, tt.wantResponse)
			}
			if reply.SessionID != session.ID() {
				t.Errorf("reply session = %q, want %q", reply.SessionID, session.ID())
			}
			if !strings.HasPrefix(reply.PermissionID, "per_") {
				t.Errorf("permission id = %q, want per_ prefix", reply.PermissionID)
			}
			if reply.V2 {
				t.Error("legacy permission.asked must be answered on the legacy endpoint")
			}

			counters := oc.Client().Counters()
			if tt.permissionAuto && counters.PermissionsAutoApproved != 1 {
				t.Errorf("PermissionsAutoApproved = %d, want 1", counters.PermissionsAutoApproved)
			}
			if !tt.permissionAuto && counters.PermissionsRejected != 1 {
				t.Errorf("PermissionsRejected = %d, want 1", counters.PermissionsRejected)
			}
		})
	}
}

// TestPermissionPolicyAsyncPath: the same consumer serves async turns — a
// gated tool call inside Session.Turn resolves without the caller touching
// permissions.
func TestPermissionPolicyAsyncPath(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	fake.permissionGate = true
	oc := startFakeOpencode(t, fake, func(cfg *RemoteConfig) { cfg.PermissionAuto = true })

	session, err := oc.SessionStart(t.Context(), nil)
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	handle, err := session.Turn(t.Context(), "gated async work", nil)
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	result, err := handle.Run(t.Context())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.FinalResponse == "" {
		t.Error("FinalResponse empty")
	}
	if replies := fake.permissionRepliesSeen(); len(replies) != 1 || replies[0].Response != PermissionOnce {
		t.Errorf("replies = %+v, want one 'once'", replies)
	}
}

// TestRespondPermissionAfterClose: the permission consumer must refuse work
// on a closing client instead of racing the Close reap (WaitGroup
// Add-during-Wait window).
func TestRespondPermissionAfterClose(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, func(cfg *RemoteConfig) { cfg.PermissionAuto = true })
	client := oc.Client()
	if err := oc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Must be a silent no-op: no panic, no reply attempt.
	client.respondPermission(Event{
		Type:       EventTypePermissionAsked,
		Properties: []byte(`{"id":"per_after_close","sessionID":"ses_x"}`),
	})
	time.Sleep(50 * time.Millisecond)
	if replies := fake.permissionRepliesSeen(); len(replies) != 0 {
		t.Fatalf("permission replies after Close = %+v, want none", replies)
	}
}

// TestPermissionV2Reply: a permission.v2.asked event is answered on the v2
// reply endpoint with the same policy.
func TestPermissionV2Reply(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	oc := startFakeOpencode(t, fake, func(cfg *RemoteConfig) { cfg.PermissionAuto = true })

	session, err := oc.SessionStart(t.Context(), nil)
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	fake.emit(fakeEvent(EventTypePermissionV2Asked, map[string]any{
		"id":        "per_v2fake",
		"sessionID": session.ID(),
		"action":    "bash",
		"resources": []string{"echo hi"},
	}))

	deadline := time.Now().Add(10 * time.Second)
	for {
		replies := fake.permissionRepliesSeen()
		if len(replies) == 1 {
			if !replies[0].V2 || replies[0].PermissionID != "per_v2fake" || replies[0].Response != PermissionOnce {
				t.Fatalf("v2 reply = %+v", replies[0])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("v2 permission never answered")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
