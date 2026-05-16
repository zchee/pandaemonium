# `pkg/toml` Upstream Pins

This document records the pinned versions of every upstream dependency
`pkg/toml` ports from or measures against. Q3 (Architect ruling
2026-05-16) moved this file from Phase 5 to Phase 1 kickoff because
the perf-gate and correctness-oracle work — which Phase 1 owns —
already depend on stable pins.

Every pin bump is a dedicated commit. The commit MUST: (1) include the
diff of any test-corpus changes, (2) re-run the full AC matrix and
record any behavioral changes, (3) split out any parser bug-fix
adjustments into separate commits before the pin bump, (4) record the
reviewer's name in the commit body.

## Toolchain

```text
Toolchain          : Go 1.27-devel from /opt/local/go.simd
Snapshot version    : go1.27-devel_47f175b024 Thu May 14 19:14:48 2026 +0900
Build flags         : X:loopvar,newinliner,jsonv2,sizespecializedmalloc,simd,runtimesecret,mapsplitgroup
Snapshot path       : /opt/local/go.simd
Snapshot recorded   : 2026-05-16
GOEXPERIMENT (.envrc): loopvar,newinliner,jsonv2,greenteagc,simd,randomizedheapbase64,sizespecializedmalloc,runtimesecret,mapsplitgroup
GOTOOLCHAIN         : auto
```

### `go.mod` `go` directive

Q7 is resolved in this step:

- `go.mod` was bumped from `go 1.26` to `go 1.27` to align module
  identity with the runtime identity recorded above.

Q7 commit SHA: pending; task-3 tracker records the Step 1 closure and will be
  filled when this pre-phase change is merged.

### Snapshot stdlib quirk (informational)

The `/opt/local/go.simd` snapshot above contains a stdlib signature
drift at `src/internal/runtime/maps/memhash_aes_simd.go`: an
`(*[4]uint32)` is passed to `archsimd.LoadUint32x4` which expects
`[]uint32`. This breaks *every* amd64 build (with or without
`goexperiment.simd`) cross-compiled from this snapshot. The
`pkg/toml/internal/scan` package's CI workflows
(`.github/workflows/toml-scan-ci.yml` and `toml-perf-gate.yml`) use
`golang.org/dl/gotip` for the amd64+simd cells — gotip has the fix —
and stable Go for the amd64 no-simd cells. The amd64+simd CI job
carries `continue-on-error: true` until the snapshot is refreshed
with a stdlib that builds cleanly.

## SIMD intrinsics — `simd/archsimd`

```text
Source         : /opt/local/go.simd/src/simd/archsimd (toolchain-bundled)
Pinned via     : toolchain snapshot above
Blast radius   : pkg/toml/internal/scan/scan_amd64.go (intrinsics path)
                 + pkg/toml/internal/scan/scan_amd64_single_byte.s (hand-rolled
                   path added by T7 for LocateNewline + ScanLiteralString;
                   see scan/doc.go for the rationale on splitting those two
                   kernels into hand-rolled .s while the other four stay on
                   archsimd intrinsics).
CI gate        : `grep -l 'simd/archsimd' pkg/toml | wc -l` must equal `1`
                 (Architect Tension T4 / risk gap G3).
```

## `go-json-experiment/json` (jsonv2)

```text
Source         : github.com/go-json-experiment/json
Pseudo-version : v0.0.0-20260505212615-e40f80bf6836  (per go.mod)
Pinned         : 2026-05-16 (date of this snapshot)
Blast radius   : pkg/toml/options.go (Phase 4 deliverable; NOT yet present).
                 Phase 1 does not import jsonv2.
Drift policy   : we mirror jsonv2's interface SHAPE (method signatures),
                 not its names. If jsonv2 renames in a way that breaks
                 our shape we hold the line until the next pinned
                 jsonv2 version. (Architect risk gap G2.)
```

## toml-rs (source of the port)

```text
Source        : https://github.com/toml-rs/toml
Tag / commit  : v0.25.11 / 45456abc190bcf7b81dfc96914b726d7b3053e41
Pinned        : 2026-05-17 (Step 2)
Snapshot date : 2026-04-07 17:15:52 -0500
Corpus source : crates/toml/tests/fixtures
Corpus path   : pkg/toml/testdata/toml-rs/corpus
Corpus files  : 68
Corpus bytes  : 2075
Corpus SHA-256: 01daf47230b2211724854b7cb731a4c9c0d60ced84fa310920ae35e9800b389c
Re-import     : ./hack/import-toml-rs/import.sh v0.25.11
Provenance    : pkg/toml/testdata/toml-rs/provenance.txt
Manifest      : pkg/toml/testdata/toml-rs/manifest.txt
```

