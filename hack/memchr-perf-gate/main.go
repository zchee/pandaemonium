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

// memchr-perf-gate is the CI hard gate behind plan AC-HARNESS-6
// (.omc/plans/2026-05-16-internal-memchr-port.md L220-226). It runs the
// internal/memchr benchmarks twice — once captured as the
// BenchmarkIndexByteStd baseline, once as the BenchmarkMemchr treatment
// — then feeds both through benchstat with the U-test at α=0.05 to
// determine whether any Memchr/n=N (N≥64) is statistically slower than
// the corresponding bytes.IndexByte baseline. On amd64 a regression
// returns exit code 1; on arm64 the gate prints "untested" and exits 0
// until a Linux/arm64 CI runner is provisioned (tracked in plan Follow-
// ups). The n=16 case is reported but excluded from the hard gate
// because the plan only requires match-or-beat at n≥64.
//
// The script prefers a `benchstat` binary on $PATH, then falls back to the
// module tool directive via `go tool benchstat`.
//
// Usage:
//
//	go run ./hack/memchr-perf-gate
//	go run ./hack/memchr-perf-gate --compare-artifacts=v3,v4
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// flagCount is the bench iteration count passed to `go test -count=N`.
// Plan AC-HARNESS-6 locks this at 10.
const flagCount = 10

// gatedSizes are the haystack sizes the hard gate enforces. n=16 is
// reported but not gated (plan §"Risks" line 6, "the n=16 case is
// excluded from the hard gate; n=16 is reported but non-blocking").
var gatedSizes = []int{64, 256, 1024, 4096, 65536}

// artifactPracticalDeltaPercent is the minimum statistically significant
// slowdown that fails --compare-artifacts hard rows. The stdlib-vs-Memchr gate
// intentionally does not use this threshold: any statistically significant
// positive sec/op delta remains a failure there.
const artifactPracticalDeltaPercent = 5.0

// benchstatPath is the path to the benchstat binary. Defaults to the
// $PATH lookup but can be overridden via --benchstat for local hacking.
var benchstatPath = flag.String("benchstat", "benchstat", "path to benchstat binary")

// goamd64Level overrides the automatically selected amd64 artifact level. The
// default follows the package plan: prefer GOAMD64=v4 when simd/archsimd reports
// AVX-512 support, otherwise use the GOAMD64=v3 AVX2 fallback when available.
var goamd64Level = flag.String("goamd64", "", "amd64 artifact level for benchmark subprocesses (default: archsimd-selected v4 or v3)")

// compareArtifacts compares two GOAMD64 artifacts as baseline,treatment, for
// example --compare-artifacts=v3,v4. This mode is used by the AVX-512 rollout
// gate so v4 regressions against the v3 AVX2 fallback fail CI.
var compareArtifacts = flag.String("compare-artifacts", "", "compare two GOAMD64 artifacts as baseline,treatment, for example v3,v4")

// artifactBenchPattern is the benchmark regex used by --compare-artifacts.
var artifactBenchPattern = flag.String("bench", "^(BenchmarkMemchr|BenchmarkMemchr2|BenchmarkMemchr3|BenchmarkMemrchr|BenchmarkMemrchr2|BenchmarkMemrchr3)$", "benchmark regex for --compare-artifacts")

