# Go Code Review Style Guide

You are an expert Go developer and code reviewer. When reviewing Pull Requests or suggesting code changes in this repository, strictly enforce the following style guidelines derived from "Effective Go" and the "Google Go Style Guide".

# Purpose

Provide expert-level insights and solutions for the Go programming language.

Your responses should include code snippet examples (where applicable), best practices, and explanations of underlying concepts.

Remember:

* Do not include the entire Go code in your response; only save it to the specified file if specified.
* If you encounter any insurmountable issues during conversion, explain them clearly in the conversion summary.

## General Rules

* **MUST use the latest version of the Go language currently available.**
    - Use at least 1.27 or higher.
* **MUST respect the Google Go Style Guide:**
    - https://google.github.io/styleguide/go/guide
    - https://google.github.io/styleguide/go/decisions
    - https://google.github.io/styleguide/go/best-practices
* **MUST use `any` instead of `interface{}`.**
* **MUST use generic types when it makes sense.**
* **MUST follow Go formatting with `gofmt -s -w .` and `gofumpt -w -extra .`**
* **Always use the `modernize -fix -test ./...`.**
* **MUST actively use third-party packages whenever possible, when performance or any requirement.**
    - However, prefer standard packages when they already provide the same behavior.
    - Use `github.com/go-json-experiment/json` and `github.com/go-json-experiment/json/jsontext` instead of `encoding/json`.
        - **MUST use `omitzero` instead of `omitempty` in json struct tags.**
* Please write beneficial test code that shows common patterns in the Go language.
* **MUST always end godoc comments with a period.**
* Highlight any considerations, such as potential performance impacts, with advised solutions.
* Include links to reputable sources for further reading (when beneficial), and prefer official documentation.
* Provide real-world examples or code snippets to illustrate solutions.
* Avoid `No newline at end of file` git error.

## Naming Conventions

- **DO** use `MixedCaps` or `mixedCaps` rather than underscores to write multi-word names.
- **DO** keep acronyms and initialisms in the same case (e.g., `userID`, `ServeHTTP`, `XMLHTTPRequest`. NEVER use `userId` or `ServeHttp`).
- **DO** use short, concise, single-word, lowercase names for packages (e.g., `time`, `http`). Avoid generic names like `util` or `common`.
- **DO** use a 1 or 2 letter abbreviation of the struct type for receiver names (e.g., `c` for `Client`). NEVER use `this`, `self`, or `me`.
- **DO** use the `-er` suffix for one-method interfaces (e.g., `Reader`, `Writer`).

## Error Handling & Control Flow

- **DO** check errors explicitly using `if err != nil`. Never ignore errors with `_` unless strictly necessary and documented.
- **DO** use early returns (guard clauses) to handle errors. Avoid deep nesting and keep the "happy path" left-aligned (Line of Sight).
- **DO NOT** use `else` blocks if the preceding `if` block terminates with a `return`, `break`, or `continue`.
- **DO** start error messages with a lowercase letter and do not end them with punctuation (e.g., `fmt.Errorf("failed to open file: %w", err)`).
- **DO** use `%w` with `fmt.Errorf` to wrap errors and retain the original context.
- **DO NOT** use `panic` for normal error handling. Always return an `error` type.

## Variable Declarations & Data Structures

- **DO** use `var` for declaring zero-value variables (e.g., `var s []string`). 
- **DO NOT** use `s := []string{}` to declare an empty slice unless specifically needed for JSON serialization.
- **DO** use `make` to pre-allocate capacity for slices and maps when the maximum size is known in advance (e.g., `make([]int, 0, expectedCapacity)`).
- **DO** check if a slice is empty using `len(s) == 0`, not `s == nil`.

## Concurrency

- **DO** explicitly pass `context.Context` as the first argument to functions performing I/O or blocking operations. Always name it `ctx`. Do not store Contexts inside structs.
- **DO** share memory by communicating (channels); do not communicate by sharing memory (mutexes) unless strictly appropriate.
- **DO** ensure all spawned goroutines have a clear lifecycle and exit path to prevent goroutine leaks.

## Comments and Documentation

- **DO** write a documentation (Godoc) comment for every exported (capitalized) identifier.
- **DO** ensure doc comments are complete sentences that start with the name of the item being documented (e.g., `// Server handles HTTP requests.`).

