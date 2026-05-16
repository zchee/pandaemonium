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

// toml-perf-gate is the CI hard gate behind plan AC-SIMD-5
// (.omc/plans/2026-05-16-port-toml-rs-to-pkg-toml.md §Cross-cutting >
// Bench protocol). For a named scan kernel, it runs the matching
// BenchmarkXxx_SIMD and BenchmarkXxx_Baseline pair under the Bench
// protocol (-count=10 -cpu=1 -benchtime=5s -benchmem), feeds both
// captures through benchstat, and asserts the lower bound of the 95%
// confidence interval on the SIMD-throughput / baseline-throughput
// ratio exceeds the gate threshold (default 1.0).
//
// Lineage: mirrors hack/memchr-perf-gate. Diverges in two ways:
//
//  1. memchr-perf-gate runs one all-sizes pair; this tool runs one
//     named scan at a time so the CI matrix dispatches six invocations
//     (one per kernel) in parallel — matches the per-scan baseline
//     declared in the AC-SIMD-5 table.
//  2. memchr-perf-gate uses `benchstat -delta-test=utest -alpha=0.05`;
//     this tool uses `benchstat -alpha=0.05` only because golang.org/x/
//     perf v0.0.0-20251112 removed the -delta-test flag (the U-test is
//     now the implicit comparator). The semantic gate is unchanged.
//
// Usage:
//
//	go run ./hack/toml-perf-gate --kind=scan --scan=ScanBareKey [--ratio=1.0]
//	  [--count=10] [--benchtime=5s] [--cpu=1] [--package=./pkg/toml/internal/scan/]
//	  [--benchstat=benchstat]
//	go run ./hack/toml-perf-gate --kind=parser --ratio=0.5
//
//	# Phase 4/5 stubs (no-ops in Phase 1):
//	go run ./hack/toml-perf-gate --kind=facade ...
//	go run ./hack/toml-perf-gate --kind=edit   ...
//
// Exit codes:
//
//	0  all gates pass (or --kind=facade|edit stub)
//	1  one or more gates fail (regression)
//	2  argument or harness error
package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	exitOK   = 0
	exitFail = 1
	exitArg  = 2
)

// validScans is the set of scan names this gate recognizes; matches
// the six kernels in pkg/toml/internal/scan/api.go and the
// AC-SIMD-5 baseline table. Reject anything else with exitArg so a
// typo on the command line never silently runs the wrong benchmark.
var validScans = map[string]bool{
	"LocateNewline":     true,
	"ScanLiteralString": true,
	"ScanBareKey":       true,
	"ScanBasicString":   true,
	"SkipWhitespace":    true,
	"ValidateUTF8":      true,
}

// Flags. Most have Bench-protocol defaults; --kind and --scan have no
// defaults to force the caller to be explicit.
var (
	flagKind      = flag.String("kind", "", "perf-gate kind: scan|facade|edit|parser (required)")
	flagScan      = flag.String("scan", "", "scan name (for --kind=scan; required): one of "+sortedScanNames())
	flagRatio     = flag.Float64("ratio", 1.0, "minimum SIMD/baseline throughput ratio that the lower 95% CI must exceed")
	flagCount     = flag.Int("count", 10, "go test -count value (Bench protocol locks at 10 for CI)")
	flagBenchtime = flag.String("benchtime", "5s", "go test -benchtime value (Bench protocol locks at 5s for CI)")
	flagCPU       = flag.Int("cpu", 1, "go test -cpu value (Bench protocol locks at 1)")
	flagPackage   = flag.String("package", "./pkg/toml/internal/scan/", "package import path containing the scan benchmarks")
	flagParserPkg = flag.String("parser-package", "./pkg/toml/internal/smoketest/", "package import path containing the Phase 2.5 parser smoketest benchmarks")
	flagBenchstat = flag.String("benchstat", "benchstat", "path to benchstat binary")
	flagAlpha     = flag.Float64("alpha", 0.05, "benchstat -alpha (U-test significance threshold)")
	flagBench     = flag.String("bench", "SmoketestUnmarshal", "benchmark stem for --kind=parser")
)

