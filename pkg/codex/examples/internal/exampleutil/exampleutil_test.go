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

package exampleutil

import (
	"bytes"
	"os"
	"testing"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestServerLabel(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		metadata codex.InitializeResponse
		want     string
	}{
		"success: server metadata wins": {
			metadata: codex.InitializeResponse{
				UserAgent:  "ignored/0.0.0",
				ServerInfo: &codex.ServerInfo{Name: "codex", Version: "1.2.3"},
			},
			want: "codex 1.2.3",
		},
		"success: user agent fallback": {
			metadata: codex.InitializeResponse{UserAgent: "codex/1.2.3"},
			want:     "codex/1.2.3",
		},
		"success: unknown fallback": {
			metadata: codex.InitializeResponse{},
			want:     "unknown",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := ServerLabel(tt.metadata); got != tt.want {
				t.Fatalf("ServerLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAssistantTextFromItems(t *testing.T) {
	t.Parallel()

	items := []codex.ThreadItem{
		codex.RawThreadItem(`{"type":"agentMessage","text":"hello "}`),
		codex.RawThreadItem(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"world"}]}`),
		codex.RawThreadItem(`{"type":"message","role":"user","content":[{"type":"output_text","text":"ignored"}]}`),
	}
	if got, want := AssistantTextFromItems(items), "hello world"; got != want {
		t.Fatalf("AssistantTextFromItems() = %q, want %q", got, want)
	}
}

func TestFindTurnByID(t *testing.T) {
	t.Parallel()

	turns := []codex.Turn{{ID: "one"}, {ID: "two"}}
	if got := FindTurnByID(turns, "two"); got == nil || got.ID != "two" {
		t.Fatalf("FindTurnByID() = %#v, want turn two", got)
	}
	if got := FindTurnByID(turns, "missing"); got != nil {
		t.Fatalf("FindTurnByID() = %#v, want nil", got)
	}
}

func TestOutputSchema(t *testing.T) {
	t.Parallel()

	got := string(OutputSchema())
	for _, want := range []string{`"summary"`, `"actions"`, `"additionalProperties":false`} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("OutputSchema() = %s, missing %s", got, want)
		}
	}
}

func TestParseRolloutPlan(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    RolloutPlan
		wantErr string
	}{
		"success: valid rollout plan": {
			input: `{"summary":"ship gradually","actions":["enable canary","watch metrics"]}`,
			want: RolloutPlan{
				Summary: "ship gradually",
				Actions: []string{"enable canary", "watch metrics"},
			},
		},
		"success: empty actions are valid": {
			input: `{"summary":"","actions":[]}`,
			want:  RolloutPlan{Summary: "", Actions: []string{}},
		},
		"error: empty output": {
			input:   "  \n\t",
			wantErr: "structured output is empty",
		},
		"error: malformed JSON": {
			input:   `{"summary":`,
			wantErr: "decode structured output",
		},
		"error: missing summary": {
			input:   `{"actions":[]}`,
			wantErr: "structured output has 1 fields",
		},
		"error: missing actions": {
			input:   `{"summary":"ship"}`,
			wantErr: "structured output has 1 fields",
		},
		"error: extra property": {
			input:   `{"summary":"ship","actions":[],"risk":"low"}`,
			wantErr: "structured output has 3 fields",
		},
		"error: wrong actions type": {
			input:   `{"summary":"ship","actions":[1]}`,
			wantErr: "decode structured output actions",
		},
		"error: null actions": {
			input:   `{"summary":"ship","actions":null}`,
			wantErr: "structured output actions must be an array",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseRolloutPlan(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseRolloutPlan() error = nil, want %q", tt.wantErr)
				}
				if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErr)) {
					t.Fatalf("ParseRolloutPlan() error = %q, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRolloutPlan() error = %v", err)
			}
			if got.Summary != tt.want.Summary {
				t.Fatalf("ParseRolloutPlan().Summary = %q, want %q", got.Summary, tt.want.Summary)
			}
			if len(got.Actions) != len(tt.want.Actions) {
				t.Fatalf("ParseRolloutPlan().Actions len = %d, want %d (%#v)", len(got.Actions), len(tt.want.Actions), got.Actions)
			}
			for i := range got.Actions {
				if got.Actions[i] != tt.want.Actions[i] {
					t.Fatalf("ParseRolloutPlan().Actions[%d] = %q, want %q", i, got.Actions[i], tt.want.Actions[i])
				}
			}
		})
	}
}

func TestTemporarySampleImagePath(t *testing.T) {
	t.Parallel()

	path, cleanup, err := TemporarySampleImagePath()
	if err != nil {
		t.Fatalf("TemporarySampleImagePath() error = %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("generated image missing PNG signature: %x", data[:8])
	}
}

func TestPickHighestModel(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		models    []codex.Model
		preferred string
		wantModel string
		wantOK    bool
	}{
		"success: preferred visible model wins": {
			models: []codex.Model{
				{ID: "old", Model: "gpt-5.3", Upgrade: new("gpt-5.4")},
				{ID: "preferred", Model: "gpt-5.4"},
				{ID: "hidden", Model: "gpt-9", Hidden: true},
			},
			preferred: "gpt-5.4",
			wantModel: "gpt-5.4",
			wantOK:    true,
		},
		"success: upgraded model is skipped when preferred is absent": {
			models: []codex.Model{
				{ID: "old", Model: "gpt-5.3", Upgrade: new("gpt-5.4")},
				{ID: "new", Model: "gpt-5.4"},
			},
			preferred: "missing",
			wantModel: "gpt-5.4",
			wantOK:    true,
		},
		"success: hidden models are used when no visible models exist": {
			models: []codex.Model{
				{ID: "hidden-low", Model: "gpt-5.3", Hidden: true},
				{ID: "hidden-high", Model: "gpt-5.4", Hidden: true},
			},
			preferred: "missing",
			wantModel: "gpt-5.4",
			wantOK:    true,
		},
		"error: empty model list": {
			wantOK: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, ok := PickHighestModel(tt.models, tt.preferred)
			if ok != tt.wantOK {
				t.Fatalf("PickHighestModel() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.Model != tt.wantModel {
				t.Fatalf("PickHighestModel() = (%#v, %v), want model %q", got, ok, tt.wantModel)
			}
		})
	}
}

func TestPickHighestTurnEffort(t *testing.T) {
	t.Parallel()

	model := codex.Model{SupportedReasoningEfforts: []codex.ReasoningEffortOption{
		{ReasoningEffort: codex.ReasoningEffortLow},
		{ReasoningEffort: codex.ReasoningEffortXhigh},
		{ReasoningEffort: codex.ReasoningEffortMedium},
	}}
	if got, want := PickHighestTurnEffort(model), codex.ReasoningEffortXhigh; got != want {
		t.Fatalf("PickHighestTurnEffort() = %q, want %q", got, want)
	}
}
