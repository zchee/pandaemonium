# pkg/codex Rust release update checklist

## 1. Establish live truth

Prefer live filesystem/tool evidence over memory, screenshots, or OCR.

```sh
pwd
git status --short --branch --untracked-files=all
git remote -v
git log -1 --oneline --decorate
go list -m
go list ./pkg/codex ./pkg/codex/tests ./pkg/codex/examples
sed -n '1,120p' pkg/codex/generate.go
```

If any command shows the wrong repo/module/package, stop and resolve that before
editing.

## 2. Determine upstream provenance

Current schema pin lives in `pkg/codex/generate.go` as a `//go:generate` URL:

```text
https://raw.githubusercontent.com/openai/codex/refs/tags/<rust-tag>/codex-rs/app-server-protocol/schema/json/codex_app_server_protocol.v2.schemas.json
```

When the target tag is not supplied, discover it from official upstream sources
or the local upstream checkout; do not guess from memory. Record both previous
and target tags in the report.

Useful local comparison shape when an upstream checkout exists:

```sh
git -C /path/to/openai/codex diff <old-tag>..<new-tag> -- codex-rs/app-server-protocol codex-rs/app-server-daemon sdk/python
```

## 3. Schema-only bump path

1. Edit only the tag in `pkg/codex/generate.go`.
2. Run `go generate ./pkg/codex`.
3. Inspect generated type changes:
   ```sh
   git diff -- pkg/codex/generate.go pkg/codex/protocol_gen.go
   go test -count=1 ./pkg/codex/...
   ```
4. If tests fail because generated output no longer satisfies rename, optional
   reference, or identity contracts, fix generator code/tests before accepting
   the generated diff.

## 4. Generator behavior path

Relevant files are usually under:

- `pkg/codex/internal/cmd/generate-protocol-types/main.go`
- `pkg/codex/internal/cmd/generate-protocol-types/*_test.go`
- `pkg/codex/protocol_gen_test.go`
- `pkg/codex/public_types_test.go`
- `pkg/codex/tests/app_server_contract_generation_test.go`
- `pkg/codex/tests/app_server_public_api_signatures_test.go`

Workflow:

1. Add or update generator/contract tests describing the intended output.
2. Change the generator.
3. Run the focused generator tests.
4. Run `go generate ./pkg/codex`.
5. Run package tests and inspect `protocol_gen.go` for unexpected public names.

## 5. Public API and examples path

If upstream adds/changes input, run, stream, login, approval, or transport
behavior, inspect local hand-written surfaces before editing:

- `pkg/codex/api.go`
- `pkg/codex/input.go`
- `pkg/codex/method.go`
- `pkg/codex/run.go`
- `pkg/codex/stream_api.go`
- `pkg/codex/router.go`
- `pkg/codex/notification*.go`
- `pkg/codex/login.go`
- `pkg/codex/examples/`
- `pkg/codex/tests/public_api_port_test.go`
- `pkg/codex/tests/app_server_public_api_signatures_test.go`

Preserve these contracts unless the task explicitly changes them:

- `Client.NextNotification` is raw passthrough.
- `TurnHandle.Stream` and `Run` own the per-turn consumer.
- Only one active consumer per `Client` is allowed.
- `RunInput`/input normalization accepts all documented typed input shapes.

## 6. Formatting and static checks

Typical formatting/check sequence:

```sh
gofmt -w <changed-go-files>
gofumpt -w -extra <changed-go-files-or-dirs>
go test -count=1 ./pkg/codex/...
go test -race -count=1 -shuffle=on ./pkg/codex/...
go build ./...
git diff --check
```

If repository hooks run only staged files and fail from missing package context,
run the equivalent package-level command manually and report both the hook
limitation and substitute validation.

## 7. Commit/report hygiene

Keep one logical change per commit/report. A schema bump may include:

- `pkg/codex/generate.go`
- `pkg/codex/protocol_gen.go`
- generator tests, contract tests, or examples required by the new schema
- testdata/golden files required by public-type parity

Final evidence should mention exact commands, exact failures if any, skipped
optional real-server checks, and current branch/status when relevant.
