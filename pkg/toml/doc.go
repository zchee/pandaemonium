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

// Package toml provides a flat TOML API backed by a shared streaming parser.
//
// The low-level API starts with Decoder and Token for callers that need to
// inspect TOML input without building Go values. The high-level facade exposes
// Marshal and Unmarshal for struct, map, scalar, array, and datetime values.
// ParseDocument returns a format-preserving Document for edit-in-place
// workflows where comments, whitespace, and untouched source spans must remain
// byte-identical.
//
// Benchmark-only comparisons against external TOML libraries live behind the
// bench build tag in tests and perf-gate tooling. Production builds of this
// package must not import those competitors; verify with:
//
//	go list -deps ./pkg/toml
//
// Use the bench test graph when checking that the benchmark comparators are
// wired:
//
//	go list -deps -test -tags=bench ./pkg/toml
//
// The force_swar build tag selects the pure-Go scan backend for verification
// of the internal scanner fallback path.
package toml
