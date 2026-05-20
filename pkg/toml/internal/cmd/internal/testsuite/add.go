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
	"time"

	"github.com/zchee/pandaemonium/pkg/toml"
)

// addTag adds JSON tags to a data structure as expected by toml-test.
func addTag(tomlData any) (any, error) {
	// Switch on the data type.
	switch orig := tomlData.(type) {
	// A table: we don't need to add any tags, just recurse for every table
	// entry.
	case map[string]any:
		return addTagMap(orig)

	// An array: we don't need to add any tags, just recurse for every table
	// entry.
	case []map[string]any:
		typed := make([]map[string]any, len(orig))
		for i, v := range orig {
			child, err := addTag(v)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			childMap, ok := child.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("[%d]: tagged table has type %T", i, child)
			}
			typed[i] = childMap
		}
		return typed, nil

	case []any:
		typed := make([]any, len(orig))
		for i, v := range orig {
			child, err := addTag(v)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			typed[i] = child
		}
		return typed, nil

	// Datetime: tag as datetime.
	case toml.LocalTime:
		return tag("time-local", orig.String()), nil

	case toml.LocalDate:
		return tag("date-local", orig.String()), nil

	case toml.LocalDateTime:
		return tag("datetime-local", orig.String()), nil

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
		return addTagReflect(tomlData)
	}
}

func addTagMap(orig map[string]any) (map[string]any, error) {
	typed := make(map[string]any, len(orig))
	for k, v := range orig {
		child, err := addTag(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		typed[k] = child
	}
	return typed, nil
}

func addTagReflect(v any) (any, error) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil, nil
	}

	switch rv.Kind() {
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("unsupported TOML map key type %s", rv.Type().Key())
		}
		typed := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			child, err := addTag(iter.Value().Interface())
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			typed[key] = child
		}
		return typed, nil
	case reflect.Slice, reflect.Array:
		typed := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			child, err := addTag(rv.Index(i).Interface())
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			typed[i] = child
		}
		return typed, nil
	default:
		return nil, fmt.Errorf("unsupported TOML value type %T", v)
	}
}

func tag(typeName string, data any) map[string]any {
	return map[string]any{
		"type":  typeName,
		"value": data,
	}
}
