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
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
// statistically significant ("~" in the vs base column) and the lower
// confidence bound is below the strict 1.0 threshold. The gate must
// fail on the lower-bound rule.
const stubInsignificantCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
LocateNewline-16,1.20e-06,2.4e-08,1.18e-06,2.4e-08,~,p=0.300 n=10
geomean,1.20e-06,,1.18e-06,,~,

,base.txt,,simd.txt,,,
,B/s,CI,B/s,CI,vs base,P
LocateNewline-16,5.46e+10,1.0e+09,5.55e+10,1.0e+09,~,p=0.300 n=10
geomean,5.46e+10,,5.55e+10,,~,
`

// stubToleratedParityCSV matches the CI failure shape for the
// stdlib-backed single-byte scans: no significant benchstat delta, but
// finite confidence bounds whose conservative lower ratio remains above
// the documented 0.98 non-regression threshold.
const stubToleratedParityCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
LocateNewline-16,1.623e-06,1.0e-09,1.623e-06,1.0e-09,~,p=0.075 n=10
geomean,1.623e-06,,1.623e-06,,~,

,base.txt,,simd.txt,,,
,B/s,CI,B/s,CI,vs base,P
LocateNewline-16,3.762e+10,1.0e+07,3.760e+10,1.0e+07,~,p=0.075 n=10
geomean,3.762e+10,,3.760e+10,,~,
`

// stubCIObservedLocateNewlineCSV matches the CI failure from run
// 26240300070: finite lower confidence bound above the documented 0.98
// non-regression threshold, but insignificant p-value.
const stubCIObservedLocateNewlineCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
LocateNewline-16,1.00e-06,0,9.970e-07,0,~,p=0.436 n=10
geomean,1.00e-06,,9.970e-07,,~,

,base.txt,,simd.txt,,,
,B/s,CI,B/s,CI,vs base,P
LocateNewline-16,5.000e+10,1.0e+07,5.017e+10,5.0e+06,~,p=0.436 n=10
geomean,5.000e+10,,5.017e+10,,~,
`

// stubCIObservedScanLiteralStringCSV matches the second CI failure from
// run 26240300070: finite parity at 1.000x is still comfortably above
// the 0.98 threshold and must not fail solely on p>=alpha.
const stubCIObservedScanLiteralStringCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
ScanLiteralString-16,1.00e-06,0,1.00e-06,0,~,p=0.853 n=10
geomean,1.00e-06,,1.00e-06,,~,

,base.txt,,simd.txt,,,
,B/s,CI,B/s,CI,vs base,P
ScanLiteralString-16,5.000e+10,1.0e+07,5.000e+10,1.0e+07,~,p=0.853 n=10
geomean,5.000e+10,,5.000e+10,,~,
`

// stubCIObservedScanLiteralStringPercentCSV matches the CI failure
// from run 26337671779 / job 77534045204: benchstat emits a scaled
// throughput unit and percent CI cells. The lower bound is above the
// 0.98 threshold, so the row must not be routed to the infinite-CI
// p-value fallback just because the p-value is insignificant.
const stubCIObservedScanLiteralStringPercentCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
ScanLiteralString-1,1.000e-09,0%,1.000e-09,0%,~,p=0.529 n=10
geomean,1.000e-09,,1.000e-09,,~,

,base.txt,,simd.txt,,,
,GiB/s,CI,GiB/s,CI,vs base,P
ScanLiteralString-1,69.10,0%,69.23,0%,~,p=0.529 n=10
geomean,69.10,,69.23,,+0.19%,
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

