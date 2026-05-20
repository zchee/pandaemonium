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

//go:build amd64 && !amd64.v3 && goexperiment.simd && !force_swar

package memchr

import simd "simd/archsimd"

var useAVX2Backend bool

func memchr(needle byte, haystack []byte) int {
	if useAVX2Backend {
		return avx2Memchr(needle, haystack)
	}
	return memchrImpl(needle, haystack)
}

func memchr2(n1, n2 byte, haystack []byte) int {
	if useAVX2Backend {
		return avx2Memchr2(n1, n2, haystack)
	}
	return memchr2Impl(n1, n2, haystack)
}

func memchr3(n1, n2, n3 byte, haystack []byte) int {
	if useAVX2Backend {
		return avx2Memchr3(n1, n2, n3, haystack)
	}
	return memchr3Impl(n1, n2, n3, haystack)
}

func memrchr(needle byte, haystack []byte) int {
	if useAVX2Backend {
		return avx2Memrchr(needle, haystack)
	}
	return memrchrImpl(needle, haystack)
}

func memrchr2(n1, n2 byte, haystack []byte) int {
	if useAVX2Backend {
		return avx2Memrchr2(n1, n2, haystack)
	}
	return memrchr2Impl(n1, n2, haystack)
}

func memrchr3(n1, n2, n3 byte, haystack []byte) int {
	if useAVX2Backend {
		return avx2Memrchr3(n1, n2, n3, haystack)
	}
	return memrchr3Impl(n1, n2, n3, haystack)
}

// init runs at package load for GOAMD64=v1/v2 amd64 SIMD builds and selects
// between the AVX2 hot paths and the SSE2 Go fallback based on
// simd/archsimd runtime CPU detection.
func init() {
	if simd.X86.AVX2() {
		useAVX2Backend = true
		memchrImpl = avx2Memchr
		memchr2Impl = avx2Memchr2
		memchr3Impl = avx2Memchr3
		memrchrImpl = avx2Memrchr
		memrchr2Impl = avx2Memrchr2
		memrchr3Impl = avx2Memrchr3
		setBackendMarkers("avx2", "avx2", "avx2", "avx2", "avx2", "avx2", "avx2")
		return
	}
	useAVX2Backend = false
	memchrImpl = sse2Memchr
	memchr2Impl = sse2Memchr2
	memchr3Impl = sse2Memchr3
	memrchrImpl = sse2Memrchr
	memrchr2Impl = sse2Memrchr2
	memrchr3Impl = sse2Memrchr3
	setBackendMarkers("sse2", "sse2", "sse2", "sse2", "sse2", "sse2", "sse2")
}
