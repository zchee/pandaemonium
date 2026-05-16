# Phase 2.5 Benchmark Evidence (Worker 2)

This artifact records the raw local benchmark evidence collected for the
Phase 2.5 parser+scan throughput trajectory lane. It intentionally does
not edit `pkg/toml/UPSTREAM.md`; the documentation lane owns the final
UPSTREAM outcome wording.

## Scope and caveats

- Worker lane: lane 2 benchmark protocol and raw evidence.
- Repository HEAD: `5ce34e4e67c0b803c3b35f9bf059d803f82e7061`.
- Host: `darwin/arm64`, Apple M3 Max.
- Corpus: `pkg/toml/testdata/corpus/cargo.lock`.
- Corpus SHA-256: `9ea94b60b3ee80c73f52186946bb280dc41c7287bbb678988618a6839533dbe9`.
- Corpus bytes: `103263`.
- Final Phase 2.5 decision ratio against BurntSushi is **not** recorded
  here because this worktree did not yet contain a committed
  `BenchmarkSmoketestUnmarshal` / BurntSushi comparison harness when
  the evidence was collected. The raw parser-token benchmark and a
  force-SWAR comparison are retained as ratio inputs and protocol proof.

## Bench protocol used

```bash
GOMAXPROCS=1
GODEBUG=
GOEXPERIMENT=loopvar,newinliner,jsonv2,greenteagc,simd,randomizedheapbase64,sizespecializedmalloc,runtimesecret,mapsplitgroup
GOTOOLCHAIN=local
/opt/local/go.simd/bin/go test -tags=bench \
  -bench='^BenchmarkDecoderTokens_CargoLock$' \
  -count=10 \
  -cpu=1 \
  -benchtime=5s \
  -benchmem \
  -run=^$ \
  ./pkg/toml
```

The force-SWAR comparison used the same command with `-tags='bench force_swar'`.

## Preflight

```text
=== command: /opt/local/go.simd/bin/go env GOOS GOARCH GOEXPERIMENT GOTOOLCHAIN ===
darwin
arm64
loopvar,newinliner,jsonv2,greenteagc,simd,randomizedheapbase64,sizespecializedmalloc,runtimesecret,mapsplitgroup
local
=== command: /opt/local/go.simd/bin/go version ===
go version go1.27-devel_b1972f9085 Sun May 17 03:53:30 2026 +0900 X:loopvar,newinliner,jsonv2,sizespecializedmalloc,simd,runtimesecret,mapsplitgroup darwin/arm64
=== command: shasum -a 256 pkg/toml/testdata/corpus/cargo.lock && wc -c ===
9ea94b60b3ee80c73f52186946bb280dc41c7287bbb678988618a6839533dbe9  pkg/toml/testdata/corpus/cargo.lock
103263 pkg/toml/testdata/corpus/cargo.lock
=== command: git rev-parse HEAD ===
5ce34e4e67c0b803c3b35f9bf059d803f82e7061
=== command: git status --short --branch ===
## HEAD (no branch)
```

## Raw candidate output: SIMD-enabled parser token benchmark

```text
=== command: GOMAXPROCS=1 GODEBUG= GOEXPERIMENT=... GOTOOLCHAIN=local /opt/local/go.simd/bin/go test -tags=bench -bench=^BenchmarkDecoderTokens_CargoLock$ -count=10 -cpu=1 -benchtime=5s -benchmem -run=^$ ./pkg/toml ===
goos: darwin
goarch: arm64
pkg: github.com/zchee/pandaemonium/pkg/toml
cpu: Apple M3 Max
BenchmarkDecoderTokens_CargoLock 	     193	  26377102 ns/op	   3.91 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     195	  34318170 ns/op	   3.01 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     243	  23779150 ns/op	   4.34 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     256	  21432396 ns/op	   4.82 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     283	  19946431 ns/op	   5.18 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     273	  22970601 ns/op	   4.50 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     308	  20844370 ns/op	   4.95 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     292	  21089445 ns/op	   4.90 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     236	  22418628 ns/op	   4.61 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     277	  20355551 ns/op	   5.07 MB/s	332874656 B/op	    5802 allocs/op
PASS
ok  	github.com/zchee/pandaemonium/pkg/toml	58.712s
```

## Raw baseline output: force-SWAR parser token benchmark

```text
=== command: GOMAXPROCS=1 GODEBUG= GOEXPERIMENT=... GOTOOLCHAIN=local /opt/local/go.simd/bin/go test -tags="bench force_swar" -bench=^BenchmarkDecoderTokens_CargoLock$ -count=10 -cpu=1 -benchtime=5s -benchmem -run=^$ ./pkg/toml ===
goos: darwin
goarch: arm64
pkg: github.com/zchee/pandaemonium/pkg/toml
cpu: Apple M3 Max
BenchmarkDecoderTokens_CargoLock 	     278	  19975769 ns/op	   5.17 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     326	  18224203 ns/op	   5.67 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     300	  20060525 ns/op	   5.15 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     249	  21380162 ns/op	   4.83 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     262	  21517019 ns/op	   4.80 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     253	  32859468 ns/op	   3.14 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     256	  27908009 ns/op	   3.70 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     206	  34553910 ns/op	   2.99 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     247	  25492691 ns/op	   4.05 MB/s	332874656 B/op	    5802 allocs/op
BenchmarkDecoderTokens_CargoLock 	     100	  54254186 ns/op	   1.90 MB/s	332874656 B/op	    5802 allocs/op
PASS
ok  	github.com/zchee/pandaemonium/pkg/toml	63.031s
```

