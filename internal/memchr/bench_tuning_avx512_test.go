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

//go:build memchr_tuning && amd64 && amd64.v4 && goexperiment.simd && !force_swar

package memchr

import "testing"

func benchmarkTuningDirectAVX512Memchr(b *testing.B, cases []tuningBenchmarkCase) {
	runTuningScan(b, "direct_avx512", cases, func(hay []byte) int {
		return avx512Memchr(benchNeedle, hay)
	})
}

func benchmarkTuningDirectAVX512Memchr2(b *testing.B, cases []tuningBenchmarkCase) {
	runTuningScan(b, "direct_avx512", cases, func(hay []byte) int {
		return avx512Memchr2(benchNeedle, 0xC3, hay)
	})
}

func benchmarkTuningDirectAVX512Memchr3(b *testing.B, cases []tuningBenchmarkCase) {
	runTuningScan(b, "direct_avx512", cases, func(hay []byte) int {
		return avx512Memchr3(benchNeedle, 0xC3, 0xD4, hay)
	})
}

func benchmarkTuningDirectAVX512Memrchr(b *testing.B, cases []tuningBenchmarkCase) {
	runTuningScan(b, "direct_avx512", cases, func(hay []byte) int {
		return avx512Memrchr(benchNeedle, hay)
	})
}

func benchmarkTuningDirectAVX512Memrchr2(b *testing.B, cases []tuningBenchmarkCase) {
	runTuningScan(b, "direct_avx512", cases, func(hay []byte) int {
		return avx512Memrchr2(benchNeedle, 0xC3, hay)
	})
}

func benchmarkTuningDirectAVX512Memrchr3(b *testing.B, cases []tuningBenchmarkCase) {
	runTuningScan(b, "direct_avx512", cases, func(hay []byte) int {
		return avx512Memrchr3(benchNeedle, 0xC3, 0xD4, hay)
	})
}