This pin now lands as part of Step 2 so parser-level (Phase 2) and
facade-level (Phase 4) imports can consume a frozen toml-rs fixture
snapshot from `pkg/toml/testdata/toml-rs/corpus`. Parser implementation
must remain blocked until this pin, import procedure, and corpus snapshot
are committed and verified.

## Cargo.lock corpus (`pkg/toml/testdata/corpus/cargo.lock`)

Q8 is resolved in this step:

```text
Source         : https://raw.githubusercontent.com/rust-lang/cargo/0.86.0/Cargo.lock
Upstream ref   : 0.86.0
SHA-256        : 9ea94b60b3ee80c73f52186946bb280dc41c7287bbb678988618a6839533dbe9
Byte count     : 103263
Pinned         : 2026-05-17 (Q8)
```

Phase 1 does NOT consume the Cargo.lock corpus. Phase 4 (AC-FAC-6
gate) is the first consumer, and Phase 5 (AC-EDIT-6 gate) is the
second. T5's perf benchmarks
(`pkg/toml/internal/scan/bench_test.go`) build their 64 KiB buffers
from a deterministic seeded PRNG and do NOT depend on this corpus.

## Q9/Q10 confirmations

- Q9 confirmed: SWAR-only non-amd64/non-arm64 verification target is
  `wasip1/wasm` in `toml-scan-ci` (build-only check). The current
  `toml-perf-gate` matrix is intentionally amd64/arm64-focused and does
  not currently include a wasip1 perf cell.
- Q10 confirmed: `MaxKeyLength` parser hard cap is `65536` bytes (`64 KiB`).

## toml-test compliance suite

```text
Source         : https://github.com/toml-lang/toml-test
Branch / commit: main / 4f76d84def032d092df152eb6efea5a6f78a0cee
Pinned         : 2026-05-17 (Phase 4 kickoff)
Corpus path    : pkg/toml/testdata/toml-test
Corpus files   : 80
Corpus bytes   : 6797
Corpus SHA-256 : 1efccd7dd9048d844313175b0ad65ebd1f1dbfe91cf15236d3eddb76246bb933
Controversial cases this port matches: bounded Phase 4 facade fixture slice;
                 parser-level unsupported cases remain syntax-test evidence.
Re-import      : ./hack/import-toml-test/import.sh 4f76d84def032d092df152eb6efea5a6f78a0cee
```

Phase 1 does not import or consume the toml-test compliance suite.
This pin lands as part of Phase 4's facade work (AC-FAC-9 — all 12
toml-test groups pass).

## Phase 2.5 trajectory

Status: **executed 2026-05-17**. Phase 2.5 is a planning-only perf
trajectory checkpoint between the parser/tokenizer work and the
datetime/facade phases; it does not add a new AC-* gate and does not
justify new parser behavior. Its purpose is to document whether the
parser plus scanner trajectory is plausible before Phase 4 invests in
reflection-cache and facade machinery.

Required smoketest harness:

