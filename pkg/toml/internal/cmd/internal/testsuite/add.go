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
	"strconv"
	"time"

	"github.com/zchee/pandaemonium/pkg/toml"
)

// addTag adds JSON tags to a data structure as expected by toml-test.
func addTag(tomlData any) any {
	// Switch on the data type.
	switch orig := tomlData.(type) {
	// A table: we don't need to add any tags, just recurse for every table
	// entry.
	case map[string]any:
		typed := make(map[string]any, len(orig))
		for k, v := range orig {
			typed[k] = addTag(v)
		}
		return typed

	// An array: we don't need to add any tags, just recurse for every table
	// entry.
	case []map[string]any:
		typed := make([]map[string]any, len(orig))
		for i, v := range orig {
			typed[i] = addTag(v).(map[string]any)
		}
		return typed

	case []any:
		typed := make([]any, len(orig))
		for i, v := range orig {
			typed[i] = addTag(v)
		}
		return typed

	// Datetime: tag as datetime.
	case toml.LocalTime:
		return tag("time-local", orig.String())

	case toml.LocalDate:
		return tag("date-local", orig.String())

	case toml.LocalDateTime:
		return tag("datetime-local", orig.String())

	case time.Time:
		return tag("datetime", orig.Format("2006-01-02T15:04:05.999999999Z07:00"))

	// Tag primitive values: bool, string, int, and float64.
	case bool:
		return tag("bool", strconv.FormatBool(orig))

	case string:
		return tag("string", orig)

	case int64:
		return tag("integer", strconv.FormatInt(orig, 10))

	case float64:
		// Special case for nan since NaN == NaN is false.
		if math.IsNaN(orig) {
			return tag("float", "nan")
		}
		return tag("float", fmt.Sprintf("%v", orig))

	default:
		// return map[string]interface{}{}
		panic(fmt.Sprintf("Unknown type: %T", tomlData))
	}
}

func tag(typeName string, data any) map[string]any {
	return map[string]any{
		"type":  typeName,
		"value": data,
	}
}
