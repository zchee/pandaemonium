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

//go:build (!amd64 && !arm64) || force_swar || (amd64 && !goexperiment.simd)

// Package memchr provides byte-search primitives ported from
// https://github.com/BurntSushi/memchr. This file holds the pure-Go SWAR
// (SIMD Within A Register) implementations. The broad build tag makes the
// six swar* identifiers linkable on every GOARCH outside the amd64-with-SIMD
// and arm64 hot paths, including the amd64-no-SIMD slot, so that Step 4's
// dispatch_swar_default.go can bind memchrImpl..memrchr3Impl to them without
// duplicating the SWAR bodies across two files.
//
// The classic "has-zero-byte" trick (Mycroft 1987; popularized by Hacker's
// Delight §6-1 and Stanford bit-twiddling-hacks):
//
//	hasZero(x) = ((x − 0x01010101_01010101) AND NOT x) AND 0x80808080_80808080
//
// detects whether any byte of a 64-bit word x is zero by exploiting the
// borrow that subtracting 0x01 from a zero-valued byte propagates into the
// byte's high bit. To search for byte `needle`, we XOR each 8-byte word
// against a needle-broadcast (uint64(needle) * 0x01010101_01010101): any byte
// that matched becomes 0, and hasZero flags its lane. Borrow propagation
// across byte boundaries can perturb non-matching lanes within the same word,
// so when hasZero reports a non-zero result we follow up with a byte-by-byte
// rescan of the word to pinpoint the actual match.
//
// Word load strategy: every 64-bit read goes through
//
//	*(*uint64)(unsafe.Add(base, i))
//
// after the head loop has advanced `i` to an 8-aligned position relative to
// base. Reads are therefore aligned on every platform that requires it. The
// alternative variant using binary.LittleEndian.Uint64 (which performs a
// bounds check per call and, on alignment-strict platforms, may decompose
// into byte loads) is exercised by BenchmarkSWARWordLoad in
// bench_swar_test.go; the unsafe.Pointer form ships as the production code
// because the bench shows it 1.5–3× faster on darwin/arm64-with-force_swar
// at sizes 64..65536. See the Step 1 commit message for the full numbers.
package memchr

import "unsafe"

const (
	swarLoBits uint64 = 0x0101010101010101
	swarHiBits uint64 = 0x8080808080808080
)

// hasZeroByte returns a value whose lane i has bit 7 set if (and possibly
// even when no other bits are set in that lane) byte i of x is zero. The
// reverse direction is reliable: if no byte of x is zero, the result is
// exactly 0. Callers therefore use hasZeroByte != 0 as a presence test and
// follow up with a byte-by-byte rescan to identify the matching lane.
func hasZeroByte(x uint64) uint64 {
	return (x - swarLoBits) &^ x & swarHiBits
}

// loadWord reads an 8-byte little-endian word at byte offset i from the
// haystack base pointer. The caller MUST ensure i+8 <= len(haystack) and
// that (uintptr(base)+uintptr(i)) is 8-aligned.
func loadWord(base unsafe.Pointer, i int) uint64 {
	return *(*uint64)(unsafe.Add(base, i))
}

// swarMemchr returns the offset of the first occurrence of needle in
// haystack, or -1 if not found.
func swarMemchr(needle byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	base := unsafe.Pointer(&haystack[0])
	bcast := uint64(needle) * swarLoBits

	// Head: byte-at-a-time until base+i is 8-aligned.
	i := 0
	for i < n && (uintptr(base)+uintptr(i))&7 != 0 {
		if haystack[i] == needle {
			return i
		}
		i++
	}
	// Aligned 8-byte word stride.
	for i+8 <= n {
		w := loadWord(base, i)
		if hasZeroByte(w^bcast) != 0 {
			for j := range 8 {
				if haystack[i+j] == needle {
					return i + j
				}
			}
		}
		i += 8
	}
	// Tail.
	for ; i < n; i++ {
		if haystack[i] == needle {
			return i
		}
	}
	return -1
}

// swarMemchr2 returns the offset of the first occurrence of either n1 or n2
// in haystack, or -1 if not found.
func swarMemchr2(n1, n2 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	base := unsafe.Pointer(&haystack[0])
	bc1 := uint64(n1) * swarLoBits
	bc2 := uint64(n2) * swarLoBits

	i := 0
	for i < n && (uintptr(base)+uintptr(i))&7 != 0 {
		if c := haystack[i]; c == n1 || c == n2 {
			return i
		}
		i++
	}
	for i+8 <= n {
		w := loadWord(base, i)
		if hasZeroByte(w^bc1)|hasZeroByte(w^bc2) != 0 {
			for j := range 8 {
				if c := haystack[i+j]; c == n1 || c == n2 {
					return i + j
				}
			}
		}
		i += 8
	}
	for ; i < n; i++ {
		if c := haystack[i]; c == n1 || c == n2 {
			return i
		}
	}
	return -1
}

