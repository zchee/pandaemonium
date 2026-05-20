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
//   - dispatch_swar_default.go — SWAR fallback for other-GOARCH, force_swar,
//     and amd64-no-SIMD slots.
//   - memchr_amd64_legacy.go   — GOAMD64=v1/v2 SSE2 or AVX2 chosen at
//     init() time from archsimd.X86.AVX2().
//   - memchr_amd64_v3.go       — GOAMD64=v3 AVX2 fallback artifact.
//   - memchr_amd64_v4.go       — GOAMD64=v4 AVX-512 Memchr primary artifact.
//   - memchr_arm64.go          — NEON.
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

// boundImpl names the aggregate backend selected at init() time. It is read
// by TestBackendBinding (Step 7, AC-HARNESS-7) so a CPU-detect regression (or
// a GODEBUG=cpu.avx2=off slip in CI) cannot silently downgrade to a slower
// backend while still passing the oracle suite. Mixed staged rollouts may set
// boundImpl to a mixed marker, while the per-function markers below report the
// real implementation behind each public routine.
var boundImpl string

// Per-function backend markers keep staged amd64 rollouts honest. In
// particular, GOAMD64=v4 binds Memchr to AVX-512 before the multi-needle and
// reverse routines are converted, so the old aggregate marker alone would be
// misleading.
var (
	boundMemchrImpl   string
	boundMemchr2Impl  string
	boundMemchr3Impl  string
	boundMemrchrImpl  string
	boundMemrchr2Impl string
	boundMemrchr3Impl string
)

func setBackendMarkers(aggregate, memchr, memchr2, memchr3, memrchr, memrchr2, memrchr3 string) {
	boundImpl = aggregate
	boundMemchrImpl = memchr
	boundMemchr2Impl = memchr2
	boundMemchr3Impl = memchr3
	boundMemrchrImpl = memrchr
	boundMemrchr2Impl = memrchr2
	boundMemrchr3Impl = memrchr3
}
