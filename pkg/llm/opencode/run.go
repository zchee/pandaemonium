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

// RunResult is the high-level result of one completed turn from Session.Run
// or TurnHandle.Run.
type RunResult struct {
	// FinalResponse is the text of the last text part of the assistant
	// message.
	FinalResponse string
	// Parts are the assistant message parts as returned by the prompt
	// response.
	Parts []Part
	// Message is the completed assistant message.
	Message AssistantMessage
	// Usage aggregates the assistant message token accounting and cost.
	Usage *TokenUsage
}

// TokenUsage aggregates turn token accounting and cost from
// AssistantMessage.Tokens and AssistantMessage.Cost.
type TokenUsage struct {
	Total      float64
	Input      float64
	Output     float64
	Reasoning  float64
	CacheRead  float64
	CacheWrite float64
	Cost       float64
}

// newRunResult builds a RunResult from a completed prompt response.
func newRunResult(resp *PromptResponse) RunResult {
	return RunResult{
		FinalResponse: finalTextResponse(resp.Parts),
		Parts:         resp.Parts,
		Message:       resp.Info,
		Usage: &TokenUsage{
			Total:      resp.Info.Tokens.Total,
			Input:      resp.Info.Tokens.Input,
			Output:     resp.Info.Tokens.Output,
			Reasoning:  resp.Info.Tokens.Reasoning,
			CacheRead:  resp.Info.Tokens.Cache.Read,
			CacheWrite: resp.Info.Tokens.Cache.Write,
			Cost:       resp.Info.Cost,
		},
	}
}

// finalTextResponse returns the text of the last non-synthetic text part.
func finalTextResponse(parts []Part) string {
	for i := len(parts) - 1; i >= 0; i-- {
		if part := &parts[i]; part.Type == "text" && !part.Synthetic && part.Text != "" {
			return part.Text
		}
	}
	return ""
}

// promptOutcome resolves a completed prompt response into (RunResult, error):
// a turn-level error on the assistant message outranks HTTP success.
func promptOutcome(sessionID string, resp *PromptResponse, err error) (RunResult, error) {
	if err != nil {
		return RunResult{}, err
	}
	if resp.Info.Error != nil {
		return RunResult{}, mapTurnError(sessionID, resp.Info.ID, *resp.Info.Error)
	}
	return newRunResult(resp), nil
}
