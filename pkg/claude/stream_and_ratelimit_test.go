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
	"strings"
	"testing"

	"github.com/go-json-experiment/json"
)

// ── Top-level events (StreamEvent, RateLimitEvent) ────────────────────────

var (
	_ Message = StreamEvent{}
	_ Message = RateLimitEvent{}
)

// TestRateLimitStatus_Literals pins the 3 wire literals (types.py:1181).
func TestRateLimitStatus_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		s    RateLimitStatus
		want string
	}{
		"allowed":         {RateLimitStatusAllowed, "allowed"},
		"allowed_warning": {RateLimitStatusAllowedWarning, "allowed_warning"},
		"rejected":        {RateLimitStatusRejected, "rejected"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.s) != tt.want {
				t.Errorf("status = %q, want %q", string(tt.s), tt.want)
			}
		})
	}
}

// TestRateLimitType_Literals pins the 5 wire literals (types.py:1182-1184).
func TestRateLimitType_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		s    RateLimitType
		want string
	}{
		"five_hour":        {RateLimitTypeFiveHour, "five_hour"},
		"seven_day":        {RateLimitTypeSevenDay, "seven_day"},
		"seven_day_opus":   {RateLimitTypeSevenDayOpus, "seven_day_opus"},
		"seven_day_sonnet": {RateLimitTypeSevenDaySonnet, "seven_day_sonnet"},
		"overage":          {RateLimitTypeOverage, "overage"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.s) != tt.want {
				t.Errorf("type = %q, want %q", string(tt.s), tt.want)
			}
		})
	}
}

// TestParseMessage_StreamEvent verifies dispatch from type=stream_event and
// that the inner Event payload survives as opaque jsontext.Value.
func TestParseMessage_StreamEvent(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"stream_event","uuid":"u_1","session_id":"s_1","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"}},"parent_tool_use_id":"tu_1"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	se, ok := msg.(StreamEvent)
	if !ok {
		t.Fatalf("type = %T, want StreamEvent", msg)
	}
	if se.UUID != "u_1" {
		t.Errorf("UUID = %q, want u_1", se.UUID)
	}
	if se.SessionID != "s_1" {
		t.Errorf("SessionID = %q, want s_1", se.SessionID)
	}
	if se.ParentToolUseID != "tu_1" {
		t.Errorf("ParentToolUseID = %q, want tu_1", se.ParentToolUseID)
	}
	// Event is kept opaque; just confirm it round-trips the inner shape.
	if !strings.Contains(string(se.Event), `"stop_reason":"end_turn"`) {
		t.Errorf("Event = %q, want to contain stop_reason", se.Event)
	}
}

// TestParseMessage_RateLimitEvent verifies dispatch from type=rate_limit_event
// and the full RateLimitInfo nested decode with all camelCase wire keys.
func TestParseMessage_RateLimitEvent(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning","resetsAt":1700000000,"rateLimitType":"five_hour","utilization":0.85,"overageStatus":"allowed","overageResetsAt":1700001000,"overageDisabledReason":""},"uuid":"u_42","session_id":"s_42"}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	rle, ok := msg.(RateLimitEvent)
	if !ok {
		t.Fatalf("type = %T, want RateLimitEvent", msg)
	}
	if rle.UUID != "u_42" {
		t.Errorf("UUID = %q, want u_42", rle.UUID)
	}
	if rle.SessionID != "s_42" {
		t.Errorf("SessionID = %q, want s_42", rle.SessionID)
	}
	info := rle.RateLimitInfo
	if info.Status != RateLimitStatusAllowedWarning {
		t.Errorf("Status = %q, want allowed_warning", info.Status)
	}
	if info.ResetsAt != 1700000000 {
		t.Errorf("ResetsAt = %d, want 1700000000 (wire key resetsAt; snake_case resets_at would not decode)", info.ResetsAt)
	}
	if info.RateLimitType != RateLimitTypeFiveHour {
		t.Errorf("RateLimitType = %q, want five_hour (wire key rateLimitType)", info.RateLimitType)
	}
	if info.Utilization != 0.85 {
		t.Errorf("Utilization = %f, want 0.85", info.Utilization)
	}
	if info.OverageStatus != RateLimitStatusAllowed {
		t.Errorf("OverageStatus = %q, want allowed (wire key overageStatus)", info.OverageStatus)
	}
	if info.OverageResetsAt != 1700001000 {
		t.Errorf("OverageResetsAt = %d, want 1700001000 (wire key overageResetsAt)", info.OverageResetsAt)
	}
}

// TestRateLimitInfo_JSONTagsParity is the wire-tag parity guard. A
// hand-marshal vs literal-map check pins all 5 camelCase wire-key renames
// (resetsAt, rateLimitType, overageStatus, overageResetsAt,
// overageDisabledReason).
func TestRateLimitInfo_JSONTagsParity(t *testing.T) {
	t.Parallel()
	in := RateLimitInfo{
		Status:                RateLimitStatusRejected,
		ResetsAt:              1700000000,
		RateLimitType:         RateLimitTypeSevenDay,
		Utilization:           1.0,
		OverageStatus:         RateLimitStatusAllowed,
		OverageResetsAt:       1700001000,
		OverageDisabledReason: "free tier",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, want := range []string{
		`"resetsAt":1700000000`,
		`"rateLimitType":"seven_day"`,
		`"overageStatus":"allowed"`,
		`"overageResetsAt":1700001000`,
		`"overageDisabledReason":"free tier"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("marshal output missing %q (wire-tag parity broken):\n%s", want, s)
		}
	}
	// Guard against accidental snake_case leakage.
	for _, banned := range []string{"resets_at", "rate_limit_type", "overage_status"} {
		if strings.Contains(s, banned) {
			t.Errorf("marshal output contains snake_case %q, want camelCase wire keys:\n%s", banned, s)
		}
	}
}

// TestParseMessage_UnknownTopLevelTypeStillRaw is the forward-compat
// regression guard: an unknown top-level "type" must STILL fall through to
// rawMessage, so the new stream_event / rate_limit_event dispatches didn't
// shrink the envelope.
func TestParseMessage_UnknownTopLevelTypeStillRaw(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"future_message_kind","payload":{"x":1}}` + "\n")
	msg, err := parseMessage(line)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if _, ok := msg.(rawMessage); !ok {
		t.Errorf("future_message_kind dispatched to %T, want rawMessage", msg)
	}
}
