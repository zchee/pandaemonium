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

	simd "simd/archsimd"
)

// TestAVX2TinyHaystacks exercises haystacks 0..95 bytes (covers tail-only,
// exactly-one-chunk, one-chunk-plus-partial-tail, exactly-two-chunks, and
// two-chunks-plus-partial-tail) against the naive oracle for every AVX2
// routine. The test calls the avx2* functions directly so it stays AVX2-
// focused regardless of the dispatcher's runtime selection.
//
// Skipped on non-AVX2 hardware (or with GODEBUG=cpu.avx2=off): the
// instruction families used by archsimd's AVX2 path will fault at runtime
// on a CPU that lacks the feature, so we gate execution on
// simd.X86.AVX2(). The cross-compile path still emits the AVX2 code;
// runtime execution requires AVX2-capable hardware.
func TestAVX2TinyHaystacks(t *testing.T) {
	t.Parallel()
	if !simd.X86.AVX2() {
		t.Skip("AVX2 not available on this CPU (or disabled via GODEBUG=cpu.avx2=off)")
	}

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

	// 0..95 covers up to two full 32-byte chunks plus a 31-byte tail.
	for n := 0; n <= 95; n++ {
		for _, p := range patterns {
			base := p.gen(n)

			t.Run(fmt.Sprintf("memchr/%s/n=%d/miss", p.name, n), func(t *testing.T) {
				t.Parallel()
				assertMatch(t, "Memchr", avx2Memchr(sentinel, base), naiveMemchr(sentinel, base))
			})
			t.Run(fmt.Sprintf("memrchr/%s/n=%d/miss", p.name, n), func(t *testing.T) {
				t.Parallel()
				assertMatch(t, "Memrchr", avx2Memrchr(sentinel, base), naiveMemrchr(sentinel, base))
			})

			for pos := 0; pos < n; pos++ {
				h := append([]byte(nil), base...)
				h[pos] = sentinel
				t.Run(fmt.Sprintf("memchr/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memchr", avx2Memchr(sentinel, h), naiveMemchr(sentinel, h))
				})
				t.Run(fmt.Sprintf("memrchr/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memrchr", avx2Memrchr(sentinel, h), naiveMemrchr(sentinel, h))
				})
				t.Run(fmt.Sprintf("memchr2/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memchr2", avx2Memchr2(sentinel, 0xD4, h), naiveMemchr2(sentinel, 0xD4, h))
				})
				t.Run(fmt.Sprintf("memrchr2/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memrchr2", avx2Memrchr2(sentinel, 0xD4, h), naiveMemrchr2(sentinel, 0xD4, h))
				})
				t.Run(fmt.Sprintf("memchr3/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memchr3", avx2Memchr3(sentinel, 0xD4, 0xE5, h), naiveMemchr3(sentinel, 0xD4, 0xE5, h))
				})
				t.Run(fmt.Sprintf("memrchr3/%s/n=%d/plant=%d", p.name, n, pos), func(t *testing.T) {
					t.Parallel()
					assertMatch(t, "Memrchr3", avx2Memrchr3(sentinel, 0xD4, 0xE5, h), naiveMemrchr3(sentinel, 0xD4, 0xE5, h))
				})
			}
		}
	}
}
