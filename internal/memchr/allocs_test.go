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
	"fmt"
	"testing"
)

// allocSizes covers the boundary regimes every backend touches: empty
// haystack, sub-vector lengths (1, 15, 31), exact vector widths
// (16, 32), one-past widths (17, 33), the typical cache-line span (64),
// and a large length (8192) that drives the chunk loop hard.
var allocSizes = []int{0, 1, 15, 16, 17, 31, 32, 33, 63, 64, 127, 128, 8192}

// TestZeroAllocs asserts that every public byte-search function
// allocates exactly zero bytes per call across the canonical size table
// on both the "hit" (planted needle at the last byte) and "miss" (no
// match) paths. This satisfies AC-API-6 and is the canonical guardrail
// against an inadvertent escape-analysis regression in the dispatcher
// or any backend's tail handler.
//
// testing.AllocsPerRun panics if called while any parallel test is
// running (it sets GOMAXPROCS to 1 for the measurement). This test
// therefore does NOT call t.Parallel() — Go's test runner schedules
// every serial test before any parallel test starts, so by the time
// this test runs no other goroutine is racing it for allocator state.
// Subtests also stay serial.
func TestZeroAllocs(t *testing.T) {
	for _, n := range allocSizes {
		hit := make([]byte, n)
		if n > 0 {
			hit[n-1] = 0xC3 // plant at end so forward scans walk full length
		}
		miss := make([]byte, n) // all zeros, no 0xC3

		for _, tc := range []struct {
			name string
			hay  []byte
		}{
			{"hit", hit},
			{"miss", miss},
		} {
			t.Run(fmt.Sprintf("Memchr/%s/n=%d", tc.name, n), func(t *testing.T) {
				assertNoAllocs(t, "Memchr", func() {
					_ = Memchr(0xC3, tc.hay)
				})
			})
			t.Run(fmt.Sprintf("Memchr2/%s/n=%d", tc.name, n), func(t *testing.T) {
				assertNoAllocs(t, "Memchr2", func() {
					_ = Memchr2(0xC3, 0xD4, tc.hay)
				})
			})
			t.Run(fmt.Sprintf("Memchr3/%s/n=%d", tc.name, n), func(t *testing.T) {
				assertNoAllocs(t, "Memchr3", func() {
					_ = Memchr3(0xC3, 0xD4, 0xE5, tc.hay)
				})
			})
			t.Run(fmt.Sprintf("Memrchr/%s/n=%d", tc.name, n), func(t *testing.T) {
				assertNoAllocs(t, "Memrchr", func() {
					_ = Memrchr(0xC3, tc.hay)
				})
			})
			t.Run(fmt.Sprintf("Memrchr2/%s/n=%d", tc.name, n), func(t *testing.T) {
				assertNoAllocs(t, "Memrchr2", func() {
					_ = Memrchr2(0xC3, 0xD4, tc.hay)
				})
			})
			t.Run(fmt.Sprintf("Memrchr3/%s/n=%d", tc.name, n), func(t *testing.T) {
				assertNoAllocs(t, "Memrchr3", func() {
					_ = Memrchr3(0xC3, 0xD4, 0xE5, tc.hay)
				})
			})
			t.Run(fmt.Sprintf("IndexByte/%s/n=%d", tc.name, n), func(t *testing.T) {
				assertNoAllocs(t, "IndexByte", func() {
					_ = IndexByte(tc.hay, 0xC3)
				})
			})
			t.Run(fmt.Sprintf("LastIndexByte/%s/n=%d", tc.name, n), func(t *testing.T) {
				assertNoAllocs(t, "LastIndexByte", func() {
					_ = LastIndexByte(tc.hay, 0xC3)
				})
			})
		}
	}
}

// assertNoAllocs fails when the supplied function allocates anything on
// the heap across 100 iterations. We use AllocsPerRun rather than
// testing.B's b.ReportAllocs so the test runs in the standard `go test`
// pass (not just under `-bench=.`).
func assertNoAllocs(t *testing.T, name string, fn func()) {
	t.Helper()
	if got := testing.AllocsPerRun(100, fn); got != 0 {
		t.Fatalf("%s: AllocsPerRun = %v, want 0", name, got)
	}
}
