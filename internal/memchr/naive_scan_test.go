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
	"slices"
	"testing"
)

// naiveMemchr is the canonical byte-by-byte oracle used by every parity test
// in this package (AC-HARNESS-2, AC-HARNESS-4 in plan). It is intentionally
// the most obvious correct implementation: a single forward pass that
// returns the first matching offset, or -1 when no byte equals needle.
func naiveMemchr(needle byte, haystack []byte) int {
	for i, b := range haystack {
		if b == needle {
			return i
		}
	}
	return -1
}

// naiveMemchr2 returns the first offset in haystack whose byte equals n1 or
// n2, or -1 when neither appears.
func naiveMemchr2(n1, n2 byte, haystack []byte) int {
	for i, b := range haystack {
		if b == n1 || b == n2 {
			return i
		}
	}
	return -1
}

// naiveMemchr3 returns the first offset in haystack whose byte equals n1,
// n2, or n3, or -1 when none appear.
func naiveMemchr3(n1, n2, n3 byte, haystack []byte) int {
	for i, b := range haystack {
		if b == n1 || b == n2 || b == n3 {
			return i
		}
	}
	return -1
}

// naiveMemrchr returns the last offset in haystack whose byte equals needle,
// or -1 when no byte matches.
func naiveMemrchr(needle byte, haystack []byte) int {
	for i, h := range slices.Backward(haystack) {
		if h == needle {
			return i
		}
	}
	return -1
}

// naiveMemrchr2 returns the last offset in haystack whose byte equals n1 or
// n2, or -1 when neither appears.
func naiveMemrchr2(n1, n2 byte, haystack []byte) int {
	for i, c := range slices.Backward(haystack) {
		if c == n1 || c == n2 {
			return i
		}
	}
	return -1
}

// naiveMemrchr3 returns the last offset in haystack whose byte equals n1,
// n2, or n3, or -1 when none appear.
func naiveMemrchr3(n1, n2, n3 byte, haystack []byte) int {
	for i, c := range slices.Backward(haystack) {
		if c == n1 || c == n2 || c == n3 {
			return i
		}
	}
	return -1
}

// TestNaiveOracleSelfCheck sanity-checks every naive oracle against a fixed
// input so that a regression in the oracle itself fails loud before it can
// silently corrupt every downstream parity test that builds on it.
func TestNaiveOracleSelfCheck(t *testing.T) {
	t.Parallel()

	h := []byte("hello, world! lorem ipsum.")
	tests := map[string]struct {
		got, want int
	}{
		"memchr/first hit at 0":       {naiveMemchr('h', h), 0},
		"memchr/last hit interior":    {naiveMemchr('w', h), 7},
		"memchr/miss":                 {naiveMemchr('Q', h), -1},
		"memchr/empty haystack":       {naiveMemchr('a', nil), -1},
		"memchr2/first of two":        {naiveMemchr2('Q', 'l', h), 2},
		"memchr2/miss both":           {naiveMemchr2('Q', 'Z', h), -1},
		"memchr3/first of three":      {naiveMemchr3('Q', 'Z', 'o', h), 4},
		"memchr3/miss all":            {naiveMemchr3('Q', 'Z', '*', h), -1},
		"memrchr/last hit":            {naiveMemrchr('l', h), 14},
		"memrchr/single occurrence":   {naiveMemrchr('w', h), 7},
		"memrchr/miss":                {naiveMemrchr('Q', h), -1},
		"memrchr/empty":               {naiveMemrchr('a', nil), -1},
		"memrchr2/last of two":        {naiveMemrchr2('o', 'h', h), 15},
		"memrchr2/miss":               {naiveMemrchr2('Q', 'Z', h), -1},
		"memrchr3/last of three":      {naiveMemrchr3('Q', 'Z', 'm', h), 24},
		"memrchr3/period at end wins": {naiveMemrchr3('.', 'h', '*', h), 25},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Fatalf("oracle mismatch: got %d, want %d", tt.got, tt.want)
			}
		})
	}
}
