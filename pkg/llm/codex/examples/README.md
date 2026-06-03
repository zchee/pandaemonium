# Codex Go SDK Examples

These examples port the upstream Python SDK examples from
`openai/codex` tag `rust-v0.137.0-alpha.5`, path `sdk/python/examples`, to the
Go SDK package in this module.

Each numbered directory is a standalone `go run` command that uses public SDK
exports from `github.com/zchee/pandaemonium/pkg/llm/codex`. Shared example-only
helpers live under `pkg/codex/examples/internal/exampleutil`.

## Prerequisites

- Go matching this module's `go.mod` toolchain.
- A compatible `codex` app-server binary on `PATH`, or a `Config` customized in
  the example before running.
- Codex authentication/configuration already set up in the current environment.

The examples compile during `go test ./...`, but they are not executed by unit
tests because running them starts a real Codex app-server process.

Plain strings are accepted anywhere an example starts or steers a turn; they
are shorthand for `codex.TextInput{Text: ...}`. Use typed inputs such as
`codex.ImageInput`, `codex.LocalImageInput`, `codex.SkillInput`, and
`codex.MentionInput` when the turn needs non-text or mixed payloads.

## Run examples

From the repository root:

```bash
go run ./pkg/codex/examples/01_quickstart_constructor
go run ./pkg/codex/examples/02_turn_run
go run ./pkg/codex/examples/03_turn_stream_events
go run ./pkg/codex/examples/04_models_and_metadata
go run ./pkg/codex/examples/05_existing_thread
go run ./pkg/codex/examples/06_thread_lifecycle_and_controls
go run ./pkg/codex/examples/07_image_and_text
go run ./pkg/codex/examples/08_local_image_and_text
go run ./pkg/codex/examples/09_stream_parity
go run ./pkg/codex/examples/10_error_handling_and_retry
go run ./pkg/codex/examples/11_cli_mini_app
go run ./pkg/codex/examples/12_turn_params_kitchen_sink
go run ./pkg/codex/examples/13_model_select_and_turn_params
go run ./pkg/codex/examples/14_turn_controls
go run ./pkg/codex/examples/15_remote_client_connect
go run ./pkg/codex/examples/16_remote_control_status_and_pairing
go run ./pkg/codex/examples/17_remote_process_spawn
```

## Remote app-server examples

The remote-control examples use an already-running app-server websocket when
`CODEX_REMOTE_APP_SERVER_URL` is set. Supported URL forms are `ws://`, `wss://`,
and `unix://`. Set `CODEX_REMOTE_APP_SERVER_BEARER_TOKEN` or
`CODEX_REMOTE_APP_SERVER_BEARER_TOKEN_FILE` when the endpoint requires bearer
auth. Plain `ws://` bearer auth is accepted only for loopback hosts unless
`CODEX_REMOTE_APP_SERVER_ALLOW_INSECURE_WS=1` is set.

`17_remote_process_spawn` runs commands on the machine that hosts the
app-server. Pass argv after an optional `--`; when omitted it runs a small `sh -c` hello
command. Set `CODEX_REMOTE_PROCESS_CWD` when the remote host should use a
specific working directory.

`16_remote_control_status_and_pairing` avoids mutating remote-control state by
default. Set `CODEX_EXAMPLE_ENABLE_REMOTE_CONTROL=1` to call
`remoteControl/enable`; set `CODEX_EXAMPLE_START_PAIRING=1` to start pairing
and print the short-lived pairing code.

## Index

- `01_quickstart_constructor/` - first run / sanity check.
- `02_turn_run/` - inspect full turn output fields.
- `03_turn_stream_events/` - stream a turn with a curated event view.
- `04_models_and_metadata/` - discover visible models for the runtime.
- `05_existing_thread/` - resume an existing thread created in-script.
- `06_thread_lifecycle_and_controls/` - lifecycle plus archive/fork/compact controls.
- `07_image_and_text/` - remote image URL and text multimodal turn.
- `08_local_image_and_text/` - generated local image and text multimodal turn.
- `09_stream_parity/` - parity-style streaming flow for Go iterators.
- `10_error_handling_and_retry/` - overload retry and typed error handling.
- `11_cli_mini_app/` - interactive chat loop.
- `12_turn_params_kitchen_sink/` - structured output and advanced turn params.
- `13_model_select_and_turn_params/` - model selection plus per-turn params.
- `14_turn_controls/` - best-effort `Steer` and `Interrupt` demos.
- `15_remote_client_connect/` - connect to an existing app-server websocket.
- `16_remote_control_status_and_pairing/` - inspect remote-control status and guarded pairing flows.
- `17_remote_process_spawn/` - stream `process/spawn` output from an existing app-server host.
