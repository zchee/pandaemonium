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

//go:build !amd64 || !goexperiment.simd || force_swar

package memchr

func memchr(needle byte, haystack []byte) int {
	return memchrImpl(needle, haystack)
}

func memchr2(n1, n2 byte, haystack []byte) int {
	return memchr2Impl(n1, n2, haystack)
}

func memchr3(n1, n2, n3 byte, haystack []byte) int {
	return memchr3Impl(n1, n2, n3, haystack)
}

func memrchr(needle byte, haystack []byte) int {
	return memrchrImpl(needle, haystack)
}

func memrchr2(n1, n2 byte, haystack []byte) int {
	return memrchr2Impl(n1, n2, haystack)
}

func memrchr3(n1, n2, n3 byte, haystack []byte) int {
	return memrchr3Impl(n1, n2, n3, haystack)
}
