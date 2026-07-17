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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRemoteAttachAuth is AC7: attach to a running password-protected fake,
// perform the Health handshake and a session round-trip, and assert the
// basic-auth header (username "opencode") rode on EVERY request including
// the SSE dial.
func TestRemoteAttachAuth(t *testing.T) {
	t.Parallel()

	const password = "remote-secret-42"
	fake := newFakeOpencode()
	fake.password = password
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)

	oc, err := NewRemoteOpencode(t.Context(), &RemoteConfig{
		BaseURL:     server.URL,
		Password:    password,
		DialTimeout: 10 * time.Second,
		DrainWindow: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewRemoteOpencode: %v", err)
	}
	t.Cleanup(func() { _ = oc.Close() })

	session, err := oc.SessionStart(t.Context(), &SessionNewParams{Title: "remote"})
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	result, err := session.Run(t.Context(), "hi", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.FinalResponse == "" {
		t.Fatal("FinalResponse empty")
	}

	requests := fake.recordedRequests()
	if len(requests) == 0 {
		t.Fatal("no requests recorded")
	}
	var sawEventDial bool
	for _, req := range requests {
		if !req.HasAuth || req.Username != basicAuthUsername || req.Password != password {
			t.Errorf("%s %s: missing/wrong basic auth (hasAuth=%t user=%q)", req.Method, req.Path, req.HasAuth, req.Username)
		}
		if req.Path == "/event" {
			sawEventDial = true
		}
	}
	if !sawEventDial {
		t.Error("SSE dial not recorded — eager bus dial missing")
	}
}

// TestRemoteAttachWrongPassword: the handshake surfaces the 401 instead of
// attaching, and the error text never contains the password.
func TestRemoteAttachWrongPassword(t *testing.T) {
	t.Parallel()

	fake := newFakeOpencode()
	fake.password = "correct-password"
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)

	_, err := NewRemoteOpencode(t.Context(), &RemoteConfig{
		BaseURL:     server.URL,
		Password:    "wrong-password",
		DialTimeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatal("attach with wrong password must fail")
	}
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != http.StatusUnauthorized {
		t.Fatalf("err = %v (%T), want 401 APIError", err, err)
	}
	for _, secret := range []string{"correct-password", "wrong-password"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("password %q leaked into error: %q", secret, err.Error())
		}
	}
}

// TestRemoteBaseURLValidation covers the Insecure gating and URL hygiene
// rules (AC7/AC8).
func TestRemoteBaseURLValidation(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		config  RemoteConfig
		wantErr string
	}{
		"success: loopback plain http allowed": {
			config: RemoteConfig{BaseURL: "http://127.0.0.1:4096"},
		},
		"success: localhost plain http allowed": {
			config: RemoteConfig{BaseURL: "http://localhost:4096"},
		},
		"success: https to any host allowed": {
			config: RemoteConfig{BaseURL: "https://opencode.internal.example:8443"},
		},
		"success: non-loopback plain http with Insecure": {
			config: RemoteConfig{BaseURL: "http://203.0.113.5:4096", Insecure: true},
		},
		"error: non-loopback plain http rejected": {
			config:  RemoteConfig{BaseURL: "http://203.0.113.5:4096"},
			wantErr: "requires RemoteConfig.Insecure",
		},
		"error: userinfo in URL rejected": {
			config:  RemoteConfig{BaseURL: "http://opencode:hunter2@127.0.0.1:4096"},
			wantErr: "must not contain userinfo",
		},
		"error: unsupported scheme rejected": {
			config:  RemoteConfig{BaseURL: "ws://127.0.0.1:4096"},
			wantErr: "unsupported remote base URL scheme",
		},
		"error: empty URL rejected": {
			config:  RemoteConfig{},
			wantErr: "remote base URL is required",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client, err := NewRemoteClient(&tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("NewRemoteClient: %v", err)
				}
				_ = client.Close()
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "hunter2") {
				t.Fatalf("credential from URL leaked into error: %q", err.Error())
			}
		})
	}
}
