# Codex Go SDK Examples

These examples port the upstream Python SDK examples from
`openai/codex` tag `rust-v0.131.0-alpha.9`, path `sdk/python/examples`, to the
Go SDK package in this module.

Each numbered directory is a standalone `go run` command that uses public SDK
exports from `github.com/zchee/pandaemonium/pkg/codex`. Shared example-only
helpers live under `pkg/codex/examples/internal/exampleutil`.

## Prerequisites

- Go matching this module's `go.mod` toolchain.
- A compatible `codex` app-server binary on `PATH`, or a `Config` customized in
  the example before running.
- Codex authentication/configuration already set up in the current environment.

The examples compile during `go test ./...`, but they are not executed by unit
tests because running them starts a real Codex app-server process.

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
```

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
