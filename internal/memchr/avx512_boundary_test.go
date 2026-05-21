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

import (
	"fmt"
	"slices"
	"testing"

	simd "simd/archsimd"
)

func TestAVX512BoundaryHaystacks(t *testing.T) {
	t.Parallel()
	if !simd.X86.AVX512() {
		t.Skip("AVX-512 not available through simd/archsimd on this host")
	}

	const (
		n1 byte = 0xC3
		n2 byte = 0xD4
		n3 byte = 0xE5
	)
	for _, n := range avx512BoundaryLengths() {
		base := avx512BoundaryBase(n)
		assertAVX512BoundaryAll(t, fmt.Sprintf("n=%d miss", n), n1, n2, n3, base)
		for _, pos := range avx512BoundaryPositions(n) {
			hay := avx512BoundaryPlant(base, pos, n1)
			assertAVX512BoundaryAll(t, fmt.Sprintf("n=%d pos=%d", n, pos), n1, n2, n3, hay)
			assertAVX512BoundaryDuplicates(t, fmt.Sprintf("n=%d pos=%d duplicate", n, pos), n1, hay)
		}
		if n > 0 {
			hay := avx512BoundaryPlant(base, 0, n1)
			hay[n/2] = n2
			hay[n-1] = n3
			assertAVX512BoundaryAll(t, fmt.Sprintf("n=%d multi-hit", n), n1, n2, n3, hay)
		}
	}
}

func TestAVX512BoundaryGenerators(t *testing.T) {
	t.Parallel()

	lengths := avx512BoundaryLengths()
	if !slices.IsSorted(lengths) {
		t.Fatalf("avx512BoundaryLengths() is not sorted: %v", lengths)
	}
	seen := map[int]bool{}
	for _, n := range lengths {
		if seen[n] {
			t.Fatalf("avx512BoundaryLengths() contains duplicate length %d: %v", n, lengths)
		}
		seen[n] = true
	}
	for _, want := range []int{0, 1, 63, 64, 65, 127, 128, 129, 1023, 1024, 1025, 16383, 16384, 16385} {
		if !seen[want] {
			t.Fatalf("avx512BoundaryLengths() missing %d: %v", want, lengths)
		}
	}

	positions := avx512BoundaryPositions(128)
	wantPositions := []int{0, 1, 63, 64, 127}
	if !slices.Equal(positions, wantPositions) {
		t.Fatalf("avx512BoundaryPositions(128) = %v, want %v", positions, wantPositions)
	}

	base := avx512BoundaryBase(4)
	if got, want := base, []byte{0, 1, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("avx512BoundaryBase(4) = %v, want %v", got, want)
	}
	hay := avx512BoundaryPlant(base, 2, 0xC3)
	if got, want := hay, []byte{0, 1, 0xC3, 3}; !slices.Equal(got, want) {
		t.Fatalf("avx512BoundaryPlant(...) = %v, want %v", got, want)
	}
	if got, want := base, []byte{0, 1, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("avx512BoundaryPlant mutated base: got %v, want %v", got, want)
	}
}

func avx512BoundaryLengths() []int {
	seen := map[int]bool{}
	var lengths []int
	add := func(n int) {
		if n < 0 || seen[n] {
			return
		}
		seen[n] = true
		lengths = append(lengths, n)
	}
	for _, n := range []int{0, 1} {
		add(n)
	}
	for _, center := range []int{64, 96, 128, 192, 256, 512, 1024, 1536, 2048, 4096, 8192, 16384} {
		add(center - 1)
		add(center)
		add(center + 1)
	}
	slices.Sort(lengths)
	return lengths
}

func avx512BoundaryPositions(n int) []int {
	seen := map[int]bool{}
	var positions []int
	add := func(pos int) {
		if pos < 0 || pos >= n || seen[pos] {
			return
		}
		seen[pos] = true
		positions = append(positions, pos)
	}
	for _, pos := range []int{0, 1, 63, 64, n / 2, n - 65, n - 64, n - 1} {
		add(pos)
	}
	slices.Sort(positions)
	return positions
}

