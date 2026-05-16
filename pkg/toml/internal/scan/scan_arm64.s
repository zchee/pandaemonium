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

//go:build !force_swar && arm64

#include "textflag.h"

// Plan 9 ARM64 NEON implementation of the six scan kernels. Each kernel
// loads 16 bytes per iteration via VLD1, computes a per-lane membership
// mask via VCMEQ (or VCMHS for unsigned range tests), and narrows the
// 16-byte 0xFF/0x00 mask to a 64-bit syndrome via VSHRN $4 reinterpreting
// the mask as 8 halfwords. Each matched source byte contributes exactly
// 4 bits to the syndrome at positions [4k:4k+3] for source lane k, so
// RBIT+CLZ followed by an LSR $2 recovers the lane index of the first
// match.
//
// "Find first non-match" kernels (ScanBareKey, SkipWhitespace) build the
// membership mask, then invert with VNOT before VSHRN so the same
// RBIT+CLZ sequence locates the first non-class byte.
//
// Common register layout (per-kernel deviations noted in comments):
//
//   R0 — current data pointer (post-incremented by 16 per main-loop iter)
//   R1 — remaining length
//   R2 — saved original data pointer (for offset = R0 - R2)
//   R3 — scratch (constants, byte values in tail)
//   R4 — syndrome / lane count
//   V0 — current 16-byte chunk (VLD1 dest)
//   V1..V8 — target constants (broadcasts of literal bytes)
//   V9..V14 — working membership masks
//
// Frame layout for func(s []byte) int: ptr+0, len+8, cap+16, ret+24 → $0-32.

// ============================================================================
// scanBareKeyNEON — first index whose byte is NOT in [A-Za-z0-9_-], or len(s).
// ============================================================================

// func scanBareKeyNEON(s []byte) int
TEXT ·scanBareKeyNEON(SB), NOSPLIT, $0-32
	MOVD	s_base+0(FP), R0
	MOVD	s_len+8(FP), R1
	MOVD	R0, R2

	// Broadcast the eight class boundary bytes to V1..V8.
	MOVD	$'A', R3
	VMOV	R3, V1.B16
	MOVD	$'Z', R3
	VMOV	R3, V2.B16
	MOVD	$'a', R3
	VMOV	R3, V3.B16
	MOVD	$'z', R3
	VMOV	R3, V4.B16
	MOVD	$'0', R3
	VMOV	R3, V5.B16
	MOVD	$'9', R3
	VMOV	R3, V6.B16
	MOVD	$'_', R3
	VMOV	R3, V7.B16
	MOVD	$'-', R3
	VMOV	R3, V8.B16

bare_loop16:
	CMP	$16, R1
	BLT	bare_tail
	VLD1	(R0), [V0.B16]

	// isUpper = (V0 >= 'A') && (V0 <= 'Z')
	VCMHS	V1.B16, V0.B16, V9.B16  // V0 >= 'A'
	VCMHS	V0.B16, V2.B16, V10.B16 // 'Z' >= V0
	VAND	V9.B16, V10.B16, V9.B16 // V9 = isUpper

	// isLower = (V0 >= 'a') && (V0 <= 'z')
	VCMHS	V3.B16, V0.B16, V10.B16
	VCMHS	V0.B16, V4.B16, V11.B16
	VAND	V10.B16, V11.B16, V10.B16
	VORR	V10.B16, V9.B16, V9.B16

	// isDigit = (V0 >= '0') && (V0 <= '9')
	VCMHS	V5.B16, V0.B16, V10.B16
	VCMHS	V0.B16, V6.B16, V11.B16
	VAND	V10.B16, V11.B16, V10.B16
	VORR	V10.B16, V9.B16, V9.B16

	// is underscore or hyphen
	VCMEQ	V7.B16, V0.B16, V10.B16
	VORR	V10.B16, V9.B16, V9.B16
	VCMEQ	V8.B16, V0.B16, V10.B16
	VORR	V10.B16, V9.B16, V9.B16

	// Want first NON-member: invert mask then locate first match.
	VNOT	V9.B16, V9.B16
	VSHRN	$4, V9.H8, V12.B8
	VMOV	V12.D[0], R4
	CBNZ	R4, bare_found

	ADD	$16, R0
	SUB	$16, R1
	B	bare_loop16

