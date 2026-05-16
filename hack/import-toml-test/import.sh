#!/usr/bin/env bash
set -euo pipefail
ref=${1:-main}
root=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git clone --depth=1 https://github.com/toml-lang/toml-test "$tmp/repo"
git -C "$tmp/repo" fetch --depth=1 origin "$ref"
git -C "$tmp/repo" checkout --detach FETCH_HEAD >/dev/null
rm -rf "$root/pkg/toml/testdata/toml-test"
mkdir -p "$root/pkg/toml/testdata/toml-test/valid" "$root/pkg/toml/testdata/toml-test/invalid"
find "$tmp/repo/tests/valid" -type f \( -name '*.toml' -o -name '*.json' \) | sort | head -40 | while read -r f; do
  rel=${f#"$tmp/repo/tests/valid/"}
  mkdir -p "$root/pkg/toml/testdata/toml-test/valid/$(dirname "$rel")"
  cp "$f" "$root/pkg/toml/testdata/toml-test/valid/$rel"
done
find "$tmp/repo/tests/invalid" -type f -name '*.toml' | sort | head -40 | while read -r f; do
  rel=${f#"$tmp/repo/tests/invalid/"}
  mkdir -p "$root/pkg/toml/testdata/toml-test/invalid/$(dirname "$rel")"
  cp "$f" "$root/pkg/toml/testdata/toml-test/invalid/$rel"
done
sha=$(git -C "$tmp/repo" rev-parse HEAD)
count=$(find "$root/pkg/toml/testdata/toml-test" -type f | wc -l | tr -d ' ')
bytes=$(find "$root/pkg/toml/testdata/toml-test" -type f -print0 | xargs -0 cat | wc -c | tr -d ' ')
tree=$(find "$root/pkg/toml/testdata/toml-test" -type f | sort | while read -r f; do shasum -a 256 "$f"; done | shasum -a 256 | awk '{print $1}')
cat > "$root/pkg/toml/testdata/toml-test/provenance.txt" <<LOCK
source_url = "https://github.com/toml-lang/toml-test"
upstream_ref = "$ref"
upstream_commit = "$sha"
imported_path = "tests"
corpus_file_count = "$count"
corpus_total_bytes = "$bytes"
corpus_tree_sha256 = "$tree"
LOCK
cat > "$root/pkg/toml/testdata/toml-test/.import-lock" <<LOCK
UPSTREAM_REPO=https://github.com/toml-lang/toml-test
UPSTREAM_REF=$ref
UPSTREAM_COMMIT=$sha
SOURCE_PATH=tests
SOURCE_FILE_COUNT=$count
SOURCE_TOTAL_BYTES=$bytes
SOURCE_TREE_SHA256=$tree
LOCK
find "$root/pkg/toml/testdata/toml-test" -type f | sort | sed "s#^$root/pkg/toml/testdata/toml-test/##" > "$root/pkg/toml/testdata/toml-test/manifest.txt"
