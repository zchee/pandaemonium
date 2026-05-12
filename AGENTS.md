# Repository Guidelines

`pandaemonium` is a Go module (`github.com/zchee/pandaemonium`) that hosts SDKs and tooling around the OpenAI Codex toolchain. The first package, `pkg/codex`, is a typed Go SDK for the Codex app-server JSON-RPC v2 protocol.

## Project Structure & Module Organization

- `pkg/codex/` — Go SDK package (`codexappserver`).
  - `api.go`, `client.go`, `methods.go`, `router.go`, `run.go`, `stream_api.go` — public client + routing surface.
  - `notification.go`, `errors.go`, `retry.go`, `input.go`, `types.go`, `public_types.go` — protocol primitives and decoders.
  - `protocol_gen.go` — **generated**; do not edit by hand (see Generated Code below).
  - `*_test.go` — unit/SDK tests; `integration_test.go` is opt-in.
  - `internal/cmd/generate-protocol-types/` — `go run` target that produces `protocol_gen.go` from the upstream JSON schema.
  - `testdata/` — golden fixtures (e.g. `python_public_types_v0.131.0-alpha.5.txt`).
  - `doc.go`, `review_notes.md` — package overview and rename/routing contracts.
- `hack/boilerplate/boilerplate.go.txt` — Apache-2.0 header prepended to every `.go` file.
- `vendor/` — committed dependencies; module uses vendoring.
- `.github/PULL_REQUEST_TEMPLATE.md` — PR template (`## Why`).
- `.envrc` — exports `GOTOOLCHAIN=auto` and the project's `GOEXPERIMENT` set.

## Build, Test, and Development Commands

Run from the repository root.

- `go build ./...` — compile every package; fails fast on type or vet-adjacent errors.
- `go test ./...` — run all unit and SDK tests (excludes the opt-in real-server lane).
- `RUN_REAL_CODEX_TESTS=1 go test ./pkg/codex/...` — run integration tests against a real `codex` binary on `PATH`.
- `go test -run TestName ./pkg/codex` — focused test execution.
- `go generate ./pkg/codex` — regenerate `protocol_gen.go` from the pinned upstream schema URL declared in `generate.go`.
- `go run ./pkg/codex/internal/cmd/generate-protocol-types -schema <path-or-url> -out ./pkg/codex/protocol_gen.go -package codexappserver` — direct generator invocation (use a different `-schema` for local experimentation).
- `go mod tidy && go mod vendor` — refresh module graph and the committed `vendor/` tree after dependency changes.

Use `direnv allow` (or source `.envrc`) so the `GOEXPERIMENT` flags (`jsonv2`, `greenteagc`, `simd`, …) are active locally — generated code and tests assume them.

## Coding Style & Naming Conventions

- Format with `gofmt -s -w .` and `gofumpt -w -extra .` before committing; tabs for indentation, LF endings (`.gitattributes`).
- Target Go 1.27+; prefer `any` over `interface{}` and generics where it sharpens the API.
- JSON: use `github.com/go-json-experiment/json` and `jsontext`; struct tags use `omitzero`, never `omitempty`.
- Test comparisons use `gocmp "github.com/google/go-cmp/cmp"` (aliased); do not introduce `testify`.
- All `.go` files start with the Apache-2.0 header from `hack/boilerplate/boilerplate.go.txt` (year `2026`).
- Package name is `codexappserver` (kept for API stability); directory is `codex`.
- Godoc comments end with a period and document exported identifiers.

## Testing Guidelines

- Standard library `testing` only. Mark CPU-safe tests `t.Parallel()`; use `t.Context()` instead of `context.Background()`.
- Table-driven tests use `tests := map[string]struct{...}{...}` keyed by descriptive names with a `success:` / `error:` prefix (see `internal/cmd/generate-protocol-types/main_test.go`).
- Real-server coverage lives behind `RUN_REAL_CODEX_TESTS=1` and additionally requires `codex` on `PATH`; never assume the binary is present in CI by default.
- When changing `notification.go` or the routing surface, update or extend the round-trip, unknown-payload, and turn-stream consumer tests called out in `review_notes.md`.
- After regenerating `protocol_gen.go`, run `go test ./pkg/codex/...` — `protocol_gen_test.go` and `public_types_test.go` enforce rename/identity parity against `testdata/`.

## Commit & Pull Request Guidelines

- Commit subjects follow `scope: lowercase imperative subject`, where `scope` is the affected path or component, e.g. `codex-app-server: route turn notifications without global loss` or `pkg/codex/protocol: regenerate with description-derived godoc`. Use `test:` for test-only changes and `all:` for repo-wide work.
- Sign commits: `git commit --gpg-sign` (project convention).
- One logical change per commit; regenerated artifacts (`protocol_gen.go`, `testdata/`) belong in the same commit as the generator or schema change that produced them.
- Pull requests must fill the `## Why` section of `.github/PULL_REQUEST_TEMPLATE.md` with the motivating change and link any upstream schema version (`rust-vX.Y.Z-...`) or issue. Note opt-in test runs (e.g. `RUN_REAL_CODEX_TESTS=1`) when they were exercised.

## Generated Code & Schema Pinning

- The upstream schema URL is pinned in `pkg/codex/generate.go` (currently `rust-v0.131.0-alpha.9`). Bump that URL in the same commit that regenerates `protocol_gen.go`.
- Do not hand-edit `protocol_gen.go`. If the generator output is wrong, fix it in `internal/cmd/generate-protocol-types/main.go` and re-run `go generate`.
- Vendor the resulting dependency graph (`go mod vendor`) so reproducible builds keep working offline.

## Agent-Specific Instructions

- Respect the routing contract in `doc.go` / `review_notes.md`: `Client.NextNotification` is raw passthrough; `TurnHandle.Stream` / `Run` own the per-turn consumer; only one active consumer per `Client` at a time.
- Never introduce plain `Config` or `Thread` root types from the generator — the rename policy is asserted by tests and is load-bearing for SDK callers.
- Read the relevant `*_test.go` before changing decoders or routers; tests express the intended invariants more precisely than this guide.
