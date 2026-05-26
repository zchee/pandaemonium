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

package cmd

import (
	"fmt"
	"math"
	"os"
	"strings"
)

type fileStore struct {
	env map[string]string
	cwd string
}

func (s fileStore) workingDirectory(input map[string]any) (string, error) {
	resolver := pathResolver{env: s.env, cwd: s.cwd}
	return resolver.workingDirectory(stringField(input, "workingDirectory"))
}

func readTextFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return string(data), nil
	}
	if os.IsNotExist(err) {
		return "", nil
	}
	return "", err
}

func requiredString(input map[string]any, key string) (string, error) {
	value, ok := input[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return value, nil
}

func optionalStringSlice(input map[string]any, key string) []string {
	values, ok := input[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toAnySlice(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	return append([]any(nil), items...)
}

func parseNonNegativeInt(value any, fallback int) (int, error) {
	if value == nil {
		return fallback, nil
	}
	switch got := value.(type) {
	case float64:
		if got < 0 || math.Trunc(got) != got {
			return 0, fmt.Errorf("daysOld must be a non-negative integer")
		}
		return int(got), nil
	case int:
		if got < 0 {
			return 0, fmt.Errorf("daysOld must be a non-negative integer")
		}
		return got, nil
	default:
		return 0, fmt.Errorf("daysOld must be a non-negative integer")
	}
}

func maxInt(value any, fallback, upper int) int {
	var n int
	switch got := value.(type) {
	case float64:
		if got <= 0 || math.Trunc(got) != got {
			return fallback
		}
		n = int(got)
	case int:
		if got <= 0 {
			return fallback
		}
		n = got
	default:
		return fallback
	}
	if upper > 0 && n > upper {
		return upper
	}
	return n
}