```text
Package        : pkg/toml/internal/smoketest
Build tag      : bench
Benchmark      : BenchmarkSmoketestUnmarshal
Corpus         : pkg/toml/testdata/corpus/cargo.lock
Corpus SHA-256 : 9ea94b60b3ee80c73f52186946bb280dc41c7287bbb678988618a6839533dbe9
Baseline       : BurntSushi/toml on the same corpus and representative struct
Protocol       : GOMAXPROCS=1, empty GODEBUG, -count=10, -cpu=1, -benchtime=5s
Command        : go test -tags=bench -bench=BenchmarkSmoketestUnmarshal \
                 -benchmem -count=10 -cpu=1 -benchtime=5s \
                 ./pkg/toml/internal/smoketest/
Deletion rule  : rm -rf pkg/toml/internal/smoketest/ before Phase 4 facade work
```
Outcome        : PASS — proceed to Phase 3.
Decision rule  : smoketest_thru / burntsushi_thru >= 0.5x.
Final ratio    : 2.788x point, 2.788x lower95, p=0.000 n=10.
Command        : CGO_ENABLED=0 GOTOOLCHAIN=local /opt/local/go.simd/bin/go run ./hack/toml-perf-gate --kind=parser --ratio=0.5 --count=10 --benchtime=5s --benchstat=/Users/zchee/go/bin/benchstat
Toolchain      : go version go1.27-devel_b1972f9085 Sun May 17 03:53:30 2026 +0900 X:loopvar,newinliner,jsonv2,sizespecializedmalloc,simd,runtimesecret,mapsplitgroup darwin/arm64
Corpus SHA-256 : 9ea94b60b3ee80c73f52186946bb280dc41c7287bbb678988618a6839533dbe9
Corpus bytes   : 103263
Benchmark      : pkg/toml/internal/smoketest BenchmarkSmoketestUnmarshal.
BurntSushi     : 2.940 ms/op, 33.49 MiB/s, 1508.0 KiB/op, 25.37k allocs/op.
Smoketest      : 1.055 ms/op, 93.38 MiB/s, 388.5 KiB/op, 12.18k allocs/op.
Profile note   : initial pilot failed at 0.153x because Decoder.scanString
                 converted the remaining corpus to string for every quoted
                 value prefix check, allocating ~333 MB/op. The Phase 2.5
                 remediation replaced that conversion with byte-prefix checks;
                 parser tokenization improved to ~303-310 us/op and ~700 B/op
                 on BenchmarkDecoderTokens_CargoLock.
Temporary      : pkg/toml/internal/smoketest/ is build-tagged bench-only and
                 carries DELETE_AT_PHASE_4.md for removal at Phase 4 step 0.
```

Current evidence status: `pkg/toml/internal/smoketest/` is present under a
temporary `bench` build tag and must be removed before Phase 4 facade
work per `pkg/toml/internal/smoketest/DELETE_AT_PHASE_4.md`. The
parser's informational `BenchmarkDecoderTokens_CargoLock` remains
useful for parser-token profiling, but the Phase 2.5 source of record
is the `BenchmarkSmoketestUnmarshal` BurntSushi comparison above.

## AC-SIMD-5 documented exceptions

**Status: no documented exceptions needed — all 6 scans pass the
AC-SIMD-5 baseline table on real arm64 NEON hardware after the T7+T8
hand-rolled-assembly rework. amd64 perf evidence pending CI on first
PR push of `.github/workflows/toml-perf-gate.yml` (worker-arch-amd64's
T7 prediction is "~1.0× stdlib bytes.IndexByte" based on structural
mirror of `internal/bytealg/indexbyte_amd64.s`).**

The plan's AC-SIMD-5 amended text allows the dispatcher to bind the
SWAR variant for a scan that fails its gate and document the failure
here. T5's first perf-gate run on the dev host (darwin/arm64 NEON)
found `LocateNewline` and `ScanLiteralString` failing their
`bytes.IndexByte` baseline (0.89× each) because the
archsimd-intrinsics path could not beat stdlib's hand-rolled
single-TEXT NEON assembly. The user chose routing option (b): re-open
T2/T3 (as tasks #7 and #8) to hand-roll the SSE2/AVX2/NEON kernels for
those two scans, mirroring `internal/bytealg/indexbyte_*.s`'s 32-byte
stride + VORR+VADDP-D2 fast-term-check + magic-constant syndrome on
match. The rework cleared the gate cleanly:

```text
LocateNewline (arm64 NEON):
  Pre-rework gate (T5)         : FAIL 0.893× (vs bytes.IndexByte)
  Post-rework gate (T6 / T8)   : PASS 1.340× (+34% over baseline; p=0.000 n=10)
  Toolchain                    : /opt/local/go.simd, darwin/arm64
  Bench buffer                 : 65536 B with single '\n' planted at end

ScanLiteralString (arm64 NEON):
  Pre-rework gate (T5)         : FAIL 0.889× (vs bytes.IndexByte)
  Post-rework gate (T6 / T8)   : PASS 1.331× (+33% over baseline; p=0.000 n=10)
  Toolchain                    : /opt/local/go.simd, darwin/arm64
  Bench buffer                 : 65536 B with single 0x27 planted at end

LocateNewline (amd64 SSE2/AVX2):
  Pre-rework gate (T5)         : not measured locally (dev host is darwin/arm64)
  Post-rework gate (T6 / T7)   : pending CI on first PR push (predicted ~1.0×;
                                 structural mirror of stdlib indexbyte_amd64.s)