func main() {
	flag.Parse()

	if runtime.GOARCH == "arm64" {
		fmt.Println("memchr-perf-gate: arm64 path is UNTESTED — no arm64 CI runner provisioned; exiting 0")
		fmt.Println("memchr-perf-gate: see plan §Follow-ups; flip this branch when a Linux/arm64 runner lands.")
		os.Exit(0)
	}

	if runtime.GOARCH != "amd64" {
		fmt.Printf("memchr-perf-gate: GOARCH=%s is UNTESTED by this gate; only amd64 is enforced; exiting 0\n", runtime.GOARCH)
		os.Exit(0)
	}

	if *compareArtifacts != "" {
		if err := runArtifactCompare(*compareArtifacts, *artifactBenchPattern); err != nil {
			die("%v", err)
		}
		return
	}

	benchGOAMD64 := selectBenchGOAMD64()
	if benchGOAMD64 == "" {
		fmt.Println("memchr-perf-gate: amd64 SIMD artifact is UNTESTED — simd/archsimd reports neither AVX-512 nor AVX2; exiting 0")
		os.Exit(0)
	}
	fmt.Printf("memchr-perf-gate: benchmarking GOAMD64=%s artifact selected via simd/archsimd\n", benchGOAMD64)

	tmp, err := os.MkdirTemp("", "memchr-perf-gate-*")
	if err != nil {
		die("mktemp: %v", err)
	}
	defer os.RemoveAll(tmp)

	baseFile, err := runBench(tmp, "BenchmarkIndexByteStd", "base", benchGOAMD64)
	if err != nil {
		die("run baseline bench: %v", err)
	}
	treatFile, err := runBench(tmp, "BenchmarkMemchr$", "treat", benchGOAMD64)
	if err != nil {
		die("run treatment bench: %v", err)
	}

	// Normalize benchmark names so benchstat pairs them. The bench
	// names in each file are different (BenchmarkIndexByteStd vs
	// BenchmarkMemchr); after this rewrite both files use the common
	// stem "BenchmarkScan" so benchstat treats them as the same row.
	if err := renameInPlace(baseFile, "BenchmarkIndexByteStd", "BenchmarkScan"); err != nil {
		die("rename baseline: %v", err)
	}
	if err := renameInPlace(treatFile, "BenchmarkMemchr", "BenchmarkScan"); err != nil {
		die("rename treatment: %v", err)
	}

	bsOut, err := runBenchstat(*benchstatPath, baseFile, treatFile)
	if err != nil {
		die("benchstat: %v", err)
	}
	fmt.Println(strings.TrimRight(bsOut, "\n"))

	regressions := findRegressions(bsOut)
	if len(regressions) == 0 {
		fmt.Println()
		fmt.Println("memchr-perf-gate: PASS (no gated Memchr size is statistically slower than bytes.IndexByte)")
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "memchr-perf-gate: FAIL — one or more gated sizes regressed:")
	for _, r := range regressions {
		fmt.Fprintf(os.Stderr, "  %s\n", r)
	}
	os.Exit(1)
}

func runArtifactCompare(spec, benchPattern string) error {
	baseline, treatment, err := parseArtifactPair(spec)
	if err != nil {
		return err
	}
	if err := validateArtifactPairSupported(baseline, treatment); err != nil {
		return err
	}
	fmt.Printf("memchr-perf-gate: comparing GOAMD64=%s baseline to GOAMD64=%s treatment\n", baseline, treatment)
	fmt.Printf("memchr-perf-gate: artifact benchmark regex: %s\n", benchPattern)

	tmp, err := os.MkdirTemp("", "memchr-perf-gate-artifacts-*")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(tmp)

	baseFile, err := runBench(tmp, benchPattern, baseline, baseline)
	if err != nil {
		return fmt.Errorf("run GOAMD64=%s baseline bench: %w", baseline, err)
	}
	treatFile, err := runBench(tmp, benchPattern, treatment, treatment)
	if err != nil {
		return fmt.Errorf("run GOAMD64=%s treatment bench: %w", treatment, err)
	}

	bsOut, err := runBenchstat(*benchstatPath, baseFile, treatFile)
	if err != nil {
		return fmt.Errorf("benchstat: %w", err)
	}
	fmt.Println(strings.TrimRight(bsOut, "\n"))

	thresholds := findArtifactThresholdRegressions(bsOut)
	if len(thresholds) > 0 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintf(os.Stderr, "memchr-perf-gate: WARN — GOAMD64=%s threshold rows need explicit tuning evidence review:\n", treatment)
		for _, r := range thresholds {
			fmt.Fprintf(os.Stderr, "  %s\n", r)
		}
	}

	regressions := findArtifactRegressions(bsOut)
	if len(regressions) == 0 {
		fmt.Println()
		fmt.Printf("memchr-perf-gate: PASS (no hard-gated GOAMD64=%s routine regressed by >=%.2f%% against GOAMD64=%s)\n", treatment, artifactPracticalDeltaPercent, baseline)
		return nil
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "memchr-perf-gate: FAIL — GOAMD64=%s regressed against GOAMD64=%s:\n", treatment, baseline)
	for _, r := range regressions {
		fmt.Fprintf(os.Stderr, "  %s\n", r)
	}
	os.Exit(1)
	return nil
}

func parseArtifactPair(spec string) (baseline, treatment string, err error) {
	parts := strings.Split(spec, ",")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("--compare-artifacts expects baseline,treatment; got %q", spec)
	}
	baseline = strings.TrimSpace(parts[0])
	treatment = strings.TrimSpace(parts[1])
	if baseline == "" || treatment == "" {
		return "", "", fmt.Errorf("--compare-artifacts expects non-empty baseline,treatment; got %q", spec)
	}
	return baseline, treatment, nil
}

