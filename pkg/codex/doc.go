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

// Package codex provides a Go SDK for the Codex app-server JSON-RPC v2 protocol.
//
// The package mirrors the practical surface of the upstream Python SDK: it can
// launch `codex app-server --listen stdio://` by default or connect to
// authenticated `ws://...` launch modes, initialize the protocol, manage
// threads and turns, stream notifications, aggregate a turn run, map JSON-RPC
// errors, and retry transient overload failures. Stable high-value protocol
// fields are typed while raw JSON extension fields preserve compatibility with
// newer app-server schema members.
//
// Notification routing is turn-aware: notifications that belong to an active
// turn are consumed by that turn's stream, while low-level consumers still see
// unscoped events through `NextNotification` when no registered turn owns them.
// This makes per-turn streaming explicit and prevents unrelated turns from
// silently stealing each other's notifications.
//
// Generated protocol bindings intentionally preserve the upstream public type
// surface where names do not collide, and they use package-root payload types
// such as `ConfigPayload` and `ThreadPayload` when the upstream schema would
// otherwise shadow existing Go SDK concepts like `Config` and `Thread`. The
// generated inventory is documented in tests so API drift fails fast instead of
// being rediscovered through stale fixtures or wrapper mismatches.
//
// Notification routing is intentionally split between decoded, typed methods
// and raw passthrough:
//   - Client.NextNotification returns the next notification exactly as read from
//     the transport, preserving unknown methods and payloads for callers that
//     need to log or forward them.
//   - DecodeNotification and the generated helpers decode only the registry
//     entries in notification.go; upstream additions that are not yet registered
//     remain available through the raw notification value instead of being
//     silently rewritten.
//   - TurnHandle.Stream and TurnHandle.Run consume the shared notification
//     stream until their matching turn completes, so only one active consumer may
//     read a Client at a time.
//
// Generated protocol bindings live in this package (for example,
// `ThreadStartParams`, `TurnStartParams`, `ThreadItem`, `ReasoningEffort`,
// `SandboxPolicy`) so callers can work with schema-backed models directly from
// the package root. When the schema would otherwise collide with public SDK
// names, the generator emits payload compatibility names such as
// `ConfigPayload` and `ThreadPayload` to keep the root package API stable.
//
// Compile-time alias compatibility checks live in public_types_test.go, and
// compatibility-sensitive aliases should be preserved unless the upstream schema
// contract explicitly changes.
//
// Concurrency contract:
//
//   - [Client], [Codex], and [Thread] are safe for concurrent use by multiple
//     goroutines.
//   - Each [TurnHandle] may have at most one active stream consumer at a time.
//     Calling [TurnHandle.Stream] or [TurnHandle.Run] while another goroutine
//     is already streaming the same turn returns an error immediately.
package codex
