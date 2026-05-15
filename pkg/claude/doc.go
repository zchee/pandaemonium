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

// Package claude provides a Go port of the anthropics/claude-agent-sdk-python.
//
// All exported types and function signatures are frozen (Phase 0). Method
// bodies currently return errors.ErrUnsupported; subsequent implementation
// phases (A–I) fill the stubs without reshaping the public surface.
//
// # Streaming surface
//
// Query returns an iter.Seq2[Message, error] that streams [AssistantMessage],
// [UserMessage], [SystemMessage], and [ResultMessage] values from the claude
// CLI subprocess over stdio. Early break of the range loop releases the
// underlying client and closes the subprocess:
//
//	for msg, err := range claude.Query(ctx, "hello", nil) {
//	    if err != nil { log.Fatal(err) }
//	    fmt.Println(msg)
//	}
//
// # Interactive surface
//
// [NewClient] returns a [*ClaudeSDKClient] for bidirectional interactive use.
// The client owns a subprocess transport following the same snapshot-as-arg +
// writeMu-symmetry race pattern as pkg/codex (commit 8c16376): the transport
// field is a plain field (not atomic.Pointer); readLoop receives a snapshot
// captured under closeMu at Start; Close clears c.transport inside the writeMu
// critical section.
//
// # In-process MCP servers
//
// [Options].MCPServers accepts [MCPServer] values created by [NewSDKMCPServer].
// Each in-process server runs as a goroutine bridged to the CLI subprocess
// over two independent io.Pipe pairs (one per direction). Closing the owning
// [ClaudeSDKClient] deterministically closes every registered MCP server.
//
// # Session store
//
// [SessionStore] is a pluggable persistence interface. The package ships an
// in-memory implementation ([NewInMemorySessionStore]) and a conformance
// harness at pkg/claude/testing/sessionstoreconformance.
//
// # Fixture refresh
//
// Golden stream-JSON fixtures in testdata/stream/*.jsonl are captured by the
// fixture-capture tool at internal/cmd/capture-fakecli-fixtures (build tag
// "capture", requires RUN_REAL_CLAUDE_TESTS=1):
//
//	go run -tags capture ./pkg/claude/internal/cmd/capture-fakecli-fixtures
//
// # iter.Seq2 early-break idiom
//
// The package returns [iter.Seq2][Message, error] from [Query] and
// [ClaudeSDKClient.ReceiveResponse]. Callers may break out of the range loop
// at any time without draining the channel; the iterator's cleanup path
// releases the underlying subprocess and closes any MCP servers:
//
//	for msg, err := range claude.Query(ctx, "hello", nil) {
//	    if err != nil { log.Fatal(err) }
//	    if _, ok := msg.(claude.ResultMessage); ok {
//	        break // clean exit — subprocess is reaped
//	    }
//	    fmt.Println(msg)
//	}
//
// The iter.Seq2 return type was chosen in Phase 0 over a channel-based
// approach because it integrates natively with range-over-func (Go 1.23+),
// avoids goroutine leaks on early break, and keeps the zero-allocation
// hot-path open for future work.
//
// # Real-CLI integration tests
//
// Set RUN_REAL_CLAUDE_TESTS=1 to opt in to the integration test lane that
// exercises the real claude binary on PATH, mirroring pkg/codex's
// RUN_REAL_CODEX_TESTS=1 lane.
package claude