func main() {
	flag.Parse()
	switch *flagKind {
	case "scan":
		runScanGate()
	case "facade", "edit":
		// Phase 4/5 will implement these; Phase 1 ships a stub so the
		// CLI surface is stable from the start.
		fmt.Printf("toml-perf-gate: --kind=%s not implemented in Phase 1 (Phase 4/5 will land it); exiting 0\n", *flagKind)
		os.Exit(exitOK)
	case "parser":
		runParserSmoketest()
	case "":
		fmt.Fprintln(os.Stderr, "toml-perf-gate: --kind is required (one of: scan, facade, edit, parser)")
		flag.Usage()
		os.Exit(exitArg)
	default:
		fmt.Fprintf(os.Stderr, "toml-perf-gate: unknown --kind=%q (valid: scan, facade, edit, parser)\n", *flagKind)
		os.Exit(exitArg)
	}
}

// runParserSmoketest runs the throwaway Phase 2.5 parser+scan
// trajectory benchmark against BurntSushi on the pinned Cargo.lock
// corpus. The gate threshold is intentionally weaker than AC-FAC-6:
// Phase 2.5 passes when the no-cache shim is at least 0.5x BurntSushi.
func runParserSmoketest() {
	if *flagRatio <= 0 {
		die(exitArg, "--ratio must be > 0; got %g", *flagRatio)
	}
	if *flagAlpha <= 0 || *flagAlpha >= 1 {
		die(exitArg, "--alpha must be in (0,1); got %g", *flagAlpha)
	}
	repoRoot, err := findRepoRoot()
	if err != nil {
		die(exitArg, "%v", err)
	}

	tmp, err := os.MkdirTemp("", "toml-parser-smoketest-*")
	if err != nil {
		die(exitArg, "mktemp: %v", err)
	}
	defer os.RemoveAll(tmp)

	stem := "Benchmark" + *flagBench
	baseFile, err := runBenchInPackage(repoRoot, tmp, *flagParserPkg, "^"+stem+"_BurntSushi$", "base", "bench")
	if err != nil {
		die(exitArg, "burntsushi parser smoketest: %v", err)
	}
	candidateFile, err := runBenchInPackage(repoRoot, tmp, *flagParserPkg, "^"+stem+"_Pandaemonium$", "candidate", "bench")
	if err != nil {
		die(exitArg, "pandaemonium parser smoketest: %v", err)
	}
	if err := renameInPlace(baseFile, stem+"_BurntSushi", stem); err != nil {
		die(exitArg, "rename burntsushi benchmark: %v", err)
	}
	if err := renameInPlace(candidateFile, stem+"_Pandaemonium", stem); err != nil {
		die(exitArg, "rename pandaemonium benchmark: %v", err)
	}

	csvOut, textOut, err := runBenchstat(*flagBenchstat, *flagAlpha, baseFile, candidateFile)
	if err != nil {
		die(exitArg, "benchstat: %v", err)
	}
	if err := writeBenchmarkOutput(os.Stdout, []byte(textOut)); err != nil {
		die(exitArg, "parser smoketest output: %v", err)
	}
	res, err := parseGate(csvOut, *flagBench, *flagRatio, *flagAlpha)
	if err != nil {
		die(exitArg, "parse benchstat CSV: %v", err)
	}
	if res.pass {
		fmt.Printf("toml-perf-gate: PASS parser point=%.3fx lower95=%.3fx threshold=%.3fx %s\n",
			res.pointRatio, res.lowerRatio, *flagRatio, res.pStr)
		os.Exit(exitOK)
	}
	fmt.Fprintf(os.Stderr, "toml-perf-gate: FAIL parser point=%.3fx lower95=%.3fx threshold=%.3fx %s reason=%s\n",
		res.pointRatio, res.lowerRatio, *flagRatio, res.pStr, res.failReason)
	os.Exit(exitFail)
}

func writeBenchmarkOutput(w io.Writer, out []byte) error {
	if _, err := w.Write(out); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}

