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
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
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
		"success: detects artifact routine row": {
			benchstat: `│ sec/op │
Memchr2/n=64-44 3.349n ± 0% 3.900n ± 0% +16.45% (p=0.000 n=10)`,
			want: []string{"Memchr2/n=64: +16.45% slower (p=0.000)"},
		},
		"success: detects artifact benchmark-prefixed routine row": {
			benchstat: `│ sec/op │
BenchmarkMemrchr3/n=65536 1.000µ ± 0% 1.090µ ± 1% +9.00% (p=0.001 n=10)`,
			want: []string{"Memrchr3/n=65536: +9.00% slower (p=0.001)"},
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

func TestFindArtifactRegressions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		benchstat string
		want      []string
	}{
		"success: ignores statistically significant hard row below practical threshold": {
			benchstat: `│ sec/op │
Memchr/n=1024-44 17.0n ± 0% 17.8n ± 0% +4.99% (p=0.001 n=10)`,
		},
		"success: detects statistically significant hard row at practical threshold": {
			benchstat: `│ sec/op │
Memchr/n=1024-44 17.0n ± 0% 17.9n ± 0% +5.00% (p=0.001 n=10)`,
			want: []string{"Memchr/n=1024: +5.00% slower (p=0.001, class=hard, threshold=5.00%)"},
		},
		"success: detects statistically significant hard row above practical threshold": {
			benchstat: `│ sec/op │
BenchmarkMemrchr3/n=65536-44 1.000µ ± 0% 1.051µ ± 0% +5.01% (p=0.001 n=10)`,
			want: []string{"Memrchr3/n=65536: +5.01% slower (p=0.001, class=hard, threshold=5.00%)"},
		},
		"success: detects non-default large hard row": {
			benchstat: `│ sec/op │
Memchr/n=2048-44 30.0n ± 0% 31.8n ± 0% +6.00% (p=0.001 n=10)`,
			want: []string{"Memchr/n=2048: +6.00% slower (p=0.001, class=hard, threshold=5.00%)"},
		},
		"success: ignores nonsignificant practical hard row": {
			benchstat: `│ sec/op │
Memchr/n=4096-44 70.0n ± 1% 84.0n ± 1% +20.00% (p=0.080 n=10)`,
		},
		"success: ignores threshold rows in artifact policy": {
			benchstat: `│ sec/op │
Memchr/n=64-44 3.0n ± 0% 6.0n ± 0% +100.00% (p=0.001 n=10)
Memchr/n=128-44 4.0n ± 0% 8.0n ± 0% +100.00% (p=0.001 n=10)
Memchr/n=256-44 5.0n ± 0% 10.0n ± 0% +100.00% (p=0.001 n=10)`,
		},
		"success: ignores tuning rows in artifact policy": {
			benchstat: `│ sec/op │
BenchmarkTuningMemchr/miss/n=1024-44 17.0n ± 0% 34.0n ± 0% +100.00% (p=0.001 n=10)`,
		},
		"success: ignores throughput table positives": {
			benchstat: `│ sec/op │
Memchr/n=1024-44 17.0n ± 0% 16.0n ± 0% -5.88% (p=0.001 n=10)

│ B/s │
Memchr/n=1024-44 54.0Gi ± 0% 70.0Gi ± 0% +29.63% (p=0.000 n=10)`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := findArtifactRegressions(tt.benchstat)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("findArtifactRegressions() got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFindArtifactThresholdRegressions(t *testing.T) {
	t.Parallel()

	benchstat := `│ sec/op │
Memchr/n=64-44 3.0n ± 0% 6.0n ± 0% +100.00% (p=0.001 n=10)
Memchr/n=128-44 4.0n ± 0% 8.0n ± 0% +100.00% (p=0.001 n=10)
Memchr/n=256-44 5.0n ± 0% 10.0n ± 0% +100.00% (p=0.080 n=10)
Memchr/n=1024-44 17.0n ± 0% 18.0n ± 0% +5.88% (p=0.001 n=10)
BenchmarkTuningMemchr/miss/n=128-44 4.0n ± 0% 8.0n ± 0% +100.00% (p=0.001 n=10)`
	want := []string{
		"Memchr/n=64: +100.00% slower (p=0.001, class=threshold, threshold=5.00%)",
		"Memchr/n=128: +100.00% slower (p=0.001, class=threshold, threshold=5.00%)",
	}
	got := findArtifactThresholdRegressions(benchstat)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findArtifactThresholdRegressions() got %#v, want %#v", got, want)
	}
}

func TestClassifyBenchstatRows(t *testing.T) {
	t.Parallel()

	benchstat := `│ sec/op │
Scan/n=64-44 3.0n ± 0% 3.1n ± 0% +3.33% (p=0.001 n=10)
Memchr/n=64-44 3.0n ± 0% 6.0n ± 0% +100.00% (p=0.001 n=10)
Memchr/n=1024-44 17.0n ± 0% 18.0n ± 0% +5.88% (p=0.001 n=10)
Memchr/n=512-44 10.0n ± 0% 11.0n ± 0% +10.00% (p=0.001 n=10)
Memchr/n=2048-44 30.0n ± 0% 32.0n ± 0% +6.67% (p=0.001 n=10)
BenchmarkTuningMemchr/miss/n=4096-44 70.0n ± 0% 140.0n ± 0% +100.00% (p=0.001 n=10)`

	stdlibRows := classifyBenchstatRows(benchstat, stdlibGate)
	if got, want := rowClasses(stdlibRows), []rowClass{rowHard, rowHard, rowHard, rowAdvisory, rowAdvisory, rowTuning}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stdlib row classes got %#v, want %#v", got, want)
	}

	artifactRows := classifyBenchstatRows(benchstat, artifactGate)
	if got, want := rowClasses(artifactRows), []rowClass{rowThreshold, rowThreshold, rowHard, rowAdvisory, rowHard, rowTuning}; !reflect.DeepEqual(got, want) {
		t.Fatalf("artifact row classes got %#v, want %#v", got, want)
	}
}

func TestRunBenchstatFallsBackToGoTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses POSIX shell script")
	}

	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	goPath := filepath.Join(dir, "go")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" > " + shellQuote(logPath) + "\n" +
		"printf '%s\\n' '│ sec/op │'\n" +
		"printf '%s\\n' 'Memchr/n=1024-44 17.0n ± 0% 18.0n ± 0% +5.88% (p=0.001 n=10)'\n"
	if err := os.WriteFile(goPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	t.Setenv("PATH", dir)
	out, err := runBenchstat("benchstat", "base.txt", "treat.txt")
	if err != nil {
		t.Fatalf("runBenchstat() error = %v", err)
	}
	if !strings.Contains(out, "Memchr/n=1024") {
		t.Fatalf("runBenchstat() output %q does not contain fake go tool output", out)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake go args: %v", err)
	}
	if got, want := strings.TrimSpace(string(args)), "tool benchstat -delta-test=utest -alpha=0.05 base.txt treat.txt"; got != want {
		t.Fatalf("fake go args got %q, want %q", got, want)
	}
}

func rowClasses(rows []benchstatRegression) []rowClass {
	out := make([]rowClass, len(rows))
	for i, row := range rows {
		out[i] = row.class
	}
	return out
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