func validateArtifactPairSupported(baseline, treatment string) error {
	widest := autodetectBenchGOAMD64()
	for _, level := range []string{baseline, treatment} {
		switch level {
		case "v4":
			if widest != "v4" {
				return errors.New("GOAMD64=v4 artifact requested but simd/archsimd did not report AVX-512 support")
			}
		case "v3":
			if widest == "" {
				return errors.New("GOAMD64=v3 artifact requested but simd/archsimd did not report AVX2 support")
			}
		}
	}
	return nil
}

func selectBenchGOAMD64() string {
	if *goamd64Level != "" {
		return *goamd64Level
	}
	return autodetectBenchGOAMD64()
}

// runBench runs `go test -bench=<pattern> -benchmem -count=N -run=^$
// ./internal/memchr/` and writes the output to <dir>/<label>.txt. It
// returns the path of the written file.
func runBench(dir, pattern, label, goamd64 string) (string, error) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return "", err
	}
	args := []string{
		"test",
		"-bench=" + pattern,
		"-benchmem",
		"-count=" + strconv.Itoa(flagCount),
		"-run=^$",
		"-timeout=600s",
		"./internal/memchr/",
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOAMD64="+goamd64)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go %s: %w\nstderr:\n%s", strings.Join(args, " "), err, stderr.String())
	}
	path := filepath.Join(dir, label+".txt")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// renameInPlace replaces every occurrence of `from` with `to` in the
// file at `path`. Used to align benchmark names across baseline and
// treatment files so benchstat pairs them.
func renameInPlace(path, from, to string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	b = bytes.ReplaceAll(b, []byte(from), []byte(to))
	return os.WriteFile(path, b, 0o644)
}

// runBenchstat shells out to the benchstat binary. Older benchstat builds
// accept the project-locked -delta-test=utest flag; newer x/perf releases
// removed that flag and use their built-in comparison test. Prefer the locked
// invocation when available, fall back to -alpha=0.05 for newer benchstat, and
// if the requested benchstat binary is unavailable fall back to the module's
// `go tool benchstat` directive.
func runBenchstat(bin, baseFile, treatFile string) (string, error) {
	commands := []benchstatCommand{{name: bin}}
	if bin == "benchstat" {
		commands = append(commands, benchstatCommand{name: "go", prefixArgs: []string{"tool", "benchstat"}})
	}
	var errs []string
	for _, command := range commands {
		out, err := runBenchstatCommand(command, baseFile, treatFile)
		if err == nil {
			return out, nil
		}
		errs = append(errs, err.Error())
	}
	return "", errors.New(strings.Join(errs, "\n"))
}

type benchstatCommand struct {
	name       string
	prefixArgs []string
}

func runBenchstatCommand(command benchstatCommand, baseFile, treatFile string) (string, error) {
	out, err := runBenchstatArgs(command.name, command.args("-delta-test=utest", "-alpha=0.05", baseFile, treatFile))
	if err == nil {
		return out, nil
	}
	if !strings.Contains(out, "flag provided but not defined: -delta-test") {
		return "", fmt.Errorf("%s: %w\nbenchstat output:\n%s", command.String(), err, out)
	}
	out, err = runBenchstatArgs(command.name, command.args("-alpha=0.05", baseFile, treatFile))
	if err != nil {
		return "", fmt.Errorf("%s: %w\nbenchstat output:\n%s", command.String(), err, out)
	}
	return out, nil
}

func (command benchstatCommand) String() string {
	return strings.Join(append([]string{command.name}, command.prefixArgs...), " ")
}

func (command benchstatCommand) args(args ...string) []string {
	out := make([]string, 0, len(command.prefixArgs)+len(args))
	out = append(out, command.prefixArgs...)
	return append(out, args...)
}

