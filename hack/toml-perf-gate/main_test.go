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
	"bytes"
	"math"
	"testing"
)

// main_test.go covers the parser-side logic of toml-perf-gate. Real
// bench runs are out of scope for unit tests (they require building +
// running the scan package, which is the integration check executed by
// `go run ./hack/toml-perf-gate` itself). The tests below use mocked
// benchstat CSV output to exercise every branch of parseGate +
// scoreRow + parseCI + parseP.

// stubGoodCSV is a benchstat-shaped CSV where the SIMD candidate is
// clearly faster than the baseline with statistical significance.
// Values mimic the layout `benchstat -format=csv` produces for two
// files containing -count=10 -cpu=1 -benchtime=5s runs.
const stubGoodCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
LocateNewline-16,1.20e-06,2.4e-08,6.00e-07,1.5e-08,-50.00%,p=0.001 n=10
geomean,1.20e-06,,6.00e-07,,-50.00%,

,base.txt,,simd.txt,,,
,B/s,CI,B/s,CI,vs base,P
LocateNewline-16,5.46e+10,1.0e+09,1.09e+11,2.5e+09,+99.81%,p=0.001 n=10
geomean,5.46e+10,,1.09e+11,,+99.81%,
`

// stubRegressedCSV is a benchstat CSV where SIMD is statistically
// SLOWER than baseline — the gate must FAIL even though p < alpha.
const stubRegressedCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
LocateNewline-16,1.00e-06,2.0e-08,1.50e-06,3.0e-08,+50.00%,p=0.001 n=10
geomean,1.00e-06,,1.50e-06,,+50.00%,

,base.txt,,simd.txt,,,
,B/s,CI,B/s,CI,vs base,P
LocateNewline-16,6.50e+10,1.3e+09,4.30e+10,8.6e+08,-33.85%,p=0.001 n=10
geomean,6.50e+10,,4.30e+10,,-33.85%,
`

// stubInsignificantCSV is a benchstat CSV where the change is NOT
// statistically significant ("~" in the vs base column). The gate
// must FAIL — without a signal, "lower 95% CI > threshold" cannot
// hold.
const stubInsignificantCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
LocateNewline-16,1.20e-06,2.4e-08,1.18e-06,2.4e-08,~,p=0.300 n=10
geomean,1.20e-06,,1.18e-06,,~,

,base.txt,,simd.txt,,,
,B/s,CI,B/s,CI,vs base,P
LocateNewline-16,5.46e+10,1.0e+09,5.55e+10,1.0e+09,~,p=0.300 n=10
geomean,5.46e+10,,5.55e+10,,~,
`

// stubInfCISigCSV is a benchstat CSV with ∞ CIs (insufficient
// samples) but a statistically-significant change in the SIMD-faster
// direction. The gate's insufficient-samples fallback (use the point
// ratio as the lower bound) must allow this to PASS at threshold 1.0.
const stubInfCISigCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
LocateNewline-16,1.20e-06,∞,8.00e-07,∞,-33.33%,p=0.030 n=4
geomean,1.20e-06,,8.00e-07,,-33.33%,

,base.txt,,simd.txt,,,
,B/s,CI,B/s,CI,vs base,P
LocateNewline-16,5.46e+10,∞,8.19e+10,∞,+50.00%,p=0.030 n=4
geomean,5.46e+10,,8.19e+10,,+50.00%,
`

// stubBareSpeedupCSV is a benchstat CSV where the point speedup is
// 1.10x (10% faster) but the CIs are wide enough that the lower
// bound falls just below 1.05x. Concretely with base=6.4e10 ± 8.0e8
// and simd=7.04e10 ± 2.6e9:
//
//	lower = (7.04e10 - 2.6e9) / (6.4e10 + 8.0e8) ≈ 1.046
//
// So threshold=1.0 passes (1.046 > 1.0) and threshold=1.05 fails
// (1.046 < 1.05). Used to verify the threshold-vs-lower-bound rule
// rejects marginal speedups when the threshold demands more than the
// CI band can support.
const stubBareSpeedupCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
ScanBareKey-16,1.000e-06,1.0e-08,9.091e-07,3.0e-08,-9.09%,p=0.001 n=10
geomean,1.000e-06,,9.091e-07,,-9.09%,