bare_found:
	RBIT	R4, R4
	CLZ	R4, R4
	LSR	$2, R4, R4
	ADD	R4, R0, R0
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET

bare_tail:
	CBZ	R1, bare_done
bare_tail_loop:
	MOVBU	(R0), R3
	// In class?
	// 'A' <= b <= 'Z'?
	SUB	$'A', R3, R4
	CMP	$25, R4
	BLS	bare_tail_next
	// 'a' <= b <= 'z'?
	SUB	$'a', R3, R4
	CMP	$25, R4
	BLS	bare_tail_next
	// '0' <= b <= '9'?
	SUB	$'0', R3, R4
	CMP	$9, R4
	BLS	bare_tail_next
	// _ or -?
	CMP	$'_', R3
	BEQ	bare_tail_next
	CMP	$'-', R3
	BEQ	bare_tail_next
	// Not in class.
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET
bare_tail_next:
	ADD	$1, R0
	SUB	$1, R1
	CBNZ	R1, bare_tail_loop

bare_done:
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET

// ============================================================================
// scanBasicStringNEON — first index of '"' or '\\', or len(s).
// ============================================================================

// func scanBasicStringNEON(s []byte) int
TEXT ·scanBasicStringNEON(SB), NOSPLIT, $0-32
	MOVD	s_base+0(FP), R0
	MOVD	s_len+8(FP), R1
	MOVD	R0, R2

	MOVD	$'"', R3
	VMOV	R3, V1.B16
	MOVD	$'\\', R3
	VMOV	R3, V2.B16

bstr_loop16:
	CMP	$16, R1
	BLT	bstr_tail
	VLD1	(R0), [V0.B16]
	VCMEQ	V1.B16, V0.B16, V9.B16
	VCMEQ	V2.B16, V0.B16, V10.B16
	VORR	V10.B16, V9.B16, V9.B16
	VSHRN	$4, V9.H8, V11.B8
	VMOV	V11.D[0], R4
	CBNZ	R4, bstr_found

	ADD	$16, R0
	SUB	$16, R1
	B	bstr_loop16

bstr_found:
	RBIT	R4, R4
	CLZ	R4, R4
	LSR	$2, R4, R4
	ADD	R4, R0, R0
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET

bstr_tail:
	CBZ	R1, bstr_done
bstr_tail_loop:
	MOVBU	(R0), R3
	CMP	$'"', R3
	BEQ	bstr_match
	CMP	$'\\', R3
	BEQ	bstr_match
	ADD	$1, R0
	SUB	$1, R1
	CBNZ	R1, bstr_tail_loop

bstr_done:
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET
bstr_match:
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET

// ============================================================================
// scanLiteralStringNEON — first index of single-quote (0x27), or len(s).
//
// T3.1 rewrite (perf): mirrors stdlib internal/bytealg/indexbyte_arm64.s as
// closely as is safe — fused single TEXT, 32-byte stride (two V regs per
// VLD1), fast termination check (VORR + VADDP.D2 + VMOV + CBNZ) that skips
// the magic-constant syndrome on the no-match fast path, magic syndrome
// reconstructed only when a hit is found. No BIC-align-down dance — that
// stdlib trick reads up to 31 bytes before the slice start which is safe
// on commodity arm64 but a future hazard on MTE-enabled platforms; the
// ~11% gap we are closing comes from the stride/fast-check work, not from
// alignment. Per-byte tail handled inline.
// ============================================================================

// func scanLiteralStringNEON(s []byte) int
TEXT ·scanLiteralStringNEON(SB), NOSPLIT, $0-32
	MOVD	s_base+0(FP), R0
	MOVD	s_len+8(FP), R1
	MOVD	R0, R11               // R11 = saved base, for final offset

	// Empty input: 0 (offset of "past end").
	CBZ	R1, lstr_drained

	// Magic constant 0x40100401: broadcast to V5.S4 yields the 16-byte
	// pattern {0x01, 0x04, 0x10, 0x40, …}. Used to AND with VCMEQ
	// results so each matched lane contributes 2 bits to the syndrome.
	MOVD	$0x40100401, R5
	VMOV	R5, V5.S4

	// Broadcast target byte ('\'') to all 16 lanes of V0.
	MOVD	$'\'', R3
	VMOV	R3, V0.B16

