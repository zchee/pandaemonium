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

// avx512Memchr returns the offset of the first byte equal to needle in
// haystack, or -1 if no byte matches. GOAMD64=v4 makes AVX-512F/BW/VL part of
// the executable startup contract; host selection is still preflighted with
// simd/archsimd before running or recommending this artifact.
//
//go:noescape
func avx512Memchr(needle byte, haystack []byte) int

func memchr(needle byte, haystack []byte) int {
	return avx512Memchr(needle, haystack)
}

// init binds the GOAMD64=v4 primary artifact. Only Memchr is AVX-512-backed in
// this first stage; the other routines keep the existing AVX2 implementations,
// so aggregate boundImpl intentionally reports a mixed backend.
func init() {
	memchrImpl = avx512Memchr
	memchr2Impl = avx2Memchr2
	memchr3Impl = avx2Memchr3
	memrchrImpl = avx2Memrchr
	memrchr2Impl = avx2Memrchr2
	memrchr3Impl = avx2Memrchr3
	setBackendMarkers("mixed-v4", "avx512-v4", "avx2-v4", "avx2-v4", "avx2-v4", "avx2-v4", "avx2-v4")
}
