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
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const (
	wantGoModDirective = "go 1.27"
	wantCargoLockSHA   = "9ea94b60b3ee80c73f52186946bb280dc41c7287bbb678988618a6839533dbe9"
	wantCargoLockBytes = 103263

	wantTomlRsURL        = "https://github.com/toml-rs/toml"
	wantTomlRsRef        = "v0.25.11"
	wantTomlRsCommit     = "45456abc190bcf7b81dfc96914b726d7b3053e41"
	wantTomlRsSourcePath = "crates/toml/tests/fixtures"
	wantTomlRsFileCount  = 68
	wantTomlRsTotalBytes = 2075
	wantTomlRsTreeSHA    = "01daf47230b2211724854b7cb731a4c9c0d60ced84fa310920ae35e9800b389c"
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

func TestUpstreamPinsTomlRsCorpusManifest(t *testing.T) {
	t.Parallel()

	provenance := parseKeyValueFile(t, "pkg/toml/testdata/toml-rs/provenance.txt")
	lock := parseKeyValueFile(t, "pkg/toml/testdata/toml-rs/.import-lock")
	for _, tc := range []struct {
		name string
		got  map[string]string
	}{
		{name: "provenance", got: provenance},
		{name: "import lock", got: lock},
	} {
		assertKeyValue(t, tc.name, tc.got, "source_url", wantTomlRsURL)
		assertKeyValue(t, tc.name, tc.got, "UPSTREAM_REPO", wantTomlRsURL)
		assertKeyValue(t, tc.name, tc.got, "upstream_ref", wantTomlRsRef)
		assertKeyValue(t, tc.name, tc.got, "UPSTREAM_REF", wantTomlRsRef)
		assertKeyValue(t, tc.name, tc.got, "upstream_commit", wantTomlRsCommit)
		assertKeyValue(t, tc.name, tc.got, "UPSTREAM_COMMIT", wantTomlRsCommit)
		assertKeyValue(t, tc.name, tc.got, "imported_path", wantTomlRsSourcePath)
		assertKeyValue(t, tc.name, tc.got, "SOURCE_PATH", wantTomlRsSourcePath)
		assertKeyValue(t, tc.name, tc.got, "corpus_file_count", strconv.Itoa(wantTomlRsFileCount))
		assertKeyValue(t, tc.name, tc.got, "SOURCE_FILE_COUNT", strconv.Itoa(wantTomlRsFileCount))
		assertKeyValue(t, tc.name, tc.got, "corpus_total_bytes", strconv.Itoa(wantTomlRsTotalBytes))
		assertKeyValue(t, tc.name, tc.got, "SOURCE_TOTAL_BYTES", strconv.Itoa(wantTomlRsTotalBytes))
		assertKeyValue(t, tc.name, tc.got, "corpus_tree_sha256", wantTomlRsTreeSHA)
		assertKeyValue(t, tc.name, tc.got, "SOURCE_TREE_SHA256", wantTomlRsTreeSHA)
	}

	upstreamBody := mustReadRepoFile(t, "pkg/toml/UPSTREAM.md")
	for _, want := range []string{
		"Tag / commit  : " + wantTomlRsRef + " / " + wantTomlRsCommit,
		"Corpus source : " + wantTomlRsSourcePath,
		"Corpus path   : pkg/toml/testdata/toml-rs/corpus",
		fmt.Sprintf("Corpus files  : %d", wantTomlRsFileCount),
		fmt.Sprintf("Corpus bytes  : %d", wantTomlRsTotalBytes),
		"Corpus SHA-256: " + wantTomlRsTreeSHA,
		"Re-import     : ./hack/import-toml-rs/import.sh " + wantTomlRsRef,
	} {
		if !strings.Contains(upstreamBody, want) {
			t.Fatalf("UPSTREAM.md missing %q", want)
		}
	}

	manifestBody := mustReadRepoFile(t, "pkg/toml/testdata/toml-rs/manifest.txt")
	entries := parseManifest(t, manifestBody)
	if got := len(entries); got != wantTomlRsFileCount {
		t.Fatalf("toml-rs manifest row count = %d, want %d", got, wantTomlRsFileCount)
	}

	corpusRoot := mustRepoPath(t, "pkg/toml/testdata/toml-rs/corpus")
	var totalBytes int
	err := filepath.WalkDir(corpusRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(corpusRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entry, ok := entries[rel]
		if !ok {
			return fmt.Errorf("%s missing from manifest", rel)
		}
		sum := sha256.Sum256(body)
		gotSHA := hex.EncodeToString(sum[:])
		if gotSHA != entry.sha256 {
			return fmt.Errorf("%s sha256 = %s, want manifest %s", rel, gotSHA, entry.sha256)
		}
		if len(body) != entry.bytes {
			return fmt.Errorf("%s bytes = %d, want manifest %d", rel, len(body), entry.bytes)
		}
		totalBytes += len(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if totalBytes != wantTomlRsTotalBytes {
		t.Fatalf("toml-rs corpus total bytes = %d, want %d", totalBytes, wantTomlRsTotalBytes)
	}

	sum := sha256.Sum256([]byte(manifestBody))
	if got := hex.EncodeToString(sum[:]); got != wantTomlRsTreeSHA {
		t.Fatalf("toml-rs manifest sha256 = %s, want %s", got, wantTomlRsTreeSHA)
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

type manifestEntry struct {
	sha256 string
	bytes  int
}

func parseManifest(t *testing.T, body string) map[string]manifestEntry {
	t.Helper()

	entries := make(map[string]manifestEntry)
	for lineNo, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if lineNo == 0 {
			if line != "# path\\tsha256\\tbytes" {
				t.Fatalf("unexpected manifest header %q", line)
			}
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("manifest line %d has %d fields, want 3: %q", lineNo+1, len(fields), line)
		}
		bytes, err := strconv.Atoi(fields[2])
		if err != nil {
			t.Fatalf("manifest line %d invalid byte count %q: %v", lineNo+1, fields[2], err)
		}
		entries[fields[0]] = manifestEntry{sha256: fields[1], bytes: bytes}
	}
	return entries
}

func parseKeyValueFile(t *testing.T, rel string) map[string]string {
	t.Helper()

	path := mustRepoPath(t, rel)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("close %s: %v", path, err)
		}
	}()

	values := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("%s contains non key/value line %q", rel, line)
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return values
}

func assertKeyValue(t *testing.T, name string, values map[string]string, key string, want string) {
	t.Helper()

	got, ok := values[key]
	if !ok {
		return
	}
	if got != want {
		t.Fatalf("%s %s = %q, want %q", name, key, got, want)
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
