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

package toml_test

import (
	"os"
	"testing"

	burntsushi "github.com/BurntSushi/toml"
	pelletier "github.com/pelletier/go-toml/v2"
	"github.com/zchee/pandaemonium/pkg/toml"
)

type benchPackage struct {
	Name     string
	Version  string
	Source   string
	Checksum string
}

type benchCargo struct {
	Version int            `toml:"version"`
	Package []benchPackage `toml:"package"`
}

var benchCargoLock = mustReadBenchCorpus()

var benchDocumentEditOutput []byte

func BenchmarkUnmarshal_BurntSushi(b *testing.B) {
	b.SetBytes(int64(len(benchCargoLock)))
	for b.Loop() {
		var dst benchCargo
		if err := burntsushi.Unmarshal(benchCargoLock, &dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Pelletier(b *testing.B) {
	b.SetBytes(int64(len(benchCargoLock)))
	for b.Loop() {
		var dst benchCargo
		if err := pelletier.Unmarshal(benchCargoLock, &dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Pandaemonium(b *testing.B) {
	b.SetBytes(int64(len(benchCargoLock)))
	for b.Loop() {
		var dst benchCargo
		if err := toml.Unmarshal(benchCargoLock, &dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDocumentEdit(b *testing.B) {
	benchmarkDocumentEditPandaemonium(b)
}

func BenchmarkDocumentEdit_Pandaemonium(b *testing.B) {
	benchmarkDocumentEditPandaemonium(b)
}

func BenchmarkDocumentEdit_Pelletier(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchCargoLock)))
	for b.Loop() {
		var dst benchCargo
		if err := pelletier.Unmarshal(benchCargoLock, &dst); err != nil {
			b.Fatal(err)
		}
		dst.Version = 4
		body, err := pelletier.Marshal(&dst)
		if err != nil {
			b.Fatal(err)
		}
		benchDocumentEditOutput = body
	}
}

func benchmarkDocumentEditPandaemonium(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchCargoLock)))
	for b.Loop() {
		doc, err := toml.ParseDocument(benchCargoLock)
		if err != nil {
			b.Fatal(err)
		}
		if err := doc.Set("version", int64(4)); err != nil {
			b.Fatal(err)
		}
		if err := doc.InsertAfter("version", "pandaemonium.edited", true); err != nil {
			b.Fatal(err)
		}
		benchDocumentEditOutput = doc.Bytes()
	}
}

func mustReadBenchCorpus() []byte {
	body, err := os.ReadFile("testdata/corpus/cargo.lock")
	if err != nil {
		panic(err)
	}
	return body
}
