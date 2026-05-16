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
	"bytes"
	"fmt"
	"testing"
)

// Benchmark sizes per plan AC-HARNESS-5: covers the boundary regimes the
// per-backend tail handlers actually touch (n=16 = exactly-one SSE2
// vector / one SWAR word + tail), and the canonical realistic sizes
// (1k / 4k / 64k) that the perf gate cares about.
var benchSizes = []int{16, 64, 256, 1024, 4096, 65536}

// benchNeedle is intentionally a byte that never appears in benchHaystack
// — the inner loop walks the whole length without short-circuiting on a
// match, so the bench measures the worst-case full-scan path.
const benchNeedle byte = 0xff

// benchHaystack returns a length-n buffer filled with `i & 0x7f`, so
// neither the benchmark needle (0xff) nor 0xC3 / 0xD4 / 0xE5 (the
// multi-needle benchmark needles) ever appears.
func benchHaystack(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i & 0x7f)
	}
	return b
}

// runScan runs body across every benchSizes entry. Calling b.SetBytes
// makes `go test -bench` report MB/s alongside ns/op, which is what
// makes the comparison vs bytes.IndexByte legible.
func runScan(b *testing.B, body func(b *testing.B, hay []byte)) {
	for _, n := range benchSizes {
		hay := benchHaystack(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.SetBytes(int64(n))
			body(b, hay)
		})
	}
}

func BenchmarkMemchr(b *testing.B) {
	runScan(b, func(b *testing.B, hay []byte) {
		for b.Loop() {
			_ = Memchr(benchNeedle, hay)
		}
	})
}

func BenchmarkMemchr2(b *testing.B) {
	runScan(b, func(b *testing.B, hay []byte) {
		for b.Loop() {
			_ = Memchr2(benchNeedle, 0xC3, hay)
		}
	})
}

func BenchmarkMemchr3(b *testing.B) {
	runScan(b, func(b *testing.B, hay []byte) {
		for b.Loop() {
			_ = Memchr3(benchNeedle, 0xC3, 0xD4, hay)
		}
	})
}

func BenchmarkMemrchr(b *testing.B) {
	runScan(b, func(b *testing.B, hay []byte) {
		for b.Loop() {
			_ = Memrchr(benchNeedle, hay)
		}
	})
}

func BenchmarkMemrchr2(b *testing.B) {
	runScan(b, func(b *testing.B, hay []byte) {
		for b.Loop() {
			_ = Memrchr2(benchNeedle, 0xC3, hay)
		}
	})
}

func BenchmarkMemrchr3(b *testing.B) {
	runScan(b, func(b *testing.B, hay []byte) {
		for b.Loop() {
			_ = Memrchr3(benchNeedle, 0xC3, 0xD4, hay)
		}
	})
}

// BenchmarkIndexByteStd is the stdlib baseline that the perf-gate
// (hack/memchr-perf-gate) compares against per AC-HARNESS-6.
// bytes.IndexByte is the architectural mirror of Memchr: forward
// single-needle byte scan, returning the first match index or -1.
func BenchmarkIndexByteStd(b *testing.B) {
	runScan(b, func(b *testing.B, hay []byte) {
		for b.Loop() {
			_ = bytes.IndexByte(hay, benchNeedle)
		}
	})
}
