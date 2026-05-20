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
// Dispatch is build-tag-first. amd64 SIMD uses separate GOAMD64 artifact
// lanes, with one legacy runtime selection for v1/v2 builds:
//
//	GOARCH / level       goexperiment.simd  force_swar  backend (file)
//	amd64 GOAMD64=v4     ON                 OFF         AVX-512 primary artifact
//	amd64 GOAMD64=v3     ON                 OFF         AVX2 fallback artifact
//	amd64 GOAMD64=v1/v2  ON                 OFF         SSE2 or AVX2 via archsimd.X86.AVX2()
//	amd64                OFF                OFF         SWAR        (dispatch_swar_default.go)
//	amd64                ANY                ON          SWAR        (dispatch_swar_default.go)
//	arm64                ANY                OFF         NEON        (memchr_arm64.go)
//	arm64                ANY                ON          SWAR        (dispatch_swar_default.go)
//	other                ANY                ANY         SWAR        (dispatch_swar_default.go)
//
// All per-arch backends are in place: the amd64 SIMD files bind the v4/v3/
// legacy amd64 slots, memchr_arm64.go binds the arm64 slot (NEON), and
// dispatch_swar_default.go binds everything else.
//
// # CPU detection on amd64
//
// For local amd64 artifact selection, use simd/archsimd: AVX-512 gates the
// GOAMD64=v4 artifact, and AVX2 gates the GOAMD64=v3 fallback. A v4 binary
// cannot runtime-fallback on a v3-only CPU; fallback is a separate artifact.
// Legacy amd64 v1/v2 SIMD builds still choose AVX2 or SSE2 at init() time
// through archsimd.X86.AVX2(). Setting GODEBUG=cpu.avx2=off downgrades that
// legacy dispatcher to SSE2 even on AVX2-capable hardware. AC-HARNESS-7's
// TestBackendBinding fails in CI when this slip happens silently, so
// misconfigured runners cannot quietly degrade the perf gate.
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
// rationale. wasm32 SIMD, AVO codegen, cgo, and adding new AVX-512 routine
// shapes beyond the current byte-search API without a benchmark-backed plan
// are explicit non-goals.
package memchr
