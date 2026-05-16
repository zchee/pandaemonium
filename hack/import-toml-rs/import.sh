#!/usr/bin/env bash
set -euo pipefail

: "${REPO_ROOT:=$(git rev-parse --show-toplevel)}"
: "${UPSTREAM_REPO:=https://github.com/toml-rs/toml}"
: "${UPSTREAM_REF:=v0.25.11}"

if [ "${1-}" ]; then
  UPSTREAM_REF="$1"
fi

WORK_DIR="$(mktemp -d -t toml-rs-import-XXXXXX)"

readonly DST_DIR="$REPO_ROOT/pkg/toml/testdata/toml-rs"
readonly TEST_SRC_SUBDIR="crates/toml/tests/fixtures"
readonly UPSTREAM_INFO="$DST_DIR/provenance.txt"
readonly MANIFEST_FILE="$DST_DIR/manifest.txt"
readonly NOTE_FILE="$DST_DIR/README.md"
readonly LOCK_FILE="$DST_DIR/.import-lock"
readonly CORPUS_DIR="$DST_DIR/corpus"

cleanup() {
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

printf 'Importing toml-rs tests corpus from %s at %s...\n' "$UPSTREAM_REPO" "$UPSTREAM_REF"


git clone --filter=blob:none --depth 1 --no-checkout "$UPSTREAM_REPO" "$WORK_DIR"

if git -C "$WORK_DIR" fetch --depth 1 origin "refs/tags/$UPSTREAM_REF:refs/tags/$UPSTREAM_REF"; then
  REF_TO_USE="$UPSTREAM_REF"
elif git -C "$WORK_DIR" fetch --depth 1 origin "$UPSTREAM_REF"; then
  REF_TO_USE="FETCH_HEAD"
else
  echo "Unable to fetch ref '$UPSTREAM_REF' from $UPSTREAM_REPO" >&2
  exit 1
fi

git -C "$WORK_DIR" checkout --quiet --detach "$REF_TO_USE"

COMMIT=$(git -C "$WORK_DIR" rev-parse HEAD)
COMMIT_SHORT=$(git -C "$WORK_DIR" rev-parse --short HEAD)
DATE=$(git -C "$WORK_DIR" show -s --format=%ci "$COMMIT")

SRC_DIR="$WORK_DIR/$TEST_SRC_SUBDIR"
if [ ! -d "$SRC_DIR" ]; then
  echo "No fixtures directory found at $SRC_DIR" >&2
  exit 1
fi

rm -rf "$CORPUS_DIR"
mkdir -p "$CORPUS_DIR"
rm -f "$MANIFEST_FILE" "$UPSTREAM_INFO" "$LOCK_FILE"
cp -R "$SRC_DIR"/. "$CORPUS_DIR/"

count_files=$(find "$CORPUS_DIR" -type f | wc -l | tr -d ' ')
size_bytes=$(find "$CORPUS_DIR" -type f -print0 | xargs -0 wc -c | tail -n 1 | awk '{print $1}')
MANIFEST_TMP="$(mktemp "$DST_DIR/.manifest.XXXXXX")"
{
  echo '# path\tsha256\tbytes'
  find "$CORPUS_DIR" -type f -print0 |
    sort -z |
    while IFS= read -r -d '' file; do
      rel="${file#"$CORPUS_DIR"/}"
      hash=$(sha256sum "$file" | awk '{print $1}')
      bytes=$(wc -c <"$file")
      printf '%s\t%s\t%s\n' "$rel" "$hash" "$bytes"
    done
} > "$MANIFEST_TMP"
tree_sha=$(sha256sum "$MANIFEST_TMP" | awk '{print $1}')

{
  echo '# toml-rs test corpus snapshot'
  echo "source_url = \"$UPSTREAM_REPO\""
  echo "upstream_ref = \"$UPSTREAM_REF\""
  echo "upstream_commit = \"$COMMIT\""
  echo "upstream_short = \"$COMMIT_SHORT\""
  echo "snapshot_date = \"$DATE\""
  echo "imported_path = \"$TEST_SRC_SUBDIR\""
  echo "corpus_file_count = $count_files"
  echo "corpus_total_bytes = $size_bytes"
  echo "corpus_tree_sha256 = \"$tree_sha\""
} > "$UPSTREAM_INFO"

{
  cat "$MANIFEST_TMP"
} > "$MANIFEST_FILE"
rm -f "$MANIFEST_TMP"

{
  echo '# toml-rs corpus snapshot'
  echo "Source: $UPSTREAM_REPO"
  echo "Ref: $UPSTREAM_REF"
  echo "Commit: $COMMIT"
  echo "Snapshot date: $DATE"
  echo "File count: $count_files"
  echo "Total bytes: $size_bytes"
  echo "Corpus tree SHA-256: $tree_sha"
  echo "Manifest: manifest.txt"
  echo
  echo 'Canonical corpus comes from crates/toml/tests/fixtures (toml crate).'
  echo 'Refresh only by running: ./hack/import-toml-rs/import.sh [tag-or-sha]'
  echo 'Do not edit corpus files manually; re-run the importer to refresh.'
} > "$NOTE_FILE"

{
  echo "UPSTREAM_REPO=$UPSTREAM_REPO"
  echo "UPSTREAM_REF=$UPSTREAM_REF"
  echo "UPSTREAM_COMMIT=$COMMIT"
  echo "SNAPSHOT_DATE=$DATE"
  echo "SOURCE_PATH=$TEST_SRC_SUBDIR"
  echo "SOURCE_FILE_COUNT=$count_files"
  echo "SOURCE_TOTAL_BYTES=$size_bytes"
  echo "SOURCE_TREE_SHA256=$tree_sha"
} > "$LOCK_FILE"

echo 'toml-rs fixtures corpus imported successfully.'
echo "  ref: $UPSTREAM_REF"
echo "  commit: $COMMIT"
echo "  file count: $count_files"
