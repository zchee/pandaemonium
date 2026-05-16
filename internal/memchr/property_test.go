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
	"math/rand/v2"
	"testing"
)

// TestNaiveScanProperty is a PRNG-seeded quickcheck-style loop that
// compares every public dispatched routine against the naive oracle on
// random inputs (AC-HARNESS-2 in plan). The seed is fixed for
// reproducibility and is also logged via t.Logf on failure so a future
// regression can be replayed deterministically.
//
// The default short-mode iteration count (5_000) keeps `go test -short`
// snappy; full mode runs 100_000 cases per routine.
func TestNaiveScanProperty(t *testing.T) {
	t.Parallel()

	const seed1, seed2 uint64 = 0x6D656D636872A1B2, 0xC3D4E5F012345678
	r := rand.New(rand.NewPCG(seed1, seed2))
	t.Logf("PRNG seed = (0x%016x, 0x%016x)", seed1, seed2)

	iters := 100_000
	if testing.Short() {
		iters = 5_000
	}

	for i := range iters {
		// Length distributed across boundary regimes: heavy at small
		// sizes (where bugs hide), tail at larger sizes (where chunk-
		// loop coverage matters).
		n := r.IntN(2048)
		hay := make([]byte, n)
		// Fill with random bytes; this gives a roughly-uniform
		// distribution of match positions and absent-needle cases.
		for j := range hay {
			hay[j] = byte(r.Uint32())
		}
		n1 := byte(r.Uint32())
		n2 := byte(r.Uint32())
		n3 := byte(r.Uint32())

		if got, want := Memchr(n1, hay), naiveMemchr(n1, hay); got != want {
			t.Fatalf("Memchr iter=%d n1=%#x len=%d: got %d, want %d", i, n1, n, got, want)
		}
		if got, want := Memrchr(n1, hay), naiveMemrchr(n1, hay); got != want {
			t.Fatalf("Memrchr iter=%d n1=%#x len=%d: got %d, want %d", i, n1, n, got, want)
		}
		if got, want := Memchr2(n1, n2, hay), naiveMemchr2(n1, n2, hay); got != want {
			t.Fatalf("Memchr2 iter=%d n1=%#x n2=%#x len=%d: got %d, want %d", i, n1, n2, n, got, want)
		}
		if got, want := Memrchr2(n1, n2, hay), naiveMemrchr2(n1, n2, hay); got != want {
			t.Fatalf("Memrchr2 iter=%d n1=%#x n2=%#x len=%d: got %d, want %d", i, n1, n2, n, got, want)
		}
		if got, want := Memchr3(n1, n2, n3, hay), naiveMemchr3(n1, n2, n3, hay); got != want {
			t.Fatalf("Memchr3 iter=%d n1=%#x n2=%#x n3=%#x len=%d: got %d, want %d", i, n1, n2, n3, n, got, want)
		}
		if got, want := Memrchr3(n1, n2, n3, hay), naiveMemrchr3(n1, n2, n3, hay); got != want {
			t.Fatalf("Memrchr3 iter=%d n1=%#x n2=%#x n3=%#x len=%d: got %d, want %d", i, n1, n2, n3, n, got, want)
		}
	}
}