lstr_loop32:
	CMP	$32, R1
	BLT	lstr_tail
	VLD1.P	(R0), [V1.B16, V2.B16]   // load 32 B, post-increment R0 by 32
	SUB	$32, R1, R1
	VCMEQ	V0.B16, V1.B16, V3.B16
	VCMEQ	V0.B16, V2.B16, V4.B16
	// Fast termination check: VORR collapses the two compare masks; the
	// VADDP.D2 pair-adds the two 64-bit halves, producing a value that
	// is zero iff every byte mismatched (every byte in V_or is 0x00).
	VORR	V4.B16, V3.B16, V6.B16
	VADDP	V6.D2, V6.D2, V6.D2
	VMOV	V6.D[0], R6
	CBNZ	R6, lstr_construct
	CBNZ	R1, lstr_loop32
	// Exactly 32-aligned drain: nothing left, no match.
	B	lstr_drained

lstr_construct:
	// Build the precise 2-bit-per-lane syndrome (lane k → bit 2k).
	VAND	V5.B16, V3.B16, V3.B16
	VAND	V5.B16, V4.B16, V4.B16
	VADDP	V4.B16, V3.B16, V6.B16   // 32 B → 16 B (interleaved)
	VADDP	V6.B16, V6.B16, V6.B16   // 16 B → 8 B
	VMOV	V6.D[0], R6
	RBIT	R6, R6
	CLZ	R6, R6                    // R6 = 2 * lane index of first match
	SUB	$32, R0, R0               // R0 was post-incremented; rewind 32
	ADD	R6>>1, R0, R0             // R0 = match address (lane index = R6/2)
	SUB	R11, R0, R0
	MOVD	R0, ret+24(FP)
	RET

lstr_tail:
	CBZ	R1, lstr_drained
lstr_tail_loop:
	MOVBU.P	1(R0), R3
	CMP	$'\'', R3
	BEQ	lstr_tail_match
	SUBS	$1, R1, R1
	BNE	lstr_tail_loop

lstr_drained:
	SUB	R11, R0, R0               // R0 - base = len(s) consumed
	MOVD	R0, ret+24(FP)
	RET

lstr_tail_match:
	SUB	$1, R0, R0                // rewind MOVBU.P to point at match
	SUB	R11, R0, R0
	MOVD	R0, ret+24(FP)
	RET

// ============================================================================
// skipWhitespaceNEON — first index that is not ' ' or '\t', or len(s).
// ============================================================================

// func skipWhitespaceNEON(s []byte) int
TEXT ·skipWhitespaceNEON(SB), NOSPLIT, $0-32
	MOVD	s_base+0(FP), R0
	MOVD	s_len+8(FP), R1
	MOVD	R0, R2

	MOVD	$' ', R3
	VMOV	R3, V1.B16
	MOVD	$'\t', R3
	VMOV	R3, V2.B16

ws_loop16:
	CMP	$16, R1
	BLT	ws_tail
	VLD1	(R0), [V0.B16]
	VCMEQ	V1.B16, V0.B16, V9.B16
	VCMEQ	V2.B16, V0.B16, V10.B16
	VORR	V10.B16, V9.B16, V9.B16
	// "find first non-member" — invert.
	VNOT	V9.B16, V9.B16
	VSHRN	$4, V9.H8, V11.B8
	VMOV	V11.D[0], R4
	CBNZ	R4, ws_found

	ADD	$16, R0
	SUB	$16, R1
	B	ws_loop16

ws_found:
	RBIT	R4, R4
	CLZ	R4, R4
	LSR	$2, R4, R4
	ADD	R4, R0, R0
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET

ws_tail:
	CBZ	R1, ws_done
ws_tail_loop:
	MOVBU	(R0), R3
	CMP	$' ', R3
	BEQ	ws_tail_next
	CMP	$'\t', R3
	BEQ	ws_tail_next
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET
ws_tail_next:
	ADD	$1, R0
	SUB	$1, R1
	CBNZ	R1, ws_tail_loop

ws_done:
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET

