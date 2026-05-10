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

// Package codexappserver provides a Go SDK for the Codex app-server JSON-RPC v2 protocol.
//
// The package mirrors the practical surface of the upstream Python SDK: it can
// launch `codex app-server --listen stdio://`, initialize the protocol, manage
// threads and turns, stream notifications, aggregate a turn run, map JSON-RPC
// errors, and retry transient overload failures. Stable high-value protocol
// fields are typed while raw JSON extension fields preserve compatibility with
// newer app-server schema members.
//
// Generated protocol bindings live in this package (for example,
// `ThreadStartParams`, `TurnStartParams`, `ThreadItem`, `ReasoningEffort`,
// `SandboxPolicy`) so callers can work with schema-backed models directly from
// the package root.
//
// Compile-time alias compatibility checks live in public_types_test.go, and
// compatibility-sensitive aliases should be preserved unless the upstream schema
// contract explicitly changes.
package codexappserver
