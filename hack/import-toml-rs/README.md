# toml-rs test corpus import helper

This helper snapshots toml-rs `crates/toml/tests/fixtures` into
`pkg/toml/testdata/toml-rs/corpus` for a pinned tag/commit.

## Usage

```sh
./hack/import-toml-rs/import.sh [<tag-or-commit>]
```

If `<tag-or-commit>` is omitted, the canonical snapshot `v0.25.11` is used.

The command:

1. Clones `https://github.com/toml-rs/toml` at the requested ref.
2. Copies `crates/toml/tests/fixtures` into
   `pkg/toml/testdata/toml-rs/corpus`.
3. Computes a manifest of SHA-256 checksums into
   `pkg/toml/testdata/toml-rs/manifest.txt`.
4. Writes reproducibility metadata into `pkg/toml/testdata/toml-rs/provenance.txt`.
5. Writes a short README note in `pkg/toml/testdata/toml-rs/README.md`.

## Constraints

- The script intentionally avoids mutating parser code.
- Corpus refresh happens only through explicit runs of this helper.
- Do not edit generated corpus files manually; re-run this script for changes.
- Verification tooling should read `manifest.txt` as source-of-truth for expected files.
