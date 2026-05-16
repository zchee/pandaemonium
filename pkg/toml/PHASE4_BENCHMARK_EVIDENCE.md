# Phase 4 Facade Benchmark Evidence (Worker 3)

This artifact records the post-integration Phase 4 facade benchmark and
competitor dependency evidence for OMX task 5. It intentionally does not
recreate or use the retired Phase 2.5 `pkg/toml/internal/smoketest`
harness.

## Scope and environment

- Worker lane: lane E post-integration facade benchmark and dependency
  hygiene.
- Repository HEAD: `9a58d0e3030b70cd6d7671c044e328c797b08006`.
- Host/toolchain: `darwin/arm64`, Apple M3 Max,
  `/opt/local/go.simd/bin/go`.
- Go version: `go1.27-devel_b1972f9085` with
  `X:loopvar,newinliner,jsonv2,sizespecializedmalloc,simd,runtimesecret,mapsplitgroup`.
- `GOEXPERIMENT`: `sizespecializedmalloc,runtimesecret`.
- `GOTOOLCHAIN`: `local`.
- Corpus: `pkg/toml/testdata/corpus/cargo.lock`.
- Corpus SHA-256: `9ea94b60b3ee80c73f52186946bb280dc41c7287bbb678988618a6839533dbe9`.
- Corpus bytes: `103263`.
- Vendor state: `vendor/` absent in this worker checkout, so no
  `vendor/modules.txt` sync was needed or produced.

## Dependency graph evidence

Production package dependencies exclude both bench-only competitors:

```text
=== command: go list -deps ./pkg/toml/... | rg competitors ===
<no output>
```

Normal test dependencies also exclude both competitors when the `bench`
tag is not enabled:

```text
=== command: go list -deps -test ./pkg/toml/... | rg competitors ===
<no output>
```

Bench-tag test dependencies include both competitors, proving the
competitor graph is isolated to bench builds:

```text
=== command: go list -deps -test -tags=bench ./pkg/toml/... | rg competitors ===
github.com/BurntSushi/toml/internal
github.com/BurntSushi/toml
github.com/pelletier/go-toml/v2/internal/characters
github.com/pelletier/go-toml/v2/unstable
github.com/pelletier/go-toml/v2/internal/tracker
github.com/pelletier/go-toml/v2
```

## Facade benchmark protocol

```text
GOMAXPROCS=1 GODEBUG= CGO_ENABLED=0 GOTOOLCHAIN=local \
  /opt/local/go.simd/bin/go test -tags=bench \
  -run='^$' -bench=BenchmarkUnmarshal -benchmem \
  -count=10 -cpu=1 -benchtime=5s ./pkg/toml
```

## Facade benchmark summary

```text
BenchmarkUnmarshal_BurntSushi    2.824-2.854 ms/op   36.18-36.56 MB/s   1424560-1424569 B/op   21719 allocs/op
BenchmarkUnmarshal_Pelletier     0.549-0.563 ms/op   183.26-187.96 MB/s 194440 B/op           2518 allocs/op
BenchmarkUnmarshal_Pandaemonium  0.979-1.006 ms/op   102.69-105.46 MB/s 497819-497820 B/op    15526 allocs/op
PASS
ok github.com/zchee/pandaemonium/pkg/toml 175.306s
```

Disposition:

- Pandaemonium is substantially faster than BurntSushi on this facade
  corpus.
- Pandaemonium is slower than Pelletier and allocates more. This is a
  real Phase 4 performance blocker for the configured Pelletier gate,
  not a dependency or harness failure.

## Facade perf-gate evidence

```text
=== command: go run ./hack/toml-perf-gate --kind=facade --benchstat=/Users/zchee/go/bin/benchstat ===

toml-perf-gate: PASS facade/burntsushi point=2.836x lower95=2.836x threshold=1.500x p=0.000 n=10

toml-perf-gate: FAIL facade/pelletier point=0.549x lower95=0.549x threshold=1.300x p=0.000 n=10 reason=lower95=0.5490x < threshold=1.3000x (point=0.5490x)
facade_perf_gate_exit=1
```

## Profile-backed Pelletier gate blocker

Focused profile command:

```text
GOMAXPROCS=1 GODEBUG= CGO_ENABLED=0 GOTOOLCHAIN=local \
  /opt/local/go.simd/bin/go test -tags=bench \
  -run='^$' -bench=^BenchmarkUnmarshal_Pandaemonium$ \
  -benchmem -count=1 -cpu=1 -benchtime=5s \
  -cpuprofile=/tmp/worker3-task5/pandaemonium.cpu.pprof \
  -memprofile=/tmp/worker3-task5/pandaemonium.mem.pprof \
  ./pkg/toml
```

Benchmark result:

```text
BenchmarkUnmarshal_Pandaemonium  6091  986985 ns/op  104.62 MB/s  497830 B/op  15526 allocs/op
PASS
```

CPU profile highlights:

```text
runtime.kevent                         3.18s  57.71% flat
runtime.madvise                        0.48s   8.71% flat
pkg/toml.(*Decoder).ReadToken          0.44s   7.99% cum
pkg/toml.bindValue                     0.38s   6.90% cum
pkg/toml.parseDocument                 1.36s  24.68% cum
pkg/toml.Unmarshal/unmarshalWithOptions 1.75s 31.76% cum
```

Allocation profile highlights (`-alloc_space`):

```text
pkg/toml.assign                         709.19MB 24.27%
pkg/toml.parseStringValue               477.03MB 16.33%
pkg/toml.parseDottedKey                 362.51MB 12.41%
pkg/toml.parseValueToken               1093.11MB 37.41% cum
pkg/toml.parseArrayValue                528.59MB 18.09% cum
pkg/toml.bindValue                      480.36MB 16.44% cum
pkg/toml.unmarshalWithOptions          2917.87MB 99.87% cum
```

Interpretation: the Pelletier gate failure is dominated by facade-level
allocation and binding/parsing overhead in Pandaemonium, especially
`assign`, string value parsing, dotted-key parsing, array parsing, and
`bindValue`. Future optimization should focus on reducing facade
allocation churn and binding work before raising or enforcing the
Pelletier ratio gate.
