# pkg/llm/codex KNOWLEDGE BASE

## OVERVIEW

Codex app-server JSON-RPC v2 SDK: public facade, low-level client, transports,
generated protocol bindings, examples, and real app-server tests.

## WHERE TO LOOK

| Task | Location | Notes |
| --- | --- | --- |
| Public facade | `api.go`, `stream_api.go`, `run.go` | `Codex`, `Thread`, `TurnHandle`, run/stream helpers. |
| Low-level client/transport | `client.go`, `remote_client.go`, `process.go`, `execserver_process.go` | stdio, websocket, unix websocket, process lifecycle. |
| Notification routing | `router.go`, `notification.go`, `notification_scan.go` | Raw passthrough vs turn/login/process consumers. |
| Input and request helpers | `input.go`, `methods.go`, `types.go`, `public_types.go` | Public API shape and compatibility aliases. |
| Generated protocol | `protocol_gen.go`, `generate.go` | Checked-in output and `go:generate` entrypoint. |
| Generator | `internal/cmd/generate-protocol-types/` | Change codegen here, then regenerate. |
| Public contract tests | `tests/`, `*_test.go` | Signature, behavior, real-server, route invariants. |
| Runnable examples | `examples/` | Standalone `go run` programs; may need Codex auth/binary. |

## CONVENTIONS

- Package name is `codex`; directory is `pkg/llm/codex`.
- `Client.NextNotification` is raw passthrough for unowned notifications only.
- `TurnHandle.Stream`, `TurnHandle.Run`, `StreamThread.RunStream`, login
  handles, and process waiters own their matching consumer registrations.
- Only one active stream consumer per turn/login/process key.
- Unix endpoints are websocket-over-Unix-stream, not raw JSON over a socket.
- Websocket bearer tokens must never appear in argv, logs, or error strings.
- `unix://` default control socket cleanup is special; do not remove the shared
  default socket.
- Real app-server tests stay behind `RUN_REAL_CODEX_TESTS=1`.

## GENERATED CODE

- Do not hand-edit `protocol_gen.go`.
- Normal schema input comes from:

```bash
codex app-server generate-json-schema --experimental --out <tmpdir>
```

- Regenerate through:

```bash
go generate ./pkg/llm/codex
```

- The `Source binary` header must match the `codex --version` used by tests and
  `.github/workflows/test.yaml`.
- Preserve generator rename policy: no root generated `Config` or `Thread`;
  use compatibility names such as `ConfigPayload` and `ThreadPayload`.

## ANTI-PATTERNS

- Do not route unix websocket support through a separate notification path.
- Do not relax remote-host guards for TCP websocket endpoints.
- Do not leak auth tokens through errors, logs, argv, or stderr tails.
- Do not use hidden flag help output as the only proof of `--remote-control`;
  probe behavior when required.
- Do not update generated code without matching generator/schema/test evidence.

## COMMANDS

```bash
go test -count=1 ./pkg/llm/codex/...
go test -race -count=1 -shuffle=on ./pkg/llm/codex/...
go test -count=1 ./pkg/llm/codex ./pkg/llm/codex/tests ./pkg/llm/codex/examples
go test -count=1 ./pkg/llm/codex/examples/...
RUN_REAL_CODEX_TESTS=1 go test -v -race -count=1 -shuffle=on ./pkg/llm/codex/...
```
