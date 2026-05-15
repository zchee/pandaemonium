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

package codex

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/go-json-experiment/json"
)

// RunResult is the high-level result from Thread.Run or TurnHandle.Run.
type RunResult struct {
	FinalResponse string
	Items         []ThreadItem
	Usage         *ThreadTokenUsage
	Turn          Turn
}

func collectRunResult(ctx context.Context, client *Client, turnID string) (RunResult, error) {
	var completed *TurnCompletedNotification
	items := []ThreadItem{}
	var usage *ThreadTokenUsage
	for {
		notification, err := client.nextTurnNotification(ctx, turnID)
		if err != nil {
			return RunResult{}, err
		}
		itemCompleted, ok, err := notification.ItemCompleted()
		if err != nil {
			return RunResult{}, err
		}
		if ok && itemCompleted.TurnID == turnID {
			items = append(items, itemCompleted.Item)
			continue
		}
		usageUpdated, ok, err := notification.ThreadTokenUsageUpdated()
		if err != nil {
			return RunResult{}, err
		}
		if ok && usageUpdated.TurnID == turnID {
			snapshot := usageUpdated.TokenUsage
			usage = &snapshot
			continue
		}
		turnCompleted, ok, err := notification.TurnCompleted()
		if err != nil {
			return RunResult{}, err
		}
		if ok && turnCompleted.Turn.ID == turnID {
			completed = &turnCompleted
			break
		}
	}
	//lint:ignore SA4031 defensive guard against future loop edits; AC-3.1
	if completed == nil {
		return RunResult{}, fmt.Errorf("turn %s ended without TurnCompleted", turnID)
	}
	if completed.Turn.Status == TurnStatusFailed {
		return RunResult{}, &TurnFailedError{TurnID: turnID, Status: completed.Turn.Status, Err: completed.Turn.Error}
	}

	return RunResult{
		FinalResponse: finalAssistantResponse(items),
		Items:         items,
		Usage:         usage,
		Turn:          completed.Turn,
	}, nil
}

func finalAssistantResponse(items []ThreadItem) string {
	var lastUnknownPhase string
	foundUnknownPhase := false
	for _, item := range slices.Backward(items) {
		item, ok := decodeThreadItem(item)
		if !ok || !item.agentMessage() {
			continue
		}
		if item.Phase == "final_answer" || item.Phase == "finalAnswer" {
			return item.Text
		}
		if item.Phase == "" && !foundUnknownPhase {
			lastUnknownPhase = item.Text
			foundUnknownPhase = true
		}
	}
	return lastUnknownPhase
}

type decodedThreadItem struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Phase string `json:"phase"`
}

func decodeThreadItem(raw ThreadItem) (decodedThreadItem, bool) {
	if raw == nil {
		return decodedThreadItem{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil || len(encoded) == 0 {
		return decodedThreadItem{}, false
	}
	var item decodedThreadItem
	if err := json.Unmarshal(encoded, &item); err != nil {
		return decodedThreadItem{}, false
	}
	return item, true
}

func (item decodedThreadItem) agentMessage() bool {
	if item.Type == "agentMessage" || item.Type == "agent_message" {
		return true
	}
	return item.Text != "" && item.Type == ""
}

// mergeParamsBaseWins merges caller-supplied params into base, with base taking
// precedence over any overlapping key from params. This ensures wrapper-level
// arguments (e.g. threadId injected by the method wrapper) cannot be silently
// overridden by caller-supplied params structs.
//
// The returned Object is always a fresh map; base is never mutated.
// An error is returned if params cannot be marshalled or unmarshalled.
func mergeParamsBaseWins(params any, base Object) (Object, error) {
	out := Object{}
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return nil, err
		}
		for key, value := range decoded {
			if value != nil {
				out[key] = value
			}
		}
	}
	// base overrides: copy after decoded params so base always wins.
	maps.Copy(out, base)
	return out, nil
}
