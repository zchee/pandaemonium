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

package memchr

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/go-json-experiment/json"
	gocmp "github.com/google/go-cmp/cmp"
)

// goldenFixture mirrors the Rust shim's serialization in
// hack/extract-memchr-corpus/src/main.rs. The JSON ships with base64-encoded
// haystacks so binary content survives the JSON transport cleanly.
type goldenFixture struct {
	Routine string `json:"routine"`
	// Needles arrive as a JSON number array (each element a u8 in
	// [0,255]). We decode into []int because the go-json-experiment v2
	// decoder treats []byte as base64 by default, and the `format:array`
	// opt-out is gated behind GOEXPERIMENT=jsonformat which would be
	// invasive to require for `go test`. Conversion to byte happens in
	// needleBytes below.
	Needles     []int  `json:"needles"`
	HaystackB64 string `json:"haystack_b64"`
	Want        int    `json:"want"`
}

// needleBytes returns the fixture's needles as a []byte, validating that
// each element fits in a u8. A Rust-side regression that emitted out-of-
// range integers would fail loud here instead of silently truncating.
func (f *goldenFixture) needleBytes(t *testing.T, i int) []byte {
	t.Helper()
	out := make([]byte, len(f.Needles))
	for j, n := range f.Needles {
		if n < 0 || n > 0xff {
			t.Fatalf("fixture %d (%s): needle %d out of byte range: %d", i, f.Routine, j, n)
		}
		out[j] = byte(n)
	}
	return out
}

// TestGoldenCorpus asserts that every dispatched Go impl agrees with the
// upstream BurntSushi/memchr =2.7.4 oracle on every committed fixture
// (AC-HARNESS-1). The corpus is extracted once via the Rust shim under
// hack/extract-memchr-corpus/ and committed at
// testdata/golden_corpus.json. The shim's design and regen procedure are
// documented in hack/extract-memchr-corpus/README.md.
//
// At this commit's HEAD only the SWAR backend is bound on every supported
// tuple (Steps 4-6 will add SSE2/AVX2/NEON), so this test currently
// exercises the SWAR backend exclusively. The test logs `boundImpl` so a
// future contributor can see which backend was actually exercised on the CI
// host.
func TestGoldenCorpus(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/golden_corpus.json")
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var fixtures []goldenFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("decode corpus: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("empty corpus — regenerate via hack/extract-memchr-corpus/README.md")
	}
	t.Logf("loaded %d fixtures; bound backend = %q", len(fixtures), boundImpl)

	// Tally fixtures per routine for the post-run summary; missing routine
	// coverage would mean the Rust shim regressed.
	counts := map[string]int{}
	for i, f := range fixtures {
		hay, err := base64.StdEncoding.DecodeString(f.HaystackB64)
		if err != nil {
			t.Fatalf("fixture %d (%s): decode haystack: %v", i, f.Routine, err)
		}
		needles := f.needleBytes(t, i)
		var got int
		switch f.Routine {
		case "memchr":
			if len(needles) != 1 {
				t.Fatalf("fixture %d: memchr expects 1 needle, got %d", i, len(needles))
			}
			got = Memchr(needles[0], hay)
		case "memchr2":
			if len(needles) != 2 {
				t.Fatalf("fixture %d: memchr2 expects 2 needles, got %d", i, len(needles))
			}
			got = Memchr2(needles[0], needles[1], hay)
		case "memchr3":
			if len(needles) != 3 {
				t.Fatalf("fixture %d: memchr3 expects 3 needles, got %d", i, len(needles))
			}
			got = Memchr3(needles[0], needles[1], needles[2], hay)
		case "memrchr":
			if len(needles) != 1 {
				t.Fatalf("fixture %d: memrchr expects 1 needle, got %d", i, len(needles))
			}
			got = Memrchr(needles[0], hay)
		case "memrchr2":
			if len(needles) != 2 {
				t.Fatalf("fixture %d: memrchr2 expects 2 needles, got %d", i, len(needles))
			}
			got = Memrchr2(needles[0], needles[1], hay)
		case "memrchr3":
			if len(needles) != 3 {
				t.Fatalf("fixture %d: memrchr3 expects 3 needles, got %d", i, len(needles))
			}
			got = Memrchr3(needles[0], needles[1], needles[2], hay)
		default:
			t.Fatalf("fixture %d: unknown routine %q", i, f.Routine)
		}
		counts[f.Routine]++
		if diff := gocmp.Diff(f.Want, got); diff != "" {
			t.Errorf("fixture %d (%s, needles=%v, haystack_len=%d): mismatch (-want +got):\n%s",
				i, f.Routine, needles, len(hay), diff)
		}
	}
	for _, r := range []string{"memchr", "memchr2", "memchr3", "memrchr", "memrchr2", "memrchr3"} {
		if counts[r] == 0 {
			t.Errorf("routine %q has no fixtures in corpus — Rust shim regression?", r)
		} else {
			t.Logf("  %-9s: %d fixtures", r, counts[r])
		}
	}
}
