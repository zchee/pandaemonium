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

//go:build force_swar || (!arm64 && (!amd64 || !goexperiment.simd))

package scan

import (
	"math/bits"
	"unicode/utf8"
	"unsafe"
)

// This file implements the SWAR (SIMD-Within-A-Register) fallbacks for
// every scan kernel. The implementations use 64-bit word loads via
// unsafe.Pointer with the classic hasZeroByte trick (Sean Anderson's bit
// twiddling hacks). Index extraction uses math/bits.TrailingZeros64 and
// therefore assumes little-endian byte ordering inside a 64-bit word — see
// the SWAR endianness invariant in doc.go.
//
// # Build tag composition (T4)
//
// The build tag intentionally allows this file to compile in two
// orthogonal regimes:
//
//   - Vanilla SWAR-only builds: amd64 without goexperiment.simd, or any
//     other arch (wasm, riscv64, 386, …) where neither a SIMD backend
//     nor NEON applies. The arch-tagged backends (scan_amd64.go,
//     scan_arm64.go) are excluded by their own build tags in this
//     regime, so this file's dispatch vars are the only ones declared.
//
//   - force_swar verification builds (`go test -tags=force_swar`,
//     AC-SIMD-7): the arch backends are excluded by their leading
//     `!force_swar &&` clause and this file's bindings dominate. This
//     lets a host with native AVX2 or NEON still exercise the SWAR
//     kernels for cross-backend equivalence testing.
//
// In both regimes exactly one file declares the dispatch vars
// (scanBareKey, scanBasicString, scanLiteralString, skipWhitespace,
// locateNewline, validateUTF8), so there is no duplicate-decl risk
// from the build-tag composition.
//
// Correctness note on hasByteEq false positives: the formula
//
//	(x - 0x01010101...) & ^x & 0x80808080...
//
// returns 0x80 in any byte position whose byte equals 0x00. When a true
// match (xor'd byte == 0x00) borrows into the byte immediately above, that
// byte can be falsely marked. Crucially, the false positive is ALWAYS in a
// position strictly above a true match — bits.TrailingZeros64 returns the
// lowest set bit, so the reported match index is still the true first
// match. We only use this primitive for "find first match" scans
// (ScanBasicString, ScanLiteralString, LocateNewline); for "find first
// non-match" scans (ScanBareKey, SkipWhitespace) we use a byte-by-byte
// scalar loop with a class lookup table to avoid that asymmetry entirely.

const (
	swarOnes  uint64 = 0x0101010101010101
	swarHighs uint64 = 0x8080808080808080
)

// hasZero returns a 64-bit mask with byte position p's high bit set when
// w's byte at position p was 0x00. Higher-positioned bytes may also have
// their high bit set as a side-effect of borrow propagation from a true
// zero below; see the file-level comment.
func hasZero(w uint64) uint64 {
	return (w - swarOnes) & ^w & swarHighs
}

// hasByteEq returns hasZero(w XOR broadcast(c)): the high bit is set in
// any byte position whose byte equals c, with the same false-positive
// caveat as hasZero.
func hasByteEq(w uint64, c byte) uint64 {
	return hasZero(w ^ (uint64(c) * swarOnes))
}

// loadu64 reads 8 little-endian bytes from b starting at offset i without
// performing further bounds checks past the single hint below. Callers
// MUST ensure i+8 <= len(b); the hint helps the compiler eliminate
// per-byte bounds checks on the subsequent expression.
//
//go:nosplit
func loadu64(b []byte, i int) uint64 {
	_ = b[i+7] // bounds-check hint
	return *(*uint64)(unsafe.Pointer(&b[i]))
}

// bareKeyClass is the lookup table for the TOML bare-key character class
// [A-Za-z0-9_-]. Built once at package init and read-only thereafter.
var bareKeyClass = func() (t [256]bool) {
	for c := byte('A'); c <= 'Z'; c++ {
		t[c] = true
	}
	for c := byte('a'); c <= 'z'; c++ {
		t[c] = true
	}
	for c := byte('0'); c <= '9'; c++ {
		t[c] = true
	}
	t['_'] = true
	t['-'] = true
	return t
}()

// Default unexported dispatch bindings (T4). On any build where this
// file is compiled, the arch-specific backend files are excluded by
// their own build tags, so these bindings are the live ones. They are
// reassignable so dispatch_test.go (AC-SIMD-7) can swap implementations
// inside t.Cleanup-restored test scopes.
var (
	scanBareKey       = scanBareKeySWAR
	scanBasicString   = scanBasicStringSWAR
	scanLiteralString = scanLiteralStringSWAR
	skipWhitespace    = skipWhitespaceSWAR
	locateNewline     = locateNewlineSWAR
	validateUTF8      = validateUTF8SWAR
)

