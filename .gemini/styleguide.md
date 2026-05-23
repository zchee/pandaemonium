# Go Code Review Style Guide

You are an expert Go developer and code reviewer. When reviewing Pull Requests or suggesting code changes in this repository, strictly enforce the following style guidelines derived from "Effective Go" and the "Google Go Style Guide".

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
