# `pkg/toml`

`pkg/toml` is Pandaemonium's flat TOML package. It keeps one public package
while exposing four surfaces; parsing paths share the same scanner and parser,
and all surfaces share the same TOML value model:

- a streaming token API: `Decoder`, `Token`, and `TokenKind`;
- a facade API: `Marshal`, `Unmarshal`, `Encoder`, `MarshalOptions`, and
  `UnmarshalOptions`;
- TOML datetime types and parsers: `LocalDateTime`, `LocalDate`, `LocalTime`,
  `ParseDateTime`, and `ParseDateTimeAsTime`;
- a format-preserving edit API: `ParseDocument` and `Document`.

Internal scanner and reflection-cache packages stay under `pkg/toml/internal/`.
The approved package shape is intentionally small:

```text
github.com/zchee/pandaemonium/pkg/toml
github.com/zchee/pandaemonium/pkg/toml/internal/reflectcache
github.com/zchee/pandaemonium/pkg/toml/internal/scan
```

## Facade API

Use `Unmarshal` and `Marshal` for ordinary struct, map, scalar, array, and
TOML datetime values:

```go
package main

import "github.com/zchee/pandaemonium/pkg/toml"

type Config struct {
	Title  string `toml:"title"`
	Server struct {
		Port int `toml:"port"`
	} `toml:"server"`
}

func load(data []byte) (Config, error) {
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func save(cfg Config) ([]byte, error) {
	return toml.Marshal(cfg)
}
```

`Encoder` writes one TOML document to an `io.Writer`:

```go
enc := toml.NewEncoder(w)
err := enc.Encode(cfg)
```

Types can bypass reflection by implementing `MarshalerTo` or
`UnmarshalerFrom`. Struct tags use `toml:"name"` and `toml:"name,omitzero"`.
The package intentionally rejects `omitempty` in TOML tags with a typed error;
use `omitzero` for zero-value omission.

`WithDefaultOptions` returns explicit zero-value option structs for call sites
that want stable option values:

```go
marshalOptions, unmarshalOptions := toml.WithDefaultOptions()
out, err := marshalOptions.Marshal(cfg)
err = unmarshalOptions.Unmarshal(out, &cfg)
```

## Streaming decoder

Use `Decoder` when callers need token-level TOML inspection without binding into
Go values:

```go
dec := toml.NewDecoderBytes(data)
for {
	tok, err := dec.ReadToken()
	if err == io.EOF {
		break
	}
	if err != nil {
		return err
	}
	fmt.Printf("%s %q at %d:%d\n", tok.Kind, tok.Bytes, tok.Line, tok.Col)
}
```

`NewDecoderBytes` lets token byte slices alias the caller-owned source buffer.
`NewDecoder` owns an internal buffer for `io.Reader` input. In both cases, copy
`Token.Bytes` if the bytes must outlive the next token read. `WithLimits`
configures parser caps for document size, nesting depth, key length, and string
length; cap violations return `*LimitError`.

## Datetime model

TOML local datetime forms do not carry an offset, so the package keeps them as
explicit TOML-local values:

- `LocalDateTime`
- `LocalDate`
- `LocalTime`

Offset datetimes parse to `time.Time` with the numeric offset preserved through
`time.FixedZone`:

```go
v, err := toml.ParseDateTime([]byte("2026-05-17T07:08:09+09:00"))
// v is a time.Time for offset date-times.
```

Decoding local datetime forms into `time.Time` returns
`*LocalTimeIntoTimeError` unless the caller opts in to UTC interpretation:

```go
opts := toml.UnmarshalOptions{
	DecoderOptions: []toml.Option{toml.WithLocalAsUTC()},
}
err := opts.Unmarshal(data, &dst)
```

## Format-preserving documents

`ParseDocument` keeps raw source bytes and parsed spans so untouched documents
round-trip byte-for-byte. Use it for edit paths that must preserve comments,
whitespace, ordering, and unchanged lexical style:

```go
doc, err := toml.ParseDocument(data)
if err != nil {
	return err
}

if port, ok := doc.Get("server.port"); ok {
	fmt.Println("old port:", port)
}

if err := doc.Set("server.port", 9443); err != nil {
	return err
}
if err := doc.InsertAfter("server.port", "server.host", "127.0.0.1"); err != nil {
	return err
}
if err := doc.Delete("legacy.enabled"); err != nil {
	return err
}

updated := doc.Bytes()
```

`Document.Raw` returns the original input bytes. `Document.Bytes` returns the
current document bytes; on an untouched document it returns the raw bytes without
rewriting. Setting a value of the same TOML kind preserves the existing lexical
style where the parser recorded enough style information. Kind-changing writes
use canonical TOML emission.

## Build tags and dependency boundaries

Two build tags matter for this package:

- `force_swar` selects the pure-Go scanner fallback and is used by scanner
  verification.
- `bench` enables benchmark-only comparator tests against
  `github.com/BurntSushi/toml` and `github.com/pelletier/go-toml/v2`.

The comparator libraries must not enter production or ordinary test dependency
graphs. Re-check the boundary with:

```bash
go list -deps ./pkg/toml/... | rg 'BurntSushi|pelletier'
go list -deps -test ./pkg/toml/... | rg 'BurntSushi|pelletier'
go list -deps -test -tags=bench ./pkg/toml/... | rg 'github.com/BurntSushi/toml$|github.com/pelletier/go-toml/v2$'
```

The first two commands should print nothing. The bench-tag command should print
both comparator roots.

## Verification and provenance

The package is grounded in pinned upstream corpora and repeatable verification
commands:

- `UPSTREAM.md` records toml-rs and toml-test provenance, import procedures,
  fuzz discipline, build tags, and final integration commands.
- `internal/scan/VERIFICATION.md` records scanner verification and perf-gate
  policy.
- `PHASE4_BENCHMARK_EVIDENCE.md` records facade and edit benchmark evidence.

Useful local checks. The facade perf command reproduces the documented
Pelletier exception and currently exits non-zero; the edit gate is a separate
passing check.

```bash
go test ./pkg/toml/...
go test -run TestCompetitorDependenciesAreBenchOnly -count=1 ./pkg/toml
go test -tags=force_swar -race -count=1 ./pkg/toml/internal/scan/
go run ./hack/toml-perf-gate --kind=facade --ratio-burntsushi=1.5 --ratio-pelletier=1.3
go run ./hack/toml-perf-gate --kind=edit --ratio-edit=0.25
```

Current performance disposition:

- the BurntSushi facade gate is documented as passing;
- the Pelletier facade ratio is a documented Phase 4 architecture exception,
  not a hidden pass;
- the edit-path gate uses `--ratio-edit` and is documented separately from the
  facade Pelletier exception.
