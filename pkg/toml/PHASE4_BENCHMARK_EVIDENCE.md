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

## Worker 2 facade allocation follow-up

Follow-up commit `c4a7e9d` keeps the facade on the existing token-stream plus
reflection-cache architecture and reduces allocation churn before revisiting the
Pelletier gate. The changes cache case-insensitive field lookups, skip ignored
struct keys before value materialization, recycle non-escaping document maps for
struct-only decodes, and preserve nested `TypeMismatchError.Path` diagnostics.

Focused verification:

```text
=== command: go test -race -count=1 -shuffle=on ./pkg/toml/... ===
ok github.com/zchee/pandaemonium/pkg/toml 1.627s
ok github.com/zchee/pandaemonium/pkg/toml/internal/reflectcache 1.015s
ok github.com/zchee/pandaemonium/pkg/toml/internal/scan 5.519s
```

Focused facade benchmark after the allocation pass:

```text
=== command: go test -tags=bench -run='^$' -bench=^BenchmarkUnmarshal_Pandaemonium$ -benchmem -count=1 -cpu=1 -benchtime=5s -memprofile=/tmp/worker2-task2-final/pandaemonium.mem.pprof ./pkg/toml ===
BenchmarkUnmarshal_Pandaemonium  9735  637582 ns/op  161.96 MB/s  147966 B/op  5800 allocs/op
PASS
```

Compared with the earlier recorded profile sample (`986985 ns/op`,
`497830 B/op`, `15526 allocs/op`), this lowers latency by roughly 35%, bytes by
roughly 70%, and allocation count by roughly 63% for the Pandaemonium facade on
the pinned Cargo.lock corpus.

Allocation profile after the pass:

```text
parseStringValue      565.03MB 40.10% flat
reflect.unsafe_NewArray 250.91MB 17.81% flat
parseValueToken       812.53MB 57.66% cum
appendArrayTableKey   168.48MB 11.96% cum
parseDocument        1136.02MB 80.62% cum
bindValue             250.91MB 17.81% cum
```

Exception status: keep the Pelletier facade gate blocked for now. Even after the
allocation pass, the focused Pandaemonium sample is still slower than the
recorded Pelletier range (`0.549-0.563 ms/op`), so the configured
Pandaemonium/Pelletier lower-bound ratio of `1.300x` remains infeasible without
a larger architecture change such as direct streaming binds that avoid
materializing value strings and intermediate table maps. The remaining profile
is now dominated by required string materialization for retained output values
and destination slice allocation rather than dependency-harness noise.

Final official facade gate run after the allocation pass:

```text
=== command: go run ./hack/toml-perf-gate --kind=facade --benchstat=$HOME/go/bin/benchstat ===
toml-perf-gate: PASS facade/burntsushi point=4.507x lower95=4.507x threshold=1.500x p=0.000 n=10
toml-perf-gate: FAIL facade/pelletier point=0.890x lower95=0.890x threshold=1.300x p=0.000 n=10 reason=lower95=0.8897x < threshold=1.3000x (point=0.8897x)
facade_perf_gate_exit=1
```

The final gate confirms the narrowed exception: the BurntSushi requirement now
passes with margin, while the Pelletier requirement remains a true architecture
threshold miss rather than a statistical-noise failure.

Step 8 final-integration rerun:

```text
=== command: GOTOOLCHAIN=local /opt/local/go.simd/bin/go run ./hack/toml-perf-gate --kind=facade --ratio-burntsushi=1.5 --ratio-pelletier=1.3 ===
toml-perf-gate: PASS facade/burntsushi point=4.331x lower95=4.331x threshold=1.500x p=0.000 n=10
toml-perf-gate: FAIL facade/pelletier point=0.890x lower95=0.890x threshold=1.300x p=0.000 n=10 reason=lower95=0.8903x < threshold=1.3000x (point=0.8903x)
facade_perf_gate_exit=1
```

The Step 8 rerun keeps the same disposition: the BurntSushi facade gate
passes, and the Pelletier gate remains a documented architecture
exception until a future streaming-bind design removes the intermediate
value materialization and table-map binding work.

## Edit perf-gate evidence

The edit-path gate compares Pandaemonium's format-preserving Document edit path
against a pelletier full unmarshal/mutate/marshal edit path on the same pinned
Cargo.lock corpus. The local gate below uses the Phase 5 `--ratio-edit=0.25`
threshold and exits 0 with a PASS verdict.

