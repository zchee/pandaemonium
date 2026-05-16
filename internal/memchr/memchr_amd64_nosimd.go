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

//go:build amd64 && !goexperiment.simd && !force_swar

package memchr

// This file is the documented anchor for the amd64-no-SIMD build slot
// (`amd64 && !goexperiment.simd && !force_swar`). It carries no executable
// code: the *Impl bindings on this slot are owned by
// dispatch_swar_default.go, whose tag's `(amd64 && !goexperiment.simd)`
// disjunct covers exactly this configuration. The SWAR routines that the
// dispatch_swar_default.go init() binds to live in memchr_swar_impl.go,
// which has no build tag and is therefore compiled here too.
//
// Keeping this anchor file means a future contributor adding a non-SWAR-
// but-non-SIMD amd64 path (e.g. AVO-generated SSE2 without the
// goexperiment.simd gate) has an obvious file to edit. See plan §"File
// Tree" L316-317 and §"Step 4" L178.
