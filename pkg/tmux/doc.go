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

// Package tmux provides a Go client for the tmux control-mode protocol.
//
// A [Client] owns one persistent `tmux -C` subprocess. Callers send normal
// tmux commands with [Client.Exec], [Client.ExecLine], or [Client.ExecRaw]; the
// client parses the guarded `%begin`/`%end`/`%error` response blocks and exposes
// asynchronous `%` notifications through [Client.Events].
//
// Client command execution is deliberately serialized: only one command is
// pending at a time and there is no pipelining. If a pending command's context
// is canceled before tmux replies, the client is aborted because the late
// response can no longer be safely associated with a future command.
//
// Notification delivery favors keeping the stdout reader live. [Client.Events]
// is bounded; when the buffer is full, the client drops the oldest buffered
// notification it can observe and increments [Client.DroppedNotifications].
// Treat the counter as a backpressure signal rather than an exact audit log
// when consumers are receiving concurrently.
//
// Client uses the single `-C` form because it communicates with tmux over
// standard-library pipes. The double `-CC` form asks tmux to change terminal
// attributes and requires a controlling terminal on current tmux releases; use
// [Parser] directly if an external PTY-backed transport needs to consume the
// extra `\x1bP1000p`/`\x1b\` enter/exit framing emitted by `-CC`.
//
// The package defaults are intentionally conservative. A client must be given
// an explicit initial command or an explicit session target, and real-tmux tests
// use isolated sockets/configuration behind RUN_REAL_TMUX_TESTS=1 so they do
// not attach to or kill a user's normal tmux server.
//
// Pane output is byte-oriented. tmux escapes control bytes and backslash in
// `%output` and `%extended-output` frames as octal sequences; use
// [DecodeOutputValue] or the typed notification helpers to recover the bytes.
package tmux