```text
=== command: GOTOOLCHAIN=local /opt/local/go.simd/bin/go run ./hack/toml-perf-gate --kind=edit --ratio-edit=0.25 --benchstat=$HOME/go/bin/benchstat --count=10 --benchtime=1s --cpu=1 ===

# edit: Pandaemonium Document vs pelletier full edit
goos: darwin
goarch: arm64
pkg: github.com/zchee/pandaemonium/pkg/toml
cpu: Apple M3 Max
DocumentEdit  sec/op    939.0µ ± 2%  895.3µ ± 1%  -4.65% (p=0.000 n=10)
DocumentEdit  B/s       104.9Mi ± 2%  110.0Mi ± 1%  +4.88% (p=0.000 n=10)
DocumentEdit  B/op      936.4Ki ± 0%  851.5Ki ± 0%  -9.06% (p=0.000 n=10)
DocumentEdit  allocs/op 7.638k ± 0%  14.878k ± 0%  +94.79% (p=0.000 n=10)
toml-perf-gate: PASS edit/pelletier point=1.049x lower95=1.049x threshold=0.250x p=0.000 n=10
edit_perf_gate_exit=0
```

## Document edit benchmark evidence

`BenchmarkDocumentEdit` exercises the format-preserving Document API edit path
on the pinned Cargo.lock corpus: parse the document, replace the existing
top-level `version` value, insert a new dotted key after it, and serialize the
edited bytes. The bench-tag run also includes the explicit Pandaemonium and
pelletier comparator names consumed by `hack/toml-perf-gate --kind=edit`.

```text
=== command: GOTOOLCHAIN=local /opt/local/go.simd/bin/go test -tags=bench -run='^$' -bench=BenchmarkDocumentEdit -benchmem -count=10 -cpu=1 -benchtime=5s ./pkg/toml/ ===
goos: darwin
goarch: arm64
pkg: github.com/zchee/pandaemonium/pkg/toml
cpu: Apple M3 Max
BenchmarkDocumentEdit               6208  928199 ns/op  111.25 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit               6510  912845 ns/op  113.12 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit               6374  916387 ns/op  112.68 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit               6602  922570 ns/op  111.93 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit               6540  950600 ns/op  108.63 MB/s  871970 B/op  14878 allocs/op
BenchmarkDocumentEdit               6584  916351 ns/op  112.69 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit               6615  924723 ns/op  111.67 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit               6788  910739 ns/op  113.38 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit               6742  930108 ns/op  111.02 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit               6501  912744 ns/op  113.13 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pandaemonium  6591  922606 ns/op  111.93 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pandaemonium  6440  916314 ns/op  112.69 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pandaemonium  6624  920644 ns/op  112.16 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pandaemonium  6471  932335 ns/op  110.76 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pandaemonium  6298  924959 ns/op  111.64 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pandaemonium  6531  918356 ns/op  112.44 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pandaemonium  6708  921950 ns/op  112.00 MB/s  871970 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pandaemonium  6352  920395 ns/op  112.19 MB/s  871970 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pandaemonium  6682  953580 ns/op  108.29 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pandaemonium  6630  913190 ns/op  113.08 MB/s  871971 B/op  14878 allocs/op
BenchmarkDocumentEdit_Pelletier     5976  962973 ns/op  107.23 MB/s  958875 B/op   7638 allocs/op
BenchmarkDocumentEdit_Pelletier     5805  991515 ns/op  104.15 MB/s  958872 B/op   7638 allocs/op
BenchmarkDocumentEdit_Pelletier     5956  965732 ns/op  106.93 MB/s  958872 B/op   7638 allocs/op
BenchmarkDocumentEdit_Pelletier     6337  969496 ns/op  106.51 MB/s  958872 B/op   7638 allocs/op
BenchmarkDocumentEdit_Pelletier     6316  967750 ns/op  106.70 MB/s  958872 B/op   7638 allocs/op
BenchmarkDocumentEdit_Pelletier     6259  973500 ns/op  106.07 MB/s  958872 B/op   7638 allocs/op
BenchmarkDocumentEdit_Pelletier     6277  976295 ns/op  105.77 MB/s  958872 B/op   7638 allocs/op
BenchmarkDocumentEdit_Pelletier     6165  966898 ns/op  106.80 MB/s  958872 B/op   7638 allocs/op
BenchmarkDocumentEdit_Pelletier     6224  984169 ns/op  104.92 MB/s  958872 B/op   7638 allocs/op
BenchmarkDocumentEdit_Pelletier     6080  971115 ns/op  106.33 MB/s  958872 B/op   7638 allocs/op
PASS
ok github.com/zchee/pandaemonium/pkg/toml 180.531s
```
