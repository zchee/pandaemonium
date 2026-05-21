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

// ─── Thinking configuration ──────────────────────────────────────────────────

// ThinkingDisplay controls whether thinking text is returned summarized or
// omitted. Opus 4.7+ defaults to "omitted" (signature-only); pass "summarized"
// to receive the text. Mirrors upstream ThinkingDisplay = Literal["summarized",
// "omitted"] (types.py:1557).
type ThinkingDisplay string

const (
	// ThinkingDisplaySummarized returns a summarized version of the model's
	// thinking text.
	ThinkingDisplaySummarized ThinkingDisplay = "summarized"

	// ThinkingDisplayOmitted suppresses the thinking text and returns only the
	// signature.
	ThinkingDisplayOmitted ThinkingDisplay = "omitted"
)

// EffortLevel controls how much effort Claude puts into its response, working
// alongside adaptive thinking to guide thinking depth. Mirrors upstream
// EffortLevel = Literal["low", "medium", "high", "xhigh", "max"] (types.py:33).
type EffortLevel string

const (
	// EffortLevelLow selects minimal thinking and the fastest responses.
	EffortLevelLow EffortLevel = "low"

	// EffortLevelMedium selects moderate thinking.
	EffortLevelMedium EffortLevel = "medium"

	// EffortLevelHigh selects deeper thinking at the cost of latency.
	EffortLevelHigh EffortLevel = "high"

	// EffortLevelXHigh selects very deep thinking, slower again than High.
	EffortLevelXHigh EffortLevel = "xhigh"

	// EffortLevelMax selects maximum effort.
	EffortLevelMax EffortLevel = "max"
)

// ThinkingConfig is the sealed interface implemented by every extended-thinking
// configuration variant for [Options].Thinking. The unexported sentinel keeps
// the set closed to this package, enabling exhaustive type-switch coverage.
//
// Concrete types: [ThinkingConfigAdaptive], [ThinkingConfigEnabled],
// [ThinkingConfigDisabled].
//
// Mirrors upstream ThinkingConfig = ThinkingConfigAdaptive |
// ThinkingConfigEnabled | ThinkingConfigDisabled (types.py:1575).
type ThinkingConfig interface {
	isThinkingConfig()
}

// ThinkingConfigAdaptive lets Claude decide when and how much to think based on
// the request. Mirrors upstream ThinkingConfigAdaptive (types.py:1560).
type ThinkingConfigAdaptive struct {
	// Display controls whether thinking text is summarized or omitted; the
	// zero value (empty string) leaves the CLI default in effect and emits no
	// --thinking-display flag.
	Display ThinkingDisplay
}

func (ThinkingConfigAdaptive) isThinkingConfig() {}

// ThinkingConfigEnabled fixes the thinking-token budget. Mirrors upstream
// ThinkingConfigEnabled (types.py:1565).
//
// Wire note: setting BudgetTokens emits --max-thinking-tokens <N> only; no
// --thinking flag is emitted for the "enabled" variant (subprocess_cli.py:378).
type ThinkingConfigEnabled struct {
	// BudgetTokens is the maximum number of tokens the model may use for
	// thinking. Required; a zero value emits --max-thinking-tokens 0.
	BudgetTokens int

	// Display controls whether thinking text is summarized or omitted; the
	// zero value (empty string) leaves the CLI default in effect.
	Display ThinkingDisplay
}

func (ThinkingConfigEnabled) isThinkingConfig() {}

// ThinkingConfigDisabled disables extended thinking entirely. Mirrors upstream
// ThinkingConfigDisabled (types.py:1571), which intentionally has no Display
// field — upstream skips --thinking-display when type == "disabled"
// (subprocess_cli.py:385).
type ThinkingConfigDisabled struct{}

func (ThinkingConfigDisabled) isThinkingConfig() {}
