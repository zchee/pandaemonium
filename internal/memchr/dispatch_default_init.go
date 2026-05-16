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

// init is the TRANSITIONAL binder for the arm64 build slot only — Step 4
// narrowed the tag from `(amd64 && goexperiment.simd && !force_swar) ||
// (arm64 && !force_swar)` to `arm64 && !force_swar` because
// memchr_amd64.go now owns the amd64-with-SIMD slot. The arm64 binding
// continues to fall through to SWAR until Step 6 lands the real NEON
// backend in memchr_arm64.go, at which point this file is deleted.
//
// The remaining disjunct stays mutually exclusive with
// dispatch_swar_default.go's tag (plan §"Tuple-coverage audit" L154-162):
// the `!force_swar` clause keeps this file out of the (arm64, force_swar)
// slot, which is owned by dispatch_swar_default.go's `force_swar` clause.
func init() {
	memchrImpl = swarMemchr
	memchr2Impl = swarMemchr2
	memchr3Impl = swarMemchr3
	memrchrImpl = swarMemrchr
	memrchr2Impl = swarMemrchr2
	memrchr3Impl = swarMemrchr3
	boundImpl = "swar"
}