ScanLiteralString (amd64 SSE2/AVX2):
  Pre-rework gate (T5)         : not measured locally (dev host is darwin/arm64)
  Post-rework gate (T6 / T7)   : pending CI on first PR push (predicted ~1.0×)
```

If the amd64 CI on first PR push reveals either single-byte scan
short of the 1.0× threshold despite T7's prediction, this section
will be updated with the documented-exception entry and the
dispatcher in `scan_amd64.go` flipped to bind `bytes.IndexByte` for
that scan. The arm64 path is settled; the amd64 path is the only
remaining open item.

## Final integration runbook

Step 8 verification keeps the public surface flat and leaves
benchmark-only dependencies out of production builds.

### Package shape

The approved package list is:

```text
github.com/zchee/pandaemonium/pkg/toml
github.com/zchee/pandaemonium/pkg/toml/internal/reflectcache
github.com/zchee/pandaemonium/pkg/toml/internal/scan
```

Re-check it with:

```bash
go list ./pkg/toml/... | sort -u
```

### Build tags and dependency graph

- `force_swar` selects the pure-Go scan backend and is mandatory in the
  scan fallback verification lane:
  `go test -tags=force_swar -race -count=1 ./pkg/toml/internal/scan/`.
- `bench` enables benchmark-only comparator tests for BurntSushi and
  pelletier. Those libraries are present in `go.mod` for reproducible
  benchmark wiring, but must not appear in the production dependency
  graph.
- Use `go list -deps ./pkg/toml` for the production graph.
- Use `go list -deps -test -tags=bench ./pkg/toml` for the benchmark
  test graph; the competitors live in `_test.go` files, so omitting
  `-test` intentionally does not show them.

### Fuzz corpus discipline

The pinned toml-rs and toml-test corpora are immutable review
artifacts. Refresh them only through:

```bash
./hack/import-toml-rs/import.sh v0.25.11
./hack/import-toml-test/import.sh 4f76d84def032d092df152eb6efea5a6f78a0cee
```

Do not hand-edit imported corpus files. Fuzz discoveries should become
small, named regression seeds or test fixtures, with provenance recorded
next to the affected corpus or in the phase closeout note.

### Fuzz execution commands

The decoder package currently exposes three fuzz targets:

- `FuzzDecoder`
- `FuzzDecoderConstructorParity`
- `FuzzTokenStream`

Run them one at a time with anchored regular expressions. `go test`
uses the fuzz flag as a regexp, so `-fuzz=FuzzDecoder` would also
match `FuzzDecoderConstructorParity`.

```bash
go test -run=^$ -fuzz='^FuzzDecoder$' -fuzztime=60s ./pkg/toml/
go test -run=^$ -fuzz='^FuzzDecoderConstructorParity$' -fuzztime=60s ./pkg/toml/
go test -run=^$ -fuzz='^FuzzTokenStream$' -fuzztime=60s ./pkg/toml/
```

### Perf gates

Run scan gates per `pkg/toml/internal/scan/VERIFICATION.md`. The
facade and edit gates use the implemented CLI flag names:

```bash
go run ./hack/toml-perf-gate --kind=facade --ratio-burntsushi=1.5 --ratio-pelletier=1.3
go run ./hack/toml-perf-gate --kind=edit --ratio-edit=0.25
```

The facade gate is two-part: BurntSushi must pass, while the Pelletier
ratio is currently a documented Phase 4 exception in
`pkg/toml/PHASE4_BENCHMARK_EVIDENCE.md`. Do not treat that exception as
an edit-path failure.

`--ratio-edit` is the Phase 5 Document edit threshold. Do not use the
older `--ratio-pelletier` spelling for edit gates; that flag belongs to
the Phase 4 facade comparison.

### Vendor and CI expectations

This checkout does not carry a `vendor/` directory. If a future lane
adds vendor mode, regenerate it with `go mod vendor` in the same commit
as the dependency change and verify `vendor/modules.txt` is in sync.

CI expectations:

- `.github/workflows/test.yaml` covers normal repository tests.
- `.github/workflows/toml-scan-ci.yml` covers cross-arch scanner build,
  vet, race, and `force_swar` verification.
- `.github/workflows/toml-perf-gate.yml` runs scan perf gates on
  opt-in perf triggers (`workflow_dispatch`, push to main, or PR label
  `perf-gate`) and uploads raw gate output as artifacts.
