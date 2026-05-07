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
	"context"
	"fmt"
	"maps"

	json "github.com/go-json-experiment/json"
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
		notification, err := client.NextNotification(ctx)
		if err != nil {
			return RunResult{}, err
		}
		itemCompleted, ok, err := decodeNotification[ItemCompletedNotification](notification, "item/completed")
		if err != nil {
			return RunResult{}, err
		}
		if ok && itemCompleted.TurnID == turnID {
			items = append(items, itemCompleted.Item)
			continue
		}
		usageUpdated, ok, err := decodeNotification[ThreadTokenUsageUpdatedNotification](notification, "thread/tokenUsage/updated")
		if err != nil {
			return RunResult{}, err
		}
		if ok && usageUpdated.TurnID == turnID {
			copy := usageUpdated.TokenUsage
			usage = &copy
			continue
		}
		turnCompleted, ok, err := decodeNotification[TurnCompletedNotification](notification, "turn/completed")
		if err != nil {
			return RunResult{}, err
		}
		if ok && turnCompleted.Turn.ID == turnID {
			completed = &turnCompleted
			break
		}
	}
	if completed.Turn.Status == TurnStatusFailed {
		if completed.Turn.Error != nil && completed.Turn.Error.Message != "" {
			return RunResult{}, fmt.Errorf("%s", completed.Turn.Error.Message)
		}
		return RunResult{}, fmt.Errorf("turn failed with status %s", completed.Turn.Status)
	}
	return RunResult{
		FinalResponse: finalAssistantResponse(items),
		Items:         items,
		Usage:         usage,
		Turn:          completed.Turn,
	}, nil
}

func finalAssistantResponse(items []ThreadItem) string {
	lastUnknownPhase := ""
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if !isAgentMessage(item) {
			continue
		}
		if item.Phase == "final_answer" || item.Phase == "finalAnswer" {
			return item.Text
		}
		if item.Phase == "" && lastUnknownPhase == "" {
			lastUnknownPhase = item.Text
		}
	}
	return lastUnknownPhase
}

func isAgentMessage(item ThreadItem) bool {
	if item.Type == "agentMessage" || item.Type == "agent_message" {
		return true
	}
	if item.Text == "" {
		return false
	}
	var raw map[string]any
	if len(item.Raw) == 0 || json.Unmarshal(item.Raw, &raw) != nil {
		return false
	}
	kind, _ := raw["type"].(string)
	return kind == "agentMessage" || kind == "agent_message"
}

func mergeParams(params any, base Object) Object {
	out := Object{}
	maps.Copy(out, base)
	if params == nil {
		return out
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return out
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return out
	}
	for key, value := range decoded {
		if value != nil {
			out[key] = value
		}
	}
	return out
}
