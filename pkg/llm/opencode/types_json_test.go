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

	"github.com/go-json-experiment/json"
	gocmp "github.com/google/go-cmp/cmp"
)

// TestRequestBodiesOmitzero asserts that unset optional fields disappear from
// request JSON (the server rejects unknown/explicit-null members on several
// endpoints via additionalProperties: false).
func TestRequestBodiesOmitzero(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in     any
		want   string
		forbid []string
	}{
		"success: minimal prompt has only parts": {
			in:     &PromptParams{Parts: []PartInput{{Type: "text", Text: "hi"}}},
			want:   `{"parts":[{"type":"text","text":"hi"}]}`,
			forbid: []string{"messageID", "model", "agent", "noReply", "tools", "system", "variant"},
		},
		"success: full prompt keeps set fields": {
			in: &PromptParams{
				Model: &ModelRef{ProviderID: "p", ModelID: "m"},
				Agent: "build",
				Parts: []PartInput{{Type: "text", Text: "hi"}},
			},
			want: `{"model":{"providerID":"p","modelID":"m"},"agent":"build","parts":[{"type":"text","text":"hi"}]}`,
		},
		"success: empty session create is empty object": {
			in:     &SessionNewParams{},
			want:   `{}`,
			forbid: []string{"parentID", "title", "agent", "model", "permission"},
		},
		"success: fork without message id is empty object": {
			in:   &SessionForkParams{},
			want: `{}`,
		},
		"success: summarize requires provider and model": {
			in:     &SummarizeParams{ProviderID: "p", ModelID: "m"},
			want:   `{"providerID":"p","modelID":"m"}`,
			forbid: []string{"auto"},
		},
		"success: revert without part id omits it": {
			in:     &RevertParams{MessageID: "msg_1"},
			want:   `{"messageID":"msg_1"}`,
			forbid: []string{"partID"},
		},
		"success: file part keeps required members only": {
			in:     &PartInput{Type: "file", Mime: "text/plain", URL: "https://example.com/f.txt"},
			want:   `{"type":"file","mime":"text/plain","url":"https://example.com/f.txt"}`,
			forbid: []string{"text", "name", "filename", "id"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if got := string(encoded); got != tt.want {
				t.Errorf("encoded = %s, want %s", got, tt.want)
			}
			for _, member := range tt.forbid {
				if strings.Contains(string(encoded), `"`+member+`"`) {
					t.Errorf("unset member %q must be omitted: %s", member, encoded)
				}
			}
		})
	}
}

