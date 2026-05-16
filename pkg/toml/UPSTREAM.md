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

```
Toolchain          : Go 1.27-devel from /opt/local/go.simd
Snapshot version    : go1.27-devel_47f175b024 Thu May 14 19:14:48 2026 +0900
Build flags         : X:loopvar,newinliner,jsonv2,sizespecializedmalloc,simd,runtimesecret,mapsplitgroup
Snapshot path       : /opt/local/go.simd
Snapshot recorded   : 2026-05-16
GOEXPERIMENT (.envrc): loopvar,newinliner,jsonv2,greenteagc,simd,randomizedheapbase64,sizespecializedmalloc,runtimesecret,mapsplitgroup
GOTOOLCHAIN         : auto
```

### `go.mod` `go` directive

The repository-wide `go.mod` currently declares `go 1.26`. Pre-Phase-1
step 1 of the plan called for bumping that directive to `1.27` to
align the declared identity with the runtime identity recorded above.
Phase 1 / T6 (this task) is intentionally NOT authorized to edit
`go.mod` per its task-spec DO-NOT list. Tracking as a Pre-Phase-1
follow-up:

```
TODO (Pre-Phase-1 step 1, deferred):
  bump go.mod `go` directive 1.26 → 1.27 in a dedicated commit;
  record the bump-commit SHA in this UPSTREAM.md once it lands.
```

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

```
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

```
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

```
TODO (Pre-Phase-1 step 4, deferred):
  Source        : https://github.com/toml-rs/toml
  Tag / commit  : <to be pinned; record date + 1-line note on which
                  version is being ported>
  Pinned        : <date>
  Re-import procedure: see hack/import-toml-rs/README.md (also TBD)
```

This pin was scheduled for Pre-Phase-1 step 4 (Architect risk-gap G4)
but never landed before Phase 1 kickoff. Phase 1's scan-kernel work
does NOT depend on toml-rs source — the SWAR + SIMD kernels are
implemented from first principles against TOML's lexical spec — so
the deferral is harmless for Phase 1 but must be resolved before any
parser-level (Phase 2) or facade-level (Phase 4) work begins, since
those phases consume toml-rs `tests/data/` for the golden corpus.

## Cargo.lock corpus (`pkg/toml/testdata/corpus/cargo.lock`)

```
TODO (Pre-Phase-1 step 3, deferred):
  Source         : <upstream URL, e.g.
                    https://github.com/rust-lang/cargo/raw/<sha>/Cargo.lock>
  Upstream ref   : <commit SHA>
  SHA-256        : <to be recorded at procurement>
  Byte count     : <to be recorded; constraint 100 KB ≤ size ≤ 110 KB>
  Pinned         : <date>
```

Phase 1 does NOT consume the Cargo.lock corpus. Phase 4 (AC-FAC-6
gate) is the first consumer, and Phase 5 (AC-EDIT-6 gate) is the
second. T5's perf benchmarks
(`pkg/toml/internal/scan/bench_test.go`) build their 64 KiB buffers
from a deterministic seeded PRNG and do NOT depend on this corpus.

## toml-test compliance suite

```
DEFERRED to Phase 4 (Q2 ruling 2026-05-16):
  Source         : https://github.com/toml-lang/toml-test
  Commit         : <to be pinned at Phase 4 kickoff>
  Pinned         : <date>
  Controversial cases this port matches: <to be enumerated at Phase 4>
  Re-import procedure: see hack/import-toml-test/README.md (Phase 4 deliverable)
```

Phase 1 does not import or consume the toml-test compliance suite.
This pin lands as part of Phase 4's facade work (AC-FAC-9 — all 12
toml-test groups pass).

## Phase 2.5 trajectory

```
Phase 2.5 (perf smoketest) has not yet executed.
Outcome will be appended here once `pkg/toml/internal/smoketest/`
ships its smoketest_thru / burntsushi_thru ratio per the plan's
Phase 2.5 decision rule.
```

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

```
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
