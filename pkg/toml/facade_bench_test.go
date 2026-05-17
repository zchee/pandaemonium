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

var benchScalarDocument = []byte(`title = "pandaemonium"
active = true
count = 42
ratio = 3.14159
description = "scalar-heavy comparison fixture"
`)

type benchScalar struct {
	Title       string  `toml:"title"`
	Active      bool    `toml:"active"`
	Count       int     `toml:"count"`
	Ratio       float64 `toml:"ratio"`
	Description string  `toml:"description"`
}

var benchCargoValue = mustDecodeBenchCargo()

var (
	benchCargoMarshalPelletierSize    = len(mustMarshalPelletierBenchCargo())
	benchCargoMarshalPandaemoniumSize = len(mustMarshalPandaemoniumBenchCargo())
)

var (
	benchCargoSink          benchCargo
	benchScalarSink         benchScalar
	benchMarshalOutput      []byte
	benchDocumentEditOutput []byte
)

func BenchmarkUnmarshal_BurntSushi(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchCargoLock)))
	for b.Loop() {
		var dst benchCargo
		if err := burntsushi.Unmarshal(benchCargoLock, &dst); err != nil {
			b.Fatal(err)
		}
		benchCargoSink = dst
	}
}

func BenchmarkUnmarshal_Pelletier(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchCargoLock)))
	for b.Loop() {
		var dst benchCargo
		if err := pelletier.Unmarshal(benchCargoLock, &dst); err != nil {
			b.Fatal(err)
		}
		benchCargoSink = dst
	}
}

func BenchmarkUnmarshal_Pandaemonium(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchCargoLock)))
	for b.Loop() {
		var dst benchCargo
		if err := toml.Unmarshal(benchCargoLock, &dst); err != nil {
			b.Fatal(err)
		}
		benchCargoSink = dst
	}
}

func BenchmarkMarshal_Pelletier(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(benchCargoMarshalPelletierSize))
	for b.Loop() {
		body, err := pelletier.Marshal(&benchCargoValue)
		if err != nil {
			b.Fatal(err)
		}
		benchMarshalOutput = body
	}
}

func BenchmarkMarshal_Pandaemonium(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(benchCargoMarshalPandaemoniumSize))
	for b.Loop() {
		body, err := toml.Marshal(&benchCargoValue)
		if err != nil {
			b.Fatal(err)
		}
		benchMarshalOutput = body
	}
}

func BenchmarkScalarUnmarshal_Pelletier(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchScalarDocument)))
	for b.Loop() {
		var dst benchScalar
		if err := pelletier.Unmarshal(benchScalarDocument, &dst); err != nil {
			b.Fatal(err)
		}
		benchScalarSink = dst
	}
}

func BenchmarkScalarUnmarshal_Pandaemonium(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchScalarDocument)))
	for b.Loop() {
		var dst benchScalar
		if err := toml.Unmarshal(benchScalarDocument, &dst); err != nil {
			b.Fatal(err)
		}
		benchScalarSink = dst
	}
}

func BenchmarkArrayTableUnmarshal_Pelletier(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchCargoLock)))
	for b.Loop() {
		var dst benchCargo
		if err := pelletier.Unmarshal(benchCargoLock, &dst); err != nil {
			b.Fatal(err)
		}
		benchCargoSink = dst
	}
}

func BenchmarkArrayTableUnmarshal_Pandaemonium(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchCargoLock)))
	for b.Loop() {
		var dst benchCargo
		if err := toml.Unmarshal(benchCargoLock, &dst); err != nil {
			b.Fatal(err)
		}
		benchCargoSink = dst
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

func mustDecodeBenchCargo() benchCargo {
	var cargo benchCargo
	if err := pelletier.Unmarshal(benchCargoLock, &cargo); err != nil {
		panic(err)
	}
	return cargo
}

func mustMarshalPelletierBenchCargo() []byte {
	body, err := pelletier.Marshal(&benchCargoValue)
	if err != nil {
		panic(err)
	}
	return body
}

func mustMarshalPandaemoniumBenchCargo() []byte {
	body, err := toml.Marshal(&benchCargoValue)
	if err != nil {
		panic(err)
	}
	return body
}
