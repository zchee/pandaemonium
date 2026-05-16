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

//go:build (!amd64 && !arm64) || force_swar

package memchr

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// swarMemchrBinary is the binary.LittleEndian.Uint64 variant of swarMemchr.
// It exists only to be benchmarked against the production unsafe.Pointer
// variant via BenchmarkSWARWordLoad per plan R-NEW-1: the spec instructs us
// to measure both word-load forms on the host arch and ship the winner, not
// pre-decide. Production code therefore continues to live in
// memchr_swar_impl.go; this function is unexported and never reached by the
// dispatcher.
//
// Unlike the production variant, this loop performs no head-alignment pass
// because binary.LittleEndian.Uint64 internally uses bounds-checked byte
// loads and is alignment-agnostic — the tradeoff being one extra bounds
// check per word.
func swarMemchrBinary(needle byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	bcast := uint64(needle) * swarLoBits

	i := 0
	for i+8 <= n {
		w := binary.LittleEndian.Uint64(haystack[i : i+8])
		if hasZeroByte(w^bcast) != 0 {
			for j := range 8 {
				if haystack[i+j] == needle {
					return i + j
				}
			}
		}
		i += 8
	}
	for ; i < n; i++ {
		if haystack[i] == needle {
			return i
		}
	}
	return -1
}

// BenchmarkSWARWordLoad is the R-NEW-1 micro-bench: it compares the
// production unsafe.Pointer 8-byte word load against the
// binary.LittleEndian.Uint64 alternative across the canonical haystack-size
// table. Run with `-tags=force_swar` on amd64 / arm64 to compile this file.
//
// The shipped variant is the one that wins (or ties within 5% in favour of
// the safer binary form, per plan R-NEW-1 mitigation text). Production
// remains the unsafe.Pointer form unless this benchmark dictates otherwise;
// the Step 1 commit message records the numbers.
func BenchmarkSWARWordLoad(b *testing.B) {
	for _, n := range []int{64, 256, 1024, 4096, 65536} {
		buf := make([]byte, n)
		for i := range buf {
			// Fill with bytes that will never equal the needle so the
			// inner rescan branch never runs and we measure the pure
			// word-load loop overhead.
			buf[i] = byte(i & 0x7f)
		}
		const needle byte = 0xff

		b.Run(fmt.Sprintf("unsafe/n=%d", n), func(b *testing.B) {
			b.SetBytes(int64(n))
			for b.Loop() {
				if got := swarMemchr(needle, buf); got != -1 {
					b.Fatalf("unexpected hit at %d", got)
				}
			}
		})
		b.Run(fmt.Sprintf("binary/n=%d", n), func(b *testing.B) {
			b.SetBytes(int64(n))
			for b.Loop() {
				if got := swarMemchrBinary(needle, buf); got != -1 {
					b.Fatalf("unexpected hit at %d", got)
				}
			}
		})
	}
}

// TestSWARParity sanity-checks that the unsafe.Pointer production variant
// and the binary.LittleEndian bench variant agree on a fixed corpus, so a
// regression in either variant fails before the perf bench even runs. This
// is the minimal parity check at Step 1; the full naive-oracle property
// suite arrives at Step 7.
func TestSWARParity(t *testing.T) {
	t.Parallel()

	inputs := []struct {
		needle byte
		hay    []byte
	}{
		{'x', nil},
		{'x', []byte{}},
		{'x', []byte("abc")},
		{'a', []byte("abc")},
		{'c', []byte("abc")},
		{'z', []byte("the quick brown fox jumps over the lazy dog")},
		{0x00, bytes256()},
		{0xff, bytes256()},
		{0x80, repeat(0x7f, 73)},
	}
	for i, in := range inputs {
		got := swarMemchr(in.needle, in.hay)
		alt := swarMemchrBinary(in.needle, in.hay)
		want := naiveMemchr(in.needle, in.hay)
		if got != want || alt != want {
			t.Fatalf("input %d (needle=%#x hay=%v): swar=%d binary=%d oracle=%d",
				i, in.needle, in.hay, got, alt, want)
		}
	}
}

// bytes256 returns a 256-byte buffer containing every byte value 0..255
// exactly once.
func bytes256() []byte {
	b := make([]byte, 256)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

// repeat returns a slice of length n with every byte set to v.
func repeat(v byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = v
	}
	return b
}