// TestResponseDecodingTolerant asserts unknown fields are ignored and known
// fields land, using real captured wire shapes (opencode 1.18.3).
func TestResponseDecodingTolerant(t *testing.T) {
	t.Parallel()

	t.Run("success: session with unknown fields", func(t *testing.T) {
		t.Parallel()

		payload := `{"id":"ses_1","slug":"s","projectID":"p","directory":"/d","title":"t","version":"1.18.3",
			"time":{"created":1,"updated":2,"someFutureField":3},
			"brandNewTopLevel":{"nested":true},
			"permission":[{"permission":"bash","pattern":"*","action":"ask"}]}`
		var info SessionInfo
		if err := json.Unmarshal([]byte(payload), &info); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		want := SessionInfo{
			ID: "ses_1", Slug: "s", ProjectID: "p", Directory: "/d", Title: "t", Version: "1.18.3",
			Time:       SessionTime{Created: 1, Updated: 2},
			Permission: []PermissionRule{{Permission: "bash", Pattern: "*", Action: "ask"}},
		}
		if diff := gocmp.Diff(want, info); diff != "" {
			t.Errorf("SessionInfo mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("success: prompt response round-trip", func(t *testing.T) {
		t.Parallel()

		// Shape captured from a real 1.18.3 sync prompt response.
		payload := `{"info":{"id":"msg_1","sessionID":"ses_1","role":"assistant","parentID":"msg_0",
			"modelID":"claude-sonnet-4.6","providerID":"github-copilot","mode":"build","agent":"build",
			"path":{"cwd":"/w","root":"/w"},"cost":0.0297,
			"tokens":{"total":93471,"input":3,"output":15,"reasoning":0,"cache":{"write":425,"read":93028}},
			"finish":"stop","time":{"created":1,"completed":2}},
			"parts":[{"id":"prt_1","sessionID":"ses_1","messageID":"msg_1","type":"step-start"},
			{"id":"prt_2","sessionID":"ses_1","messageID":"msg_1","type":"text","text":"probe-a"},
			{"id":"prt_3","sessionID":"ses_1","messageID":"msg_1","type":"step-finish","futureField":1}]}`
		var resp PromptResponse
		if err := json.Unmarshal([]byte(payload), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Info.Tokens.Cache.Read != 93028 || resp.Info.Finish != "stop" {
			t.Errorf("assistant message fields lost: %+v", resp.Info)
		}
		if got := finalTextResponse(resp.Parts); got != "probe-a" {
			t.Errorf("finalTextResponse = %q, want probe-a", got)
		}

		result := newRunResult(&resp)
		wantUsage := &TokenUsage{Total: 93471, Input: 3, Output: 15, CacheRead: 93028, CacheWrite: 425, Cost: 0.0297}
		if diff := gocmp.Diff(wantUsage, result.Usage); diff != "" {
			t.Errorf("usage mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("success: assistant error decodes", func(t *testing.T) {
		t.Parallel()

		payload := `{"id":"msg_1","sessionID":"ses_1","role":"assistant","parentID":"msg_0","modelID":"m",
			"providerID":"p","mode":"build","agent":"build","path":{"cwd":"/","root":"/"},"cost":0,
			"tokens":{"input":0,"output":0,"reasoning":0,"cache":{"read":0,"write":0}},
			"error":{"name":"MessageAbortedError","data":{"message":"stopped"}},"time":{"created":1}}`
		var message AssistantMessage
		if err := json.Unmarshal([]byte(payload), &message); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if message.Error == nil || message.Error.Name != "MessageAbortedError" || message.Error.Message() != "stopped" {
			t.Errorf("error field lost: %+v", message.Error)
		}
	})
}

// TestEventDecodeGoldens decodes real captured /event frames (opencode
// 1.18.3) including a type absent from the server's own OpenAPI document.
func TestEventDecodeGoldens(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		payload       string
		wantType      string
		wantSessionID string
		check         func(t *testing.T, ev Event)
	}{
		"success: server.connected": {
			payload:  `{"id":"evt_f6e8f086001pyujK","type":"server.connected","properties":{}}`,
			wantType: "server.connected",
		},
		"success: server.heartbeat is unknown to the OpenAPI union but decodes": {
			payload:  `{"id":"evt_f6e8a3502001Ezfd1HlHykTjr0","type":"server.heartbeat","properties":{}}`,
			wantType: "server.heartbeat",
		},
		"success: session.idle carries sessionID": {
			payload:       `{"id":"evt_1","type":"session.idle","properties":{"sessionID":"ses_x"}}`,
			wantType:      EventTypeSessionIdle,
			wantSessionID: "ses_x",
		},
		"success: session.error with both fields optional": {
			payload:  `{"id":"evt_1","type":"session.error","properties":{}}`,
			wantType: EventTypeSessionError,
			check: func(t *testing.T, ev Event) {
				props, ok := ev.SessionError()
				if !ok {
					t.Fatal("SessionError() not ok")
				}
				if props.SessionID != "" || props.Error != nil {
					t.Errorf("expected empty session error props, got %+v", props)
				}
			},
		},
		"success: message.part.delta (real frame)": {
			payload:       `{"id":"evt_f6e8aa632001GP8x0Y6oQlkXaZ","type":"message.part.delta","properties":{"sessionID":"ses_0917639f4ffeQnTOVR2m4LTBCm","messageID":"msg_f6e8a9e4300167V3kiDC67UyO7","partID":"prt_f6e8aa631001zhtma11jSJUg0n","field":"text","delta":"The"}}`,
			wantType:      EventTypeMessagePartDelta,
			wantSessionID: "ses_0917639f4ffeQnTOVR2m4LTBCm",
		},
		"success: message.part.updated (real frame)": {
			payload:       `{"id":"evt_f6e8b173b001bRIsJ2df53bVdB","type":"message.part.updated","properties":{"sessionID":"ses_09174ef79ffesppxPc37GsrnhU","part":{"type":"text","text":"Write out the numbers.","messageID":"msg_f6e8b12bd0019vU9oGphV4m3gS","sessionID":"ses_09174ef79ffesppxPc37GsrnhU","id":"prt_f6e8b12be001jCspOc1pdWVymr"},"time":1784266037051}}`,
			wantType:      EventTypeMessagePartUpdated,
			wantSessionID: "ses_09174ef79ffesppxPc37GsrnhU",
			check: func(t *testing.T, ev Event) {
				props, ok := ev.MessagePartUpdated()
				if !ok {
					t.Fatal("MessagePartUpdated() not ok")
				}
				if props.Part.Type != "text" || props.Part.ID != "prt_f6e8b12be001jCspOc1pdWVymr" {
					t.Errorf("part fields lost: %+v", props.Part)
				}
			},
		},
		"success: permission.asked (real frame)": {
			payload:       `{"id":"evt_f6e8a2266002tYBw7u3WDQVml6","type":"permission.asked","properties":{"id":"per_f6e8a2266001rk409hL4uxLFmj","sessionID":"ses_0917639f4ffeQnTOVR2m4LTBCm","permission":"bash","patterns":["echo probe-perm"],"metadata":{"command":"echo probe-perm"},"always":["echo *"],"tool":{"messageID":"msg_f6e8a1393001jFo9SvmfQ3oYYr","callID":"toolu_bdrk_01DgNcvuW8YsCRfM5aSQZLeQ"}}}`,
			wantType:      EventTypePermissionAsked,
			wantSessionID: "ses_0917639f4ffeQnTOVR2m4LTBCm",
			check: func(t *testing.T, ev Event) {
				props, ok := ev.PermissionAsked()
				if !ok {
					t.Fatal("PermissionAsked() not ok")
				}
				if props.ID != "per_f6e8a2266001rk409hL4uxLFmj" || props.Permission != "bash" {
					t.Errorf("permission fields lost: %+v", props)
				}
				if props.Tool == nil || props.Tool.CallID == "" {
					t.Errorf("tool reference lost: %+v", props.Tool)
				}
			},
		},
		"success: permission.v2.asked decodes through the same accessor": {
			payload:       `{"id":"evt_1","type":"permission.v2.asked","properties":{"id":"per_v2","sessionID":"ses_x","action":"bash","resources":["echo hi"]}}`,
			wantType:      EventTypePermissionV2Asked,
			wantSessionID: "ses_x",
			check: func(t *testing.T, ev Event) {
				props, ok := ev.PermissionAsked()
				if !ok {
					t.Fatal("PermissionAsked() not ok for v2")
				}
				if props.ID != "per_v2" || props.Action != "bash" {
					t.Errorf("v2 fields lost: %+v", props)
				}
			},
		},
		"success: unknown future type passes through raw": {
			payload:       `{"id":"evt_1","type":"totally.new.event","properties":{"sessionID":"ses_x","payload":{"deep":[1,2,3]}}}`,
			wantType:      "totally.new.event",
			wantSessionID: "ses_x",
			check: func(t *testing.T, ev Event) {
				if !strings.Contains(string(ev.Properties), `"deep":[1,2,3]`) {
					t.Errorf("raw properties not preserved: %s", ev.Properties)
				}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var ev Event
			if err := json.Unmarshal([]byte(tt.payload), &ev); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if ev.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ev.Type, tt.wantType)
			}
			if got := ev.SessionID(); got != tt.wantSessionID {
				t.Errorf("SessionID() = %q, want %q", got, tt.wantSessionID)
			}
			if tt.check != nil {
				tt.check(t, ev)
			}
		})
	}
}

func TestNormalizeInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   RunInput
		want    []PartInput
		wantErr bool
	}{
		"success: string shorthand becomes one text part": {
			input: "hello",
			want:  []PartInput{{Type: "text", Text: "hello"}},
		},
		"success: typed inputs mix in a slice": {
			input: []any{
				TextInput{Text: "look at this"},
				FileInput{Mime: "image/png", URL: "https://example.com/x.png"},
				AgentInput{Name: "reviewer"},
			},
			want: []PartInput{
				{Type: "text", Text: "look at this"},
				{Type: "file", Mime: "image/png", URL: "https://example.com/x.png"},
				{Type: "agent", Name: "reviewer"},
			},
		},
		"success: raw part input passes through": {
			input: PartInput{Type: "text", Text: "raw"},
			want:  []PartInput{{Type: "text", Text: "raw"}},
		},
		"error: file input without mime rejected": {
			input:   FileInput{URL: "https://example.com/x"},
			wantErr: true,
		},
		"error: agent input without name rejected": {
			input:   AgentInput{},
			wantErr: true,
		},
		"error: unsupported type rejected": {
			input:   42,
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %t", err, tt.wantErr)
			}
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parts mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
