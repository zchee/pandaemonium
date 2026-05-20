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

package testsuite

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/zchee/pandaemonium/pkg/toml"
)

// addTag adds JSON tags to a data structure as expected by toml-test.
func addTag(tomlData any) (any, error) {
	return addTagAt("$", tomlData)
}

func addTagAt(path string, tomlData any) (any, error) {
	// Switch on the data type.
	switch orig := tomlData.(type) {
	// A table: we don't need to add any tags, just recurse for every table
	// entry.
	case map[string]any:
		typed := make(map[string]any, len(orig))
		for k, v := range orig {
			child, err := addTagAt(path+"."+k, v)
			if err != nil {
				return nil, err
			}
			typed[k] = child
		}
		return typed, nil

	// An array: we don't need to add any tags, just recurse for every table
	// entry.
	case []map[string]any:
		typed := make([]map[string]any, len(orig))
		for i, v := range orig {
			child, err := addTagAt(fmt.Sprintf("%s[%d]", path, i), v)
			if err != nil {
				return nil, err
			}
			childMap, ok := child.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("tagged TOML table at %s[%d] has type %T", path, i, child)
			}
			typed[i] = childMap
		}
		return typed, nil

	case []any:
		typed := make([]any, len(orig))
		for i, v := range orig {
			child, err := addTagAt(fmt.Sprintf("%s[%d]", path, i), v)
			if err != nil {
				return nil, err
			}
			typed[i] = child
		}
		return typed, nil

	// Datetime: tag as datetime.
	case toml.LocalTime:
		return tag("time-local", canonicalizeLocalTimeText(orig.String())), nil

	case toml.LocalDate:
		return tag("date-local", orig.String()), nil

	case toml.LocalDateTime:
		return tag("datetime-local", canonicalizeLocalDateTimeText(orig.String())), nil

	case time.Time:
		return tag("datetime", orig.Format("2006-01-02T15:04:05.999999999Z07:00")), nil

	// Tag primitive values: bool, string, int, and float64.
	case bool:
		return tag("bool", strconv.FormatBool(orig)), nil

	case string:
		return tag("string", orig), nil

	case int64:
		return tag("integer", strconv.FormatInt(orig, 10)), nil

	case float64:
		// Special case for nan since NaN == NaN is false.
		if math.IsNaN(orig) {
			if math.Signbit(orig) {
				return tag("float", "-nan"), nil
			}
			return tag("float", "nan"), nil
		}
		return tag("float", fmt.Sprintf("%v", orig)), nil
	default:
		return nil, fmt.Errorf("unsupported TOML value at %s: %T", path, tomlData)
	}
}

func tag(typeName string, data any) map[string]any {
	return map[string]any{
		"type":  typeName,
		"value": data,
	}
}

func canonicalizeLocalTimeText(text string) string {
	if strings.Count(text, ":") == 1 {
		return text + ":00"
	}
	return text
}

func canonicalizeLocalDateTimeText(text string) string {
	if idx := strings.LastIndexAny(text, "T "); idx >= 0 {
		tail := text[idx+1:]
		if strings.Count(tail, ":") == 1 {
			return text + ":00"
		}
	}
	return text
}
