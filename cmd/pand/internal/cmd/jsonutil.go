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
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

var safeSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func parseJSONObject(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return nil, fmt.Errorf("--input must be valid JSON: %w", err)
	}
	if got == nil {
		return nil, fmt.Errorf("--input must decode to a JSON object")
	}
	return got, nil
}

func marshalJSON(value any, compact bool) ([]byte, error) {
	if compact {
		return json.Marshal(value)
	}
	return json.Marshal(value, jsontext.WithIndent("  "))
}

func writeJSON(out io.Writer, value any, compact bool) error {
	raw, err := marshalJSON(value, compact)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(raw))
	return err
}

func readJSONFile(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		return nil, err
	}
	if got == nil {
		got = map[string]any{}
	}
	return got, nil
}

func writeJSONFileAtomic(path string, value any) error {
	raw, err := json.Marshal(value, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(raw, '\n'))
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp.%d.%d", path, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func validateSafeSegment(kind string, value any) (string, error) {
	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", kind)
	}
	normalized := strings.TrimSpace(s)
	if normalized == "" {
		return "", fmt.Errorf("%s must be a non-empty string", kind)
	}
	if strings.Contains(normalized, "..") {
		return "", fmt.Errorf("%s must not contain \"..\"", kind)
	}
	if strings.ContainsAny(normalized, `/\\`) {
		return "", fmt.Errorf("%s must not contain path separators", kind)
	}
	if !safeSegmentPattern.MatchString(normalized) {
		return "", fmt.Errorf("%s must match ^[A-Za-z0-9_-]{1,64}$", kind)
	}
	return normalized, nil
}

func optionalSafeSegment(kind string, value any) (string, bool, error) {
	if value == nil {
		return "", false, nil
	}
	segment, err := validateSafeSegment(kind, value)
	return segment, err == nil, err
}

func stringField(input map[string]any, key string) string {
	if s, ok := input[key].(string); ok {
		return s
	}
	return ""
}

func boolField(input map[string]any, key string) bool {
	if b, ok := input[key].(bool); ok {
		return b
	}
	return false
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
