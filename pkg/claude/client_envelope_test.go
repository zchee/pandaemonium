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

package claude

import (
	"testing"

	"github.com/go-json-experiment/json"
)

// TestUserEnvelope verifies that userEnvelope produces the upstream stream-JSON
// user-turn wire shape sent on the CLI subprocess stdin after the initialize
// handshake (M3). The envelope must match the Python SDK client.py shape:
//
//	{"type":"user","session_id":"<id>","message":{"role":"user","content":"<prompt>"},"parent_tool_use_id":null}
//
// parent_tool_use_id must serialize as JSON null, and session_id must always be
// present (the empty string when no session is active).
func TestUserEnvelope(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sessionID string
		prompt    string
		want      string
	}{
		"success: empty session id": {
			sessionID: "",
			prompt:    "hi",
			want:      `{"type":"user","session_id":"","message":{"role":"user","content":"hi"},"parent_tool_use_id":null}`,
		},
		"success: session id is included when set": {
			sessionID: "sess-abc123",
			prompt:    "hello",
			want:      `{"type":"user","session_id":"sess-abc123","message":{"role":"user","content":"hello"},"parent_tool_use_id":null}`,
		},
		"success: prompt with special characters is escaped": {
			sessionID: "",
			prompt:    `quote " and newline ` + "\n",
			want:      `{"type":"user","session_id":"","message":{"role":"user","content":"quote \" and newline \n"},"parent_tool_use_id":null}`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := userEnvelope(tt.sessionID, tt.prompt)
			if err != nil {
				t.Fatalf("userEnvelope() error = %v", err)
			}

			// Byte-level verification. userEnvelope marshals a struct, so the
			// field order is fixed and must match the upstream wire shape
			// exactly (type, session_id, message, parent_tool_use_id).
			if string(got) != tt.want {
				t.Errorf("userEnvelope(%q, %q):\n got = %s\nwant = %s", tt.sessionID, tt.prompt, got, tt.want)
			}

			// Structural verification: decode and assert each field, including
			// that parent_tool_use_id decodes to a JSON null (nil pointer).
			var decoded struct {
				Type    string `json:"type"`
				Session string `json:"session_id"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				ParentToolUseID *string `json:"parent_tool_use_id"`
			}
			if err := json.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("Unmarshal(%s) error = %v", got, err)
			}
			if decoded.Type != "user" {
				t.Errorf("type = %q, want %q", decoded.Type, "user")
			}
			if decoded.Session != tt.sessionID {
				t.Errorf("session_id = %q, want %q", decoded.Session, tt.sessionID)
			}
			if decoded.Message.Role != "user" {
				t.Errorf("message.role = %q, want %q", decoded.Message.Role, "user")
			}
			if decoded.Message.Content != tt.prompt {
				t.Errorf("message.content = %q, want %q", decoded.Message.Content, tt.prompt)
			}
			if decoded.ParentToolUseID != nil {
				t.Errorf("parent_tool_use_id = %q, want JSON null", *decoded.ParentToolUseID)
			}
		})
	}
}
