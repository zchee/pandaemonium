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

#include "textflag.h"

// func avx512Memchr(needle byte, haystack []byte) int
// Frame: 0; arg/return area: 40 bytes
//   needle  +0(FP) (byte, padded to 8)
//   hay ptr +8(FP)
//   hay len +16(FP)
//   hay cap +24(FP) (unused)
//   ret     +32(FP) (int)
//
// LLVM 20 EVEX validation sketch:
//   vpbroadcastb %xmm0, %zmm1
//   vmovdqu8 (%rsi), %zmm2
//   vpcmpeqb %zmm1, %zmm2, %k1
//   kortestq %k1, %k1
//   kmovq %k1, %rdx
//   tzcntq %rdx, %rdx
//
// The loop performs only in-bounds 64-byte vector loads. For len >= 64 it
// finishes with the last in-bounds 64-byte chunk, which may overlap earlier
// chunks but never crosses outside the slice object. This removes scalar-tail
// overhead from the full-scan path without needing a guard-page exception.
TEXT ·avx512Memchr(SB), NOSPLIT, $0-40
	MOVBLZX needle+0(FP), AX
	MOVQ    haystack_base+8(FP), SI
	MOVQ    haystack_len+16(FP), BX
	TESTQ   BX, BX
	JEQ     avx512_fail

	MOVQ SI, R8        // base pointer for offset calculation
	CMPQ BX, $64
	JLT  avx512_scalar

	MOVD         AX, X0
	VPBROADCASTB X0, Z1
	LEAQ         -64(SI)(BX*1), R9 // start of final in-bounds 64-byte chunk
	JMP          avx512_loop_entry

	PCALIGN $32

avx512_loop:
	VMOVDQU8 (SI), Z2
	VPCMPEQB Z1, Z2, K1
	KORTESTQ K1, K1
	JNZ      avx512_success
	ADDQ     $64, SI

avx512_loop_entry:
	CMPQ SI, R9
	JLT  avx512_loop

	MOVQ     R9, SI
	VMOVDQU8 (SI), Z2
	VPCMPEQB Z1, Z2, K1
	KORTESTQ K1, K1
	JNZ      avx512_success
	JMP      avx512_fail_vzero

avx512_scalar:
	CMPB (SI), AL
	JEQ  avx512_scalar_success
	INCQ SI
	DECQ BX
	JNZ  avx512_scalar

avx512_fail_vzero:
	VZEROUPPER

avx512_fail:
	MOVQ $-1, ret+32(FP)
	RET

avx512_success:
	KMOVQ  K1, DX
	TZCNTQ DX, DX
	SUBQ   R8, SI
	ADDQ   SI, DX
	MOVQ   DX, ret+32(FP)
	VZEROUPPER
	RET

avx512_scalar_success:
	SUBQ R8, SI
	MOVQ SI, ret+32(FP)
	VZEROUPPER
	RET
