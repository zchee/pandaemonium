# internal/memchr

`internal/memchr` is a pure-Go byte-search package modeled on the byte-search
subset of BurntSushi's Rust [`memchr`](https://github.com/BurntSushi/memchr)
crate. It provides fast single-, double-, and triple-needle searches in forward
and reverse directions, plus `bytes`-style wrapper names for call sites that
prefer haystack-first argument order.

The package is internal to this module. It is not a general-purpose dependency
surface for external modules, and it should not grow beyond the byte-search
subset documented here without a separate design and benchmark review.

## API surface

All functions return the byte offset of the match or `-1` when no byte matches,
matching `bytes.IndexByte` / `bytes.LastIndexByte` sentinel behavior.

| Shape | Functions | Argument order | Result |
| --- | --- | --- | --- |
| Canonical forward scan | `Memchr`, `Memchr2`, `Memchr3` | needle(s), haystack | first matching offset |
| Canonical reverse scan | `Memrchr`, `Memrchr2`, `Memrchr3` | needle(s), haystack | last matching offset |
| `bytes`-style forward wrappers | `IndexByte`, `IndexByte2`, `IndexByte3` | haystack, needle(s) | first matching offset |
| `bytes`-style reverse wrappers | `LastIndexByte`, `LastIndexByte2`, `LastIndexByte3` | haystack, needle(s) | last matching offset |

Example:

```go
package example

import "github.com/zchee/pandaemonium/internal/memchr"

func findTerminator(buf []byte) int {
	return memchr.IndexByte2(buf, '\n', '\r')
}
```

Wrappers in `api.go` are intentionally one-line calls through package-private
shims. `GOAMD64=v3/v4` artifacts bind those shims directly to their AVX2 or
AVX-512 routines, while legacy and portable tuples keep the same dispatcher
shape through package-level function pointers.

## Backends and build tags

Dispatch is build-tag-first. amd64 SIMD uses separate `GOAMD64` artifact lanes:
`GOAMD64=v4` binds the AVX-512 primary artifact for all current routines,
`GOAMD64=v3` binds the AVX2 fallback, and legacy v1/v2 builds keep one runtime
CPU decision through `simd/archsimd`. The v4 public shims use AVX-512
single-needle assembly plus `simd/archsimd` Int8x64 multi-needle paths so the
compiler can inline the hot wrappers and hoist constant needle broadcasts.

| GOARCH / level | `goexperiment.simd` | `force_swar` | Backend | Binding file |
| --- | --- | --- | --- | --- |
| `amd64`, `GOAMD64=v4` | on | off | AVX-512 primary artifact | `memchr_amd64_v4.go`, `memchr_amd64_v4_archsimd.go`, `memchr_amd64_v4.s` |
| `amd64`, `GOAMD64=v3` | on | off | AVX2 fallback artifact | `memchr_amd64_v3.go`, `memchr_amd64_avx2.s` |
| `amd64`, `GOAMD64=v1/v2` | on | off | AVX2 when `archsimd.X86.AVX2()` is true, otherwise SSE2 | `memchr_amd64_legacy.go`, `memchr_amd64.go`, `memchr_amd64_avx2.s` |
| `amd64` | off | off | SWAR | `dispatch_swar_default.go` |
| `amd64` | any | on | SWAR | `dispatch_swar_default.go` |
| `arm64` | any | off | NEON | `memchr_arm64.go` and `*_arm64.s` |
| `arm64` | any | on | SWAR | `dispatch_swar_default.go` |
| other | any | any | SWAR | `dispatch_swar_default.go` |

The invariant is that every supported `(GOARCH, goexperiment.simd, force_swar)`
tuple has exactly one file that binds each dispatched `*Impl` function pointer.
`TestBackendBinding` checks the selected backend through the package-private
`boundImpl` marker and per-function `boundMemchrImpl`-style markers. The
per-function markers prove that the v4 artifact has AVX-512 implementations
bound for every routine and that v3 remains the AVX2 fallback artifact.

### amd64 CPU detection and artifact selection

Use `simd/archsimd` for local artifact preflight: `archsimd.X86.AVX512()` gates
execution or recommendation of the `GOAMD64=v4` artifact, and
`archsimd.X86.AVX2()` gates the `GOAMD64=v3` fallback on local hardware. Do not
use `golang.org/x/sys/cpu` for this package decision. A `GOAMD64=v4` binary
cannot runtime-fallback on a v3-only CPU; fallback means building a separate
`GOAMD64=v3` artifact.

For legacy amd64 v1/v2 SIMD builds, `memchr_amd64_legacy.go` chooses AVX2 or
SSE2 at `init` time through `archsimd.X86.AVX2()`. If a runner sets
`GODEBUG=cpu.avx2=off`, AVX2-capable hardware is deliberately downgraded to
SSE2. `TestBackendBinding` exists to catch accidental silent downgrades in CI
before performance-gate results are trusted.

### `force_swar`

