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

package main

import (
	"reflect"
	"testing"
)

func TestFindRegressions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		benchstat string
		want      []string
	}{
		"success: detects current benchstat scan row": {
			benchstat: `│ sec/op │
Scan/n=64-44 3.349n ± 0% 3.900n ± 0% +16.45% (p=0.000 n=10)`,
			want: []string{"n=64: +16.45% slower (p=0.000)"},
		},
		"success: detects legacy benchmark-prefixed row": {
			benchstat: `│ sec/op │
BenchmarkScan/n=256 5.800n ± 0% 6.100n ± 0% +5.17% (p=0.012 n=10)`,
			want: []string{"n=256: +5.17% slower (p=0.012)"},
		},
		"success: ignores ungated n=16 regression": {
			benchstat: `│ sec/op │
Scan/n=16-44 2.400n ± 0% 11.000n ± 1% +358.33% (p=0.000 n=10)`,
		},
		"success: ignores nonsignificant gated increase": {
			benchstat: `│ sec/op │
Scan/n=1024-44 17.0n ± 2% 17.2n ± 2% +1.18% (p=0.080 n=10)`,
		},
		"success: ignores improvements": {
			benchstat: `│ sec/op │
Scan/n=4096-44 70.0n ± 0% 35.0n ± 0% -50.00% (p=0.000 n=10)
`,
		},
		"success: ignores throughput table positives": {
			benchstat: `│ sec/op │
Scan/n=4096-44 70.0n ± 0% 54.0n ± 0% -22.86% (p=0.000 n=10)

│ B/s │
Scan/n=4096-44 54.0Gi ± 0% 70.0Gi ± 0% +29.63% (p=0.000 n=10)
`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := findRegressions(tt.benchstat)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("findRegressions() got %#v, want %#v", got, tt.want)
			}
		})
	}
}
