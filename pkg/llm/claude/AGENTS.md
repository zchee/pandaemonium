# pkg/llm/claude KNOWLEDGE BASE

## OVERVIEW

Go port of `anthropics/claude-agent-sdk-python`, backed by the `claude` CLI
over stdio with typed options, streaming messages, MCP, hooks, plugins,
settings, sandboxing, and Go-side session forking.

## WHERE TO LOOK

| Task | Location | Notes |
| --- | --- | --- |
| Public API and options | `api.go`, `options.go`, `types.go`, `README.md` | Query/client/options parity surface. |
| CLI process transport | `client.go`, `transport.go`, `control_protocol.go` | Lazy subprocess start and control requests. |
| Session forking | `session_store.go`, `client_fork.go` | Go-side transcript store only. |
| Session-store contract | `testing/sessionstoreconformance/` | Reusable conformance harness. |
| MCP/tools/hooks | `mcp.go`, `tools.go`, `hooks.go`, `permission*.go` | In-process and external MCP behavior. |
| Hermetic CLI tests | `internal/fakecli/`, `testdata/stream/` | Fake CLI and captured JSONL fixtures. |
| Examples | `examples/` | Parity-tested; sessionstore examples isolate deps. |

## CONVENTIONS

- The zero-value `Options` is valid.
- `Query` and `ReceiveResponse` return `iter.Seq2[Message, error]`; early
  `break` must cleanly release subprocess resources.
- `SessionStore` is Go-native only. It is not sent to the CLI as a flag, init
  field, or control-protocol payload.
- CLI-side session lifetime is `SessionID`, `Resume`, and
  `ContinueConversation`.
- Subagents are sent in streaming initialize requests, not CLI flags.
- Agents are never passed through struct `json.Marshal` in production; preserve
  explicit wire conversion.
- Real CLI integration stays behind `RUN_REAL_CLAUDE_TESTS=1`.

## EXAMPLES

- `examples/` are runnable package demos and parity-tested with `FakeCLI`.
- `examples/sessionstores/{postgres,redis,s3}` are separate example modules;
  keep third-party storage driver dependencies out of the main module graph.
- Example code should demonstrate public API usage, not reach into internals.

## ANTI-PATTERNS

- Do not wire `SessionStore` to the CLI.
- Do not leak goroutines when a caller breaks an iterator early.
- Do not start the real `claude` binary in ordinary unit tests.
- Do not broaden examples with dependencies that should live only in
  sessionstore example modules.

## COMMANDS

```bash
go test -count=1 ./pkg/llm/claude/...
go test -race -count=1 -shuffle=on ./pkg/llm/claude/...
RUN_REAL_CLAUDE_TESTS=1 go test -v -race -count=1 -shuffle=on ./pkg/llm/claude/...
go run -tags capture ./pkg/llm/claude/internal/cmd/capture-fakecli-fixtures
```