// stubInfCIInsigCSV keeps the same infinite-CI point speedup as
// stubInfCISigCSV but reports no statistically significant change.
// The insufficient-samples fallback must reject it because there is no
// finite lower bound to use as proof.
const stubInfCIInsigCSV = `,base.txt,,simd.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
LocateNewline-16,1.20e-06,∞,8.00e-07,∞,~,p=0.300 n=4
geomean,1.20e-06,,8.00e-07,,~,

,base.txt,,simd.txt,,,
,B/s,CI,B/s,CI,vs base,P
LocateNewline-16,5.46e+10,∞,8.19e+10,∞,~,p=0.300 n=4
geomean,5.46e+10,,8.19e+10,,~,
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
			wantFailMatch: "lower95=",
		},
		{
			name:         "success:tolerated_parity_without_significant_delta",
			csv:          stubToleratedParityCSV,
			scan:         "LocateNewline",
			threshold:    0.98,
			alpha:        0.05,
			wantPass:     true,
			wantPointMin: 0.999,
			wantPointMax: 1.000,
			wantLowerMin: 0.998,
			wantLowerMax: 1.000,
			wantPValue:   0.075,
		},
		{
			name:         "success:ci_observed_locate_newline_non_regression",
			csv:          stubCIObservedLocateNewlineCSV,
			scan:         "LocateNewline",
			threshold:    0.98,
			alpha:        0.05,
			wantPass:     true,
			wantPointMin: 1.002,
			wantPointMax: 1.004,
			wantLowerMin: 1.002,
			wantLowerMax: 1.004,
			wantPValue:   0.436,
		},
		{
			name:         "success:ci_observed_scan_literal_string_non_regression",
			csv:          stubCIObservedScanLiteralStringCSV,
			scan:         "ScanLiteralString",
			threshold:    0.98,
			alpha:        0.05,
			wantPass:     true,
			wantPointMin: 1.000,
			wantPointMax: 1.000,
			wantLowerMin: 0.999,
			wantLowerMax: 1.000,
			wantPValue:   0.853,
		},
		{
			name:         "success:ci_observed_scan_literal_string_percent_ci_non_regression",
			csv:          stubCIObservedScanLiteralStringPercentCSV,
			scan:         "ScanLiteralString",
			threshold:    0.98,
			alpha:        0.05,
			wantPass:     true,
			wantPointMin: 1.001,
			wantPointMax: 1.003,
			wantLowerMin: 1.001,
			wantLowerMax: 1.003,
			wantPValue:   0.529,
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
			name:          "fail:inf_ci_without_signal",
			csv:           stubInfCIInsigCSV,
			scan:          "LocateNewline",
			threshold:     1.0,
			alpha:         0.05,
			wantPass:      false,
			wantPointMin:  1.49,
			wantPointMax:  1.51,
			wantLowerMin:  1.49,
			wantLowerMax:  1.51,
			wantPValue:    0.300,
			wantFailMatch: "p=0.3 >= alpha=0.05",
		},
		{
			name:          "fail:inf_ci_point_below_threshold",
			csv:           stubInfCISigCSV,
			scan:          "LocateNewline",
			threshold:     2.0,
			alpha:         0.05,
			wantPass:      false,
			wantPointMin:  1.49,
			wantPointMax:  1.51,
			wantLowerMin:  1.49,
			wantLowerMax:  1.51,
			wantPValue:    0.030,
			wantFailMatch: "point=1.5000x < threshold=2.0000x",
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
				} else if !strings.Contains(got.failReason, tc.wantFailMatch) {
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
		metric   float64
		in       string
		wantW    float64
		wantInf  bool
		wantSkip bool // when true, the test asserts wantInf only
	}{
		"success:empty":              {100, "", 0, true, false},
		"success:inf_unicode":        {100, "∞", 0, true, false},
		"success:inf_text":           {100, "inf", 0, true, false},
		"success:inf_cap":            {100, "Inf", 0, true, false},
		"success:numeric":            {100, "1.5e+09", 1.5e+09, false, false},
		"success:whitespace":         {100, "  2.4e-08  ", 2.4e-08, false, false},
		"success:percent_zero":       {69.10, "0%", 0, false, false},
		"success:percent_fraction":   {100, "0.5%", 0.5, false, false},
		"success:percent_whitespace": {200, " 1.25% ", 2.5, false, false},
		"success:nan_falls":          {100, "NaN", 0, true, true},
		"success:malformed":          {100, "hello", 0, true, false},
		"success:malformed_percent":  {100, "hello%", 0, true, false},
		"success:negative_percent":   {100, "-1%", 0, true, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, inf := parseCI(tc.in, tc.metric)
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
	tests := map[string]struct {
		unit     string
		base     string
		wantBase string
	}{
		"success:raw_bytes_per_second": {"B/s", "5.46e+10", "5.46e+10"},
		"success:gibibytes_per_second": {"GiB/s", "50.8", "50.8"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rows := [][]string{
				{"", "base.txt", "", "simd.txt", "", "", ""},
				{"", "sec/op", "CI", "sec/op", "CI", "vs base", "P"},
				{"LocateNewline-16", "1.20e-06", "2.4e-08", "6.00e-07", "1.5e-08", "-50.00%", "p=0.001 n=10"},
				{"geomean", "1.20e-06", "", "6.00e-07", "", "-50.00%", ""},
				{},
				{"", "base.txt", "", "simd.txt", "", "", ""},
				{"", tc.unit, "CI", tc.unit, "CI", "vs base", "P"},
				{"LocateNewline-16", tc.base, "1.0e+09", "1.09e+11", "2.5e+09", "+99.81%", "p=0.001 n=10"},
				{"geomean", tc.base, "", "1.09e+11", "", "+99.81%", ""},
				{},
				{"", "base.txt", "", "simd.txt", "", "", ""},
				{"", "B/op", "CI", "B/op", "CI", "vs base", "P"},
				{"LocateNewline-16", "0", "0%", "0", "0%", "~", "p=1.000 n=10"},
			}
			// benchstat -format=csv strips the "Benchmark" prefix from the
			// row name (it presents just the suffix: "LocateNewline-16"). The
			// gate's parseGate looks up by scan name only, not the
			// "Benchmark"-prefixed form.
			row, err := findBpsRow(rows, "LocateNewline")
			if err != nil {
				t.Fatalf("findBpsRow: %v", err)
			}
			if got := row[1]; got != tc.wantBase {
				t.Errorf("returned row[1] = %q, want %q (wrong table — got non-throughput row?)", got, tc.wantBase)
			}
		})
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

	got := benchmarkArgs("./pkg/toml/", "^BenchmarkUnmarshal_BurntSushi$", "bench", "")
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

	got := benchmarkArgs("./pkg/toml/", "^BenchmarkDocumentEdit$", "bench", "")
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

func TestBenchmarkArgs_SubmoduleBypassesVendor(t *testing.T) {
	oldCount, oldCPU, oldBenchtime := *flagCount, *flagCPU, *flagBenchtime
	*flagCount, *flagCPU, *flagBenchtime = 10, 1, "5s"
	defer func() {
		*flagCount, *flagCPU, *flagBenchtime = oldCount, oldCPU, oldBenchtime
	}()

	got := benchmarkArgs(".", "^BenchmarkUnmarshal_Pandaemonium$", "", "mod")
	want := []string{
		"test",
		"-bench=^BenchmarkUnmarshal_Pandaemonium$",
		"-benchmem",
		"-count=10",
		"-cpu=1",
		"-benchtime=5s",
		"-run=^$",
		"-timeout=1800s",
		"-mod=mod",
		".",
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

func TestResolveBenchmarkTarget(t *testing.T) {
	t.Parallel()

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	tests := map[string]struct {
		pkg         string
		wantDir     string
		wantPkg     string
		wantModMode string
	}{
		"success: root module package stays rooted at repository": {
			pkg:     "./pkg/toml/",
			wantDir: repoRoot,
			wantPkg: "./pkg/toml/",
		},
		"success: benchmark submodule runs from submodule root": {
			pkg:         "./pkg/toml/benchmark",
			wantDir:     filepath.Join(repoRoot, "pkg", "toml", "benchmark"),
			wantPkg:     ".",
			wantModMode: "mod",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveBenchmarkTarget(repoRoot, tc.pkg)
			if err != nil {
				t.Fatalf("resolveBenchmarkTarget: %v", err)
			}
			if got.dir != tc.wantDir {
				t.Fatalf("dir = %q, want %q", got.dir, tc.wantDir)
			}
			if got.pkg != tc.wantPkg {
				t.Fatalf("pkg = %q, want %q", got.pkg, tc.wantPkg)
			}
			if got.modMode != tc.wantModMode {
				t.Fatalf("modMode = %q, want %q", got.modMode, tc.wantModMode)
			}
		})
	}
}

func TestSafeBenchmarkLabel(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in   string
		want string
	}{
		"success: slash label":     {"facade/reference-map/pelletier-base", "facade-reference-map-pelletier-base"},
		"success: backslash label": {`marshal\pelletier`, "marshal-pelletier"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := safeBenchmarkLabel(tc.in); got != tc.want {
				t.Fatalf("safeBenchmarkLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTOMLPerfGateWorkflowScanRatios(t *testing.T) {
	t.Parallel()

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "toml-perf-gate.yaml")
	workflowBytes, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read workflow %s: %v", workflowPath, err)
	}
	workflow := string(workflowBytes)

	want := map[string]string{
		"CountLines":            "1.0",
		"LocateNewline":         "0.98",
		"ScanLiteralString":     "0.98",
		"ScanBareKey":           "1.0",
		"ScanBasicString":       "1.0",
		"ScanBasicStringStrict": "1.0",
		"ScanCommentBody":       "1.0",
		"ScanBareValueEnd":      "1.0",
		"SkipWhitespace":        "1.0",
		"ValidateUTF8":          "1.0",
	}

	for _, job := range []string{"amd64-perf", "arm64-perf"} {
		t.Run(job, func(t *testing.T) {
			t.Parallel()

			block := workflowJobBlock(t, workflow, job)
			got := workflowScanRatios(block)
			if len(got) != len(want) {
				t.Fatalf("job %s: got %d scan ratios, want %d: %#v", job, len(got), len(want), got)
			}
			for scan, wantRatio := range want {
				gotRatio, ok := got[scan]
				if !ok {
					t.Fatalf("job %s: missing scan %s in matrix ratios %#v", job, scan, got)
				}
				if gotRatio != wantRatio {
					t.Fatalf("job %s scan %s: ratio=%s, want %s", job, scan, gotRatio, wantRatio)
				}
			}
			if gotCount := strings.Count(block, `--ratio="${{ matrix.ratio }}"`); gotCount != 1 {
				t.Fatalf("job %s: got %d matrix ratio command uses, want 1", job, gotCount)
			}
			if gotCount := strings.Count(block, "add-gotip-bin-to-path: true"); gotCount != 1 {
				t.Fatalf("job %s: got %d gotip PATH enables, want 1", job, gotCount)
			}
			if gotCount := strings.Count(block, "go run -a ./hack/toml-perf-gate"); gotCount != 1 {
				t.Fatalf("job %s: got %d fresh harness run commands, want 1", job, gotCount)
			}
			if strings.Contains(block, "gotip run ./hack/toml-perf-gate") {
				t.Fatalf("job %s: stale gotip run harness command is still present", job)
			}
		})
	}
}

func TestTOMLPerfGateWorkflowFinalGateJobs(t *testing.T) {
	t.Parallel()

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "toml-perf-gate.yaml")
	workflowBytes, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read workflow %s: %v", workflowPath, err)
	}
	workflow := string(workflowBytes)

	aggregate := workflowJobBlock(t, workflow, "aggregate-gates")
	for _, want := range []string{
		"runner: ubuntu-24.04",
		"runner: macos-15",
		"kind: facade",
		"kind: marshal",
		"kind: edit",
		`--kind="${{ matrix.kind }}"`,
		"--count=10",
		"--benchtime=5s",
		"--cpu=1",
		"add-gotip-bin-to-path: true",
	} {
		if !strings.Contains(aggregate, want) {
			t.Fatalf("aggregate-gates job missing %q\n%s", want, aggregate)
		}
	}
	if gotCount := strings.Count(aggregate, "kind: facade"); gotCount != 2 {
		t.Fatalf("aggregate-gates facade matrix count = %d, want 2", gotCount)
	}
	if gotCount := strings.Count(aggregate, "kind: marshal"); gotCount != 2 {
		t.Fatalf("aggregate-gates marshal matrix count = %d, want 2", gotCount)
	}
	if gotCount := strings.Count(aggregate, "kind: edit"); gotCount != 2 {
		t.Fatalf("aggregate-gates edit matrix count = %d, want 2", gotCount)
	}

	forceSWAR := workflowJobBlock(t, workflow, "force-swar-correctness")
	for _, want := range []string{
		"runs-on: ubuntu-24.04",
		"go test -tags=force_swar -race -count=1 ./pkg/toml/internal/scan ./pkg/toml",
		"add-gotip-bin-to-path: true",
	} {
		if !strings.Contains(forceSWAR, want) {
			t.Fatalf("force-swar-correctness job missing %q\n%s", want, forceSWAR)
		}
	}
}

func workflowJobBlock(t *testing.T, workflow, job string) string {
	t.Helper()

	startMarker := "\n  " + job + ":\n"
	start := strings.Index(workflow, startMarker)
	if start < 0 {
		t.Fatalf("workflow missing job %s", job)
	}
	rest := workflow[start+len(startMarker):]
	nextJob := regexp.MustCompile(`(?m)^  [A-Za-z0-9_-]+:$`).FindStringIndex(rest)
	if nextJob == nil {
		return rest
	}
	return rest[:nextJob[0]]
}

func workflowScanRatios(jobBlock string) map[string]string {
	re := regexp.MustCompile(`(?m)^\s+- scan: ([A-Za-z0-9_]+)\n\s+ratio: "([0-9.]+)"$`)
	matches := re.FindAllStringSubmatch(jobBlock, -1)
	ratios := make(map[string]string, len(matches))
	for _, match := range matches {
		ratios[match[1]] = match[2]
	}
	return ratios
}
