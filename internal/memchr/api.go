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

package memchr

// Each wrapper is intentionally a one-line forward to the dispatched impl so
// the Go inliner promotes the wrapper into its caller (AC-API-5). The
// indirect call through the *Impl funcptr is one indirect call per Memchr
// invocation (R-INDIRECT-COST) and cannot itself be inlined because the
// target is set at init() time, not at compile time.

// Memchr returns the offset of the first byte in haystack equal to needle,
// or -1 if no byte matches.
func Memchr(needle byte, haystack []byte) int {
	return memchrImpl(needle, haystack)
}

// Memchr2 returns the offset of the first byte in haystack equal to n1 or
// n2, or -1 if no byte matches either needle.
func Memchr2(n1, n2 byte, haystack []byte) int {
	return memchr2Impl(n1, n2, haystack)
}

// Memchr3 returns the offset of the first byte in haystack equal to any of
// n1, n2, or n3, or -1 if no byte matches any of them.
func Memchr3(n1, n2, n3 byte, haystack []byte) int {
	return memchr3Impl(n1, n2, n3, haystack)
}

// Memrchr returns the offset of the LAST byte in haystack equal to needle,
// or -1 if no byte matches.
func Memrchr(needle byte, haystack []byte) int {
	return memrchrImpl(needle, haystack)
}

// Memrchr2 returns the offset of the LAST byte in haystack equal to n1 or
// n2, or -1 if no byte matches either needle.
func Memrchr2(n1, n2 byte, haystack []byte) int {
	return memrchr2Impl(n1, n2, haystack)
}

// Memrchr3 returns the offset of the LAST byte in haystack equal to any of
// n1, n2, or n3, or -1 if no byte matches any of them.
func Memrchr3(n1, n2, n3 byte, haystack []byte) int {
	return memrchr3Impl(n1, n2, n3, haystack)
}

// IndexByte returns the offset of the first byte in haystack equal to
// needle, or -1 if no byte matches. The argument order mirrors
// bytes.IndexByte for drop-in idiom compatibility.
func IndexByte(haystack []byte, needle byte) int {
	return memchrImpl(needle, haystack)
}

// IndexByte2 returns the offset of the first byte in haystack equal to n1 or
// n2, or -1 if no byte matches either needle.
func IndexByte2(haystack []byte, n1, n2 byte) int {
	return memchr2Impl(n1, n2, haystack)
}

// IndexByte3 returns the offset of the first byte in haystack equal to any
// of n1, n2, or n3, or -1 if no byte matches any of them.
func IndexByte3(haystack []byte, n1, n2, n3 byte) int {
	return memchr3Impl(n1, n2, n3, haystack)
}

// LastIndexByte returns the offset of the LAST byte in haystack equal to
// needle, or -1 if no byte matches. The argument order mirrors
// bytes.LastIndexByte for drop-in idiom compatibility.
func LastIndexByte(haystack []byte, needle byte) int {
	return memrchrImpl(needle, haystack)
}

// LastIndexByte2 returns the offset of the LAST byte in haystack equal to
// n1 or n2, or -1 if no byte matches either needle.
func LastIndexByte2(haystack []byte, n1, n2 byte) int {
	return memrchr2Impl(n1, n2, haystack)
}

// LastIndexByte3 returns the offset of the LAST byte in haystack equal to
// any of n1, n2, or n3, or -1 if no byte matches any of them.
func LastIndexByte3(haystack []byte, n1, n2, n3 byte) int {
	return memrchr3Impl(n1, n2, n3, haystack)
}
