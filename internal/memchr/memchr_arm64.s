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

// func memchrNEON(needle byte, haystack []byte) int
// Frame: 0; arg/return area: 40 bytes
//   needle  +0(FP) (byte, padded to 8)
//   hay ptr +8(FP)
//   hay len +16(FP)
//   hay cap +24(FP) (unused)
//   ret     +32(FP) (int)
//
// Recipe (plan §"Step 6" L195-203): scalar-aligned-up head, syndrome-trick
// 32-byte main loop modelled on /opt/local/go.simd/src/internal/bytealg/
// indexbyte_arm64.s:46-89, scalar tail. Forward bit position = RBIT+CLZ of
// 64-bit syndrome, divided by 2 (R-NEW-3: forward uses trailing-zero shape).
TEXT ·memchrNEON(SB),NOSPLIT,$0-40
	MOVBU	needle+0(FP), R0         // R0 = needle byte
	MOVD	haystack_base+8(FP), R1  // R1 = ptr
	MOVD	haystack_len+16(FP), R2  // R2 = len
	CBZ	R2, mc_fail
	MOVD	R1, R11                  // R11 = saved base ptr
	CMP	$32, R2
	BLT	mc_scalar                // <32 bytes: scalar only

	// Align R1 up to a 32-byte boundary, scanning 0..31 head bytes.
	AND	$0x1f, R1, R3
	CBZ	R3, mc_chunks_setup
	NEG	R3, R3
	AND	$0x1f, R3, R3            // R3 = (32 - (ptr mod 32)) mod 32
mc_head_loop:
	CBZ	R3, mc_chunks_setup
	MOVBU	(R1), R4
	CMP	R0, R4
	BEQ	mc_match_at_R1
	ADD	$1, R1, R1
	SUB	$1, R2, R2
	SUB	$1, R3, R3
	B	mc_head_loop

mc_match_at_R1:
	SUB	R11, R1, R0
	MOVD	R0, ret+32(FP)
	RET

mc_chunks_setup:
	// V0 = broadcast needle, V5 = syndrome mask 0x40100401.
	VMOV	R0, V0.B16
	MOVD	$0x40100401, R5
	VMOV	R5, V5.S4

mc_chunk_loop:
	CMP	$32, R2
	BLT	mc_tail
	VLD1.P	(R1), [V1.B16, V2.B16]   // load 32B, R1 += 32 (natural width)
	SUB	$32, R2, R2
	VCMEQ	V0.B16, V1.B16, V3.B16
	VCMEQ	V0.B16, V2.B16, V4.B16
	VAND	V5.B16, V3.B16, V3.B16
	VAND	V5.B16, V4.B16, V4.B16
	VADDP	V4.B16, V3.B16, V6.B16   // 256 -> 128
	VADDP	V6.B16, V6.B16, V6.B16   // 128 -> 64
	VMOV	V6.D[0], R6
	CBZ	R6, mc_chunk_loop

	// Match in this chunk. R1 = chunk_end (after post-increment).
	RBIT	R6, R6
	CLZ	R6, R6                   // R6 = trailing-zero count in original
	LSR	$1, R6, R6               // syndrome packs 2 bits per byte
	SUB	$32, R1, R7              // R7 = chunk start
	ADD	R6, R7, R0
	SUB	R11, R0, R0
	MOVD	R0, ret+32(FP)
	RET

mc_tail:
	CBZ	R2, mc_fail
	MOVBU	(R1), R4
	CMP	R0, R4
	BEQ	mc_match_at_R1
	ADD	$1, R1, R1
	SUB	$1, R2, R2
	B	mc_tail

mc_scalar:
	CBZ	R2, mc_fail
	MOVBU	(R1), R4
	CMP	R0, R4
	BEQ	mc_match_at_R1
	ADD	$1, R1, R1
	SUB	$1, R2, R2
	B	mc_scalar

mc_fail:
	MOVD	$-1, R0
	MOVD	R0, ret+32(FP)
	RET