// swarMemchr3 returns the offset of the first occurrence of any of n1, n2,
// or n3 in haystack, or -1 if not found.
func swarMemchr3(n1, n2, n3 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	base := unsafe.Pointer(&haystack[0])
	bc1 := uint64(n1) * swarLoBits
	bc2 := uint64(n2) * swarLoBits
	bc3 := uint64(n3) * swarLoBits

	i := 0
	for i < n && (uintptr(base)+uintptr(i))&7 != 0 {
		if c := haystack[i]; c == n1 || c == n2 || c == n3 {
			return i
		}
		i++
	}
	for i+8 <= n {
		w := loadWord(base, i)
		if hasZeroByte(w^bc1)|hasZeroByte(w^bc2)|hasZeroByte(w^bc3) != 0 {
			for j := range 8 {
				if c := haystack[i+j]; c == n1 || c == n2 || c == n3 {
					return i + j
				}
			}
		}
		i += 8
	}
	for ; i < n; i++ {
		if c := haystack[i]; c == n1 || c == n2 || c == n3 {
			return i
		}
	}
	return -1
}

// swarMemrchr returns the offset of the LAST occurrence of needle in
// haystack, or -1 if not found. The reverse-scan recipe walks the tail
// byte-at-a-time until i is 8-aligned, then strides downward in 8-byte words,
// and finally walks any leading sub-word bytes byte-at-a-time. Each word that
// flags a match is rescanned high-index-first so the last match wins. This is
// the SWAR mirror of the recipe documented at R-NEW-3 in the plan: forward
// scans use TrailingZeros; reverse scans use LeadingZeros / high-index-first
// rescans. Do not copy-paste a forward scan into the reverse direction.
func swarMemrchr(needle byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	base := unsafe.Pointer(&haystack[0])
	bcast := uint64(needle) * swarLoBits

	// Tail: byte-at-a-time from the end until base+i is 8-aligned.
	i := n
	for i > 0 && (uintptr(base)+uintptr(i))&7 != 0 {
		i--
		if haystack[i] == needle {
			return i
		}
	}
	// Aligned word stride going downward; (base+i) is 8-aligned on entry,
	// so (base+i-8) is also 8-aligned.
	for i >= 8 {
		i -= 8
		w := loadWord(base, i)
		if hasZeroByte(w^bcast) != 0 {
			for j := 7; j >= 0; j-- {
				if haystack[i+j] == needle {
					return i + j
				}
			}
		}
	}
	// Head bytes 0..i-1 (only present when &haystack[0] was not 8-aligned).
	for j := i - 1; j >= 0; j-- {
		if haystack[j] == needle {
			return j
		}
	}
	return -1
}

// swarMemrchr2 returns the offset of the LAST occurrence of either n1 or n2
// in haystack, or -1 if not found.
func swarMemrchr2(n1, n2 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	base := unsafe.Pointer(&haystack[0])
	bc1 := uint64(n1) * swarLoBits
	bc2 := uint64(n2) * swarLoBits

	i := n
	for i > 0 && (uintptr(base)+uintptr(i))&7 != 0 {
		i--
		if c := haystack[i]; c == n1 || c == n2 {
			return i
		}
	}
	for i >= 8 {
		i -= 8
		w := loadWord(base, i)
		if hasZeroByte(w^bc1)|hasZeroByte(w^bc2) != 0 {
			for j := 7; j >= 0; j-- {
				if c := haystack[i+j]; c == n1 || c == n2 {
					return i + j
				}
			}
		}
	}
	for j := i - 1; j >= 0; j-- {
		if c := haystack[j]; c == n1 || c == n2 {
			return j
		}
	}
	return -1
}

// swarMemrchr3 returns the offset of the LAST occurrence of any of n1, n2,
// or n3 in haystack, or -1 if not found.
func swarMemrchr3(n1, n2, n3 byte, haystack []byte) int {
	n := len(haystack)
	if n == 0 {
		return -1
	}
	base := unsafe.Pointer(&haystack[0])
	bc1 := uint64(n1) * swarLoBits
	bc2 := uint64(n2) * swarLoBits
	bc3 := uint64(n3) * swarLoBits

	i := n
	for i > 0 && (uintptr(base)+uintptr(i))&7 != 0 {
		i--
		if c := haystack[i]; c == n1 || c == n2 || c == n3 {
			return i
		}
	}
	for i >= 8 {
		i -= 8
		w := loadWord(base, i)
		if hasZeroByte(w^bc1)|hasZeroByte(w^bc2)|hasZeroByte(w^bc3) != 0 {
			for j := 7; j >= 0; j-- {
				if c := haystack[i+j]; c == n1 || c == n2 || c == n3 {
					return i + j
				}
			}
		}
	}
	for j := i - 1; j >= 0; j-- {
		if c := haystack[j]; c == n1 || c == n2 || c == n3 {
			return j
		}
	}
	return -1
}
