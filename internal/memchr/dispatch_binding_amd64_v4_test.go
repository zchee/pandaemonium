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

//go:build amd64 && amd64.v4 && goexperiment.simd && !force_swar

package memchr

import "testing"

// expectedBackend on the GOAMD64=v4 artifact is mixed during the staged rollout:
// Memchr is AVX-512, while unconverted routines stay on AVX2.
func expectedBackend(t *testing.T) string {
	t.Helper()
	return "mixed-v4"
}

func expectedFunctionBackends(t *testing.T) backendMarkers {
	t.Helper()
	return backendMarkers{
		memchr:   "avx512-v4",
		memchr2:  "avx2-v4",
		memchr3:  "avx2-v4",
		memrchr:  "avx2-v4",
		memrchr2: "avx2-v4",
		memrchr3: "avx2-v4",
	}
}
