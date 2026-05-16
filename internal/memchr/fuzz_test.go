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

import "testing"

// Each FuzzXxx target seeds the corpus with one fixed case so `go test`
// (no -fuzz flag) at least exercises that seed; under -fuzz the engine
// mutates. The body compares the dispatched impl against the naive
// oracle and Fatal()s on any disagreement. Plan AC-HARNESS-3 requires
// each target to be runnable for ≥ 60 s in a dedicated CI job; this file
// supplies the targets, the duration is set by CI.

func FuzzMemchr(f *testing.F) {
	f.Add(byte(0xC3), []byte("hello world"))
	f.Add(byte(0x00), []byte{})
	f.Fuzz(func(t *testing.T, needle byte, haystack []byte) {
		if got, want := Memchr(needle, haystack), naiveMemchr(needle, haystack); got != want {
			t.Fatalf("Memchr(%#x, len=%d): got %d, want %d", needle, len(haystack), got, want)
		}
	})
}

func FuzzMemchr2(f *testing.F) {
	f.Add(byte(0xC3), byte(0xD4), []byte("hello world"))
	f.Add(byte(0x00), byte(0xff), []byte{})
	f.Fuzz(func(t *testing.T, n1, n2 byte, haystack []byte) {
		if got, want := Memchr2(n1, n2, haystack), naiveMemchr2(n1, n2, haystack); got != want {
			t.Fatalf("Memchr2(%#x, %#x, len=%d): got %d, want %d", n1, n2, len(haystack), got, want)
		}
	})
}

func FuzzMemchr3(f *testing.F) {
	f.Add(byte(0xC3), byte(0xD4), byte(0xE5), []byte("hello world"))
	f.Add(byte(0x00), byte(0x80), byte(0xff), []byte{})
	f.Fuzz(func(t *testing.T, n1, n2, n3 byte, haystack []byte) {
		if got, want := Memchr3(n1, n2, n3, haystack), naiveMemchr3(n1, n2, n3, haystack); got != want {
			t.Fatalf("Memchr3(%#x, %#x, %#x, len=%d): got %d, want %d", n1, n2, n3, len(haystack), got, want)
		}
	})
}

func FuzzMemrchr(f *testing.F) {
	f.Add(byte(0xC3), []byte("hello world"))
	f.Add(byte(0x00), []byte{})
	f.Fuzz(func(t *testing.T, needle byte, haystack []byte) {
		if got, want := Memrchr(needle, haystack), naiveMemrchr(needle, haystack); got != want {
			t.Fatalf("Memrchr(%#x, len=%d): got %d, want %d", needle, len(haystack), got, want)
		}
	})
}

func FuzzMemrchr2(f *testing.F) {
	f.Add(byte(0xC3), byte(0xD4), []byte("hello world"))
	f.Add(byte(0x00), byte(0xff), []byte{})
	f.Fuzz(func(t *testing.T, n1, n2 byte, haystack []byte) {
		if got, want := Memrchr2(n1, n2, haystack), naiveMemrchr2(n1, n2, haystack); got != want {
			t.Fatalf("Memrchr2(%#x, %#x, len=%d): got %d, want %d", n1, n2, len(haystack), got, want)
		}
	})
}

func FuzzMemrchr3(f *testing.F) {
	f.Add(byte(0xC3), byte(0xD4), byte(0xE5), []byte("hello world"))
	f.Add(byte(0x00), byte(0x80), byte(0xff), []byte{})
	f.Fuzz(func(t *testing.T, n1, n2, n3 byte, haystack []byte) {
		if got, want := Memrchr3(n1, n2, n3, haystack), naiveMemrchr3(n1, n2, n3, haystack); got != want {
			t.Fatalf("Memrchr3(%#x, %#x, %#x, len=%d): got %d, want %d", n1, n2, n3, len(haystack), got, want)
		}
	})
}
