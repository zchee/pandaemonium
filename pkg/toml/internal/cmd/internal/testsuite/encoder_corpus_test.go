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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/zchee/pandaemonium/pkg/toml"
)

func TestEncodeStdinCorpusArrayShapes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		rel string
	}{
		"array-subtables": {
			rel: "pkg/toml/testdata/toml-test/valid/array/array-subtables",
		},
		"mixed-string-table": {
			rel: "pkg/toml/testdata/toml-test/valid/array/mixed-string-table",
		},
		"nested-inline-table": {
			rel: "pkg/toml/testdata/toml-test/valid/array/nested-inline-table",
		},
		"open-parent-table": {
			rel: "pkg/toml/testdata/toml-test/valid/array/open-parent-table",
		},
		"table-array-string-backslash": {
			rel: "pkg/toml/testdata/toml-test/valid/array/table-array-string-backslash",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			want := mustReadTaggedJSON(t, tc.rel+".json")
			encoded := mustMarshalTomlTestJSON(t, tc.rel+".json")
			var decoded map[string]any
			if err := toml.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("toml.Unmarshal(marshaled %s) error = %v\n%s", tc.rel, err, encoded)
			}
			gotBody, err := ValueToTaggedJSON(decoded)
			if err != nil {
				t.Fatalf("ValueToTaggedJSON(marshaled %s) error = %v", tc.rel, err)
			}
			var got any
			if err := json.Unmarshal(gotBody, &got); err != nil {
				t.Fatalf("json.Unmarshal(round-trip %s) error = %v", tc.rel, err)
			}
			if diff := gocmp.Diff(want, got); diff != "" {
				t.Fatalf("toml-test JSON round trip mismatch (-want +got):\n%s\nencoded TOML:\n%s", diff, encoded)
			}
		})
	}
}

func mustReadTaggedJSON(t testing.TB, rel string) any {
	t.Helper()
	body := mustReadRepoFile(t, rel)
	var tagged any
	if err := json.Unmarshal(body, &tagged); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", rel, err)
	}
	return tagged
}

func mustMarshalTomlTestJSON(t testing.TB, rel string) []byte {
	t.Helper()
	body := mustReadRepoFile(t, rel)
	var typed any
	if err := json.Unmarshal(body, &typed); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", rel, err)
	}
	raw, err := rmTag(typed)
	if err != nil {
		t.Fatalf("rmTag(%s) error = %v", rel, err)
	}
	got, err := toml.Marshal(raw)
	if err != nil {
		t.Fatalf("toml.Marshal(%s) error = %v", rel, err)
	}
	return got
}

func mustReadRepoFile(t testing.TB, rel string) []byte {
	t.Helper()
	root := mustRepoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", rel, err)
	}
	return body
}

func mustRepoRoot(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../.."))
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("os.Stat(%s) error = %v", root, err)
	}
	return root
}
