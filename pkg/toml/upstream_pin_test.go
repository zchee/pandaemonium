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

package toml_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	wantGoModDirective = "go 1.27"
	wantCargoLockSHA   = "9ea94b60b3ee80c73f52186946bb280dc41c7287bbb678988618a6839533dbe9"
	wantCargoLockBytes = 103263
)

func TestUpstreamPinsGoModDirective(t *testing.T) {
	t.Parallel()

	body := mustReadRepoFile(t, "go.mod")
	if !strings.Contains(body, wantGoModDirective+"\n") {
		t.Fatalf("go.mod missing directive %q\n--- go.mod ---\n%s", wantGoModDirective, body)
	}
}

func TestUpstreamPinsCargoLockCorpus(t *testing.T) {
	t.Parallel()

	path := mustRepoPath(t, "pkg/toml/testdata/corpus/cargo.lock")
	body := mustReadRepoFile(t, "pkg/toml/testdata/corpus/cargo.lock")

	sum := sha256.Sum256([]byte(body))
	if got := hex.EncodeToString(sum[:]); got != wantCargoLockSHA {
		t.Fatalf("%s sha256 = %s, want %s", path, got, wantCargoLockSHA)
	}
	if got := len(body); got != wantCargoLockBytes {
		t.Fatalf("%s byte length = %d, want %d", path, got, wantCargoLockBytes)
	}
}

func TestUpstreamPinsWorkflowToolchainPins(t *testing.T) {
	t.Parallel()

	scanCI := mustReadRepoFile(t, ".github/workflows/toml-scan-ci.yml")
	for _, want := range []string{
		"name: wasip1/wasm build-only",
		"GOOS: wasip1",
		"GOARCH: wasm",
		"go-version: \"stable\"",
	} {
		if !strings.Contains(scanCI, want) {
			t.Fatalf("toml-scan-ci.yml missing %q\n--- toml-scan-ci.yml ---\n%s", want, scanCI)
		}
	}

	testYAML := mustReadRepoFile(t, ".github/workflows/test.yaml")
	if !strings.Contains(testYAML, "go-version-file: \"go.mod\"") {
		t.Fatalf("test.yaml missing go-version-file pin to go.mod\n--- test.yaml ---\n%s", testYAML)
	}
}

func mustReadRepoFile(t *testing.T, rel string) string {
	t.Helper()

	path := mustRepoPath(t, rel)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", path, err)
	}
	return string(body)
}

func mustRepoPath(t *testing.T, rel string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(repoRoot, filepath.FromSlash(rel))
}
