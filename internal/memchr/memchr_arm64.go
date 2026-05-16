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

package memchr

// Six hand-written NEON assembly routines back the dispatched impls on arm64.
// Each .s file follows the stdlib syndrome-trick recipe modelled directly on
// /opt/local/go.simd/src/internal/bytealg/indexbyte_arm64.s:36-70:
//
//   VMOV    R, V0.B16          // broadcast needle to all 16 lanes
//   VLD1.P  (R), [V1, V2]      // load two consecutive 16-byte chunks
//   VCMEQ   V0, V1, V3         // per-lane equality (0xFF on match)
//   VAND    V5, V3, V3         // mask each lane to its syndrome bit
//                              // (V5 = 0x40100401 broadcast, lanes 0..3
//                              // contribute bits 0,2,4,6 within the byte)
//   VADDP   V4, V3, V6         // horizontal pairwise add 256->128
//   VADDP   V6, V6, V6         // and 128->64
//   VMOV    V6.D[0], R         // extract syndrome to GPR
//   RBIT+CLZ                   // forward: trailing-zero count → first match
//   CLZ alone                  // reverse: leading-zero count → last match
//
// The shared control flow is aligned-up rather than the stdlib aligned-down:
// the head 0..31 bytes are scanned byte-at-a-time to advance the pointer to
// a 32-byte boundary, then aligned chunks are walked, then the trailing
// <32-byte tail is scanned byte-at-a-time. Stdlib's aligned-down trick
// reads up to 31 bytes BEFORE the haystack start (masked off via R9) which
// can fault near page boundaries; the aligned-up variant trades a slightly
// slower head loop for SEGV-safety on every Linux/darwin/iOS slice.
//
// Per-routine register layout (Go ABIInternal):
//   memchrNEON(needle byte, haystack []byte) int
//     R0 = needle  R1 = ptr  R2 = len  R3 = cap
//   memchr2NEON(n1, n2 byte, haystack []byte) int
//     R0 = n1      R1 = n2   R2 = ptr  R3 = len  R4 = cap
//   memchr3NEON(n1, n2, n3 byte, haystack []byte) int
//     R0 = n1      R1 = n2   R2 = n3   R3 = ptr  R4 = len  R5 = cap
//   memrchrNEON / memrchr2NEON / memrchr3NEON have the same arg layout.

//go:noescape
func memchrNEON(needle byte, haystack []byte) int

//go:noescape
func memchr2NEON(n1, n2 byte, haystack []byte) int

//go:noescape
func memchr3NEON(n1, n2, n3 byte, haystack []byte) int

//go:noescape
func memrchrNEON(needle byte, haystack []byte) int

//go:noescape
func memrchr2NEON(n1, n2 byte, haystack []byte) int

//go:noescape
func memrchr3NEON(n1, n2, n3 byte, haystack []byte) int

// init binds the dispatcher pointers to the NEON implementations and sets
// boundImpl = "neon". Because arm64 ABI guarantees ASIMD (NEON) on every
// AArch64 box that runs Go, no runtime feature detect is needed — pure
// build-tag dispatch suffices (plan §"Decision Drivers" item 2).
//
// At Step 6's commit the transitional file dispatch_default_init.go is
// DELETED entirely; the arm64 binding now lives here. See plan §"Step 6"
// L189-194.
func init() {
	memchrImpl = memchrNEON
	memchr2Impl = memchr2NEON
	memchr3Impl = memchr3NEON
	memrchrImpl = memrchrNEON
	memrchr2Impl = memrchr2NEON
	memrchr3Impl = memrchr3NEON
	boundImpl = "neon"
}
