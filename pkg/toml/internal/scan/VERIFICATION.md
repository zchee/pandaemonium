# `pkg/toml/internal/scan` Verification Runbook

This document is the canonical reproducible runbook for verifying that
the Phase 1 scan-kernel package satisfies every AC-SIMD-1..8 gate. The
commands here are the exact ones the AC-SIMD-5 perf gate, the
CI matrix in `.github/workflows/toml-scan-ci.yml`, and the
PHASE1_CLOSEOUT.md audit table all reproduce.

Run order matters: the build/vet/format checks come first because they
gate everything else, then correctness tests (vanilla + force_swar
backends), then fuzz, then perf. The full suite should complete in
roughly 7–8 minutes on a c6i-class amd64 runner with AVX2 enabled, or
~5–6 minutes on an Apple-Silicon M-series host running the NEON path.

## Environment

```bash
# Toolchain (per pkg/toml/UPSTREAM.md)
PATH_add /opt/local/go.simd/bin
export GOTOOLCHAIN=auto
export GOEXPERIMENT='loopvar,newinliner,jsonv2,greenteagc,simd,randomizedheapbase64,sizespecializedmalloc,runtimesecret,mapsplitgroup'

# Bench protocol (plan §Cross-cutting > Bench protocol, line 843+)
export GOMAXPROCS=1
export GODEBUG=''
```

If the `/opt/local/go.simd` snapshot stdlib bug
(`internal/runtime/maps/memhash_aes_simd.go` signature drift) blocks a
local cross-arch verification, install `golang.org/dl/gotip` and
substitute `gotip` for `go` in the build/vet steps below. The CI
workflows use this substitution automatically for the amd64+simd cells.

## Phase 1 verification commands (from plan line 1055+)

Run each of the 7 commands in order. Each must succeed (exit 0). The
first failure aborts the suite — fix it before continuing because
later commands may compile-cascade-fail on the same underlying issue.

```bash
# 1. Build
go build ./pkg/toml/internal/scan/

# 2. Vet (standard)
go vet ./pkg/toml/internal/scan/

# 3. Vet -unsafeptr (AC-SIMD-8: scan_swar.go uses unsafe.Pointer for
#    64-bit word loads — this check is mandatory because we cast
#    *byte → *uint64).
go vet -unsafeptr ./pkg/toml/internal/scan/

# 4. Format (both must produce no output)
test -z "$(gofmt -s -d ./pkg/toml/internal/scan/)"
test -z "$(gofumpt -l ./pkg/toml/internal/scan/)"

# 5. Correctness (default backend: NEON on arm64, AVX2/SSE2 on
#    amd64+simd, SWAR otherwise; -shuffle=on randomizes intra-run
#    test order to surface order-dependence bugs).
go test -race -count=1 -shuffle=on ./pkg/toml/internal/scan/

# 6. Correctness (SWAR-only path forced; AC-SIMD-7 enforcement
#    on a host whose default backend is NOT SWAR).
go test -tags=force_swar -race -count=1 ./pkg/toml/internal/scan/

# 7. Fuzz (one target per scan; 60s budget each; surfaces inputs
#    where the dispatched scan disagrees with the naive oracle).
for s in ScanBareKey ScanBasicString ScanLiteralString \
         SkipWhitespace LocateNewline ValidateUTF8; do
    go test -run=^$ -fuzz=Fuzz$s -fuzztime=60s \
        ./pkg/toml/internal/scan/ || exit 1
done
```

## AC-SIMD-5 perf-gate commands (per-scan)

These are the perf gates that hard-fail PRs that regress a scan's
SIMD-vs-baseline throughput ratio. Run on real amd64+AVX2 hardware
for the amd64 cells; on `ubuntu-24.04-arm` (Q6 ruling) for the arm64
cells.

```bash
# All 6 scans. Default --ratio=1.0 ; tighten via --ratio=1.05 etc.
# if you want a stricter margin. --count=10 and --benchtime=5s match
# the Bench protocol exactly.
for s in LocateNewline ScanLiteralString ScanBareKey \
         ScanBasicString SkipWhitespace ValidateUTF8; do
    go run ./hack/toml-perf-gate \
        --kind=scan --scan="$s" \
        --ratio=1.0 --count=10 --benchtime=5s --cpu=1 \
    || exit 1
done
```

CI also exposes `--kind=facade` and `--kind=edit` stubs but those
are no-ops in Phase 1 (Phase 4 / Phase 5 will implement them).

