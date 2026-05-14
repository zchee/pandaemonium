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
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/zchee/pandaemonium/pkg/codex"
)

const defaultModel = "gpt-5.4"

// RolloutPlan is the structured output shape requested by advanced examples.
type RolloutPlan struct {
	Summary string
	Actions []string
}

// NewContext returns a cancellable example context with a conservative timeout.
func NewContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

// NewCodex starts the app-server using the process-backed Go SDK.
func NewCodex(ctx context.Context) (*codex.Codex, error) {
	return codex.NewCodex(ctx, &codex.Config{})
}

// DefaultThreadParams returns model and reasoning defaults shared by examples.
func DefaultThreadParams() *codex.ThreadStartParams {
	reasoningEffort := jsontext.Value(`"high"`)
	return &codex.ThreadStartParams{
		Model: new(defaultModel),
		Config: map[string]jsontext.Value{
			"model_reasoning_effort": reasoningEffort,
		},
	}
}

// DefaultModel returns the model used by the upstream examples for first-run flows.
func DefaultModel() string {
	return defaultModel
}

// ServerLabel returns a compact display label for initialize metadata.
func ServerLabel(metadata codex.InitializeResponse) string {
	if metadata.ServerInfo != nil {
		serverName := strings.TrimSpace(metadata.ServerInfo.Name)
		serverVersion := strings.TrimSpace(metadata.ServerInfo.Version)
		if serverName != "" && serverVersion != "" {
			return serverName + " " + serverVersion
		}
	}
	if userAgent := strings.TrimSpace(metadata.UserAgent); userAgent != "" {
		return userAgent
	}
	return "unknown"
}

// FindTurnByID returns the turn with id from turns.
func FindTurnByID(turns []codex.Turn, id string) *codex.Turn {
	for i := range turns {
		if turns[i].ID == id {
			return &turns[i]
		}
	}
	return nil
}

// AssistantTextFromTurn concatenates assistant text from known turn items.
func AssistantTextFromTurn(turn *codex.Turn) string {
	if turn == nil {
		return ""
	}
	return AssistantTextFromItems(turn.Items)
}

// AssistantTextFromItems concatenates assistant text from known thread items.
func AssistantTextFromItems(items []codex.ThreadItem) string {
	var chunks []string
	for _, item := range items {
		encoded, err := json.Marshal(item)
		if err != nil || len(encoded) == 0 {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(encoded, &raw); err != nil {
			continue
		}
		switch raw["type"] {
		case "agentMessage":
			if text, ok := raw["text"].(string); ok && text != "" {
				chunks = append(chunks, text)
			}
		case "message":
			if role, _ := raw["role"].(string); role != "assistant" {
				continue
			}
			contents, _ := raw["content"].([]any)
			for _, content := range contents {
				contentMap, _ := content.(map[string]any)
				if contentMap["type"] != "output_text" {
					continue
				}
				if text, ok := contentMap["text"].(string); ok && text != "" {
					chunks = append(chunks, text)
				}
			}
		}
	}
	return strings.Join(chunks, "")
}

// MustJSONValue returns value encoded as raw JSON or panics for programmer errors.
func MustJSONValue(value any) jsontext.Value {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return jsontext.Value(encoded)
}

// OutputSchema returns the structured-output schema used by advanced examples.
func OutputSchema() jsontext.Value {
	return MustJSONValue(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{"type": "string"},
			"actions": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required":             []string{"summary", "actions"},
		"additionalProperties": false,
	})
}

// ParseRolloutPlan decodes and validates text against OutputSchema's object shape.
func ParseRolloutPlan(text string) (RolloutPlan, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return RolloutPlan{}, errors.New("structured output is empty")
	}
	var fields map[string]jsontext.Value
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return RolloutPlan{}, fmt.Errorf("decode structured output: %w", err)
	}
	if len(fields) != 2 {
		return RolloutPlan{}, fmt.Errorf("structured output has %d fields, want summary and actions", len(fields))
	}
	summaryRaw, ok := fields["summary"]
	if !ok {
		return RolloutPlan{}, errors.New("structured output missing summary")
	}
	actionsRaw, ok := fields["actions"]
	if !ok {
		return RolloutPlan{}, errors.New("structured output missing actions")
	}

	var summary string
	if err := json.Unmarshal(summaryRaw, &summary); err != nil {
		return RolloutPlan{}, fmt.Errorf("decode structured output summary: %w", err)
	}
	if bytes.Equal(bytes.TrimSpace(actionsRaw), []byte("null")) {
		return RolloutPlan{}, errors.New("structured output actions must be an array")
	}
	var actions []string
	if err := json.Unmarshal(actionsRaw, &actions); err != nil {
		return RolloutPlan{}, fmt.Errorf("decode structured output actions: %w", err)
	}
	return RolloutPlan{Summary: summary, Actions: actions}, nil
}

// ReadOnlySandboxPolicy returns the read-only sandbox policy used by examples.
func ReadOnlySandboxPolicy() codex.SandboxPolicy {
	return codex.ReadOnlySandboxPolicy{Type: "readOnly"}
}

