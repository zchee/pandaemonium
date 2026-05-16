# extract-memchr-corpus

One-shot Rust shim that produces the deterministic golden corpus used by
`internal/memchr`'s `TestGoldenCorpus` (plan AC-HARNESS-1).

The Rust crate pins upstream `BurntSushi/memchr` at the exact-equality
version `=2.7.4`. Bumping the pin requires editing **both** `Cargo.toml`
and `Cargo.lock` in the same commit as the regenerated
`internal/memchr/testdata/golden_corpus.json`.

## Regenerate the corpus

From the repository root:

```sh
(cd hack/extract-memchr-corpus && /opt/local/rust/cargo/bin/cargo run --release) \
    > internal/memchr/testdata/golden_corpus.json
```

The output is base64-encoded JSON. Inspect the diff under
`internal/memchr/testdata/golden_corpus.json` and commit it together with
this directory's `Cargo.toml` / `Cargo.lock`.

## Why a Rust shim and not Go-only tables

`AGENTS.md` mandates the Go test suite use the standard library only and
forbids `cgo` — that precludes a live Rust parity oracle at `go test`
time. Extracting once via a Rust crate that calls upstream `memchr`
directly captures the upstream test author's edge cases (planted-needle
positions, head/tail/aligned-word boundaries) without requiring a Rust
toolchain at Go build or test time. The committed JSON is what ships.

## Why pinned exactly

A floating `^2.7` pin would let upstream patch releases silently shift
fixture expectations, requiring a corpus regen on every contributor's
machine. The `=2.7.4` pin plus committed `Cargo.lock` makes regen a
single deterministic command with byte-identical output.

## Bumping `memchr`

Yearly housekeeping (per plan Risks R-7):

1. Bump `memchr = "=2.7.X"` in `Cargo.toml`.
2. Run `cargo update -p memchr` to refresh `Cargo.lock`.
3. Re-run the regen command above; review the resulting JSON diff.
4. Commit the three files (Cargo.toml, Cargo.lock, golden_corpus.json)
   together with a commit subject like `internal/memchr: bump golden
   corpus to memchr =2.7.X`.

## Not on the Go build path

This crate is **not** vendored or referenced by `go.mod`. The Go test
suite reads only the committed JSON. The Rust toolchain is therefore
required only for maintainers regenerating the corpus.
