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

// Package memchr provides byte-search primitives ported from
// https://github.com/BurntSushi/memchr (Rust crate, version =2.7.4). The
// public API exposes twelve functions split into two argument-order shapes:
//
//   - Canonical, needle-first forms: Memchr, Memchr2, Memchr3 (forward
//     scans, return the first matching offset) and Memrchr, Memrchr2,
//     Memrchr3 (reverse scans, return the last matching offset).
//   - bytes-package wrappers, haystack-first: IndexByte, IndexByte2,
//     IndexByte3, LastIndexByte, LastIndexByte2, LastIndexByte3. These are
//     thin shims that delegate to the canonical impls, matching the
//     bytes.IndexByte argument order so callers can swap idioms freely.
//
// Every function returns -1 when no byte matches, matching the
// bytes.IndexByte sentinel convention.
//
// # Build-tag matrix
//
// Dispatch is build-tag-first, with one runtime selection on amd64:
//
//	GOARCH    goexperiment.simd  force_swar  backend (file)
//	amd64     ON                 OFF         SSE2 or AVX2 (memchr_amd64.go;  Step 4+5)
//	amd64     OFF                OFF         SWAR        (dispatch_swar_default.go)
//	amd64     ANY                ON          SWAR        (dispatch_swar_default.go)
//	arm64     ANY                OFF         NEON        (memchr_arm64.go;   Step 6)
//	arm64     ANY                ON          SWAR        (dispatch_swar_default.go)
//	other     ANY                ANY         SWAR        (dispatch_swar_default.go)
//
// Until Steps 4-6 land the per-arch SIMD/NEON backends, the transitional
// file dispatch_default_init.go binds the amd64-with-SIMD and arm64 slots
// to SWAR so the commit train stays green at every HEAD. Step 4 narrows
// that file to arm64-only; Step 6 deletes it.
//
// # CPU detection on amd64
//
// On amd64-with-SIMD the chosen backend at init() time depends on
// archsimd.X86.AVX2(). Setting GODEBUG=cpu.avx2=off downgrades the
// dispatcher to SSE2 even on AVX2-capable hardware. AC-HARNESS-7's
// TestBackendBinding (Step 7) fails in CI when this slip happens silently,
// so misconfigured runners cannot quietly degrade the perf gate.
//
// To intentionally exercise the SWAR fallback on amd64 or arm64 hardware
// (e.g. for parity testing or to compare backends on identical hardware),
// build with -tags=force_swar.
//
// # Non-goals
//
// Memmem (substring search), iterator types, and stateful Memchr* objects
// from the upstream Rust crate are deliberately out of scope; see the
// project spec deep-interview-port-burntsushi-memchr-to-go.md for the
// rationale. AVX-512, wasm32 SIMD, AVO codegen, cgo, and a hard
// GOAMD64=v3 floor are explicit non-goals.
package memchr
