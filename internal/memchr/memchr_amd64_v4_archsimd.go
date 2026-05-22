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
	"math/bits"
	simd "simd/archsimd"
	"unsafe"
)

// loadChunkAVX512 reads 64 bytes starting at haystack[i] into an Int8x64
// vector. The caller MUST ensure i+64 <= len(haystack).
func loadChunkAVX512(haystack []byte, i int) simd.Int8x64 {
	return simd.LoadInt8x64Array((*[64]int8)(unsafe.Pointer(&haystack[i])))
}

func avx512ArchMemchr2(n1, n2 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x64(int8(n1))
	v2 := simd.BroadcastInt8x64(int8(n2))
	i := 0
	for i+64 <= n {
		chunk := loadChunkAVX512(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits()
		if mask != 0 {
			return i + bits.TrailingZeros64(mask)
		}
		i += 64
	}
	for ; i < n; i++ {
		if c := haystack[i]; c == n1 || c == n2 {
			return i
		}
	}
	return -1
}

func avx512ArchMemchr3(n1, n2, n3 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x64(int8(n1))
	v2 := simd.BroadcastInt8x64(int8(n2))
	v3 := simd.BroadcastInt8x64(int8(n3))
	i := 0
	for i+64 <= n {
		chunk := loadChunkAVX512(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits() | chunk.Equal(v3).ToBits()
		if mask != 0 {
			return i + bits.TrailingZeros64(mask)
		}
		i += 64
	}
	for ; i < n; i++ {
		if c := haystack[i]; c == n1 || c == n2 || c == n3 {
			return i
		}
	}
	return -1
}

func avx512ArchMemrchr2(n1, n2 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x64(int8(n1))
	v2 := simd.BroadcastInt8x64(int8(n2))
	i := n
	if n&63 != 0 {
		aligned := n &^ 63
		for j := n - 1; j >= aligned; j-- {
			if c := haystack[j]; c == n1 || c == n2 {
				return j
			}
		}
		i = aligned
	}
	for i >= 64 {
		i -= 64
		chunk := loadChunkAVX512(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits()
		if mask != 0 {
			return i + 63 - bits.LeadingZeros64(mask)
		}
	}
	return -1
}

func avx512ArchMemrchr3(n1, n2, n3 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	v1 := simd.BroadcastInt8x64(int8(n1))
	v2 := simd.BroadcastInt8x64(int8(n2))
	v3 := simd.BroadcastInt8x64(int8(n3))
	i := n
	if n&63 != 0 {
		aligned := n &^ 63
		for j := n - 1; j >= aligned; j-- {
			if c := haystack[j]; c == n1 || c == n2 || c == n3 {
				return j
			}
		}
		i = aligned
	}
	for i >= 64 {
		i -= 64
		chunk := loadChunkAVX512(haystack, i)
		mask := chunk.Equal(v1).ToBits() | chunk.Equal(v2).ToBits() | chunk.Equal(v3).ToBits()
		if mask != 0 {
			return i + 63 - bits.LeadingZeros64(mask)
		}
	}
	return -1
}
