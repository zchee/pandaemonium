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

//go:build memchr_tuning

package memchr

import (
	"fmt"
	"slices"
	"testing"
)

var (
	tuningResultSink int
	tuningScalarSink uint64
)

type tuningBenchmarkCase struct {
	name    string
	prepare func([]byte)
}

func TestTuningBenchmarkCases(t *testing.T) {
	t.Parallel()

	cases := tuningBenchmarkCases()
	if got, want := len(cases), 4; got != want {
		t.Fatalf("tuningBenchmarkCases() len = %d, want %d", got, want)
	}
	wantNames := []string{"miss", "hit_first", "hit_middle", "hit_last"}
	for i, tc := range cases {
		if tc.name != wantNames[i] {
			t.Fatalf("tuningBenchmarkCases()[%d].name = %q, want %q", i, tc.name, wantNames[i])
		}
	}

	hay := benchHaystack(5)
	cases[1].prepare(hay)
	if got, want := Memchr(benchNeedle, hay), 0; got != want {
		t.Fatalf("hit_first prepare placed needle at %d, want %d", got, want)
	}
	hay = benchHaystack(5)
	cases[2].prepare(hay)
	if got, want := Memchr(benchNeedle, hay), 2; got != want {
		t.Fatalf("hit_middle prepare placed needle at %d, want %d", got, want)
	}
	hay = benchHaystack(5)
	cases[3].prepare(hay)
	if got, want := Memchr(benchNeedle, hay), 4; got != want {
		t.Fatalf("hit_last prepare placed needle at %d, want %d", got, want)
	}
}

func TestTuningBenchSizes(t *testing.T) {
	t.Parallel()

	sizes := tuningBenchSizes()
	if !slices.IsSorted(sizes) {
		t.Fatalf("tuningBenchSizes() is not sorted: %v", sizes)
	}
	seen := map[int]bool{}
	for _, n := range sizes {
		if seen[n] {
			t.Fatalf("tuningBenchSizes() contains duplicate size %d: %v", n, sizes)
		}
		seen[n] = true
	}
	for _, want := range []int{0, 1, 16, 31, 32, 33, 63, 64, 65, 127, 128, 129, 1023, 1024, 1025, 65536, 1048576} {
		if !seen[want] {
			t.Fatalf("tuningBenchSizes() missing %d: %v", want, sizes)
		}
	}
}

func TestTuningScalarWork(t *testing.T) {
	t.Parallel()

	hay := []byte{1, 2, 3, 4}
	// Each byte is added with the low-five-bit index contribution.
	if got, want := tuningScalarWork(hay), uint64(16); got != want {
		t.Fatalf("tuningScalarWork(%v) = %d, want %d", hay, got, want)
	}
}

func BenchmarkTuningMemchr(b *testing.B) {
	cases := tuningBenchmarkCases()
	runTuningScan(b, "public", cases, func(hay []byte) int {
		return Memchr(benchNeedle, hay)
	})
	benchmarkTuningDirectAVX512Memchr(b, cases)
}

func BenchmarkTuningMemchr2(b *testing.B) {
	cases := tuningBenchmarkCases()
	runTuningScan(b, "public", cases, func(hay []byte) int {
		return Memchr2(benchNeedle, 0xC3, hay)
	})
	benchmarkTuningDirectAVX512Memchr2(b, cases)
}

func BenchmarkTuningMemchr3(b *testing.B) {
	cases := tuningBenchmarkCases()
	runTuningScan(b, "public", cases, func(hay []byte) int {
		return Memchr3(benchNeedle, 0xC3, 0xD4, hay)
	})
	benchmarkTuningDirectAVX512Memchr3(b, cases)
}

func BenchmarkTuningMemrchr(b *testing.B) {
	cases := tuningBenchmarkCases()
	runTuningScan(b, "public", cases, func(hay []byte) int {
		return Memrchr(benchNeedle, hay)
	})
	benchmarkTuningDirectAVX512Memrchr(b, cases)
}

func BenchmarkTuningMemrchr2(b *testing.B) {
	cases := tuningBenchmarkCases()
	runTuningScan(b, "public", cases, func(hay []byte) int {
		return Memrchr2(benchNeedle, 0xC3, hay)
	})
	benchmarkTuningDirectAVX512Memrchr2(b, cases)
}

func BenchmarkTuningMemrchr3(b *testing.B) {
	cases := tuningBenchmarkCases()
	runTuningScan(b, "public", cases, func(hay []byte) int {
		return Memrchr3(benchNeedle, 0xC3, 0xD4, hay)
	})
	benchmarkTuningDirectAVX512Memrchr3(b, cases)
}

func BenchmarkTuningMixedMemchr(b *testing.B) {
	for _, n := range []int{64, 256, 1024, 4096, 65536} {
		hay := benchHaystack(n)
		b.Run(fmt.Sprintf("public/scalar_interleave/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			var result int
			var scalar uint64
			for b.Loop() {
				result ^= Memchr(benchNeedle, hay)
				scalar += tuningScalarWork(hay)
			}
			tuningResultSink = result
			tuningScalarSink = scalar
		})
	}
}

func tuningBenchmarkCases() []tuningBenchmarkCase {
	return []tuningBenchmarkCase{
		{name: "miss"},
		{name: "hit_first", prepare: func(hay []byte) {
			if len(hay) > 0 {
				hay[0] = benchNeedle
			}
		}},
		{name: "hit_middle", prepare: func(hay []byte) {
			if len(hay) > 0 {
				hay[len(hay)/2] = benchNeedle
			}
		}},
		{name: "hit_last", prepare: func(hay []byte) {
			if len(hay) > 0 {
				hay[len(hay)-1] = benchNeedle
			}
		}},
	}
}

func runTuningScan(b *testing.B, variant string, cases []tuningBenchmarkCase, body func([]byte) int) {
	for _, tc := range cases {
		for _, n := range tuningBenchSizes() {
			hay := benchHaystack(n)
			if tc.prepare != nil {
				tc.prepare(hay)
			}
			b.Run(fmt.Sprintf("%s/%s/n=%d", variant, tc.name, n), func(b *testing.B) {
				b.SetBytes(int64(n))
				b.ReportAllocs()
				var result int
				for b.Loop() {
					result ^= body(hay)
				}
				tuningResultSink = result
			})
		}
	}
}

func tuningBenchSizes() []int {
	seen := map[int]bool{}
	var sizes []int
	add := func(n int) {
		if n < 0 || seen[n] {
			return
		}
		seen[n] = true
		sizes = append(sizes, n)
	}
	for _, n := range []int{0, 1, 2, 3, 7, 8, 15, 16} {
		add(n)
	}
	for _, center := range []int{32, 48, 64, 96, 128, 192, 256, 512, 1024, 2048, 4096, 8192, 16384} {
		add(center - 1)
		add(center)
		add(center + 1)
	}
	add(65536)
	add(1048576)
	slices.Sort(sizes)
	return sizes
}

func tuningScalarWork(hay []byte) uint64 {
	var acc uint64
	for i, c := range hay {
		acc += uint64(c) + uint64(i&31)
	}
	return acc
}
