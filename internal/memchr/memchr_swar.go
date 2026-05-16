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

//go:build (!amd64 && !arm64) || force_swar

package memchr

// This file is the per-plan anchor for the SWAR-as-dispatched-backend build
// slot ((!amd64 && !arm64) || force_swar). It carries no executable code:
// the actual SWAR routines live in memchr_swar_impl.go behind the broader
// tag ((!amd64 && !arm64) || force_swar || (amd64 && !goexperiment.simd))
// so that the amd64-no-SIMD binding in Step 4 (dispatch_swar_default.go) can
// link to swarMemchr..swarMemrchr3 without duplicating the bodies across two
// files. The file split mirrors plan §"File Tree" lines 297–300; keeping
// this anchor compiled on the narrower tag preserves a documented place to
// land bench-time variants of the SWAR loop that should only be available
// when SWAR is the dispatched backend (not when SWAR is merely an
// amd64-no-SIMD fallback). The first such variant (a binary.LittleEndian
// word-load comparison) lives in bench_swar_test.go.