,base.txt,,simd.txt,,,
,B/s,CI,B/s,CI,vs base,P
ScanBareKey-16,6.4e+10,8.0e+08,7.04e+10,2.6e+09,+10.00%,p=0.001 n=10
geomean,6.4e+10,,7.04e+10,,+10.00%,
`

func TestParseGate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		csv           string
		scan          string
		threshold     float64
		alpha         float64
		wantPass      bool
		wantPointMin  float64 // lower bound on expected pointRatio
		wantPointMax  float64 // upper bound
		wantLowerMin  float64
		wantLowerMax  float64
		wantPValue    float64 // exact match
		wantFailMatch string  // substring of failReason on fail (empty when wantPass)
	}{
		{
			name:         "success:large_significant_speedup",
			csv:          stubGoodCSV,
			scan:         "LocateNewline",
			threshold:    1.0,
			alpha:        0.05,
			wantPass:     true,
			wantPointMin: 1.99,
			wantPointMax: 2.01,
			wantLowerMin: 1.90,
			wantLowerMax: 2.00,
			wantPValue:   0.001,
		},
		{
			name:          "fail:regressed",
			csv:           stubRegressedCSV,
			scan:          "LocateNewline",
			threshold:     1.0,
			alpha:         0.05,
			wantPass:      false,
			wantPointMin:  0.65,
			wantPointMax:  0.67,
			wantLowerMin:  0.0,
			wantLowerMax:  0.67,
			wantPValue:    0.001,
			wantFailMatch: "lower95=",
		},
		{
			// "~" in the vs-base column means "delta not significant
			// at alpha" but benchstat still fills the P column. The
			// gate correctly fails on "p >= alpha".
			name:          "fail:not_significant",
			csv:           stubInsignificantCSV,
			scan:          "LocateNewline",
			threshold:     1.0,
			alpha:         0.05,
			wantPass:      false,
			wantPointMin:  1.01,
			wantPointMax:  1.02,
			wantLowerMin:  0.97,
			wantLowerMax:  0.99,
			wantPValue:    0.300,
			wantFailMatch: "p=0.3 >= alpha=0.05",
		},
		{
			name:         "success:inf_ci_with_signal",
			csv:          stubInfCISigCSV,
			scan:         "LocateNewline",
			threshold:    1.0,
			alpha:        0.05,
			wantPass:     true,
			wantPointMin: 1.49,
			wantPointMax: 1.51,
			wantLowerMin: 1.49,
			wantLowerMax: 1.51,
			wantPValue:   0.030,
		},
		{
			name:          "fail:marginal_speedup_strict_threshold",
			csv:           stubBareSpeedupCSV,
			scan:          "ScanBareKey",
			threshold:     1.05,
			alpha:         0.05,
			wantPass:      false,
			wantPointMin:  1.099,
			wantPointMax:  1.101,
			wantLowerMin:  1.04,
			wantLowerMax:  1.05,
			wantPValue:    0.001,
			wantFailMatch: "lower95=",
		},
		{
			name:         "success:marginal_speedup_loose_threshold",
			csv:          stubBareSpeedupCSV,
			scan:         "ScanBareKey",
			threshold:    1.0,
			alpha:        0.05,
			wantPass:     true,
			wantPointMin: 1.099,
			wantPointMax: 1.101,
			wantLowerMin: 1.04,
			wantLowerMax: 1.05,
			wantPValue:   0.001,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseGate(tc.csv, tc.scan, tc.threshold, tc.alpha)
			if err != nil {
				t.Fatalf("parseGate: unexpected error: %v", err)
			}
			if got.pass != tc.wantPass {
				t.Errorf("pass = %v, want %v (failReason=%q)", got.pass, tc.wantPass, got.failReason)
			}
			if got.pointRatio < tc.wantPointMin || got.pointRatio > tc.wantPointMax {
				t.Errorf("pointRatio = %g, want in [%g, %g]", got.pointRatio, tc.wantPointMin, tc.wantPointMax)
			}
			if got.lowerRatio < tc.wantLowerMin || got.lowerRatio > tc.wantLowerMax {
				t.Errorf("lowerRatio = %g, want in [%g, %g]", got.lowerRatio, tc.wantLowerMin, tc.wantLowerMax)
			}
			if got.pValue != tc.wantPValue && !(math.IsNaN(got.pValue) && math.IsNaN(tc.wantPValue)) {
				t.Errorf("pValue = %g, want %g", got.pValue, tc.wantPValue)
			}
			if !tc.wantPass && tc.wantFailMatch != "" {
				if got.failReason == "" {
					t.Errorf("failReason is empty; want substring %q", tc.wantFailMatch)
				} else if !contains(got.failReason, tc.wantFailMatch) {
					t.Errorf("failReason = %q, want substring %q", got.failReason, tc.wantFailMatch)
				}
			}
		})
	}
}

func TestWriteBenchmarkOutputWritesRawText(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := writeBenchmarkOutput(&buf, []byte("BenchmarkParser-1 1 2 ns/op\nPASS\n")); err != nil {
		t.Fatalf("writeBenchmarkOutput: %v", err)
	}

	want := "BenchmarkParser-1 1 2 ns/op\nPASS\n\n"
	if got := buf.String(); got != want {
		t.Fatalf("benchmark output = %q, want %q", got, want)
	}
}

func TestParseCI(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		in       string
		wantW    float64
		wantInf  bool
		wantSkip bool // when true, the test asserts wantInf only
	}{
		"success:empty":       {"", 0, true, false},
		"success:inf_unicode": {"∞", 0, true, false},
		"success:inf_text":    {"inf", 0, true, false},
		"success:inf_cap":     {"Inf", 0, true, false},
		"success:numeric":     {"1.5e+09", 1.5e+09, false, false},
		"success:whitespace":  {"  2.4e-08  ", 2.4e-08, false, false},
		"success:nan_falls":   {"NaN", 0, true, true},
		"success:malformed":   {"hello", 0, true, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, inf := parseCI(tc.in)
			if inf != tc.wantInf {
				t.Errorf("infinite = %v, want %v", inf, tc.wantInf)
			}
			if !tc.wantSkip && !tc.wantInf && w != tc.wantW {
				t.Errorf("width = %g, want %g", w, tc.wantW)
			}
		})
	}
}

func TestParseP(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		in   string
		want float64
	}{
		"success:empty":       {"", -1},
		"success:tilde":       {"~", -1},
		"success:standard":    {"p=0.001 n=10", 0.001},
		"success:no_n":        {"p=0.030", 0.030},
		"success:whitespace":  {"  p=0.500 n=4  ", 0.500},
		"success:malformed":   {"p=hello n=10", -1},
		"success:no_p_prefix": {"0.001 n=10", -1},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := parseP(tc.in)
			if got != tc.want {
				t.Errorf("parseP(%q) = %g, want %g", tc.in, got, tc.want)
			}
		})
	}
}

func TestFindBpsRow(t *testing.T) {
	t.Parallel()
	rows := [][]string{
		{"", "base.txt", "", "simd.txt", "", "", ""},
		{"", "sec/op", "CI", "sec/op", "CI", "vs base", "P"},
		{"LocateNewline-16", "1.20e-06", "2.4e-08", "6.00e-07", "1.5e-08", "-50.00%", "p=0.001 n=10"},
		{"geomean", "1.20e-06", "", "6.00e-07", "", "-50.00%", ""},
		{},
		{"", "base.txt", "", "simd.txt", "", "", ""},
		{"", "B/s", "CI", "B/s", "CI", "vs base", "P"},
		{"LocateNewline-16", "5.46e+10", "1.0e+09", "1.09e+11", "2.5e+09", "+99.81%", "p=0.001 n=10"},
		{"geomean", "5.46e+10", "", "1.09e+11", "", "+99.81%", ""},
	}
	// benchstat -format=csv strips the "Benchmark" prefix from the
	// row name (it presents just the suffix: "LocateNewline-16"). The
	// gate's parseGate looks up by scan name only, not the
	// "Benchmark"-prefixed form.
	row, err := findBpsRow(rows, "LocateNewline")
	if err != nil {
		t.Fatalf("findBpsRow: %v", err)
	}
	if got, want := row[1], "5.46e+10"; got != want {
		t.Errorf("returned row[1] = %q, want %q (wrong table — got sec/op row?)", got, want)
	}
}

func TestParseGate_MalformedCSV(t *testing.T) {
	t.Parallel()
	// Empty input — no rows, must error.
	if _, err := parseGate("", "LocateNewline", 1.0, 0.05); err == nil {
		t.Error("parseGate on empty input: want error, got nil")
	}
	// CSV without a B/s table — must error.
	const noBps = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
LocateNewline-16,1.20e-06,2.4e-08,6.00e-07,1.5e-08,-50.00%,p=0.001 n=10
`
	if _, err := parseGate(noBps, "LocateNewline", 1.0, 0.05); err == nil {
		t.Error("parseGate without B/s table: want error, got nil")
	}
}

