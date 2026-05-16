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

//go:build arm64 && !force_swar

#include "textflag.h"

// func memchr2NEON(n1, n2 byte, haystack []byte) int
// Frame: 0; arg/return area: 40 bytes (Go packs both byte args into the
// first 8-byte slot; the slice begins at +8 like memchrNEON).
//   n1     +0(FP)
//   n2     +1(FP)
//   ptr    +8(FP)
//   len    +16(FP)
//   cap    +24(FP) (unused)
//   ret    +32(FP)
//
// Same recipe as memchrNEON, extended: broadcast n1→V0 and n2→V7; for
// each loaded chunk, OR the two per-lane match masks together BEFORE the
// syndrome reduction. R12 holds n2 in scalar paths.
TEXT ·memchr2NEON(SB), NOSPLIT, $0-40
	MOVBU n1+0(FP), R0
	MOVBU n2+1(FP), R12
	MOVD  haystack_base+8(FP), R1
	MOVD  haystack_len+16(FP), R2
	CBZ   R2, m2_fail
	MOVD  R1, R11
	CMP   $32, R2
	BLT   m2_scalar

	AND $0x1f, R1, R3
	CBZ R3, m2_chunks_setup
	NEG R3, R3
	AND $0x1f, R3, R3

m2_head_loop:
	CBZ   R3, m2_chunks_setup
	MOVBU (R1), R4
	CMP   R0, R4
	BEQ   m2_match_at_R1
	CMP   R12, R4
	BEQ   m2_match_at_R1
	ADD   $1, R1, R1
	SUB   $1, R2, R2
	SUB   $1, R3, R3
	B     m2_head_loop

m2_match_at_R1:
	SUB  R11, R1, R0
	MOVD R0, ret+32(FP)
	RET

m2_chunks_setup:
	VMOV R0, V0.B16      // broadcast n1
	VMOV R12, V7.B16     // broadcast n2
	MOVD $0x40100401, R5
	VMOV R5, V5.S4

m2_chunk_loop:
	CMP    $32, R2
	BLT    m2_tail
	VLD1.P (R1), [V1.B16, V2.B16]
	SUB    $32, R2, R2
	VCMEQ  V0.B16, V1.B16, V3.B16
	VCMEQ  V7.B16, V1.B16, V8.B16
	VORR   V8.B16, V3.B16, V3.B16 // V3 = (V1 == n1) | (V1 == n2)
	VCMEQ  V0.B16, V2.B16, V4.B16
	VCMEQ  V7.B16, V2.B16, V8.B16
	VORR   V8.B16, V4.B16, V4.B16 // V4 = (V2 == n1) | (V2 == n2)
	VAND   V5.B16, V3.B16, V3.B16
	VAND   V5.B16, V4.B16, V4.B16
	VADDP  V4.B16, V3.B16, V6.B16
	VADDP  V6.B16, V6.B16, V6.B16
	VMOV   V6.D[0], R6
	CBZ    R6, m2_chunk_loop

	RBIT R6, R6
	CLZ  R6, R6
	LSR  $1, R6, R6
	SUB  $32, R1, R7
	ADD  R6, R7, R0
	SUB  R11, R0, R0
	MOVD R0, ret+32(FP)
	RET

m2_tail:
	CBZ   R2, m2_fail
	MOVBU (R1), R4
	CMP   R0, R4
	BEQ   m2_match_at_R1
	CMP   R12, R4
	BEQ   m2_match_at_R1
	ADD   $1, R1, R1
	SUB   $1, R2, R2
	B     m2_tail

m2_scalar:
	CBZ   R2, m2_fail
	MOVBU (R1), R4
	CMP   R0, R4
	BEQ   m2_match_at_R1
	CMP   R12, R4
	BEQ   m2_match_at_R1
	ADD   $1, R1, R1
	SUB   $1, R2, R2
	B     m2_scalar

m2_fail:
	MOVD $-1, R0
	MOVD R0, ret+32(FP)
	RET