Use `-tags=force_swar` only for tests, backend isolation, and profile
comparison. It intentionally bypasses amd64 SIMD and arm64 NEON bindings so the
portable SWAR backend can be exercised on hardware that would normally use a
faster backend.

## Non-goals

Do not add these without a new plan and benchmark evidence:

- substring search / `memmem`;
- upstream Rust iterator or stateful `Memchr*` types;
- Rabin-Karp, Two-Way, Shift-Or, packed-pair, or other `arch::all` algorithms;
- cgo, AVO code generation, wasm32 SIMD, or adding new AVX-512 routine shapes
  beyond the current byte-search API without a new benchmark-backed plan;
- production behavior gated behind the test-only `force_swar` tag.

## Correctness tests

Run the normal package suite from the repository root:

```sh
go test ./internal/memchr
```

Useful focused checks:

```sh
go test ./internal/memchr -run 'TestGoldenCorpus|TestBackendBinding|TestZeroAllocs'
go test -tags=force_swar ./internal/memchr
GOEXPERIMENT=nosimd go test ./internal/memchr
```

The suite layers multiple oracles:

- `golden_test.go` reads `testdata/golden_corpus.json`, generated from the
  upstream Rust crate.
- `naive_scan_test.go`, `property_test.go`, `exhaustive_test.go`, and fuzz
  targets compare public functions against byte-by-byte same-package oracles.
- `allocs_test.go` asserts zero allocations across representative sizes.
- `dispatch_binding_*_test.go` plus `dispatch_test.go` verify the aggregate and
  per-function backend markers bound for each build-tag tuple.
- `sse2_test.go`, `avx2_test.go`, `avx512_test.go`, and the arm64 assembly
  bindings exercise backend-specific edge cases when those files compile and the
  CPU supports the instructions.

## Golden corpus maintenance

The committed corpus lives at:

```text
internal/memchr/testdata/golden_corpus.json
```

It is generated by the Rust shim under `hack/extract-memchr-corpus/`, which pins
upstream `memchr` with exact equality. Regenerate from the repository root:

```sh
(cd hack/extract-memchr-corpus && cargo run --release) \
    > internal/memchr/testdata/golden_corpus.json
```

When bumping the upstream corpus version, commit these together:

- `hack/extract-memchr-corpus/Cargo.toml`;
- `hack/extract-memchr-corpus/Cargo.lock`;
- `internal/memchr/testdata/golden_corpus.json`.

The Rust shim is not on the Go build path. Normal Go tests consume only the
committed JSON fixture.

## Benchmarks and performance gate

Run the benchmark suite directly when investigating local changes:

```sh
GOAMD64=v4 go test -bench=. -benchmem -run=^$ ./internal/memchr
GOAMD64=v3 go test -bench=. -benchmem -run=^$ ./internal/memchr
```

The standard-library baseline is `BenchmarkIndexByteStd`; the package benchmarks
cover `Memchr*` and `Memrchr*` at representative haystack sizes.

Dense threshold, hit-position, public-vs-direct, and mixed-workload experiments
live under the `BenchmarkTuning...` namespace behind the `memchr_tuning` build
tag:

```sh
GOAMD64=v4 go test -tags=memchr_tuning -bench='BenchmarkTuning' -benchmem -run=^$ ./internal/memchr
```

Keep these exploratory rows out of default CI and perf-gate binaries: the stable
`BenchmarkMemchr*` and `BenchmarkIndexByteStd` names remain the hard-gate
surface, while `BenchmarkTuning...` rows are for opt-in local/evaluator
analysis.

The CI-oriented perf gate lives under `hack/memchr-perf-gate/`:

```sh
go run ./hack/memchr-perf-gate
go run ./hack/memchr-perf-gate --compare-artifacts=v3,v4
```

It compares `BenchmarkMemchr` against `BenchmarkIndexByteStd` with `benchstat`
at `alpha=0.05`. On amd64, it uses `simd/archsimd` to benchmark the v4 artifact
when AVX-512 is available and the v3 fallback when AVX2 is the widest local
feature. Statistically significant slowdowns at gated sizes `n >= 64` fail the
stdlib baseline gate. Artifact comparisons use `GOAMD64=v3` as the baseline and
`GOAMD64=v4` as the treatment; hard rows at `n >= 1024` fail only when the
slowdown is statistically significant and at least the practical threshold
configured in the perf-gate tool. Threshold rows (`n=64`, `n=128`, `n=256`) and
`BenchmarkTuning...` rows are reported/classified separately from the default
hard gate. The tool prefers a `benchstat` binary on `PATH` and falls back to the
module tool directive through `go tool benchstat` when needed. On arm64 and
other architectures, the gate currently reports the tuple as untested and exits
successfully until a matching CI runner is provisioned.

Before changing assembly files, format them with:

```sh
go tool asmfmt -w internal/memchr/*.s
```

## Adoption guidance

Use this package only after checking the target workload. Replacing a local
scanner with `internal/memchr` is not automatically a win: benchmark the exact
call site, include the relevant backend tuple, and keep the existing scanner
when local measurements are faster or simpler.
