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
// The script requires the `benchstat` binary on $PATH. It is added as a
// tool-only dependency to the module via Step 9 (`go get -tool
// golang.org/x/perf/cmd/benchstat@latest`).
//
// Usage:
//
//	go run ./hack/memchr-perf-gate
//
// Or, after pre-generating bench output:
//
//	go test -bench=. -benchmem -count=10 -run=^$
//	    ./internal/memchr/ | tee bench.txt
//	go run ./hack/memchr-perf-gate bench.txt   # not yet implemented; runs benchmarks itself
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

// benchstatPath is the path to the benchstat binary. Defaults to the
// $PATH lookup but can be overridden via --benchstat for local hacking.
var benchstatPath = flag.String("benchstat", "benchstat", "path to benchstat binary")

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

	tmp, err := os.MkdirTemp("", "memchr-perf-gate-*")
	if err != nil {
		die("mktemp: %v", err)
	}
	defer os.RemoveAll(tmp)

	baseFile, err := runBench(tmp, "BenchmarkIndexByteStd", "base")
	if err != nil {
		die("run baseline bench: %v", err)
	}
	treatFile, err := runBench(tmp, "BenchmarkMemchr$", "treat")
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

// runBench runs `go test -bench=<pattern> -benchmem -count=N -run=^$
// ./internal/memchr/` and writes the output to <dir>/<label>.txt. It
// returns the path of the written file.
func runBench(dir, pattern, label string) (string, error) {
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

// runBenchstat shells out to the benchstat binary with the
// project-locked -delta-test=utest -alpha=0.05 flags. AC-HARNESS-6
// pins those flags inline.
func runBenchstat(bin, baseFile, treatFile string) (string, error) {
	cmd := exec.Command(bin, "-delta-test=utest", "-alpha=0.05", baseFile, treatFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w\nbenchstat output:\n%s", bin, err, string(out))
	}
	return string(out), nil
}

// regressionLine captures a benchstat row that flagged the treatment as
// slower than the baseline with statistical significance. The
// human-readable form is included in the failure summary.
//
// Example benchstat row (the perf-gate parses the table format):
//
//	BenchmarkScan/n=64       42.0n ± 2%   55.0n ± 3%   +30.95% (p=0.000 n=10)
//
// A `+X.XX% (p=<0.05 ...)` cell on a gatedSizes row is a regression.
var regressionRowRe = regexp.MustCompile(`BenchmarkScan/n=(\d+).*?\+([\d.]+)%\s+\(p=([\d.]+)\s+n=\d+\)`)

// findRegressions scans benchstat output for any gated Memchr size
// whose treatment ns/op is statistically slower than the baseline at
// α=0.05.
func findRegressions(bsOut string) []string {
	var out []string
	gated := map[int]bool{}
	for _, n := range gatedSizes {
		gated[n] = true
	}
	for _, line := range strings.Split(bsOut, "\n") {
		m := regressionRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if !gated[n] {
			continue
		}
		p, _ := strconv.ParseFloat(m[3], 64)
		if p > 0.05 {
			continue
		}
		out = append(out, fmt.Sprintf("n=%d: +%s%% slower (p=%s)", n, m[2], m[3]))
	}
	return out
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
