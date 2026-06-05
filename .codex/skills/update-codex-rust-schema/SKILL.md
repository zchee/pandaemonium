---
name: update-codex-rust-schema
description: Update the Go pkg/codex SDK in pandaemonium from an upstream OpenAI Codex Rust release tag such as rust-v0.XXX.X. Use when Codex needs to refresh the local codex app-server generated protocol schema, regenerate pkg/llm/codex/protocol_gen.go, preserve generated-code boundaries, port adjacent public API or example changes from upstream Rust/Python Codex releases, and verify package/tests/examples before commit or report.
---

# Update Codex Rust Schema

## Goal

Port an upstream Codex `rust-v0.*` release into `pkg/llm/codex` without losing
schema provenance, public API compatibility, or routing invariants.

Use this as a workflow skill, not as a substitute for reading the live repo.
Always verify the actual worktree path, branch, module path, package name, and
`git status` before editing; prior notes and OCR may confuse `pandaemonium`,
`pandemonium`, `pkg/llm/codex`, `pkg/llm/coder`, or `apk/llm/codex`.

## Decision tree

1. **Schema-only refresh**: ensure the intended `codex` binary is on `PATH`
   (or use the generator's `-codex-bin` override for manual runs), run
   `go generate ./pkg/llm/codex`, inspect generated/type/test diffs, and verify.
   The normal generator input comes from
   `codex app-server generate-json-schema --experimental --out <tmpdir>`,
   not an upstream repository URL in `pkg/llm/codex/generate.go`.
   The checked-in `protocol_gen.go` `Source binary` header must match
   `codex --version` for the schema-regeneration contract test to pass.
2. **Schema plus generator behavior**: change
   `pkg/llm/codex/internal/cmd/generate-protocol-types/`, add/update generator
   tests first, then regenerate `pkg/llm/codex/protocol_gen.go`.
3. **Schema plus public API parity**: inspect upstream release diffs and local
   public tests; update hand-written files, examples, docs, and signature tests
   with the generated change.
4. **Transport/routing/login changes**: read `pkg/llm/codex/doc.go`,
   `pkg/llm/codex/review_notes.md`, and routing/stream tests before editing.
   Preserve the single-active-consumer contract.

For the detailed checklist, read `references/pkg-codex-rust-update.md`.

## Required first checks

Run from the repository root:

```sh
git status --short --branch --untracked-files=all
go list -m
go env GOTOOLCHAIN GOEXPERIMENT
sed -n '1,120p' pkg/llm/codex/generate.go
```

Then identify the current installed `codex` binary and, when comparing against
upstream source changes, the current and target upstream tags. Prefer exact tags
such as `rust-v0.134.0-alpha.3`, not vague `latest`, unless the user
explicitly asks to research the current upstream release.

## Edit rules

- Do not hand-edit `pkg/llm/codex/protocol_gen.go`; fix the generator or the
  installed/generated schema source and regenerate it.
- Keep generated changes and generator/schema-test changes in the same logical
  commit/report unit.
- Preserve package naming and rename policy. Do not introduce root `Config` or
  `Thread` types if tests assert renamed/public identities.
- Preserve JSON v2 conventions: `github.com/go-json-experiment/json`,
  `jsontext`, and `omitzero` rather than `omitempty`.
- Reuse existing test helpers and comparison style (`gocmp` alias). Do not add
  `testify`.
- Keep real app-server tests behind `RUN_REAL_CODEX_TESTS=1` unless explicitly
  requested.

## Verification baseline

Choose the smallest sufficient set, then expand if touched areas require it:

```sh
go test -count=1 ./pkg/llm/codex/...
go test -race -count=1 -shuffle=on ./pkg/llm/codex/...
go build ./...
git diff --check
```

For examples/public API changes, also run:

```sh
go test -count=1 ./pkg/llm/codex ./pkg/llm/codex/tests ./pkg/llm/codex/examples
go test -count=1 ./pkg/llm/codex/examples/...
```

For optional real-server validation, only when a real `codex` binary is intended
and available:

```sh
RUN_REAL_CODEX_TESTS=1 go test -v -race -count=1 -shuffle=on ./pkg/llm/codex/...
```

Report skipped optional lanes explicitly; do not imply they passed.

## Final report shape

Include:

- Target upstream tag and previous tag when upstream comparison was part of the
  task; otherwise report the `codex` binary path/version used for local schema
  generation.
- Files changed, separating generated, generator, tests, examples, and docs.
- Exact verification commands and outcomes.
- Any known unrun optional checks or branch/worktree caveats.
- Current `git status --short --branch --untracked-files=all` if the user asked
  for commit readiness or clean-state evidence.
