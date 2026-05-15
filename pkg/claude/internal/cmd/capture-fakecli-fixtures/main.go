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

// capture-fakecli-fixtures connects to the real claude CLI and captures raw
// stream-JSON output to pkg/claude/testdata/stream/*.jsonl.
//
// # Usage
//
//	RUN_REAL_CLAUDE_TESTS=1 go run -tags capture \
//	  ./pkg/claude/internal/cmd/capture-fakecli-fixtures
//
// The tool skips execution unless RUN_REAL_CLAUDE_TESTS=1 is set to prevent
// accidental invocation in CI without a real claude binary.
//
// # Fixture refresh workflow
//
// Run the tool any time the claude CLI stream-JSON protocol changes. The
// generated .jsonl files in testdata/stream/ are committed to the repository
// and used by message_parser_test.go. After refreshing:
//
//  1. Verify go test -race -count=1 ./pkg/claude/... passes.
//  2. Commit the updated fixtures with the CLI version in the commit message.
//
// All source files in this package carry the //go:build capture build tag so
// the package never participates in the default go build or go test run.
//
//go:build capture

package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	if os.Getenv("RUN_REAL_CLAUDE_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "capture-fakecli-fixtures: set RUN_REAL_CLAUDE_TESTS=1 to enable capture")
		os.Exit(1)
	}

	// Locate the claude binary on PATH.
	cliPath, err := exec.LookPath("claude")
	if err != nil {
		log.Fatalf("claude binary not found on PATH: %v", err)
	}
	log.Printf("using claude binary: %s", cliPath)

	// Output directory: pkg/claude/testdata/stream/ relative to this file.
	_, thisFile, _, _ := runtime.Caller(0)
	outDir := filepath.Join(filepath.Dir(thisFile), "../../../../testdata/stream")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}

	// Scripted captures: each entry produces one .jsonl fixture file.
	captures := []struct {
		name   string
		prompt string
	}{
		{"assistant_text", "Say exactly: Hello, world!"},
		{"tool_use", "Run: echo hello_fixture"},
		{"result", "Reply with a single word: ok"},
	}

	for _, c := range captures {
		outPath := filepath.Join(outDir, c.name+".jsonl")
		log.Printf("capturing %s → %s", c.name, outPath)
		if err := capture(cliPath, c.prompt, outPath); err != nil {
			log.Fatalf("capture %s: %v", c.name, err)
		}
	}
	log.Println("capture complete")
}

// capture runs claude with the given prompt in stream-json mode and writes
// the raw output to outPath, one JSON object per line.
func capture(cliPath, prompt, outPath string) error {
	cmd := exec.Command(cliPath,
		"--output-format", "stream-json",
		"--print", prompt,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	sc := bufio.NewScanner(io.LimitReader(stdout, 16<<20)) // 16 MiB cap
	for sc.Scan() {
		line := sc.Text()
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("write: %w", err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("scan stdout: %w", err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("wait: %w", err)
	}
	return nil
}
