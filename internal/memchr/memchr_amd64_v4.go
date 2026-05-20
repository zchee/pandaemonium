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

//go:build amd64 && amd64.v4 && goexperiment.simd && !force_swar

package memchr

// The avx512* routines return the offset of the first/last matching byte in
// haystack, or -1 if no byte matches. GOAMD64=v4 makes AVX-512F/BW/VL part of
// the executable startup contract; host selection is still preflighted with
// simd/archsimd before running or recommending this artifact.
//
//go:noescape
func avx512Memchr(needle byte, haystack []byte) int

//go:noescape
func avx512Memchr2(n1, n2 byte, haystack []byte) int

//go:noescape
func avx512Memchr3(n1, n2, n3 byte, haystack []byte) int

//go:noescape
func avx512Memrchr(needle byte, haystack []byte) int

//go:noescape
func avx512Memrchr2(n1, n2 byte, haystack []byte) int

//go:noescape
func avx512Memrchr3(n1, n2, n3 byte, haystack []byte) int

func memchr(needle byte, haystack []byte) int {
	return avx512Memchr(needle, haystack)
}

func memchr2(n1, n2 byte, haystack []byte) int {
	return avx512ArchMemchr2(n1, n2, haystack)
}

func memchr3(n1, n2, n3 byte, haystack []byte) int {
	return avx512ArchMemchr3(n1, n2, n3, haystack)
}

func memrchr(needle byte, haystack []byte) int {
	return avx512Memrchr(needle, haystack)
}

func memrchr2(n1, n2 byte, haystack []byte) int {
	return avx512ArchMemrchr2(n1, n2, haystack)
}

func memrchr3(n1, n2, n3 byte, haystack []byte) int {
	return avx512ArchMemrchr3(n1, n2, n3, haystack)
}

// init binds the GOAMD64=v4 primary artifact. All current routines are
// AVX-512-backed in this artifact, while GOAMD64=v3 remains the separate AVX2
// fallback artifact.
func init() {
	memchrImpl = avx512Memchr
	memchr2Impl = avx512ArchMemchr2
	memchr3Impl = avx512ArchMemchr3
	memrchrImpl = avx512Memrchr
	memrchr2Impl = avx512ArchMemrchr2
	memrchr3Impl = avx512ArchMemrchr3
	setBackendMarkers("avx512", "avx512-v4", "avx512-v4", "avx512-v4", "avx512-v4", "avx512-v4", "avx512-v4")
}
