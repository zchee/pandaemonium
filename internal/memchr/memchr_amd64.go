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
	"math/bits"
	"unsafe"

	simd "simd/archsimd"
)

// init binds the dispatcher pointers to the SSE2 implementations and sets
// boundImpl = "sse2". Step 5 swaps this binding to a runtime-selected
// AVX2-or-SSE2 split via simd.X86.AVX2(); until then every amd64-with-SIMD
// build runs through the SSE2 hot path.
//
// At Step 4's commit the transitional file dispatch_default_init.go is
// narrowed from `(amd64 && goexperiment.simd && !force_swar) || (arm64 &&
// !force_swar)` to `arm64 && !force_swar`, eliminating the amd64-with-SIMD
// slot that this file now owns. See plan §"Step 4" L171-181 and the post-
// Step-4 tuple-coverage audit in the commit message.
func init() {
	memchrImpl = sse2Memchr
	memchr2Impl = sse2Memchr2
	memchr3Impl = sse2Memchr3
	memrchrImpl = sse2Memrchr
	memrchr2Impl = sse2Memrchr2
	memrchr3Impl = sse2Memrchr3
	boundImpl = "sse2"
}

// loadChunkSSE2 reads 16 bytes starting at haystack[i] into an Int8x16
// vector. The caller MUST ensure `i+16 <= len(haystack)` so the load stays
// in bounds. Loads are unaligned-safe: the archsimd helper emits an
// unaligned-load instruction equivalent to MOVDQU on x86.
func loadChunkSSE2(haystack []byte, i int) simd.Int8x16 {
	return simd.LoadInt8x16Array((*[16]int8)(unsafe.Pointer(&haystack[i])))
}

// sse2Memchr returns the offset of the first byte equal to needle in
// haystack, or -1 if no byte matches. Strategy: walk aligned 16-byte
// chunks, compare against the needle broadcast, mask non-matching lanes,
// bits.TrailingZeros16 finds the first match. The trailing <16-byte tail
// is scanned byte-by-byte (each SIMD backend owns its tail per plan
// Fork 3, mitigation tracked at R-TAIL-DUP).
func sse2Memchr(needle byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	nv := simd.BroadcastInt8x16(int8(needle))
	i := 0
	for i+16 <= n {
		mask := loadChunkSSE2(haystack, i).Equal(nv).ToBits()
		if mask != 0 {
			return i + bits.TrailingZeros16(mask)
		}
		i += 16
	}
	for ; i < n; i++ {
		if haystack[i] == needle {
			return i
		}
	}
	return -1
}

// sse2Memchr2 returns the offset of the first byte equal to n1 or n2 in
// haystack, or -1 if no byte matches either.
func sse2Memchr2(n1, n2 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x16(int8(n1))
	v2 := simd.BroadcastInt8x16(int8(n2))
	i := 0
	for i+16 <= n {
		chunk := loadChunkSSE2(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits()
		if mask != 0 {
			return i + bits.TrailingZeros16(mask)
		}
		i += 16
	}
	for ; i < n; i++ {
		if c := haystack[i]; c == n1 || c == n2 {
			return i
		}
	}
	return -1
}

// sse2Memchr3 returns the offset of the first byte equal to n1, n2, or n3
// in haystack, or -1 if no byte matches any of them.
func sse2Memchr3(n1, n2, n3 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x16(int8(n1))
	v2 := simd.BroadcastInt8x16(int8(n2))
	v3 := simd.BroadcastInt8x16(int8(n3))
	i := 0
	for i+16 <= n {
		chunk := loadChunkSSE2(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits() | chunk.Equal(v3).ToBits()
		if mask != 0 {
			return i + bits.TrailingZeros16(mask)
		}
		i += 16
	}
	for ; i < n; i++ {
		if c := haystack[i]; c == n1 || c == n2 || c == n3 {
			return i
		}
	}
	return -1
}

// sse2Memrchr returns the offset of the LAST byte equal to needle in
// haystack, or -1 if no byte matches.
//
// Reverse scans MUST use bits.LeadingZeros16 over the bitmap, NOT
// TrailingZeros16 — this asymmetry is R-NEW-3 in the plan; copy-pasting a
// forward scan into the reverse direction is the most common source of
// off-by-one Memrchr bugs. For a chunk at offset i with bitmap `mask`, the
// LAST matching lane is at chunk-relative position
//
//	`15 - bits.LeadingZeros16(mask)`
//
// because LeadingZeros16 counts the number of zero bits from the MSB of a
// 16-bit word (so a set bit at position 15 has LeadingZeros 0, a set bit
// at position 0 has LeadingZeros 15). The haystack offset is therefore
//
//	`i + 15 - bits.LeadingZeros16(mask)`.
//
// Layout: the partial tail (length n%16) is scanned byte-by-byte from the
// high end first; then aligned 16-byte chunks are walked downward.
func sse2Memrchr(needle byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	nv := simd.BroadcastInt8x16(int8(needle))
	i := n
	if n&15 != 0 {
		aligned := n &^ 15
		for j := n - 1; j >= aligned; j-- {
			if haystack[j] == needle {
				return j
			}
		}
		i = aligned
	}
	for i >= 16 {
		i -= 16
		mask := loadChunkSSE2(haystack, i).Equal(nv).ToBits()
		if mask != 0 {
			return i + 15 - bits.LeadingZeros16(mask)
		}
	}
	return -1
}

// sse2Memrchr2 returns the offset of the LAST byte equal to n1 or n2 in
// haystack, or -1 if no byte matches either.
func sse2Memrchr2(n1, n2 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x16(int8(n1))
	v2 := simd.BroadcastInt8x16(int8(n2))
	i := n
	if n&15 != 0 {
		aligned := n &^ 15
		for j := n - 1; j >= aligned; j-- {
			if c := haystack[j]; c == n1 || c == n2 {
				return j
			}
		}
		i = aligned
	}
	for i >= 16 {
		i -= 16
		chunk := loadChunkSSE2(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits()
		if mask != 0 {
			return i + 15 - bits.LeadingZeros16(mask)
		}
	}
	return -1
}

// sse2Memrchr3 returns the offset of the LAST byte equal to n1, n2, or n3
// in haystack, or -1 if no byte matches any of them.
func sse2Memrchr3(n1, n2, n3 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x16(int8(n1))
	v2 := simd.BroadcastInt8x16(int8(n2))
	v3 := simd.BroadcastInt8x16(int8(n3))
	i := n
	if n&15 != 0 {
		aligned := n &^ 15
		for j := n - 1; j >= aligned; j-- {
			if c := haystack[j]; c == n1 || c == n2 || c == n3 {
				return j
			}
		}
		i = aligned
	}
	for i >= 16 {
		i -= 16
		chunk := loadChunkSSE2(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits() | chunk.Equal(v3).ToBits()
		if mask != 0 {
			return i + 15 - bits.LeadingZeros16(mask)
		}
	}
	return -1
}
