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

package memchr

// memchrImpl..memrchr3Impl are the dispatched backends. Exactly one file in
// this package binds each var in its init():
//
//   - dispatch_swar_default.go    (permanent)  — SWAR fallback for
//     other-GOARCH, force_swar, and amd64-no-SIMD slots.
//   - dispatch_default_init.go    (transitional, deleted in Step 6) — SWAR
//     for amd64-with-SIMD and arm64 until Steps 4-6 land real backends.
//   - memchr_amd64.go             (Step 4+5)  — SSE2 or AVX2 chosen at
//     init() time from archsimd.X86.AVX2().
//   - memchr_arm64.go             (Step 6)    — NEON.
//
// The commit-train invariant is: for every (arch, goexperiment.simd,
// force_swar) tuple at every HEAD, exactly one file binds each *Impl var.
// Plan §"Tuple-coverage audit" L154-162 walks the table.
//
// The vars are package-level (not build-tag-only direct funcs) because amd64
// needs runtime AVX2-vs-SSE2 selection, which pure build tags cannot
// express. The cost is one indirect call per Memchr (R-INDIRECT-COST in the
// plan); the wrapper around each var stays one-line so the inliner still
// promotes the wrapper into its caller.
var (
	memchrImpl   func(needle byte, haystack []byte) int
	memchr2Impl  func(n1, n2 byte, haystack []byte) int
	memchr3Impl  func(n1, n2, n3 byte, haystack []byte) int
	memrchrImpl  func(needle byte, haystack []byte) int
	memrchr2Impl func(n1, n2 byte, haystack []byte) int
	memrchr3Impl func(n1, n2, n3 byte, haystack []byte) int
)

// boundImpl names the backend selected at init() time. It is read by
// TestBackendBinding (Step 7, AC-HARNESS-7) so a CPU-detect regression (or a
// GODEBUG=cpu.avx2=off slip in CI) cannot silently downgrade to a slower
// backend while still passing the oracle suite. Per-backend init() writers
// must set boundImpl to exactly one of "avx2", "sse2", "neon", or "swar".
var boundImpl string