func TestBenchmarkArgs_ParserHarness(t *testing.T) {
	oldCount, oldCPU, oldBenchtime := *flagCount, *flagCPU, *flagBenchtime
	*flagCount, *flagCPU, *flagBenchtime = 10, 1, "5s"
	defer func() {
		*flagCount, *flagCPU, *flagBenchtime = oldCount, oldCPU, oldBenchtime
	}()

	got := benchmarkArgs("./pkg/toml/", "^BenchmarkUnmarshal_BurntSushi$", "bench")
	want := []string{
		"test",
		"-bench=^BenchmarkUnmarshal_BurntSushi$",
		"-benchmem",
		"-count=10",
		"-cpu=1",
		"-benchtime=5s",
		"-run=^$",
		"-timeout=1800s",
		"-tags=bench",
		"./pkg/toml/",
	}
	if len(got) != len(want) {
		t.Fatalf("benchmarkArgs len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("benchmarkArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBenchmarkArgs_EditHarness(t *testing.T) {
	oldCount, oldCPU, oldBenchtime := *flagCount, *flagCPU, *flagBenchtime
	*flagCount, *flagCPU, *flagBenchtime = 10, 1, "5s"
	defer func() {
		*flagCount, *flagCPU, *flagBenchtime = oldCount, oldCPU, oldBenchtime
	}()

	got := benchmarkArgs("./pkg/toml/", "^BenchmarkDocumentEdit$", "bench")
	want := []string{
		"test",
		"-bench=^BenchmarkDocumentEdit$",
		"-benchmem",
		"-count=10",
		"-cpu=1",
		"-benchtime=5s",
		"-run=^$",
		"-timeout=1800s",
		"-tags=bench",
		"./pkg/toml/",
	}
	if len(got) != len(want) {
		t.Fatalf("benchmarkArgs len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("benchmarkArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
