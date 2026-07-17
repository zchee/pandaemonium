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

// Package opencode controls an OpenCode (sst/opencode, opencode.ai) server
// from Go, presenting the same API shape and ergonomics as pkg/llm/codex so
// callers can drive either agent through a near-identical surface.
//
// The package is fully self-contained: a hand-written typed HTTP client for
// the wrapped REST subset, a hand-built SSE client for GET /event, a server
// process launcher ([NewOpencode] spawns `opencode serve`), and a remote
// attach path ([NewRemoteOpencode]). It does not depend on the upstream
// generated Go SDK; shape ground truth is the committed OpenAPI snapshot
// (testdata/openapi.json) plus real-server probes, and decoding is tolerant:
// unknown JSON fields are ignored and unknown event types pass through raw.
//
// Turn input accepts the [RunInput] contract used by [Session.Run] and
// [Session.Turn]. A plain string is shorthand for one text input part, while
// typed values such as [TextInput], [FileInput], [AgentInput], [InputItem]
// slices, and raw [PartInput] values remain available.
//
// # Divergences from pkg/llm/codex
//
// The codex package speaks JSON-RPC to a process it owns; OpenCode exposes
// HTTP REST plus an SSE event stream. The differences below are semantic and
// deliberate — this package names methods for what OpenCode actually does
// instead of faking codex semantics:
//
//  1. Transport: codex is process JSON-RPC over stdio/ws/unix; opencode is
//     HTTP REST + SSE. Notification routing becomes SSE-event routing keyed
//     by sessionID (narrowed by assistant messageID once known).
//  2. No pairing or remote control: remote attachment is a base URL plus
//     HTTP basic auth ([RemoteConfig]); there is no pairing protocol.
//  3. Approvals: codex's ApprovalHandler callback has no analog. OpenCode
//     permissions are event-driven: the client-lifetime permission consumer
//     always replies to permission.asked ("once" when Config.PermissionAuto,
//     "reject" otherwise), so a permission-gated tool call can never stall a
//     turn. An interactive permission handler is a deferred follow-up.
//  4. Archive/Unarchive → [Session.Share] / [Session.Unshare]. OpenCode's
//     closest lifecycle operation is sharing; the methods are named for what
//     they do.
//  5. Codex's stream/non-stream thread split collapses into the single
//     [Session] type; [Session.Turn] returns the streaming handle.
//  6. Async turns are client-synthesized: the server exposes only a blocking
//     prompt (POST /session/{id}/message), so [TurnHandle] pairs a prompt
//     goroutine (bound to the client lifetime) with a session-scoped event
//     subscription registered before the prompt is sent. There is no
//     server-side turn resource.
//  7. No mid-turn steering: a prompt sent to a session with an active turn
//     does not steer it (probed: a noReply injection only becomes visible to
//     the next turn), so TurnHandle has no Steer.
//  8. Sync-run cancellation: canceling [Session.Run]'s ctx aborts only the
//     HTTP request — the server keeps generating. Run installs a best-effort
//     POST /session/{id}/abort on cancellation, bounded by the client
//     lifetime; callers needing guaranteed server-side stop should call
//     [TurnHandle.Interrupt] or [Client.Abort] explicitly.
//
// # Event topology
//
// There is exactly one client-lifetime SSE connection to GET /event, owned
// by [Client] and fanned out to consumers. The facade constructors
// ([NewOpencode], [NewRemoteOpencode]) dial the bus eagerly and fail fast if
// the dial or the first server.connected handshake does not complete within
// Config.DialTimeout; the low-level [Client] dials lazily on first use.
// Per-turn subscriptions are router registrations, never new connections.
// The bus owns bounded auto-reconnect (Config.Retry); because /event has no
// resume cursor, every registered consumer receives a gap event
// ([EventTypeGap]) after a reconnect, and reconnect exhaustion surfaces
// [TransportClosedError].
//
// The terminal signal of a turn is dual-source: the prompt goroutine's
// return is necessary and authoritative, while session-scoped session.idle /
// session.error events enrich it inside a bounded drain window
// (Config.DrainWindow). A missing terminal event ends the stream cleanly and
// increments a counter instead of hanging; an explicit session.error for the
// turn's session outweighs HTTP success.
//
// # Concurrency contract
//
//   - [Client], [Opencode], and [Session] are safe for concurrent use by
//     multiple goroutines.
//   - Each [Session] supports at most one active turn at a time; starting a
//     second returns an error immediately.
//   - Each [TurnHandle] may have at most one active stream consumer at a
//     time. Calling [TurnHandle.Stream] or [TurnHandle.Run] while another
//     consumer is active returns an error immediately, as does consuming a
//     turn that already completed.
//   - Turn subscriptions are registered on the live bus before the prompt is
//     issued, so a stream observes every event of its own turn.
//
// # Secrets
//
// Config.Password and RemoteConfig.Password never appear in argv, URLs,
// error strings, or logs. The spawned server receives the password via the
// OPENCODE_SERVER_PASSWORD environment variable; requests carry it only in
// the Authorization header (basic auth, username "opencode").
package opencode
