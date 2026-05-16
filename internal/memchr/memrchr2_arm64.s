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

// func memrchr2NEON(n1, n2 byte, haystack []byte) int
// Frame: 0; arg/return area: 40 bytes (same layout as memchr2NEON).
//
// Combines memrchrNEON's reverse-walk control flow with memchr2NEON's
// two-needle compare. R12 holds n2 in scalar paths.
TEXT ·memrchr2NEON(SB), NOSPLIT, $0-40
	MOVBU n1+0(FP), R0
	MOVBU n2+1(FP), R12
	MOVD  haystack_base+8(FP), R1
	MOVD  haystack_len+16(FP), R2
	CBZ   R2, mr2_fail
	MOVD  R1, R11
	ADD   R1, R2, R7
	CMP   $32, R2
	BLT   mr2_scalar

	BIC $0x1f, R7, R8

mr2_tail_loop:
	CMP   R8, R7
	BLS   mr2_chunks_setup
	SUB   $1, R7, R7
	MOVBU (R7), R4
	CMP   R0, R4
	BEQ   mr2_match_at_R7
	CMP   R12, R4
	BEQ   mr2_match_at_R7
	B     mr2_tail_loop

mr2_match_at_R7:
	SUB  R11, R7, R0
	MOVD R0, ret+32(FP)
	RET

mr2_chunks_setup:
	VMOV R0, V0.B16
	VMOV R12, V7.B16
	MOVD $0x40100401, R5
	VMOV R5, V5.S4

mr2_chunk_loop:
	SUB   $32, R8, R13            // R13 = candidate chunk start (using R13 since R12 holds n2)
	CMP   R1, R13
	BLO   mr2_head_loop
	VLD1  (R13), [V1.B16, V2.B16]
	VCMEQ V0.B16, V1.B16, V3.B16
	VCMEQ V7.B16, V1.B16, V8.B16
	VORR  V8.B16, V3.B16, V3.B16
	VCMEQ V0.B16, V2.B16, V4.B16
	VCMEQ V7.B16, V2.B16, V8.B16
	VORR  V8.B16, V4.B16, V4.B16
	VAND  V5.B16, V3.B16, V3.B16
	VAND  V5.B16, V4.B16, V4.B16
	VADDP V4.B16, V3.B16, V6.B16
	VADDP V6.B16, V6.B16, V6.B16
	VMOV  V6.D[0], R6
	CBZ   R6, mr2_chunk_no_match
	CLZ   R6, R6
	MOVD  $63, R14
	SUB   R6, R14, R14
	LSR   $1, R14, R14
	ADD   R13, R14, R0
	SUB   R11, R0, R0
	MOVD  R0, ret+32(FP)
	RET

mr2_chunk_no_match:
	MOVD R13, R8
	B    mr2_chunk_loop

mr2_head_loop:
	CMP   R1, R8
	BLS   mr2_fail
	SUB   $1, R8, R8
	MOVBU (R8), R4
	CMP   R0, R4
	BEQ   mr2_match_at_R8
	CMP   R12, R4
	BEQ   mr2_match_at_R8
	B     mr2_head_loop

mr2_match_at_R8:
	SUB  R11, R8, R0
	MOVD R0, ret+32(FP)
	RET

mr2_scalar:
	CMP   R1, R7
	BLS   mr2_fail
	SUB   $1, R7, R7
	MOVBU (R7), R4
	CMP   R0, R4
	BEQ   mr2_match_at_R7
	CMP   R12, R4
	BEQ   mr2_match_at_R7
	B     mr2_scalar

mr2_fail:
	MOVD $-1, R0
	MOVD R0, ret+32(FP)
	RET
