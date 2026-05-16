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

//go:build bench
// +build bench

package smoketest_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	burnttoml "github.com/BurntSushi/toml"
	pelletiertoml "github.com/pelletier/go-toml/v2"
	"github.com/zchee/pandaemonium/pkg/toml/internal/smoketest"
)

type cargoLock struct {
	Version  int            `toml:"version"`
	Package  []cargoPackage `toml:"package"`
	Metadata cargoMetadata  `toml:"metadata"`
}

type cargoPackage struct {
	Name         string   `toml:"name"`
	Version      string   `toml:"version"`
	Source       string   `toml:"source"`
	Checksum     string   `toml:"checksum"`
	Dependencies []string `toml:"dependencies"`
}

type cargoMetadata struct {
	Checksum string `toml:"checksum"`
}

func TestSmoketestUnmarshalCargoLock(t *testing.T) {
	body := mustReadCorpus(t)
	var got cargoLock
	if err := smoketest.Unmarshal(body, &got); err != nil {
		t.Fatalf("smoketest.Unmarshal(cargo.lock) error = %v", err)
	}
	if got.Version != 3 {
		t.Fatalf("Version = %d, want 3", got.Version)
	}
	if len(got.Package) < 200 {
		t.Fatalf("Package count = %d, want representative Cargo.lock package table population", len(got.Package))
	}
	if got.Package[0].Name == "" || got.Package[0].Version == "" {
		t.Fatalf("first package not populated: %+v", got.Package[0])
	}
}

func BenchmarkSmoketestUnmarshal_Pandaemonium(b *testing.B) {
	body := mustReadCorpus(b)
	benchmarkUnmarshal(b, body, func(dst *cargoLock) error {
		return smoketest.Unmarshal(body, dst)
	})
}

func BenchmarkSmoketestUnmarshal_BurntSushi(b *testing.B) {
	body := mustReadCorpus(b)
	benchmarkUnmarshal(b, body, func(dst *cargoLock) error {
		return burnttoml.Unmarshal(body, dst)
	})
}

func BenchmarkSmoketestUnmarshal_Pelletier(b *testing.B) {
	body := mustReadCorpus(b)
	benchmarkUnmarshal(b, body, func(dst *cargoLock) error {
		return pelletiertoml.Unmarshal(body, dst)
	})
}

func benchmarkUnmarshal(b *testing.B, body []byte, fn func(*cargoLock) error) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	var warm cargoLock
	if err := fn(&warm); err != nil {
		b.Fatalf("warm-up unmarshal failed: %v", err)
	}

	for b.Loop() {
		var out cargoLock
		if err := fn(&out); err != nil {
			b.Fatalf("unmarshal failed: %v", err)
		}
	}
}

func mustReadCorpus(tb testing.TB) []byte {
	tb.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "corpus", "cargo.lock")
	body, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("os.ReadFile(%s) error = %v", path, err)
	}
	return body
}
