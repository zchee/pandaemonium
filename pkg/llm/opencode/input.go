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
	"fmt"
)

// InputItem is one prompt input part accepted by a turn.
type InputItem interface {
	wireInputItem() (PartInput, error)
}

// RunInput is any caller input shape accepted by Session.Run and
// Session.Turn.
//
// Supported values are string, [InputItem], []InputItem, [PartInput],
// []PartInput, and []any values containing supported input shapes.
// Unsupported values fail during request normalization.
type RunInput any

// TextInput is a plain text prompt input.
type TextInput struct {
	Text string
}

var _ InputItem = (*TextInput)(nil)

func (i TextInput) wireInputItem() (PartInput, error) {
	return PartInput{
		Type: "text",
		Text: i.Text,
	}, nil
}

// FileInput attaches a file by URL. Mime and URL are required by the server.
type FileInput struct {
	Mime     string
	URL      string
	Filename string
}

var _ InputItem = (*FileInput)(nil)

func (i FileInput) wireInputItem() (PartInput, error) {
	if i.Mime == "" || i.URL == "" {
		return PartInput{}, fmt.Errorf("file input requires mime and url (mime=%q, url=%q)", i.Mime, i.URL)
	}
	return PartInput{
		Type:     "file",
		Mime:     i.Mime,
		URL:      i.URL,
		Filename: i.Filename,
	}, nil
}

// AgentInput mentions a subagent by name. Name is required by the server.
type AgentInput struct {
	Name string
}

var _ InputItem = (*AgentInput)(nil)

func (i AgentInput) wireInputItem() (PartInput, error) {
	if i.Name == "" {
		return PartInput{}, fmt.Errorf("agent input requires a name")
	}
	return PartInput{
		Type: "agent",
		Name: i.Name,
	}, nil
}

func normalizeInput(input RunInput) ([]PartInput, error) {
	switch input := input.(type) {
	case string:
		return []PartInput{
			{
				Type: "text",
				Text: input,
			},
		}, nil

	case InputItem:
		part, err := input.wireInputItem()
		if err != nil {
			return nil, err
		}
		return []PartInput{part}, nil

	case []InputItem:
		out := make([]PartInput, len(input))
		for i, item := range input {
			part, err := item.wireInputItem()
			if err != nil {
				return nil, err
			}
			out[i] = part
		}
		return out, nil

	case PartInput:
		return []PartInput{input}, nil

	case []PartInput:
		return input, nil

	case []any:
		out := make([]PartInput, 0, len(input))
		for _, item := range input {
			normalized, err := normalizeInput(item)
			if err != nil {
				return nil, err
			}
			out = append(out, normalized...)
		}
		return out, nil

	default:
		return nil, fmt.Errorf("unsupported input type: %T", input)
	}
}