// scanBareKeySWAR is the SWAR implementation of ScanBareKey.
//
// The bare-key class is a five-component union ([A-Z]|[a-z]|[0-9]|_|-).
// SWAR vectorization of a five-component byte class is awkward enough
// that the lookup-table scalar loop wins on every interesting size; we
// intentionally do not try to widen this to a word-stride loop here.
func scanBareKeySWAR(s []byte) int {
	for i, b := range s {
		if !bareKeyClass[b] {
			return i
		}
	}
	return len(s)
}

// scanBasicStringSWAR is the SWAR implementation of ScanBasicString. It
// stride-loops 8 bytes at a time looking for the first '"' or '\\', then
// finishes the tail byte-by-byte.
func scanBasicStringSWAR(s []byte) int {
	i := 0
	for i+8 <= len(s) {
		w := loadu64(s, i)
		m := hasByteEq(w, '"') | hasByteEq(w, '\\')
		if m != 0 {
			return i + bits.TrailingZeros64(m)/8
		}
		i += 8
	}
	for ; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			return i
		}
	}
	return len(s)
}

// scanLiteralStringSWAR is the SWAR implementation of ScanLiteralString.
// It stride-loops 8 bytes at a time looking for the first single-quote
// byte (0x27), then finishes the tail byte-by-byte.
func scanLiteralStringSWAR(s []byte) int {
	i := 0
	for i+8 <= len(s) {
		w := loadu64(s, i)
		m := hasByteEq(w, '\'')
		if m != 0 {
			return i + bits.TrailingZeros64(m)/8
		}
		i += 8
	}
	for ; i < len(s); i++ {
		if s[i] == '\'' {
			return i
		}
	}
	return len(s)
}

// skipWhitespaceSWAR is the SWAR implementation of SkipWhitespace.
//
// "Find first non-match in a two-byte class" cannot use hasByteEq
// directly because hasByteEq's false-positive case (borrow into the byte
// above a true match) would incorrectly mark the byte above a whitespace
// run as also-whitespace and the loop would skip over a non-whitespace
// byte. We use the scalar loop here for correctness; SWAR pays off only
// for long whitespace runs, which TOML almost never has.
func skipWhitespaceSWAR(s []byte) int {
	for i, b := range s {
		if b != ' ' && b != '\t' {
			return i
		}
	}
	return len(s)
}

// locateNewlineSWAR is the SWAR implementation of LocateNewline. It
// stride-loops 8 bytes at a time looking for the first '\n', returning
// -1 (not len(s)) when no newline is present anywhere in s.
func locateNewlineSWAR(s []byte) int {
	i := 0
	for i+8 <= len(s) {
		w := loadu64(s, i)
		m := hasByteEq(w, '\n')
		if m != 0 {
			return i + bits.TrailingZeros64(m)/8
		}
		i += 8
	}
	for ; i < len(s); i++ {
		if s[i] == '\n' {
			return i
		}
	}
	return -1
}

// validateUTF8SWAR is the SWAR implementation of ValidateUTF8.
//
// The fast path is a word-stride loop that advances by 8 whenever every
// byte in the word has its high bit clear (pure-ASCII chunk; every
// single-byte ASCII sequence is trivially valid UTF-8). When any
// high-bit byte is found, the loop falls through to a per-byte decode
// using unicode/utf8.DecodeRune; the per-byte loop continues past the
// multi-byte rune and resumes from the byte after, which lets a single
// invalid sequence be reported at the correct first-byte offset without
// skipping subsequent valid sequences.
func validateUTF8SWAR(s []byte) int {
	i := 0
	// ASCII fast path: any word whose every byte has the high bit
	// clear is entirely ASCII and therefore valid UTF-8.
	for i+8 <= len(s) {
		w := loadu64(s, i)
		if w&swarHighs == 0 {
			i += 8
			continue
		}
		break
	}
	// Slow path: per-byte loop, using utf8.DecodeRune for multi-byte
	// sequences. utf8.DecodeRune returns (RuneError, 1) on invalid
	// input, which is the canonical signal for "first byte of invalid
	// sequence".
	for i < len(s) {
		b := s[i]
		if b < 0x80 {
			i++
			continue
		}
		r, size := utf8.DecodeRune(s[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return len(s)
}
