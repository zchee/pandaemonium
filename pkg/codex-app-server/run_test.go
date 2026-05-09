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
	"testing"

	json "github.com/go-json-experiment/json"
	gocmp "github.com/google/go-cmp/cmp"

	"github.com/zchee/omxx/pkg/codex-app-server/protocol"
)

func TestDecodeThreadItemRoundTripPreservesNestedSlices(t *testing.T) {
	t.Parallel()

	original := []protocol.ThreadItem{
		protocol.RawThreadItem(`{"type":"agentMessage","text":"alpha","phase":"draft"}`),
		protocol.RawThreadItem(`{"type":"agent_message","text":"beta","phase":"finalAnswer"}`),
		protocol.RawThreadItem(`{"type":"unknown","text":"ignored","nested":[{"type":"agentMessage","text":"nested"}]}`),
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded []protocol.RawThreadItem
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	got := make([]protocol.ThreadItem, len(decoded))
	for i := range decoded {
		got[i] = decoded[i]
	}
	if diff := gocmp.Diff(original, got); diff != "" {
		t.Fatalf("round-trip mismatch (-want +got):\n%s", diff)
	}

	if item, ok := decodeThreadItem(got[2]); !ok {
		t.Fatalf("decodeThreadItem() ok = false, want true for syntactically valid payload")
	} else if item.agentMessage() {
		t.Fatalf("decodeThreadItem() accepted unknown discriminator as agent message: %#v", item)
	}
}

func TestFinalAssistantResponseIgnoresUnknownDiscriminatorAndFallsBack(t *testing.T) {
	t.Parallel()

	items := []protocol.ThreadItem{
		protocol.RawThreadItem(`{"type":"unknown","text":"ignored"}`),
		protocol.RawThreadItem(`{"type":"agentMessage","text":"answer one"}`),
		protocol.RawThreadItem(`{"type":"agent_message","text":"answer two","phase":"final_answer"}`),
	}

	if got := finalAssistantResponse(items); got != "answer two" {
		t.Fatalf("finalAssistantResponse() = %q, want %q", got, "answer two")
	}
}

func TestDecodeThreadItemRejectsMalformedPayload(t *testing.T) {
	t.Parallel()

	if _, ok := decodeThreadItem(protocol.RawThreadItem(`{"type":"agentMessage","text":"missing brace"`)); ok {
		t.Fatal("decodeThreadItem() ok = true, want false for malformed payload")
	}
}
