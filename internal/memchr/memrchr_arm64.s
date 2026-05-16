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

// func memrchrNEON(needle byte, haystack []byte) int
// Frame: 0; arg/return area: 40 bytes (same layout as memchrNEON).
//
// Reverse-scan recipe (R-NEW-3): walk haystack from end to start.
// Trailing <32-byte tail is scanned byte-by-byte high-to-low first;
// then 32-byte chunks are walked downward; then the head <32-byte
// region is scanned byte-by-byte. Within a matching chunk the LAST
// match is at chunk-relative position `(63 - CLZ(syndrome)) / 2` (the
// HIGH bit of the 64-bit syndrome, packed 2 bits per byte).
//
// R-NEW-3 reminder: forward uses RBIT+CLZ; reverse uses CLZ alone.
// Copy-pasting the forward routine into the reverse direction is the
// most common source of off-by-one Memrchr bugs.
TEXT ·memrchrNEON(SB), NOSPLIT, $0-40
	MOVBU needle+0(FP), R0
	MOVD  haystack_base+8(FP), R1
	MOVD  haystack_len+16(FP), R2
	CBZ   R2, mr_fail
	MOVD  R1, R11                 // R11 = saved base
	ADD   R1, R2, R7              // R7 = end (exclusive)
	CMP   $32, R2
	BLT   mr_scalar

	// Align R7 down to 32-byte boundary. R8 = aligned end.
	BIC $0x1f, R7, R8

mr_tail_loop:
	CMP   R8, R7
	BLS   mr_chunks_setup
	SUB   $1, R7, R7
	MOVBU (R7), R4
	CMP   R0, R4
	BEQ   mr_match_at_R7
	B     mr_tail_loop

mr_match_at_R7:
	SUB  R11, R7, R0
	MOVD R0, ret+32(FP)
	RET

mr_chunks_setup:
	VMOV R0, V0.B16
	MOVD $0x40100401, R5
	VMOV R5, V5.S4

mr_chunk_loop:
	SUB   $32, R8, R12            // R12 = candidate chunk start
	CMP   R1, R12
	BLO   mr_head_loop            // R12 < R1: no full chunk fits
	VLD1  (R12), [V1.B16, V2.B16]
	VCMEQ V0.B16, V1.B16, V3.B16
	VCMEQ V0.B16, V2.B16, V4.B16
	VAND  V5.B16, V3.B16, V3.B16
	VAND  V5.B16, V4.B16, V4.B16
	VADDP V4.B16, V3.B16, V6.B16
	VADDP V6.B16, V6.B16, V6.B16
	VMOV  V6.D[0], R6
	CBZ   R6, mr_chunk_no_match

	// LAST match: lane = (63 - CLZ) / 2.
	CLZ  R6, R6
	MOVD $63, R13
	SUB  R6, R13, R13
	LSR  $1, R13, R13
	ADD  R12, R13, R0
	SUB  R11, R0, R0
	MOVD R0, ret+32(FP)
	RET

mr_chunk_no_match:
	MOVD R12, R8       // step down by 32
	B    mr_chunk_loop

mr_head_loop:
	CMP   R1, R8
	BLS   mr_fail
	SUB   $1, R8, R8
	MOVBU (R8), R4
	CMP   R0, R4
	BEQ   mr_match_at_R8
	B     mr_head_loop

mr_match_at_R8:
	SUB  R11, R8, R0
	MOVD R0, ret+32(FP)
	RET

mr_scalar:
	CMP   R1, R7
	BLS   mr_fail
	SUB   $1, R7, R7
	MOVBU (R7), R4
	CMP   R0, R4
	BEQ   mr_match_at_R7
	B     mr_scalar

mr_fail:
	MOVD $-1, R0
	MOVD R0, ret+32(FP)
	RET