// ============================================================================
// locateNewlineNEON — first index of '\n', or -1 (NOT len(s)) on miss.
//
// T3.1 rewrite (perf): same fused / 32-byte / fast-term-check / magic-syndrome
// shape as scanLiteralStringNEON above; differs only in the target byte and
// the "miss → -1" return convention (vs ScanLiteralString's "miss → len(s)").
// See the comment block on scanLiteralStringNEON for the algorithmic
// rationale, lane-reduction math, and MTE-safety note on skipping the
// stdlib BIC-align-down trick.
// ============================================================================

// func locateNewlineNEON(s []byte) int
TEXT ·locateNewlineNEON(SB), NOSPLIT, $0-32
	MOVD	s_base+0(FP), R0
	MOVD	s_len+8(FP), R1
	MOVD	R0, R11

	CBZ	R1, nl_notfound

	MOVD	$0x40100401, R5
	VMOV	R5, V5.S4
	MOVD	$'\n', R3
	VMOV	R3, V0.B16

nl_loop32:
	CMP	$32, R1
	BLT	nl_tail
	VLD1.P	(R0), [V1.B16, V2.B16]
	SUB	$32, R1, R1
	VCMEQ	V0.B16, V1.B16, V3.B16
	VCMEQ	V0.B16, V2.B16, V4.B16
	VORR	V4.B16, V3.B16, V6.B16
	VADDP	V6.D2, V6.D2, V6.D2
	VMOV	V6.D[0], R6
	CBNZ	R6, nl_construct
	CBNZ	R1, nl_loop32
	B	nl_notfound

nl_construct:
	VAND	V5.B16, V3.B16, V3.B16
	VAND	V5.B16, V4.B16, V4.B16
	VADDP	V4.B16, V3.B16, V6.B16
	VADDP	V6.B16, V6.B16, V6.B16
	VMOV	V6.D[0], R6
	RBIT	R6, R6
	CLZ	R6, R6
	SUB	$32, R0, R0
	ADD	R6>>1, R0, R0
	SUB	R11, R0, R0
	MOVD	R0, ret+24(FP)
	RET

nl_tail:
	CBZ	R1, nl_notfound
nl_tail_loop:
	MOVBU.P	1(R0), R3
	CMP	$'\n', R3
	BEQ	nl_tail_match
	SUBS	$1, R1, R1
	BNE	nl_tail_loop

nl_notfound:
	MOVD	$-1, R0
	MOVD	R0, ret+24(FP)
	RET

nl_tail_match:
	SUB	$1, R0, R0
	SUB	R11, R0, R0
	MOVD	R0, ret+24(FP)
	RET

// ============================================================================
// validateUTF8NEONBulk — first index whose byte has high bit set, or len(s).
// The Go-side wrapper validateUTF8NEON continues from this index with a
// scalar unicode/utf8.DecodeRune loop (mirrors the SSE2/AVX2 pattern).
// ============================================================================

// func validateUTF8NEONBulk(s []byte) int
TEXT ·validateUTF8NEONBulk(SB), NOSPLIT, $0-32
	MOVD	s_base+0(FP), R0
	MOVD	s_len+8(FP), R1
	MOVD	R0, R2

	MOVD	$0x80, R3
	VMOV	R3, V1.B16

u8_loop16:
	CMP	$16, R1
	BLT	u8_tail
	VLD1	(R0), [V0.B16]
	// b >= 0x80 ⇔ high bit set; VCMHS gives 0xFF per lane that matches.
	VCMHS	V1.B16, V0.B16, V9.B16
	VSHRN	$4, V9.H8, V11.B8
	VMOV	V11.D[0], R4
	CBNZ	R4, u8_found

	ADD	$16, R0
	SUB	$16, R1
	B	u8_loop16

u8_found:
	RBIT	R4, R4
	CLZ	R4, R4
	LSR	$2, R4, R4
	ADD	R4, R0, R0
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET

u8_tail:
	CBZ	R1, u8_done
u8_tail_loop:
	MOVBU	(R0), R3
	CMP	$0x80, R3
	BHS	u8_match
	ADD	$1, R0
	SUB	$1, R1
	CBNZ	R1, u8_tail_loop

u8_done:
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET
u8_match:
	SUB	R2, R0, R0
	MOVD	R0, ret+24(FP)
	RET
