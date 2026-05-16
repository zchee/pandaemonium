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

import (
	"runtime"
	"testing"
)

// TestBackendBinding asserts that the *Impl funcptrs were bound to the
// expected backend at init() time (AC-HARNESS-7 in plan). The "expected"
// side is computed per build-tag combination by the per-tag helper
// dispatch_binding_*_test.go files, each of which defines exactly one
// expectedBackend(t) string. Exactly one such helper compiles per
// (arch, goexperiment.simd, force_swar) tuple.
//
// This is the silent-downgrade trap: an AVX2-capable CI runner that
// somehow ended up bound to SSE2 (CPU-detect regression,
// GODEBUG=cpu.avx2=off slip, archsimd API rename inverting the bool)
// would still pass the oracle suite but fail here, BEFORE the perf gate
// runs.
//
// boundImpl is an unexported package-level var declared in dispatch.go;
// reading it directly from this same-package _test.go file is the
// standard Go test-only access pattern and requires no special build
// tag (per plan §"AC-HARNESS-7 (locked)" L130).
func TestBackendBinding(t *testing.T) {
	t.Parallel()
	want := expectedBackend(t)
	if boundImpl != want {
		t.Fatalf("AC-HARNESS-7: backend binding mismatch — boundImpl=%q want=%q "+
			"(GOARCH=%s); a silent downgrade in the dispatcher would slip past "+
			"the oracle suite but is caught here. Likely causes: cpu-detect "+
			"regression in archsimd.X86.AVX2(), GODEBUG=cpu.avx2=off slipping "+
			"into CI, archsimd API rename inverting the bool, or NEON stubs "+
			"not binding on arm64.",
			boundImpl, want, runtime.GOARCH)
	}
	t.Logf("backend binding OK: boundImpl=%q on GOARCH=%s", boundImpl, runtime.GOARCH)
}