func avx512BoundaryBase(n int) []byte {
	base := make([]byte, n)
	for i := range base {
		base[i] = byte(i & 0x7f)
	}
	return base
}

func avx512BoundaryPlant(base []byte, pos int, needle byte) []byte {
	hay := append([]byte(nil), base...)
	hay[pos] = needle
	return hay
}

func assertAVX512BoundaryAll(t *testing.T, label string, n1, n2, n3 byte, hay []byte) {
	t.Helper()
	assertMatch(t, "direct AVX512 Memchr "+label, avx512Memchr(n1, hay), naiveMemchr(n1, hay))
	assertMatch(t, "direct AVX512 Memchr2 "+label, avx512Memchr2(n1, n2, hay), naiveMemchr2(n1, n2, hay))
	assertMatch(t, "direct AVX512 Memchr3 "+label, avx512Memchr3(n1, n2, n3, hay), naiveMemchr3(n1, n2, n3, hay))
	assertMatch(t, "direct AVX512 Memrchr "+label, avx512Memrchr(n1, hay), naiveMemrchr(n1, hay))
	assertMatch(t, "direct AVX512 Memrchr2 "+label, avx512Memrchr2(n1, n2, hay), naiveMemrchr2(n1, n2, hay))
	assertMatch(t, "direct AVX512 Memrchr3 "+label, avx512Memrchr3(n1, n2, n3, hay), naiveMemrchr3(n1, n2, n3, hay))

	assertMatch(t, "public AVX512 Memchr "+label, Memchr(n1, hay), naiveMemchr(n1, hay))
	assertMatch(t, "public AVX512 Memchr2 "+label, Memchr2(n1, n2, hay), naiveMemchr2(n1, n2, hay))
	assertMatch(t, "public AVX512 Memchr3 "+label, Memchr3(n1, n2, n3, hay), naiveMemchr3(n1, n2, n3, hay))
	assertMatch(t, "public AVX512 Memrchr "+label, Memrchr(n1, hay), naiveMemrchr(n1, hay))
	assertMatch(t, "public AVX512 Memrchr2 "+label, Memrchr2(n1, n2, hay), naiveMemrchr2(n1, n2, hay))
	assertMatch(t, "public AVX512 Memrchr3 "+label, Memrchr3(n1, n2, n3, hay), naiveMemrchr3(n1, n2, n3, hay))
}

func assertAVX512BoundaryDuplicates(t *testing.T, label string, needle byte, hay []byte) {
	t.Helper()
	assertMatch(t, "direct AVX512 Memchr2 "+label, avx512Memchr2(needle, needle, hay), naiveMemchr2(needle, needle, hay))
	assertMatch(t, "direct AVX512 Memchr3 "+label, avx512Memchr3(needle, needle, needle, hay), naiveMemchr3(needle, needle, needle, hay))
	assertMatch(t, "direct AVX512 Memrchr2 "+label, avx512Memrchr2(needle, needle, hay), naiveMemrchr2(needle, needle, hay))
	assertMatch(t, "direct AVX512 Memrchr3 "+label, avx512Memrchr3(needle, needle, needle, hay), naiveMemrchr3(needle, needle, needle, hay))

	assertMatch(t, "public AVX512 Memchr2 "+label, Memchr2(needle, needle, hay), naiveMemchr2(needle, needle, hay))
	assertMatch(t, "public AVX512 Memchr3 "+label, Memchr3(needle, needle, needle, hay), naiveMemchr3(needle, needle, needle, hay))
	assertMatch(t, "public AVX512 Memrchr2 "+label, Memrchr2(needle, needle, hay), naiveMemrchr2(needle, needle, hay))
	assertMatch(t, "public AVX512 Memrchr3 "+label, Memrchr3(needle, needle, needle, hay), naiveMemrchr3(needle, needle, needle, hay))
}
