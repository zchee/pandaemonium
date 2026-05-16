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
- `go test -v -race -count=1 -shuffle=on ./...` — run all unit and SDK tests (excludes the opt-in real-server lane).
- `RUN_REAL_CODEX_TESTS=1 go test -v -race -count=1 -shuffle=on ./pkg/codex/...` — run integration tests against a real `codex` binary on `PATH`.
- `go test -v -race -count=1 -shuffle=on -run TestName ./pkg/codex` — focused test execution.
- `go generate ./pkg/codex` — regenerate `protocol_gen.go` from the pinned upstream schema URL declared in `generate.go`.
- `go run ./pkg/codex/internal/cmd/generate-protocol-types -schema <path-or-url> -out ./pkg/codex/protocol_gen.go -package codexappserver` — direct generator invocation (use a different `-schema` for local experimentation).
- `go mod tidy && go mod vendor` — refresh module graph and the committed `vendor/` tree after dependency changes.

## Coding Style & Naming Conventions

- Format with `gofmt -s -w .` and `gofumpt -w -extra .` before committing; tabs for indentation, LF endings (`.gitattributes`).
- Target Go 1.27+; prefer `any` over `interface{}` and generics where it sharpens the API.
- JSON: use `github.com/go-json-experiment/json` and `jsontext`; struct tags use `omitzero`, never `omitempty`.
- Test comparisons use `gocmp "github.com/google/go-cmp/cmp"` (aliased); do not introduce `testify`.
- All `.go` files start with the Apache-2.0 header from `hack/boilerplate/boilerplate.go.txt` (year `2026`).
- Package name is `codex` (kept for API stability); directory is `codex`.
- Godoc comments end with a period and document exported identifiers.

## Testing Guidelines

- Standard library `testing` only. Mark CPU-safe tests `t.Parallel()`; use `t.Context()` instead of `context.Background()`.
- Table-driven tests use `tests := map[string]struct{...}{...}` keyed by descriptive names with a `success:` / `error:` prefix (see `internal/cmd/generate-protocol-types/main_test.go`).
- Real-server coverage lives behind `RUN_REAL_CODEX_TESTS=1` and additionally requires `codex` on `PATH`; never assume the binary is present in CI by default.
- When changing `notification.go` or the routing surface, update or extend the round-trip, unknown-payload, and turn-stream consumer tests called out in `review_notes.md`.
- After regenerating `protocol_gen.go`, run `go test ./pkg/codex/...` — `protocol_gen_test.go` and `public_types_test.go` enforce rename/identity parity against `testdata/`.

## Commit & Pull Request Guidelines

- Commit subjects follow `scope: lowercase imperative subject`, where `scope` is the affected path or component, e.g. `codex-app-server: route turn notifications without global loss` or `pkg/codex/generator: regenerate with description-derived godoc`. Use `test:` for test-only changes and `all:` for repo-wide work.
- Sign commits: `git commit --gpg-sign` (project convention).
- One logical change per commit; regenerated artifacts (`protocol_gen.go`, `testdata/`) belong in the same commit as the generator or schema change that produced them.
- Pull requests must fill the `## Why` section of `.github/PULL_REQUEST_TEMPLATE.md` with the motivating change and link any upstream schema version (`rust-vX.Y.Z-...`) or issue. Note opt-in test runs (e.g. `RUN_REAL_CODEX_TESTS=1`) when they were exercised.

## Generated Code & Schema Pinning

- The upstream schema URL is pinned in `pkg/codex/generate.go` (currently `rust-v0.131.0-alpha.9`). Bump that URL in the same commit that regenerates `protocol_gen.go`.
- Do not hand-edit `protocol_gen.go`. If the generator output is wrong, fix it in `internal/cmd/generate-protocol-types/main.go` and re-run `go generate`.
- Vendor the resulting dependency graph (`go mod vendor`) so reproducible builds keep working offline.

## internal/memchr