// runScanGate is the core Phase 1 path: build temp files, run the two
// matching benchmarks, hand both to benchstat, parse the CSV result,
// and decide pass/fail against --ratio.
func runScanGate() {
	if !validScans[*flagScan] {
		die(exitArg, "--scan=%q is not a recognized scan name; valid: %s", *flagScan, sortedScanNames())
	}
	if *flagRatio <= 0 {
		die(exitArg, "--ratio must be > 0; got %g", *flagRatio)
	}
	if *flagAlpha <= 0 || *flagAlpha >= 1 {
		die(exitArg, "--alpha must be in (0,1); got %g", *flagAlpha)
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		die(exitArg, "%v", err)
	}

	tmp, err := os.MkdirTemp("", "toml-perf-gate-*")
	if err != nil {
		die(exitArg, "mktemp: %v", err)
	}
	defer os.RemoveAll(tmp)

	// Run baseline first so its samples seed file timestamps the way
	// benchstat presents columns (base on the left, candidate on the right).
	baseFile, err := runBench(repoRoot, tmp, "^Benchmark"+*flagScan+"_Baseline$", "base")
	if err != nil {
		die(exitArg, "baseline bench: %v", err)
	}
	simdFile, err := runBench(repoRoot, tmp, "^Benchmark"+*flagScan+"_SIMD$", "simd")
	if err != nil {
		die(exitArg, "simd bench: %v", err)
	}

	// Align bench names so benchstat pairs them. The original names
	// are different (Benchmark<X>_SIMD vs Benchmark<X>_Baseline); after
	// renaming both files use the bare stem "Benchmark<X>" so benchstat
	// treats them as the same row to compare.
	stem := "Benchmark" + *flagScan
	if err := renameInPlace(baseFile, stem+"_Baseline", stem); err != nil {
		die(exitArg, "rename baseline: %v", err)
	}
	if err := renameInPlace(simdFile, stem+"_SIMD", stem); err != nil {
		die(exitArg, "rename simd: %v", err)
	}

	csvOut, textOut, err := runBenchstat(*flagBenchstat, *flagAlpha, baseFile, simdFile)
	if err != nil {
		die(exitArg, "benchstat: %v", err)
	}
	// Always print the human-readable benchstat table — it is the
	// audit artifact captured into CI logs and PR comments.
	fmt.Print(textOut)

	res, err := parseGate(csvOut, *flagScan, *flagRatio, *flagAlpha)
	if err != nil {
		die(exitArg, "parse benchstat CSV: %v", err)
	}

	// Machine-readable summary line — CI consumers grep for the
	// "toml-perf-gate:" prefix so this format is stable.
	fmt.Println()
	if res.pass {
		fmt.Printf("toml-perf-gate: PASS %s point=%.3fx lower95=%.3fx threshold=%.3fx %s\n",
			*flagScan, res.pointRatio, res.lowerRatio, *flagRatio, res.pStr)
		os.Exit(exitOK)
	}
	fmt.Fprintf(os.Stderr, "toml-perf-gate: FAIL %s point=%.3fx lower95=%.3fx threshold=%.3fx %s reason=%s\n",
		*flagScan, res.pointRatio, res.lowerRatio, *flagRatio, res.pStr, res.failReason)
	os.Exit(exitFail)
}

// runBench runs `go test -bench=<pattern> -benchmem -count=N -cpu=K
// -benchtime=T -run=^$ <package>` and writes the output to
// <dir>/<label>.txt. It returns the path of the written file.
func runBench(repoRoot, dir, pattern, label string) (string, error) {
	return runBenchInPackage(repoRoot, dir, *flagPackage, pattern, label, "")
}

func runBenchInPackage(repoRoot, dir, pkg, pattern, label, tags string) (string, error) {
	out, err := runBenchmarkOnly(repoRoot, pkg, pattern, tags)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, label+".txt")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// runBenchmarkOnly runs a single go test benchmark pattern with the