func runBenchstatArgs(bin string, args []string) (string, error) {
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// benchstatRowRe captures a benchstat row that flagged the treatment as
// slower than the baseline. Statistical significance and row policy are applied
// after parsing. The human-readable form is included in the failure summary.
//
// Example benchstat row (the perf-gate parses the table format):
//
//	BenchmarkScan/n=64       42.0n ± 2%   55.0n ± 3%   +30.95% (p=0.000 n=10)
//	Memchr2/n=64             42.0n ± 2%   55.0n ± 3%   +30.95% (p=0.000 n=10)
//
// A positive `+X.XX% (p=...)` cell on a sec/op row may be a regression,
// depending on the active policy.
var benchstatRowRe = regexp.MustCompile(`^\s*(?:Benchmark)?([A-Za-z0-9_./=-]*?)/n=(\d+)(?:-\d+)?.*?\+([\d.]+)%\s+\(p=([\d.]+)\s+n=\d+\)`)

type gatePolicy int

const (
	stdlibGate gatePolicy = iota
	artifactGate
)

type rowClass string

const (
	rowHard      rowClass = "hard"
	rowThreshold rowClass = "threshold"
	rowAdvisory  rowClass = "advisory"
	rowTuning    rowClass = "tuning"
)

type benchstatRegression struct {
	name         string
	size         int
	deltaPercent float64
	pValue       float64
	class        rowClass
}

// findRegressions scans benchstat output for any gated Memchr size
// whose treatment ns/op is statistically slower than the baseline at
// α=0.05.
func findRegressions(bsOut string) []string {
	return findPolicyRegressions(bsOut, stdlibGate)
}

func findArtifactRegressions(bsOut string) []string {
	return findPolicyRegressions(bsOut, artifactGate)
}

func findArtifactThresholdRegressions(bsOut string) []string {
	var out []string
	for _, row := range classifyBenchstatRows(bsOut, artifactGate) {
		if row.class != rowThreshold || row.pValue > 0.05 {
			continue
		}
		out = append(out, row.String(artifactGate))
	}
	return out
}

func findPolicyRegressions(bsOut string, policy gatePolicy) []string {
	var out []string
	for _, row := range classifyBenchstatRows(bsOut, policy) {
		if !row.fails(policy) {
			continue
		}
		out = append(out, row.String(policy))
	}
	return out
}

func classifyBenchstatRows(bsOut string, policy gatePolicy) []benchstatRegression {
	var out []benchstatRegression
	inSecPerOp := false
	for line := range strings.SplitSeq(bsOut, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "sec/op") {
			inSecPerOp = true
			continue
		}
		if inSecPerOp && trimmed == "" {
			break
		}
		if !inSecPerOp {
			continue
		}
		m := benchstatRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		n, _ := strconv.Atoi(m[2])
		delta, _ := strconv.ParseFloat(m[3], 64)
		p, _ := strconv.ParseFloat(m[4], 64)
		out = append(out, benchstatRegression{
			name:         name,
			size:         n,
			deltaPercent: delta,
			pValue:       p,
			class:        classifyRow(name, n, policy),
		})
	}
	return out
}

func classifyRow(name string, size int, policy gatePolicy) rowClass {
	if strings.HasPrefix(name, "Tuning") || strings.Contains(name, "/Tuning") {
		return rowTuning
	}
	switch policy {
	case artifactGate:
		if size == 64 || size == 128 || size == 256 {
			return rowThreshold
		}
		if size >= 1024 {
			return rowHard
		}
		return rowAdvisory
	default:
		for _, gated := range gatedSizes {
			if size == gated {
				return rowHard
			}
		}
		return rowAdvisory
	}
}

func (row benchstatRegression) fails(policy gatePolicy) bool {
	if row.class != rowHard || row.pValue > 0.05 {
		return false
	}
	if policy == artifactGate {
		return row.deltaPercent >= artifactPracticalDeltaPercent
	}
	return true
}

func (row benchstatRegression) String(policy gatePolicy) string {
	label := fmt.Sprintf("n=%d", row.size)
	if row.name != "Scan" {
		label = fmt.Sprintf("%s/%s", row.name, label)
	}
	if policy == artifactGate {
		return fmt.Sprintf("%s: +%.2f%% slower (p=%.3f, class=%s, threshold=%.2f%%)", label, row.deltaPercent, row.pValue, row.class, artifactPracticalDeltaPercent)
	}
	return fmt.Sprintf("%s: +%.2f%% slower (p=%.3f)", label, row.deltaPercent, row.pValue)
}

// findRepoRoot walks upward from the current directory looking for
// go.mod. Returns an absolute path. Required so this program can run
// from any working directory and still reach the package.
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
			return "", errors.New("memchr-perf-gate: no go.mod found in any ancestor directory")
		}
		dir = parent
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "memchr-perf-gate: "+format+"\n", args...)
	os.Exit(2)
}
