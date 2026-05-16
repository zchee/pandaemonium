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

//go:build amd64 && goexperiment.simd && !force_swar

package memchr

import (
	"fmt"
	"testing"
)

// TestSSE2TinyHaystacks exercises haystacks 0..47 bytes (covers tail-only,
// exactly-one-chunk, one-chunk-plus-partial-tail, and exactly-two-chunks)
// against the naive oracle for every SSE2 routine (AC-X86-3). The test
// calls the sse2* functions directly rather than going through the *Impl
// funcptr so it stays SSE2-focused even after Step 5 swaps the dispatcher
// to runtime AVX2-vs-SSE2 selection.
//
// Build tag matches memchr_amd64.go (`amd64 && goexperiment.simd &&
// !force_swar`) because the sse2* identifiers it references are only
// defined on that slot. The plan File Tree L334 omits the `!force_swar`
// clause; including it here avoids a compile-time failure under
// `-tags=force_swar` where memchr_amd64.go does not compile.
func TestSSE2TinyHaystacks(t *testing.T) {
	t.Parallel()

	// Sentinel needle that never appears in the synthetic patterns below
	// (so plant-vs-no-plant cases give distinct results).
	const sentinel byte = 0xC3

	patterns := []struct {
		name string
		gen  func(int) []byte
	}{
		{"zeros", func(n int) []byte { return make([]byte, n) }},
		{"ones", func(n int) []byte {
			b := make([]byte, n)
			for i := range b {
				b[i] = 0xff
			}
			return b
		}},
		{"alt", func(n int) []byte {
			b := make([]byte, n)
			for i := range b {
				if i&1 == 0 {
					b[i] = 0x55
				} else {
					b[i] = 0xaa
				}
			}
			return b
		}},
		{"seq", func(n int) []byte {
			b := make([]byte, n)
			for i := range b {
				b[i] = byte(i & 0x7f)
			}
			return b
		}},
	}

	for n := 0; n <= 47; n++ {
		for _, p := range patterns {
			base := p.gen(n)

			// Case 1: no match — needle absent from the pattern.
			t.Run(fmt.Sprintf("memchr/%s/n=%d/miss", p.name, n), func(t *testing.T) {
				t.Parallel()
				assertMatch(t, "Memchr", sse2Memchr(sentinel, base), naiveMemchr(sentinel, base))
			})
			t.Run(fmt.Sprintf("memrchr/%s/n=%d/miss", p.name, n), func(t *testing.T) {
				t.Parallel()
				assertMatch(t, "Memrchr", sse2Memrchr(sentinel, base), naiveMemrchr(sentinel, base))
			})

			// Case 2: planted at every position — finds first vs last.
			for pos := 0; pos < n; pos++ {
				h := append([]byte(nil), base...)
				h[pos] = sentinel
				t.Run(fmt.Sprintf("memchr/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memchr", sse2Memchr(sentinel, h), naiveMemchr(sentinel, h))
				})
				t.Run(fmt.Sprintf("memrchr/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memrchr", sse2Memrchr(sentinel, h), naiveMemrchr(sentinel, h))
				})

				// Multi-needle (n2 / n3) variants: plant sentinel at pos,
				// look for sentinel or any other byte already in pattern.
				t.Run(fmt.Sprintf("memchr2/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memchr2", sse2Memchr2(sentinel, 0xD4, h), naiveMemchr2(sentinel, 0xD4, h))
				})
				t.Run(fmt.Sprintf("memrchr2/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memrchr2", sse2Memrchr2(sentinel, 0xD4, h), naiveMemrchr2(sentinel, 0xD4, h))
				})
				t.Run(fmt.Sprintf("memchr3/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memchr3", sse2Memchr3(sentinel, 0xD4, 0xE5, h), naiveMemchr3(sentinel, 0xD4, 0xE5, h))
				})
				t.Run(fmt.Sprintf("memrchr3/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memrchr3", sse2Memrchr3(sentinel, 0xD4, 0xE5, h), naiveMemrchr3(sentinel, 0xD4, 0xE5, h))
				})
			}
		}
	}
}

// assertMatch is a tiny helper that fails the test when an SSE2 routine
// disagrees with the naive oracle for the same input. It centralizes the
// error message so every assertion in the table-driven test above prints
// the same shape.
func assertMatch(t *testing.T, routine string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got=%d want=%d", routine, got, want)
	}
}