// Bench protocol flags and returns the combined benchmark output.
func runBenchmarkOnly(repoRoot, pkg, pattern, tags string) ([]byte, error) {
	args := benchmarkArgs(pkg, pattern, tags)
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot
	// Bench protocol: GOMAXPROCS=1, empty GODEBUG. Inherit everything
	// else (including GOEXPERIMENT from .envrc) so the toolchain match
	// matches the production build.
	cmd.Env = append(os.Environ(), "GOMAXPROCS=1", "GODEBUG=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go %s: %w\nstderr:\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return out, nil
}

func benchmarkArgs(pkg, pattern, tags string) []string {
	args := []string{
		"test",
		"-bench=" + pattern,
		"-benchmem",
		"-count=" + strconv.Itoa(*flagCount),
		"-cpu=" + strconv.Itoa(*flagCPU),
		"-benchtime=" + *flagBenchtime,
		"-run=^$",
		"-timeout=1800s",
	}
	if tags != "" {
		args = append(args, "-tags="+tags)
	}
	args = append(args, pkg)
	return args
}

// renameInPlace replaces every occurrence of `from` with `to` in the
// file at `path`. Used to align benchmark names across baseline and
// candidate files so benchstat pairs them.
func renameInPlace(path, from, to string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	b = bytes.ReplaceAll(b, []byte(from), []byte(to))
	return os.WriteFile(path, b, 0o644)
}

// runBenchstat invokes benchstat twice on the same inputs: once in CSV
// format (for the gate parser) and once in text format (for the audit
// log). Text is printed verbatim; the CSV bytes are returned for
// parseGate. The -alpha argument matches the Bench protocol's α.
func runBenchstat(bin string, alpha float64, baseFile, simdFile string) (csvOut, textOut string, err error) {
	csvBytes, err := runBenchstatOne(bin, []string{
		"-format=csv",
		"-alpha=" + strconv.FormatFloat(alpha, 'f', -1, 64),
		baseFile, simdFile,
	})
	if err != nil {
		return "", "", fmt.Errorf("csv invocation: %w", err)
	}
	textBytes, err := runBenchstatOne(bin, []string{
		"-alpha=" + strconv.FormatFloat(alpha, 'f', -1, 64),
		baseFile, simdFile,
	})
	if err != nil {
		return "", "", fmt.Errorf("text invocation: %w", err)
	}
	return string(csvBytes), string(textBytes), nil
}

func runBenchstatOne(bin string, args []string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	// benchstat writes warnings (e.g., "need >= 6 samples for CI") to
	// stderr; we capture both streams together so the audit log shows
	// the full picture.
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w\noutput:\n%s", bin, strings.Join(args, " "), err, string(out))
	}
	return out, nil
}

// gateResult is the parsed verdict from a benchstat CSV plus the
// numeric inputs that drove it. Kept as a value type so main_test.go
// can build it directly from mocked inputs.
type gateResult struct {
	pointRatio float64 // SIMD throughput / baseline throughput, median
	lowerRatio float64 // conservative lower bound: (SIMD_med - SIMD_CI) / (base_med + base_CI)
	pStr       string  // raw "p=… n=…" string from benchstat for audit
	pValue     float64 // parsed p; -1 if benchstat reported "~" (no significant change)
	pass       bool
	failReason string // human-readable reason on fail; empty on pass
}

// parseGate consumes the CSV produced by `benchstat -format=csv` of a
// baseline/candidate pair and decides whether the SIMD throughput
// satisfies the configured threshold. It always uses the B/s table
// (the second of the two stanzas benchstat emits when -benchmem is on
// and SetBytes is set), because that table has the units the gate is
// expressed in (throughput, not latency).
//
// Decision rule:
//
//	point   = SIMD_Bps / baseline_Bps                 (median throughput ratio)
//	lower   = (SIMD_Bps - SIMD_CI) / (base_Bps + base_CI)   (conservative lower bound)
//	pass    = lower >= threshold AND p < alpha
//	         OR (CIs are ∞ AND point >= threshold AND p < alpha)   (insufficient-samples fallback)
//
// The "OR" branch handles benchstat reporting "∞" CIs for small
// sample counts (< 6); without it the gate would always fail in that
// regime. CI runs use count=10 which avoids that branch entirely.
func parseGate(csvText, scanName string, ratioThreshold, alpha float64) (gateResult, error) {
	// benchstat -format=csv writes:
	//   ,base.txt,,simd.txt,,,
	//   ,sec/op,CI,sec/op,CI,vs base,P
	//   <BenchName>-N,<base_sec>,<base_CI>,<simd_sec>,<simd_CI>,<delta>,<p_n>
	//   geomean,...
	//   <blank>
	//   ,base.txt,,simd.txt,,,
	//   ,B/s,CI,B/s,CI,vs base,P
	//   <BenchName>-N,<base_Bps>,<base_CI>,<simd_Bps>,<simd_CI>,<delta>,<p_n>
	//   geomean,...
	// The B/s table is the second one. We scan for it by header row.
	rows, err := readBenchstatCSV(csvText)
	if err != nil {
		return gateResult{}, err
	}
	// benchstat -format=csv strips the "Benchmark" prefix and keeps
	// the GOMAXPROCS suffix ("-N"), e.g. "LocateNewline-16". Look up
	// by the scan name alone.
	row, err := findBpsRow(rows, scanName)
	if err != nil {
		return gateResult{}, err
	}
	res, err := scoreRow(row, ratioThreshold, alpha)
	if err != nil {
		return gateResult{}, err
	}
	return res, nil
}

