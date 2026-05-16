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

//go:build (!amd64 && !arm64) || force_swar || (amd64 && !goexperiment.simd)

package memchr

// init is the PERMANENT binder for the SWAR fallback. It owns these three
// (arch, goexperiment.simd, force_swar) tuples (plan §"Tuple-coverage audit"
// L154-162):
//
//   - (other-GOARCH, *, *)             — !amd64 && !arm64
//   - (amd64,  ON,  force_swar=ON)     — force_swar
//   - (amd64,  OFF, force_swar=OFF)    — amd64 && !goexperiment.simd
//   - (amd64,  OFF, force_swar=ON)     — force_swar
//   - (arm64,  *,   force_swar=ON)     — force_swar
//
// The amd64-with-SIMD and arm64-no-force_swar slots are owned by
// dispatch_default_init.go (transitional, Steps 2-5) and then by
// memchr_amd64.go / memchr_arm64.go (Steps 4-6). The tags are mutually
// exclusive at every commit.
func init() {
	memchrImpl = swarMemchr
	memchr2Impl = swarMemchr2
	memchr3Impl = swarMemchr3
	memrchrImpl = swarMemrchr
	memrchr2Impl = swarMemrchr2
	memrchr3Impl = swarMemrchr3
	boundImpl = "swar"
}
