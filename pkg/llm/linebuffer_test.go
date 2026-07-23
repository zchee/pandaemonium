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

package llm

import (
	"fmt"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
)

func TestLineBuffer(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		max       int
		appends   int
		wantLen   int
		wantFirst string
		wantLast  string
	}{
		"success: under the bound keeps everything": {
			max:       5,
			appends:   3,
			wantLen:   3,
			wantFirst: "line-000",
			wantLast:  "line-002",
		},
		"success: overflow keeps only the newest": {
			max:       400,
			appends:   405,
			wantLen:   400,
			wantFirst: "line-005",
			wantLast:  "line-404",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b := NewLineBuffer(tt.max)
			for i := range tt.appends {
				b.Append(fmt.Sprintf("line-%03d", i))
			}
			got := b.Lines()
			if len(got) != tt.wantLen {
				t.Fatalf("Lines() len = %d, want %d", len(got), tt.wantLen)
			}
			if got[0] != tt.wantFirst || got[len(got)-1] != tt.wantLast {
				t.Errorf("Lines() bounds = %q..%q, want %q..%q", got[0], got[len(got)-1], tt.wantFirst, tt.wantLast)
			}
		})
	}
}

func TestLineBufferTail(t *testing.T) {
	t.Parallel()

	b := NewLineBuffer(4)
	for _, line := range []string{"one", "two", "three", "four"} {
		b.Append(line)
	}
	if diff := gocmp.Diff("three\nfour", b.Tail(2)); diff != "" {
		t.Errorf("Tail(2) mismatch (-want +got):\n%s", diff)
	}
	if diff := gocmp.Diff("one\ntwo\nthree\nfour", b.Tail(10)); diff != "" {
		t.Errorf("Tail(10) mismatch (-want +got):\n%s", diff)
	}
}