// readBenchstatCSV reads the CSV bytes benchstat writes to stdout/
// stderr (which we capture together via CombinedOutput) and returns
// the data rows minus the comment lines benchstat sometimes prefaces
// with (e.g., "B7: need >= 6 samples ..."). encoding/csv accepts
// variable-field-count records when FieldsPerRecord = -1.
func readBenchstatCSV(text string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip the warning lines benchstat prints before the CSV
			// proper (they aren't valid CSV — e.g.,
			// "B7: need >= 6 samples for confidence interval at level 0.95").
			// A real malformed-CSV error returns through the outer wrap.
			continue
		}
		rows = append(rows, rec)
	}
	if len(rows) == 0 {
		return nil, errors.New("no CSV rows parsed from benchstat output")
	}
	return rows, nil
}

// findBpsRow returns the data row for the named benchmark inside the
// B/s table. It identifies the B/s table by looking for a header row
// whose second column equals "B/s", then takes the next row whose
// first column starts with benchPrefix.
func findBpsRow(rows [][]string, benchPrefix string) ([]string, error) {
	inBps := false
	for _, r := range rows {
		if len(r) < 2 {
			continue
		}
		if r[1] == "B/s" {
			inBps = true
			continue
		}
		if !inBps {
			continue
		}
		// A new file-header row resets the table context, but B/s is
		// the LAST table benchstat emits for our shape, so we never
		// expect to leave it. Defensively reset on a sec/op header.
		if r[1] == "sec/op" {
			inBps = false
			continue
		}
		if strings.HasPrefix(r[0], benchPrefix) && r[0] != "geomean" {
			return r, nil
		}
	}
	return nil, fmt.Errorf("could not find B/s row for %q", benchPrefix)
}