## Ratio input summary: force-SWAR vs SIMD-enabled parser tokens

```text
=== command: benchstat -alpha=0.05 parser-force-swar.bench.txt parser-simd.bench.txt ===
goos: darwin
goarch: arm64
pkg: github.com/zchee/pandaemonium/pkg/toml
cpu: Apple M3 Max
                        │ /tmp/worker2-task3/parser-force-swar.bench.txt │ /tmp/worker2-task3/parser-simd.bench.txt │
                        │                     sec/op                     │         sec/op          vs base          │
DecoderTokens_CargoLock                                     23.50m ± 47%             21.93m ± 20%  ~ (p=0.631 n=10)

                        │ /tmp/worker2-task3/parser-force-swar.bench.txt │ /tmp/worker2-task3/parser-simd.bench.txt │
                        │                      B/s                       │          B/s            vs base          │
DecoderTokens_CargoLock                                    4.220Mi ± 32%            4.497Mi ± 17%  ~ (p=0.631 n=10)

                        │ /tmp/worker2-task3/parser-force-swar.bench.txt │ /tmp/worker2-task3/parser-simd.bench.txt │
                        │                      B/op                      │         B/op          vs base            │
DecoderTokens_CargoLock                                     317.5Mi ± 0%           317.5Mi ± 0%  ~ (p=1.000 n=10) ¹
¹ all samples are equal

                        │ /tmp/worker2-task3/parser-force-swar.bench.txt │ /tmp/worker2-task3/parser-simd.bench.txt │
                        │                   allocs/op                    │      allocs/op        vs base            │
DecoderTokens_CargoLock                                      5.802k ± 0%            5.802k ± 0%  ~ (p=1.000 n=10) ¹
¹ all samples are equal
```

## Evidence disposition

The parser token benchmark completed successfully under the required
bench protocol. The SIMD-enabled median was not statistically different
from force-SWAR on this high-allocation parser-token path (`p=0.631`,
`n=10`), which is expected to be only an informational trajectory input
until the Phase 2.5 smoketest harness records the required BurntSushi
comparison ratio.


## Worker 4 final Phase 2.5 gate and remediation

Worker 4 found that the initial parser-token benchmark was dominated by
allocation in `Decoder.scanString`: a CPU/memory profile on
`BenchmarkDecoderTokens_CargoLock` showed `scanString` accounting for
nearly all allocation space, with the local benchmark reporting
`332875089 B/op` and `5802 allocs/op`. The remediation replaced
whole-suffix string conversion for triple-quote prefix checks with a
byte-prefix check. After that fix, the same local parser-token
benchmark reported `303794 ns/op`, `339.91 MB/s`, `700 B/op`, and
`20 allocs/op` in the quick check.

Final protocol command:

```text
GOTOOLCHAIN=local PATH=/opt/local/go.simd/bin:$PATH \
  /opt/local/go.simd/bin/go run ./hack/toml-perf-gate \
  --kind=parser --ratio=0.5 --count=10 --benchtime=5s --cpu=1
```

Final benchstat output:

```text
goos: darwin
goarch: arm64
pkg: github.com/zchee/pandaemonium/pkg/toml/internal/smoketest
cpu: Apple M3 Max
                   │ base.txt │ candidate.txt │
                   │  sec/op  │    sec/op     vs base                │
SmoketestUnmarshal    2.949m ± 1%   1.045m ± 4%  -64.56% (p=0.000 n=10)

                   │  B/s     │     B/s       vs base                │
SmoketestUnmarshal   33.39Mi ± 1%  94.24Mi ± 3%  +182.21% (p=0.000 n=10)

                   │  B/op    │     B/op      vs base                │
SmoketestUnmarshal  1508.0Ki ± 0%  388.5Ki ± 0%  -74.24% (p=0.000 n=10)

                   │ allocs/op │ allocs/op    vs base                │
SmoketestUnmarshal   25.37k ± 0%   12.18k ± 0%  -51.99% (p=0.000 n=10)

toml-perf-gate: PASS parser point=2.822x lower95=2.822x threshold=0.500x p=0.000 n=10
```

Disposition: Phase 2.5 trajectory passes the documented `>= 0.5x`
proceed rule. Proceed to Phase 3 is allowed; do not start Phase 4 until
`pkg/toml/internal/smoketest/` is deleted or converted into the real
facade benchmark surface.
