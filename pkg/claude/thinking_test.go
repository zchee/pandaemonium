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
)

// TestEffortLevel_Literals verifies the EffortLevel constants serialize to the
// exact wire literals upstream uses (types.py:33).
func TestEffortLevel_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		lvl  EffortLevel
		want string
	}{
		"low":    {EffortLevelLow, "low"},
		"medium": {EffortLevelMedium, "medium"},
		"high":   {EffortLevelHigh, "high"},
		"xhigh":  {EffortLevelXHigh, "xhigh"},
		"max":    {EffortLevelMax, "max"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.lvl) != tt.want {
				t.Errorf("EffortLevel = %q, want %q", string(tt.lvl), tt.want)
			}
		})
	}
}

// TestThinkingDisplay_Literals verifies the ThinkingDisplay constants serialize
// to the exact wire literals upstream uses (types.py:1557).
func TestThinkingDisplay_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		d    ThinkingDisplay
		want string
	}{
		"summarized": {ThinkingDisplaySummarized, "summarized"},
		"omitted":    {ThinkingDisplayOmitted, "omitted"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.d) != tt.want {
				t.Errorf("ThinkingDisplay = %q, want %q", string(tt.d), tt.want)
			}
		})
	}
}

// TestThinkingConfig_SealedInterface verifies each variant implements
// ThinkingConfig and that all three are assignable to a ThinkingConfig
// variable. The unexported isThinkingConfig() method keeps the set closed.
func TestThinkingConfig_SealedInterface(t *testing.T) {
	t.Parallel()
	var configs = []ThinkingConfig{
		ThinkingConfigAdaptive{},
		ThinkingConfigAdaptive{Display: ThinkingDisplaySummarized},
		ThinkingConfigEnabled{BudgetTokens: 1024},
		ThinkingConfigEnabled{BudgetTokens: 2048, Display: ThinkingDisplayOmitted},
		ThinkingConfigDisabled{},
	}
	if len(configs) != 5 {
		t.Errorf("got %d variants, want 5", len(configs))
	}
}
