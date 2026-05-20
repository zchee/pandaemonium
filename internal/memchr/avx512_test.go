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
	"testing"

	simd "simd/archsimd"
)

// TestAVX512TinyHaystacks exercises the direct AVX-512 Memchr assembly entry
// across tail-only, one-vector, two-vector, and partial-tail lengths. It is
// v4-only because the symbol is part of the GOAMD64=v4 artifact, and it still
// checks simd/archsimd so local execution fails safe on misconfigured hosts.
func TestAVX512TinyHaystacks(t *testing.T) {
	t.Parallel()
	if !simd.X86.AVX512() {
		t.Skip("AVX-512 not available through simd/archsimd on this host")
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

	for _, p := range patterns {
		p := p
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			for n := 0; n <= 191; n++ {
				base := p.gen(n)
				assertMatch(t, fmt.Sprintf("AVX512Memchr %s n=%d miss", p.name, n), avx512Memchr(sentinel, base), naiveMemchr(sentinel, base))
				for pos := 0; pos < n; pos++ {
					h := append([]byte(nil), base...)
					h[pos] = sentinel
					assertMatch(t, fmt.Sprintf("AVX512Memchr %s n=%d plant=%d", p.name, n, pos), avx512Memchr(sentinel, h), naiveMemchr(sentinel, h))
				}
			}
		})
	}
}
