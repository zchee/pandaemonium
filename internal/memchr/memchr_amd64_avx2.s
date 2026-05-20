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

#include "textflag.h"

// func avx2Memchr(needle byte, haystack []byte) int
// Frame: 0; arg/return area: 40 bytes
//   needle  +0(FP) (byte, padded to 8)
//   hay ptr +8(FP)
//   hay len +16(FP)
//   hay cap +24(FP) (unused)
//   ret     +32(FP) (int)
//
// The loop performs only in-bounds 32-byte vector loads. For medium lengths it
// finishes with the last in-bounds 32-byte chunk, which may overlap earlier
// chunks but never crosses outside the slice object. Large full-scan inputs use
// a 4x unrolled full-chunk loop plus an overlapping final chunk only when a
// non-zero tail exists.
TEXT ·avx2Memchr(SB), NOSPLIT, $0-40
	MOVBLZX needle+0(FP), AX
	MOVQ    haystack_base+8(FP), SI
	MOVQ    haystack_len+16(FP), BX
	TESTQ   BX, BX
	JEQ     avx2_fail

	MOVQ SI, R8      // base pointer for offset calculation
	CMPQ BX, $32
	JLE  avx2_scalar

	MOVD         AX, X0
	VPBROADCASTB X0, Y1
	CMPQ         BX, $2048
	JGE          avx2_large
	LEAQ         -32(SI)(BX*1), R9 // start of final in-bounds 32-byte chunk

	PCALIGN $32

avx2_loop:
	VMOVDQU  (SI), Y2
	VPCMPEQB Y1, Y2, Y3
	VPTEST   Y3, Y3
	JNZ      avx2_success
	ADDQ     $32, SI
	CMPQ     SI, R9
	JLT      avx2_loop

	MOVQ     R9, SI
	VMOVDQU  (SI), Y2
	VPCMPEQB Y1, Y2, Y3
	VPTEST   Y3, Y3
	JNZ      avx2_success
	JMP      avx2_fail_vzero

avx2_scalar:
	CMPB (SI), AL
	JEQ  avx2_scalar_success
	INCQ SI
	DECQ BX
	JNZ  avx2_scalar

avx2_fail_vzero:
	VZEROUPPER

avx2_fail:
	MOVQ $-1, ret+32(FP)
	RET

// avx2_large is the full-scan path for large haystacks. It processes full
// chunks four at a time, then optionally checks the final overlapping chunk
// only when len%32 != 0. The threshold is high enough to avoid hurting small
// buffers with extra code size while removing loop overhead from throughput
// sizes.
avx2_large:
	MOVQ BX, R10
	ANDQ $31, BX
	MOVQ R10, CX
	SHRQ $5, CX
	LEAQ -32(SI)(R10*1), R9 // final chunk for non-zero tail

	PCALIGN $32

avx2_large_loop4:
	CMPQ     CX, $4
	JLT      avx2_large_remainder
	VMOVDQU  (SI), Y2
	VPCMPEQB Y1, Y2, Y3
	VPTEST   Y3, Y3
	JNZ      avx2_success
	VMOVDQU  32(SI), Y2
	VPCMPEQB Y1, Y2, Y3
	VPTEST   Y3, Y3
	JNZ      avx2_success_32
	VMOVDQU  64(SI), Y2
	VPCMPEQB Y1, Y2, Y3
	VPTEST   Y3, Y3
	JNZ      avx2_success_64
	VMOVDQU  96(SI), Y2
	VPCMPEQB Y1, Y2, Y3
	VPTEST   Y3, Y3
	JNZ      avx2_success_96
	ADDQ     $128, SI
	SUBQ     $4, CX
	JMP      avx2_large_loop4

avx2_large_remainder:
	TESTQ CX, CX
	JEQ   avx2_large_tail

avx2_large_remainder_loop:
	VMOVDQU  (SI), Y2
	VPCMPEQB Y1, Y2, Y3
	VPTEST   Y3, Y3
	JNZ      avx2_success
	ADDQ     $32, SI
	DECQ     CX
	JNZ      avx2_large_remainder_loop

avx2_large_tail:
	TESTQ    BX, BX
	JEQ      avx2_fail_vzero
	MOVQ     R9, SI
	VMOVDQU  (SI), Y2
	VPCMPEQB Y1, Y2, Y3
	VPTEST   Y3, Y3
	JNZ      avx2_success
	JMP      avx2_fail_vzero

avx2_success_32:
	ADDQ $32, SI
	JMP  avx2_success

avx2_success_64:
	ADDQ $64, SI
	JMP  avx2_success

avx2_success_96:
	ADDQ $96, SI
	JMP  avx2_success

avx2_success:
	VPMOVMSKB Y3, DX
	BSFL      DX, DX
	SUBQ      R8, SI
	ADDQ      SI, DX
	MOVQ      DX, ret+32(FP)
	VZEROUPPER
	RET

avx2_scalar_success:
	SUBQ R8, SI
	MOVQ SI, ret+32(FP)
	VZEROUPPER
	RET