// TemporarySampleImagePath creates a generated PNG and returns its path.
func TemporarySampleImagePath() (string, func(), error) {
	dir, err := os.MkdirTemp("", "codex-go-example-image-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "generated_sample.png")
	if err := os.WriteFile(path, generatedSamplePNGBytes(), 0o600); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

// FormatUsage formats token usage for concise CLI display.
func FormatUsage(usage *codex.ThreadTokenUsage) string {
	if usage == nil {
		return "usage> (none)"
	}
	return fmt.Sprintf(
		"usage>\n  last: input=%d output=%d reasoning=%d total=%d cached=%d\n  total: input=%d output=%d reasoning=%d total=%d cached=%d",
		usage.Last.InputTokens,
		usage.Last.OutputTokens,
		usage.Last.ReasoningOutputTokens,
		usage.Last.TotalTokens,
		usage.Last.CachedInputTokens,
		usage.Total.InputTokens,
		usage.Total.OutputTokens,
		usage.Total.ReasoningOutputTokens,
		usage.Total.TotalTokens,
		usage.Total.CachedInputTokens,
	)
}

// PickHighestModel chooses the preferred model when present, otherwise the top non-upgrade model.
func PickHighestModel(models []codex.Model, preferred string) (codex.Model, bool) {
	if len(models) == 0 {
		return codex.Model{}, false
	}
	visible := slices.DeleteFunc(slices.Clone(models), func(model codex.Model) bool {
		return model.Hidden
	})
	if len(visible) == 0 {
		visible = slices.Clone(models)
	}
	for _, model := range visible {
		if model.Model == preferred || model.ID == preferred {
			return model, true
		}
	}
	knownNames := map[string]struct{}{}
	for _, model := range visible {
		knownNames[model.ID] = struct{}{}
		knownNames[model.Model] = struct{}{}
	}
	candidates := slices.DeleteFunc(slices.Clone(visible), func(model codex.Model) bool {
		return model.Upgrade != nil && *model.Upgrade != "" && containsKey(knownNames, *model.Upgrade)
	})
	if len(candidates) == 0 {
		candidates = visible
	}
	return slices.MaxFunc(candidates, func(left, right codex.Model) int {
		if cmp := strings.Compare(left.Model, right.Model); cmp != 0 {
			return cmp
		}
		return strings.Compare(left.ID, right.ID)
	}), true
}

// PickHighestTurnEffort chooses the highest supported reasoning effort.
func PickHighestTurnEffort(model codex.Model) codex.ReasoningEffort {
	if len(model.SupportedReasoningEfforts) == 0 {
		return codex.ReasoningEffortMedium
	}
	best := slices.MaxFunc(model.SupportedReasoningEfforts, func(left, right codex.ReasoningEffortOption) int {
		return effortRank(left.ReasoningEffort) - effortRank(right.ReasoningEffort)
	})
	return best.ReasoningEffort
}

func generatedSamplePNGBytes() []byte {
	const width = 96
	const height = 96
	topLeft := [3]byte{120, 180, 255}
	topRight := [3]byte{255, 220, 90}
	bottomLeft := [3]byte{90, 180, 95}
	bottomRight := [3]byte{180, 85, 85}

	rows := make([]byte, 0, height*(1+width*3))
	for y := range height {
		rows = append(rows, 0)
		for x := range width {
			color := topLeft
			switch {
			case y < height/2 && x >= width/2:
				color = topRight
			case y >= height/2 && x < width/2:
				color = bottomLeft
			case y >= height/2 && x >= width/2:
				color = bottomRight
			}
			rows = append(rows, color[:]...)
		}
	}

	var header bytes.Buffer
	_ = binary.Write(&header, binary.BigEndian, uint32(width))
	_ = binary.Write(&header, binary.BigEndian, uint32(height))
	header.Write([]byte{8, 2, 0, 0, 0})

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, _ = zw.Write(rows)
	_ = zw.Close()

	var out bytes.Buffer
	out.Write([]byte("\x89PNG\r\n\x1a\n"))
	out.Write(pngChunk([]byte("IHDR"), header.Bytes()))
	out.Write(pngChunk([]byte("IDAT"), compressed.Bytes()))
	out.Write(pngChunk([]byte("IEND"), nil))
	return out.Bytes()
}

func pngChunk(chunkType, data []byte) []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, uint32(len(data)))
	out.Write(chunkType)
	out.Write(data)
	checksum := crc32.ChecksumIEEE(append(slices.Clone(chunkType), data...))
	_ = binary.Write(&out, binary.BigEndian, checksum)
	return out.Bytes()
}

func effortRank(effort codex.ReasoningEffort) int {
	switch effort {
	case codex.ReasoningEffortNone:
		return 0
	case codex.ReasoningEffortMinimal:
		return 1
	case codex.ReasoningEffortLow:
		return 2
	case codex.ReasoningEffortMedium:
		return 3
	case codex.ReasoningEffortHigh:
		return 4
	case codex.ReasoningEffortXhigh:
		return 5
	default:
		return -1
	}
}

func containsKey(values map[string]struct{}, key string) bool {
	_, ok := values[key]
	return ok
}