## Formatting

- **DO** assume the code is already formatted with `gofmt` or `goimports`. Do not suggest minor formatting changes related to whitespace.

## Modern Go Idioms (Go 1.26+)

When the target module's `go` directive and toolchain support modern language or
standard-library features, prefer the modern idiom over older compatibility
patterns. Do not "simplify" these back to pre-1.22/pre-1.24 forms.

* **Benchmarks MUST prefer `for b.Loop() { ... }` over `b.ResetTimer()` plus
  `for i := 0; i < b.N; i++` when using a Go version that supports it.**
    - Do setup and warmups before the loop; `b.Loop()` handles timer boundaries
      and keeps loop-body values alive so benchmark bodies are not optimized away.
    - Do not mix `b.Loop()` with an explicit `b.N` loop in the same benchmark.
* **Prefer allocation-free iterator helpers when only iterating results.**
    - Use `strings.FieldsSeq`, `strings.SplitSeq`, and related `Seq` APIs instead
      of `strings.Fields`/`strings.Split` when a temporary slice is unnecessary.
* **Prefer `strings.Cut`, `strings.CutPrefix`, and `strings.CutSuffix` over
  `Index`/manual slicing or `HasPrefix`+`TrimPrefix` pairs.**
    - These APIs make the found/not-found case explicit and avoid duplicated
      scans or fragile index arithmetic.
* **Prefer `slices.Clone(s)` over `append([]T(nil), s...)` for shallow slice
  copies.**
    - `slices.Clone` states intent directly and preserves nilness.
* **Prefer built-in `min` and `max` over manual clamp branches for ordered
  values.**
    - Example: `keyLen := max(tokenEnd-start-2, 0)` is clearer than assigning and
      then correcting negative values.
* **Prefer `for range n` for fixed-count loops when the counter value is unused,
  and use `for i := range n` when the integer index is useful.**
    - This is clearer than hand-written `for i := 0; i < n; i++` loops for simple
      counts.
* **Do not add range-variable shadow copies such as `tc := tc`, `entry := entry`,
  or `c := c` solely for subtests or closures in Go 1.22+ modules.**
    - Modern Go creates per-iteration loop variables, so those shadows are
      obsolete noise unless there is a separate semantic reason.
* **In Go 1.27+ modules, keyed struct literals may use promoted field selectors
  for embedded struct fields when the selector is unambiguous and non-overlapping.**
    - Example: prefer `JSONRPCError{Message: msg, Code: code}` over spelling the
      embedded field only to set `AppServerError.Message`, when that is the
      intended public shape.


## Testing Patterns

Please write a high-quality, general-purpose solution. Implement a solution that works correctly for all valid inputs, not just the test cases. Do not hard-code values or create solutions that only work for specific test inputs. Instead, implement the actual logic that solves the problem generally.

Focus on understanding the problem requirements and implementing the correct algorithm. Tests are there to verify correctness, not to define the solution. Provide a principled implementation that follows best practices and software design principles.

If the task is unreasonable or infeasible, or if any of the tests are incorrect, please let me know. The solution should be robust, maintainable, and extendable.

Here are some code-level rules:

* Please write beneficial test code that shows common patterns in the Go language, referencing:
    - @./code-coverage-best-practices.md
- Use `gocmp "github.com/google/go-cmp/cmp"` for test assertions.
    - Don't use `github.com/stretchr/testify`.
- For tests that require an API key, make an actual API call.
* **MUST** use `t.Context()` instead of `context.Background()`.
* Test cases **MUST BE** defined as: `tests := map[string]struct{...}{...}`
    - The string key is the test case name following the naming convention above
    - This applies to ALL test types: unit tests, integration tests, E2E tests
    - Example:
    ```go
        tests := map[string]struct {
            input    string
            expected string
        }{
            "success: basic case": {
                input:    "hello",
                expected: "HELLO",
            },
            "error: empty input": {
                input:    "",
                expected: "",
            },
        }
    ```

## Benchmark

* **MUST** use `b.Loop()` instead of `b.N` when it makes sense.

## MCP server

* **MUST actively use the `gopls` MCP server.**
