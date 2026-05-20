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

//go:build amd64 && amd64.v3 && !amd64.v4 && goexperiment.simd && !force_swar

package memchr

func memchr(needle byte, haystack []byte) int {
	return avx2Memchr(needle, haystack)
}

func memchr2(n1, n2 byte, haystack []byte) int {
	return avx2Memchr2(n1, n2, haystack)
}

func memchr3(n1, n2, n3 byte, haystack []byte) int {
	return avx2Memchr3(n1, n2, n3, haystack)
}

func memrchr(needle byte, haystack []byte) int {
	return avx2Memrchr(needle, haystack)
}

func memrchr2(n1, n2 byte, haystack []byte) int {
	return avx2Memrchr2(n1, n2, haystack)
}

func memrchr3(n1, n2, n3 byte, haystack []byte) int {
	return avx2Memrchr3(n1, n2, n3, haystack)
}

// init binds the separate GOAMD64=v3 fallback artifact. GOAMD64=v3 makes AVX2
// part of the program startup contract, so all public shims can bind directly
// to the AVX2 routines.
func init() {
	memchrImpl = avx2Memchr
	memchr2Impl = avx2Memchr2
	memchr3Impl = avx2Memchr3
	memrchrImpl = avx2Memrchr
	memrchr2Impl = avx2Memrchr2
	memrchr3Impl = avx2Memrchr3
	setBackendMarkers("avx2", "avx2-v3", "avx2-v3", "avx2-v3", "avx2-v3", "avx2-v3", "avx2-v3")
}
