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
	"fmt"
)

// InputItem is one user input item accepted by a turn.
type InputItem interface {
	wireInputItem() Object
}

// TextInput is a plain text turn input.
type TextInput struct {
	Text string
}

func (i TextInput) wireInputItem() Object { return Object{"type": "text", "text": i.Text} }

// ImageInput is a remote image URL turn input.
type ImageInput struct {
	URL string
}

func (i ImageInput) wireInputItem() Object { return Object{"type": "image", "url": i.URL} }

// LocalImageInput is a local image path turn input.
type LocalImageInput struct {
	Path string
}

func (i LocalImageInput) wireInputItem() Object { return Object{"type": "localImage", "path": i.Path} }

// SkillInput references a skill by name and path.
type SkillInput struct {
	Name string
	Path string
}

func (i SkillInput) wireInputItem() Object {
	return Object{"type": "skill", "name": i.Name, "path": i.Path}
}

// MentionInput references a mention by name and path.
type MentionInput struct {
	Name string
	Path string
}

func (i MentionInput) wireInputItem() Object {
	return Object{"type": "mention", "name": i.Name, "path": i.Path}
}

func normalizeInput(input any) ([]Object, error) {
	switch typed := input.(type) {
	case string:
		return []Object{{"type": "text", "text": typed}}, nil
	case InputItem:
		return []Object{typed.wireInputItem()}, nil
	case []InputItem:
		out := make([]Object, 0, len(typed))
		for _, item := range typed {
			out = append(out, item.wireInputItem())
		}
		return out, nil
	case []Object:
		return typed, nil
	case Object:
		return []Object{typed}, nil
	case []any:
		out := make([]Object, 0, len(typed))
		for _, item := range typed {
			normalized, err := normalizeInput(item)
			if err != nil {
				return nil, err
			}
			out = append(out, normalized...)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported input type %T", input)
	}
}
