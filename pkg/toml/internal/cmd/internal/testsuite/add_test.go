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
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestValueToTaggedJSONTagsNestedPublicContainers(t *testing.T) {
	t.Parallel()

	doc := map[string]any{
		"title": "demo",
		"owner": map[string]any{
			"name":   "alice",
			"active": true,
			"limits": map[string]any{
				"cpu": int64(2),
			},
		},
		"points": []any{
			map[string]any{"x": int64(1), "y": int64(2)},
			map[string]any{"x": int64(3), "y": int64(4)},
		},
	}

	body, err := ValueToTaggedJSON(doc)
	if err != nil {
		t.Fatalf("ValueToTaggedJSON() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("json.Unmarshal(ValueToTaggedJSON()) error = %v\n%s", err, body)
	}

	want := map[string]any{
		"title": map[string]any{"type": "string", "value": "demo"},
		"owner": map[string]any{
			"name":   map[string]any{"type": "string", "value": "alice"},
			"active": map[string]any{"type": "bool", "value": "true"},
			"limits": map[string]any{
				"cpu": map[string]any{"type": "integer", "value": "2"},
			},
		},
		"points": []any{
			map[string]any{
				"x": map[string]any{"type": "integer", "value": "1"},
				"y": map[string]any{"type": "integer", "value": "2"},
			},
			map[string]any{
				"x": map[string]any{"type": "integer", "value": "3"},
				"y": map[string]any{"type": "integer", "value": "4"},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("ValueToTaggedJSON() mismatch (-want +got):\n%s", diff)
	}
}

func TestValueToTaggedJSONRejectsUnsupportedValues(t *testing.T) {
	t.Parallel()

	_, err := ValueToTaggedJSON(map[string]any{
		"bad": struct{ Name string }{Name: "not a TOML value"},
	})
	if err == nil {
		t.Fatal("ValueToTaggedJSON() error = nil, want unsupported value error")
	}
	if got, want := err.Error(), "bad: unsupported TOML value type struct"; !strings.Contains(got, want) {
		t.Fatalf("ValueToTaggedJSON() error = %q, want substring %q", got, want)
	}
}
