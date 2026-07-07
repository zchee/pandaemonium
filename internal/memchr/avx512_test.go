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

//go:build amd64 && amd64.v4 && goexperiment.simd && !force_swar

package memchr

import (
	"fmt"
	simd "simd/archsimd"
	"testing"
)

// TestAVX512TinyHaystacks exercises the direct AVX-512 assembly entries across
// tail-only, one-vector, two-vector, and partial-tail lengths. It is v4-only
// because these symbols are part of the GOAMD64=v4 artifact, and it still
// checks simd/archsimd so local execution fails safe on misconfigured hosts.
func TestAVX512TinyHaystacks(t *testing.T) {
	t.Parallel()
	if !simd.X86.AVX512() {
		t.Skip("AVX-512 not available through simd/archsimd on this host")
	}

	const (
		n1 byte = 0xC3
		n2 byte = 0xD4
		n3 byte = 0xE5
	)
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

	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			for n := 0; n <= 191; n++ {
				base := p.gen(n)
				assertAVX512Misses(t, p.name, n, n1, n2, n3, base)
				for pos := 0; pos < n; pos++ {
					assertAVX512Planted(t, p.name, n, pos, n1, n2, n3, base, n1)
					assertAVX512Planted(t, p.name, n, pos, n1, n2, n3, base, n2)
					assertAVX512Planted(t, p.name, n, pos, n1, n2, n3, base, n3)
					assertAVX512DuplicateNeedles(t, p.name, n, pos, n1, base)
				}
				if n > 0 {
					h := append([]byte(nil), base...)
					h[0] = n1
					h[n/2] = n2
					h[n-1] = n3
					assertMatch(t, fmt.Sprintf("AVX512Memchr3 %s n=%d multi-hit", p.name, n), avx512Memchr3(n1, n2, n3, h), naiveMemchr3(n1, n2, n3, h))
					assertMatch(t, fmt.Sprintf("AVX512Memrchr3 %s n=%d multi-hit", p.name, n), avx512Memrchr3(n1, n2, n3, h), naiveMemrchr3(n1, n2, n3, h))

					h2 := append([]byte(nil), base...)
					h2[0] = n1
					h2[n-1] = n2
					assertMatch(t, fmt.Sprintf("AVX512Memchr2 %s n=%d multi-hit", p.name, n), avx512Memchr2(n1, n2, h2), naiveMemchr2(n1, n2, h2))
					assertMatch(t, fmt.Sprintf("AVX512Memrchr2 %s n=%d multi-hit", p.name, n), avx512Memrchr2(n1, n2, h2), naiveMemrchr2(n1, n2, h2))
				}
			}
		})
	}
}

func assertAVX512Misses(t *testing.T, pattern string, n int, n1, n2, n3 byte, h []byte) {
	t.Helper()
	assertMatch(t, fmt.Sprintf("AVX512Memchr %s n=%d miss", pattern, n), avx512Memchr(n1, h), naiveMemchr(n1, h))
	assertMatch(t, fmt.Sprintf("AVX512Memchr2 %s n=%d miss", pattern, n), avx512Memchr2(n1, n2, h), naiveMemchr2(n1, n2, h))
	assertMatch(t, fmt.Sprintf("AVX512Memchr3 %s n=%d miss", pattern, n), avx512Memchr3(n1, n2, n3, h), naiveMemchr3(n1, n2, n3, h))
	assertMatch(t, fmt.Sprintf("AVX512Memrchr %s n=%d miss", pattern, n), avx512Memrchr(n1, h), naiveMemrchr(n1, h))
	assertMatch(t, fmt.Sprintf("AVX512Memrchr2 %s n=%d miss", pattern, n), avx512Memrchr2(n1, n2, h), naiveMemrchr2(n1, n2, h))
	assertMatch(t, fmt.Sprintf("AVX512Memrchr3 %s n=%d miss", pattern, n), avx512Memrchr3(n1, n2, n3, h), naiveMemrchr3(n1, n2, n3, h))
}

func assertAVX512Planted(t *testing.T, pattern string, n, pos int, n1, n2, n3 byte, base []byte, planted byte) {
	t.Helper()
	h := append([]byte(nil), base...)
	h[pos] = planted
	assertMatch(t, fmt.Sprintf("AVX512Memchr %s n=%d plant=%d byte=%#02x", pattern, n, pos, planted), avx512Memchr(planted, h), naiveMemchr(planted, h))
	assertMatch(t, fmt.Sprintf("AVX512Memrchr %s n=%d plant=%d byte=%#02x", pattern, n, pos, planted), avx512Memrchr(planted, h), naiveMemrchr(planted, h))
	assertMatch(t, fmt.Sprintf("AVX512Memchr2 %s n=%d plant=%d byte=%#02x", pattern, n, pos, planted), avx512Memchr2(n1, n2, h), naiveMemchr2(n1, n2, h))
	assertMatch(t, fmt.Sprintf("AVX512Memrchr2 %s n=%d plant=%d byte=%#02x", pattern, n, pos, planted), avx512Memrchr2(n1, n2, h), naiveMemrchr2(n1, n2, h))
	assertMatch(t, fmt.Sprintf("AVX512Memchr3 %s n=%d plant=%d byte=%#02x", pattern, n, pos, planted), avx512Memchr3(n1, n2, n3, h), naiveMemchr3(n1, n2, n3, h))
	assertMatch(t, fmt.Sprintf("AVX512Memrchr3 %s n=%d plant=%d byte=%#02x", pattern, n, pos, planted), avx512Memrchr3(n1, n2, n3, h), naiveMemrchr3(n1, n2, n3, h))
}

func assertAVX512DuplicateNeedles(t *testing.T, pattern string, n, pos int, needle byte, base []byte) {
	t.Helper()
	h := append([]byte(nil), base...)
	h[pos] = needle
	assertMatch(t, fmt.Sprintf("AVX512Memchr2 %s n=%d plant=%d duplicate", pattern, n, pos), avx512Memchr2(needle, needle, h), naiveMemchr2(needle, needle, h))
	assertMatch(t, fmt.Sprintf("AVX512Memchr3 %s n=%d plant=%d duplicate", pattern, n, pos), avx512Memchr3(needle, needle, needle, h), naiveMemchr3(needle, needle, needle, h))
	assertMatch(t, fmt.Sprintf("AVX512Memrchr2 %s n=%d plant=%d duplicate", pattern, n, pos), avx512Memrchr2(needle, needle, h), naiveMemrchr2(needle, needle, h))
	assertMatch(t, fmt.Sprintf("AVX512Memrchr3 %s n=%d plant=%d duplicate", pattern, n, pos), avx512Memrchr3(needle, needle, needle, h), naiveMemrchr3(needle, needle, needle, h))
}