- Pure-Go port of the byte-search subset of [BurntSushi/memchr](https://github.com/BurntSushi/memchr): `Memchr/Memchr2/Memchr3` + `Memrchr/Memrchr2/Memrchr3` canonical, plus `IndexByte/IndexByte2/IndexByte3 / LastIndexByte/LastIndexByte2/LastIndexByte3` `bytes`-style wrappers. **Non-Goals (do not add):** `memmem`, the supporting `arch::all` algorithms (Rabin-Karp, Two-Way, Shift-Or, packed-pair), iterators, and the `Memchr*` stateful types.
- **Build-tag matrix** (every HEAD must build green on every tuple, exactly one file binds each `*Impl`):
  - `amd64` + `goexperiment.simd` + `!force_swar`: `memchr_amd64.go` (runtime AVX2-vs-SSE2 pick via `archsimd.X86.AVX2()`; `boundImpl` = `"avx2"` or `"sse2"`).
  - `amd64` + `!goexperiment.simd` + `!force_swar`: `memchr_amd64_nosimd.go` (anchor) + `dispatch_swar_default.go` (binds SWAR; `boundImpl="swar"`).
  - `arm64` + `!force_swar`: `memchr_arm64.go` + hand-written `*_arm64.s` (NEON syndrome trick; `boundImpl="neon"`).
  - any GOARCH + `force_swar`, or other GOARCH (386/ppc64le/riscv64/…): `dispatch_swar_default.go` binds SWAR.
- **`-tags=force_swar`** is the project-internal override for exercising the SWAR fallback on amd64/arm64 hardware (regression test, profile comparison). It is a TEST-ONLY tag — do not ship gated production paths behind it.
- **GODEBUG=cpu.avx2=off** (R-NEW-4): users disabling AVX2 via this knob will be silently downgraded to SSE2 by `archsimd.X86.AVX2()`. `internal/memchr/dispatch_test.go::TestBackendBinding` (AC-HARNESS-7) fails in CI if this slips into a default config. To intentionally force SWAR on AVX2 hardware, build with `-tags=force_swar`.
- **Corpus regen** (`internal/memchr/testdata/golden_corpus.json`): the JSON corpus is extracted from upstream `BurntSushi/memchr` via the Rust shim at `hack/extract-memchr-corpus/`. To regenerate after bumping the upstream pin in `hack/extract-memchr-corpus/Cargo.toml`:
  ```
  (cd hack/extract-memchr-corpus && cargo run --release) > internal/memchr/testdata/golden_corpus.json
  ```
  The shim pins `memchr = "=2.7.4"` exact-equality; `Cargo.lock` is committed for reproducibility.
- **Perf gate** (`hack/memchr-perf-gate/`): CI hard gate behind AC-HARNESS-6. Runs `go test -bench=. -count=10` for `BenchmarkMemchr*` vs `BenchmarkIndexByteStd`, pipes both through `benchstat -delta-test=utest -alpha=0.05`. **amd64**: a statistically-significant slowdown at any gated size (`n >= 64`) exits 1. **arm64**: prints `UNTESTED — no arm64 CI runner provisioned; exiting 0` and exits 0 (Follow-up: provision a Linux/arm64 runner, then flip the branch).
- **Tools** (`tool` directives in `go.mod`):
  - `golang.org/x/perf/cmd/benchstat` — invoke as `go tool benchstat …`; consumed by `hack/memchr-perf-gate`.
  - `github.com/klauspost/asmfmt/cmd/asmfmt` — invoke as `go tool asmfmt -w internal/memchr/*.s` before committing changes to the six NEON `.s` files. The NEON sources are formatted with asmfmt; `go vet`'s asm-vet stays clean across asmfmt-style indentation.
- **Verifying the `!goexperiment.simd` build tuple** from the `/opt/local/go.simd` toolchain (which has `simd` in its experiment defaults): either explicit form works — `GOEXPERIMENT=nosimd go build ./internal/memchr/` or `GOEXPERIMENT="$(echo $GOEXPERIMENT | tr ',' '\n' | grep -v '^simd$' | paste -sd,)" go build ./internal/memchr/`. The `nosimd` form is shorter; the strip form is portable to other toolchains.

## Agent-Specific Instructions

- Respect the routing contract in `doc.go` / `review_notes.md`: `Client.NextNotification` is raw passthrough; `TurnHandle.Stream` / `Run` own the per-turn consumer; only one active consumer per `Client` at a time.
- Never introduce plain `Config` or `Thread` root types from the generator — the rename policy is asserted by tests and is load-bearing for SDK callers.
- Read the relevant `*_test.go` before changing decoders or routers; tests express the intended invariants more precisely than this guide.
