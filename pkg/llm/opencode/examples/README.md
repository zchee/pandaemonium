# OpenCode wrapper examples

Each numbered directory is a standalone `go run` command using public exports
from `github.com/zchee/pandaemonium/pkg/llm/opencode`.

## Prerequisites

- Go matching this module's `go.mod` toolchain.
- An `opencode` binary on `PATH` (opencode.ai), or a `Config.OpencodeBin`
  override edited into the example.
- Provider credentials already configured (`opencode auth login`); prompts use
  your configured default model unless the example says otherwise.

The examples compile during `go build ./...` but are not executed by unit
tests, because running them starts a real `opencode serve` process and spends
provider tokens.

## Index

| Example | What it shows |
| --- | --- |
| `01_quickstart` | Spawn `opencode serve`, health check, one blocking `Session.Run`. `OPENCODE_MODEL=provider/model` overrides the server default. |
| `02_run` | Provider discovery, explicit-model sync run, `SetTitle`, `Session.Read` history. |
| `03_stream_events` | Async `Session.Turn` + `TurnHandle.Stream` over the shared SSE bus; prints deltas live and the wrapper counters after the turn. |
| `04_remote_attach` | `NewRemoteOpencode` against an already-running server (`OPENCODE_BASE_URL`, `OPENCODE_SERVER_PASSWORD`). |
| `05_shell` | `Session.Shell`: run a shell command in the session context and print its parts. |

Run one with, e.g.:

```sh
go run ./pkg/llm/opencode/examples/01_quickstart
```