## Cross-cutting gates (plan §Cross-cutting verification, line 1101+)

These checks aren't scan-specific but every Phase 1 closeout should
re-run them to catch repo-wide regressions the focused commands
above miss.

```bash
# Apache-2.0 header on every source file
! grep -rL 'Apache License' pkg/toml --include='*.go' --include='*.s' | grep .

# Production graph clean (no BurntSushi/pelletier in non-bench deps).
# This is a partial check until Phase 2/4 land — Phase 1's scan
# package only depends on stdlib + simd/archsimd, so the assertion
# is trivially satisfied today.
! go list -deps ./pkg/toml/internal/scan/ | grep -qE 'BurntSushi|pelletier'

# archsimd isolation: blast radius is one file (T7 may add a .s
# file under the same import; the count remains 1 since .s files
# don't import archsimd themselves).
test "$(grep -rl 'simd/archsimd' pkg/toml | wc -l | tr -d ' ')" = '1'

# Repeated go vet from §Phase 1 step 2 above; cross-cutting variant
# at the package-tree level rather than the scan-only subset.
go vet ./pkg/toml/...

# Toolchain identity assertion (Critic CC5). Requires a
# .toolchain-pin file landed at Pre-Phase-1 step 1; currently
# deferred per UPSTREAM.md — this assertion is SKIPPED until that
# file exists.
test -f .toolchain-pin && go version -m "$(which go)" | grep -q "$(cat .toolchain-pin)"
```

## Cross-arch matrix (the CI version)

The `.github/workflows/toml-scan-ci.yml` workflow runs the equivalent
of commands 1–6 above on:

- `linux/amd64` + `GOEXPERIMENT=...,simd,...` via gotip
  (`continue-on-error: true` until the snapshot stdlib bug
  propagates upstream — see UPSTREAM.md);
- `linux/amd64` without `goexperiment.simd` via stable Go (SWAR
  fallback on amd64);
- `linux/arm64` via gotip + GOEXPERIMENT (NEON path; Q6 runner);
- `linux/arm64` without `goexperiment.simd` via stable Go (NEON
  again, since NEON is ABI-guaranteed on arm64);
- `wasip1/wasm` build-only via stable Go (SWAR-only target);
- `-tags=force_swar` on ubuntu-24.04 via stable Go (AC-SIMD-7).

The perf-gate workflow `.github/workflows/toml-perf-gate.yml` runs the
six `go run ./hack/toml-perf-gate --kind=scan --scan=Xxx` invocations
on `ubuntu-24.04` (amd64) and `ubuntu-24.04-arm` (arm64). The trigger
is `workflow_dispatch`, `push` to main, or a PR labeled `perf-gate`.

## What "PASS" means per AC

| AC | What "PASS" requires |
|----|----------------------|
| AC-SIMD-1 | All 6 exported kernels exist in `api.go` and route through `scanBareKey/.../validateUTF8` dispatch vars. |
| AC-SIMD-2 | `scan_amd64.go`'s `init()` rebinds dispatch vars to `*AVX2` when `archsimd.X86.AVX2()` returns true; verified via gotip cross-arch vet + CI runtime evidence. |
| AC-SIMD-3 | `scan_arm64.go` statically binds dispatch vars to `*NEON`; `scan_arm64.s` assembly entry points compile via Plan 9 ARM64 syntax. |
| AC-SIMD-4 | `scan_swar.go` ships pure-Go SWAR for all 6 kernels; compiles on `!arm64 && (!amd64 || !goexperiment.simd)` and on `force_swar`. |
| AC-SIMD-5 | `hack/toml-perf-gate --kind=scan --scan=Xxx` exits 0 for every scan on amd64+AVX2 hardware AND on arm64 hardware. Lower-95% CI of the SIMD/baseline throughput ratio exceeds 1.0 per the Bench protocol's α=0.05 U-test. |
| AC-SIMD-6 | `naive_scan_test.go` oracle + `property_test.go` (100 K cases / scan) + `fuzz_test.go` (60s / scan) all pass with zero disagreement between the dispatched scan and the naive oracle. |
| AC-SIMD-7 | `dispatch_test.go` (+ per-arch siblings) forces each available variant (AVX2/SSE2/NEON/SWAR) through the dispatch path and re-runs the shared smoke fixture; every (variant, scan) pair PASS. |
| AC-SIMD-8 | `go vet -unsafeptr ./pkg/toml/internal/scan/` reports zero diagnostics. |

PHASE1_CLOSEOUT.md cites this table verbatim in its per-AC audit row.