// scoreRow turns one CSV row into a gateResult.
//
// Row shape (per the comment in parseGate):
//
//	0: bench name (e.g. "LocateNewline-16")
//	1: base B/s (median, in B/s — e.g., "5.52e+10")
//	2: base CI  (absolute width in B/s — e.g., "∞" or "1.2e+08")
//	3: simd B/s (median)
//	4: simd CI  (absolute width)
//	5: vs base  (e.g., "-27.95%", "~", or empty)
//	6: P        (e.g., "p=0.001 n=10" or empty)
//
// Bias note: the lower bound returned by this function is the
// "worst-case envelope" — baseline shifted up by its CI half-width and
// SIMD shifted down by its CI half-width, taken as independent. A
// proper 95% CI on the ratio would account for correlation between
// the two samples (bootstrap resampling of the paired distribution).
// The envelope is strictly wider than the true CI, which means the
// gate is biased toward FALSE REJECTIONS near the threshold: a true
// 1.05x speedup with noisy samples may be reported as a 1.03x lower
// bound and fail a 1.05x threshold. This is acceptable for a hard
// gate because false negatives only mean a CI flake re-run resolves
// the case; false positives (accepting a regression) would be much
// worse. Document this trade-off in any operator runbook that owns
// re-running flaky perf-gate jobs.
func scoreRow(r []string, ratioThreshold, alpha float64) (gateResult, error) {
	if len(r) < 7 {
		return gateResult{}, fmt.Errorf("row has %d columns, want 7: %q", len(r), r)
	}
	baseBps, err := strconv.ParseFloat(strings.TrimSpace(r[1]), 64)
	if err != nil {
		return gateResult{}, fmt.Errorf("parse base B/s %q: %w", r[1], err)
	}
	simdBps, err := strconv.ParseFloat(strings.TrimSpace(r[3]), 64)
	if err != nil {
		return gateResult{}, fmt.Errorf("parse simd B/s %q: %w", r[3], err)
	}
	if baseBps <= 0 {
		return gateResult{}, fmt.Errorf("base B/s is non-positive: %g", baseBps)
	}
	baseCI, baseCIInf := parseCI(r[2])
	simdCI, simdCIInf := parseCI(r[4])
	pStr := strings.TrimSpace(r[6])
	pValue := parseP(pStr)

	point := simdBps / baseBps

	var lower float64
	if baseCIInf || simdCIInf {
		// Insufficient samples for CI; the lower bound is undefined,
		// so the conservative path is to use the point estimate as
		// the lower bound (i.e., insist on a significant change AND
		// point >= threshold).
		lower = point
	} else {
		// Conservative lower bound: assume baseline came in at its
		// upper CI and SIMD came in at its lower CI. This is wider
		// than a proper ratio CI (which would correlate the two) but
		// matches benchstat's "lower 95% CI" convention for the
		// metric — a true 95% CI on the ratio requires bootstrap
		// resampling of the raw samples, which is outside the scope
		// of a CSV-driven gate.
		lower = (simdBps - simdCI) / (baseBps + baseCI)
		if lower < 0 {
			lower = 0
		}
	}

	res := gateResult{
		pointRatio: point,
		lowerRatio: lower,
		pStr:       pStr,
		pValue:     pValue,
	}
	switch {
	case pValue < 0:
		// "~" / no significant change. Per the protocol's
		// "lower-95%-CI > threshold" rule, no signal = no pass.
		res.failReason = "no statistically significant change"
	case pValue >= alpha:
		res.failReason = fmt.Sprintf("p=%g >= alpha=%g", pValue, alpha)
	case lower < ratioThreshold:
		res.failReason = fmt.Sprintf("lower95=%.4fx < threshold=%.4fx (point=%.4fx)", lower, ratioThreshold, point)
	default:
		res.pass = true
	}
	return res, nil
}

// parseCI parses the CI column. benchstat writes "∞" when there are
// not enough samples to compute a CI at the requested level. Numeric
// values are absolute widths in the same units as the metric column
// to their left.
func parseCI(s string) (width float64, infinite bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "∞" || s == "inf" || s == "Inf" {
		return 0, true
	}
	w, err := strconv.ParseFloat(s, 64)
	if err != nil {
		// Treat malformed CIs as infinite so the gate falls back to
		// point-estimate mode instead of erroring.
		return 0, true
	}
	// strconv.ParseFloat accepts "NaN" / "Inf" without error; both
	// indicate benchstat couldn't compute a usable CI for this row.
	// Force them onto the infinite-fallback path.
	if math.IsNaN(w) || math.IsInf(w, 0) {
		return 0, true
	}
	return w, false
}

// parseP extracts the numeric p-value from benchstat's "p=… n=…"
// column. Returns -1 if the column is empty or "~" (benchstat's
// "no significant change" sentinel).
func parseP(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "~" {
		return -1
	}
	// Format: "p=0.001 n=10". Split on whitespace, take the p=… part.
	for tok := range strings.FieldsSeq(s) {
		if after, ok := strings.CutPrefix(tok, "p="); ok {
			v, err := strconv.ParseFloat(after, 64)
			if err != nil {
				return -1
			}
			return v
		}
	}
	return -1
}

// findRepoRoot walks upward from the current directory looking for
// go.mod; returns an absolute path. Lets the tool be invoked from any
// working directory and still reach the package.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("toml-perf-gate: no go.mod found in any ancestor directory")
		}
		dir = parent
	}
}

// sortedScanNames returns the validScans keys joined for help text.
func sortedScanNames() string {
	keys := make([]string, 0, len(validScans))
	for k := range validScans {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func die(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "toml-perf-gate: "+format+"\n", args...)
	os.Exit(code)
}
