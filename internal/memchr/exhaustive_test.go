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

import "testing"

// TestExhaustiveOffsets enumerates every (haystack_len ∈ 0..256, needle
// position 0..n-1) pair against the naive oracle for every dispatched
// public routine (AC-HARNESS-4). This is the boundary-handler gate: it
// catches off-by-one bugs in head alignment, tail handling, and the
// chunk-loop entry/exit conditions across SWAR (8-byte word), SSE2
// (16-byte vector), AVX2 (32-byte vector), and NEON (32-byte chunked)
// tails.
//
// The base haystack is `i & 0x7f`, so neither 0x80 nor 0xff occurs
// naturally — that lets us pick 0xC3 as a "sentinel" needle that only
// matches where we plant it. A miss case per length exercises the
// all-miss code path.
//
// The inner enumeration uses plain looping (no t.Run per (len, pos))
// because the cross product would spawn ~400 000 subtests otherwise;
// that's slower to schedule than the actual comparisons take to run,
// and obscures error reports.
func TestExhaustiveOffsets(t *testing.T) {
	t.Parallel()
	const sentinel byte = 0xC3

	t.Run("Memchr", func(t *testing.T) {
		t.Parallel()
		for n := range 257 {
			base := exhBase(n)
			if got, want := Memchr(sentinel, base), naiveMemchr(sentinel, base); got != want {
				t.Fatalf("miss: n=%d got=%d want=%d", n, got, want)
			}
			for pos := range n {
				hay := exhPlant(base, pos, sentinel)
				if got, want := Memchr(sentinel, hay), naiveMemchr(sentinel, hay); got != want {
					t.Fatalf("plant: n=%d pos=%d got=%d want=%d", n, pos, got, want)
				}
			}
		}
	})

	t.Run("Memrchr", func(t *testing.T) {
		t.Parallel()
		for n := range 257 {
			base := exhBase(n)
			if got, want := Memrchr(sentinel, base), naiveMemrchr(sentinel, base); got != want {
				t.Fatalf("miss: n=%d got=%d want=%d", n, got, want)
			}
			for pos := range n {
				hay := exhPlant(base, pos, sentinel)
				if got, want := Memrchr(sentinel, hay), naiveMemrchr(sentinel, hay); got != want {
					t.Fatalf("plant: n=%d pos=%d got=%d want=%d", n, pos, got, want)
				}
			}
		}
	})

	t.Run("Memchr2", func(t *testing.T) {
		t.Parallel()
		for n := range 257 {
			base := exhBase(n)
			for pos := range n {
				hay := exhPlant(base, pos, sentinel)
				if got, want := Memchr2(sentinel, 0xD4, hay), naiveMemchr2(sentinel, 0xD4, hay); got != want {
					t.Fatalf("n=%d pos=%d got=%d want=%d", n, pos, got, want)
				}
			}
		}
	})

	t.Run("Memrchr2", func(t *testing.T) {
		t.Parallel()
		for n := range 257 {
			base := exhBase(n)
			for pos := range n {
				hay := exhPlant(base, pos, sentinel)
				if got, want := Memrchr2(sentinel, 0xD4, hay), naiveMemrchr2(sentinel, 0xD4, hay); got != want {
					t.Fatalf("n=%d pos=%d got=%d want=%d", n, pos, got, want)
				}
			}
		}
	})

	t.Run("Memchr3", func(t *testing.T) {
		t.Parallel()
		for n := range 257 {
			base := exhBase(n)
			for pos := range n {
				hay := exhPlant(base, pos, sentinel)
				if got, want := Memchr3(sentinel, 0xD4, 0xE5, hay), naiveMemchr3(sentinel, 0xD4, 0xE5, hay); got != want {
					t.Fatalf("n=%d pos=%d got=%d want=%d", n, pos, got, want)
				}
			}
		}
	})

	t.Run("Memrchr3", func(t *testing.T) {
		t.Parallel()
		for n := range 257 {
			base := exhBase(n)
			for pos := range n {
				hay := exhPlant(base, pos, sentinel)
				if got, want := Memrchr3(sentinel, 0xD4, 0xE5, hay), naiveMemrchr3(sentinel, 0xD4, 0xE5, hay); got != want {
					t.Fatalf("n=%d pos=%d got=%d want=%d", n, pos, got, want)
				}
			}
		}
	})
}

// exhBase returns a length-n haystack filled with bytes that never equal
// 0xC3 / 0xD4 / 0xE5 (the sentinel triple used by the exhaustive test).
func exhBase(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i & 0x7f)
	}
	return b
}

// exhPlant returns a copy of base with the sentinel byte planted at pos.
func exhPlant(base []byte, pos int, sentinel byte) []byte {
	h := append([]byte(nil), base...)
	h[pos] = sentinel
	return h
}
