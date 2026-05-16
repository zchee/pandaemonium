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

//go:build (amd64 && goexperiment.simd && !force_swar) || (arm64 && !force_swar)

package memchr

// init is the TRANSITIONAL binder for the amd64-with-SIMD and arm64 build
// slots. It exists only between Steps 2 and 6 of the plan and keeps the
// commit train building green while real SSE2/AVX2 (Step 4-5) and NEON
// (Step 6) backends are still in flight. Both currently bind to SWAR.
//
// The two-disjunct tag is mutually exclusive with dispatch_swar_default.go's
// tag (plan §"Tuple-coverage audit" L154-162):
//
//   - The !force_swar clauses on both disjuncts keep this file out of every
//     force_swar slot, which is owned by dispatch_swar_default.go.
//   - The goexperiment.simd clause on the amd64 disjunct keeps it out of the
//     amd64-no-SIMD slot, which is also owned by dispatch_swar_default.go.
//
// Step 4 will narrow this file's tag from
// `(amd64 && goexperiment.simd && !force_swar) || (arm64 && !force_swar)`
// to `arm64 && !force_swar` (amd64-with-SIMD then owned by memchr_amd64.go).
// Step 6 deletes the file entirely (arm64 then owned by memchr_arm64.go).
func init() {
	memchrImpl = swarMemchr
	memchr2Impl = swarMemchr2
	memchr3Impl = swarMemchr3
	memrchrImpl = swarMemrchr
	memrchr2Impl = swarMemrchr2
	memrchr3Impl = swarMemrchr3
	boundImpl = "swar"
}
