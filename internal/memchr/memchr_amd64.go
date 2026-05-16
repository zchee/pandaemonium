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

// init runs at package load and selects between the AVX2 (256-bit
// Int8x32) and SSE2 (128-bit Int8x16) hot paths based on runtime CPU
// detection via simd.X86.AVX2(). AVX2 is the win on Haswell-and-later
// boxes; SSE2 is the fallback for pre-Haswell amd64. Setting
// GODEBUG=cpu.avx2=off forces the SSE2 path — that downgrade is exactly
// what AC-HARNESS-7's TestBackendBinding catches.
//
// boundImpl records the actual chosen backend ("avx2" or "sse2") so the
// AC-HARNESS-7 binding test can reject silent downgrades on CI.
//
// This file owns the (amd64, goexperiment.simd, !force_swar) tuple. The
// rest of the dispatcher's tuple-coverage map is in dispatch.go.
func init() {
	if simd.X86.AVX2() {
		memchrImpl = avx2Memchr
		memchr2Impl = avx2Memchr2
		memchr3Impl = avx2Memchr3
		memrchrImpl = avx2Memrchr
		memrchr2Impl = avx2Memrchr2
		memrchr3Impl = avx2Memrchr3
		boundImpl = "avx2"
		return
	}
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

// =====================================================================
// AVX2 (256-bit Int8x32) implementations. Selected at init() when
// simd.X86.AVX2() is true. The recipe is the SSE2 routine widened to
// 32-byte vectors plus uint32 mask reduction: 32 lanes per chunk means
// bits.TrailingZeros32 / bits.LeadingZeros32 over the bitmap. R-NEW-3
// reminder: reverse scans MUST use LeadingZeros32; the chunk-relative
// LAST-match position is `31 - bits.LeadingZeros32(mask)`.
//
// Mask8x32.ToBits is verified present at /opt/local/go.simd/src/simd/
// archsimd/types_amd64.go:575 (asm VPMOVMSKB, AVX2 feature).
//
// Each AVX2 backend owns its own scalar tail (per plan Fork 3); the
// trailing <32-byte tail uses the same byte-at-a-time loop as the SSE2
// path so the inliner does not have to coordinate cross-vector-width
// helpers.
// =====================================================================

// loadChunkAVX2 reads 32 bytes starting at haystack[i] into an Int8x32
// vector. The caller MUST ensure `i+32 <= len(haystack)` so the load
// stays in bounds.
func loadChunkAVX2(haystack []byte, i int) simd.Int8x32 {
	return simd.LoadInt8x32Array((*[32]int8)(unsafe.Pointer(&haystack[i])))
}

// avx2Memchr returns the offset of the first byte equal to needle in
// haystack, or -1 if no byte matches.
func avx2Memchr(needle byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	nv := simd.BroadcastInt8x32(int8(needle))
	i := 0
	for i+32 <= n {
		mask := loadChunkAVX2(haystack, i).Equal(nv).ToBits()
		if mask != 0 {
			return i + bits.TrailingZeros32(mask)
		}
		i += 32
	}
	for ; i < n; i++ {
		if haystack[i] == needle {
			return i
		}
	}
	return -1
}

// avx2Memchr2 returns the offset of the first byte equal to n1 or n2 in
// haystack, or -1 if no byte matches either.
func avx2Memchr2(n1, n2 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x32(int8(n1))
	v2 := simd.BroadcastInt8x32(int8(n2))
	i := 0
	for i+32 <= n {
		chunk := loadChunkAVX2(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits()
		if mask != 0 {
			return i + bits.TrailingZeros32(mask)
		}
		i += 32
	}
	for ; i < n; i++ {
		if c := haystack[i]; c == n1 || c == n2 {
			return i
		}
	}
	return -1
}

// avx2Memchr3 returns the offset of the first byte equal to n1, n2, or
// n3 in haystack, or -1 if no byte matches any of them.
func avx2Memchr3(n1, n2, n3 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x32(int8(n1))
	v2 := simd.BroadcastInt8x32(int8(n2))
	v3 := simd.BroadcastInt8x32(int8(n3))
	i := 0
	for i+32 <= n {
		chunk := loadChunkAVX2(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits() | chunk.Equal(v3).ToBits()
		if mask != 0 {
			return i + bits.TrailingZeros32(mask)
		}
		i += 32
	}
	for ; i < n; i++ {
		if c := haystack[i]; c == n1 || c == n2 || c == n3 {
			return i
		}
	}
	return -1
}

// avx2Memrchr returns the offset of the LAST byte equal to needle in
// haystack, or -1 if no byte matches. The reverse-scan chunk-relative
// LAST-match position is `31 - bits.LeadingZeros32(mask)` (R-NEW-3).
func avx2Memrchr(needle byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	nv := simd.BroadcastInt8x32(int8(needle))
	i := n
	if n&31 != 0 {
		aligned := n &^ 31
		for j := n - 1; j >= aligned; j-- {
			if haystack[j] == needle {
				return j
			}
		}
		i = aligned
	}
	for i >= 32 {
		i -= 32
		mask := loadChunkAVX2(haystack, i).Equal(nv).ToBits()
		if mask != 0 {
			return i + 31 - bits.LeadingZeros32(mask)
		}
	}
	return -1
}

// avx2Memrchr2 returns the offset of the LAST byte equal to n1 or n2 in
// haystack, or -1 if no byte matches either.
func avx2Memrchr2(n1, n2 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x32(int8(n1))
	v2 := simd.BroadcastInt8x32(int8(n2))
	i := n
	if n&31 != 0 {
		aligned := n &^ 31
		for j := n - 1; j >= aligned; j-- {
			if c := haystack[j]; c == n1 || c == n2 {
				return j
			}
		}
		i = aligned
	}
	for i >= 32 {
		i -= 32
		chunk := loadChunkAVX2(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits()
		if mask != 0 {
			return i + 31 - bits.LeadingZeros32(mask)
		}
	}
	return -1
}

// avx2Memrchr3 returns the offset of the LAST byte equal to n1, n2, or
// n3 in haystack, or -1 if no byte matches any of them.
func avx2Memrchr3(n1, n2, n3 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x32(int8(n1))
	v2 := simd.BroadcastInt8x32(int8(n2))
	v3 := simd.BroadcastInt8x32(int8(n3))
	i := n
	if n&31 != 0 {
		aligned := n &^ 31
		for j := n - 1; j >= aligned; j-- {
			if c := haystack[j]; c == n1 || c == n2 || c == n3 {
				return j
			}
		}
		i = aligned
	}
	for i >= 32 {
		i -= 32
		chunk := loadChunkAVX2(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits() | chunk.Equal(v3).ToBits()
		if mask != 0 {
			return i + 31 - bits.LeadingZeros32(mask)
		}
	}
	return -1
}
