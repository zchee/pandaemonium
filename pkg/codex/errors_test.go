// Copyright 2026 The pandaemonium Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package codex

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// buildWrappers returns one instance of every concrete wrapper type, each built
// via the same internal factory paths used in production.
// Returns (wrapperName, err) pairs for table-driven tests.
func buildWrappers() []struct {
	name string
	err  error
} {
	const (
		rawMsg = "thread busy"
		code   = int64(-32000)
	)
	// mapJSONRPCError routes by error-code for the standard RPC codes. For
	// ServerBusyError and RetryLimitExceededError we construct directly because
	// mapJSONRPCError requires specific message text / data markers to select
	// those paths, which would change the rawMsg used in assertions.
	base := appServerRPCError(code, rawMsg, nil, "server_busy")
	return []struct {
		name string
		err  error
	}{
		{"ParseError", mapJSONRPCError(-32700, rawMsg, nil)},
		{"InvalidRequestError", mapJSONRPCError(-32600, rawMsg, nil)},
		{"MethodNotFoundError", mapJSONRPCError(-32601, rawMsg, nil)},
		{"InvalidParamsError", mapJSONRPCError(-32602, rawMsg, nil)},
		{"InternalRPCError", mapJSONRPCError(-32603, rawMsg, nil)},
		{"ServerBusyError", &ServerBusyError{AppServerRPCError: base}},
		{
			"RetryLimitExceededError",
			&RetryLimitExceededError{ServerBusyError: &ServerBusyError{AppServerRPCError: base}},
		},
		{"AppServerRPCError", appServerRPCError(code, rawMsg, nil, "app_server_rpc")},
	}
}

// TestErrorsAsTraversal verifies AC-2.3: for every concrete wrapper type,
// errors.As succeeds for *JSONRPCError, *AppServerRPCError, *AppServerError,
// and the concrete type itself.
func TestErrorsAsTraversal(t *testing.T) {
	t.Parallel()

	const rawMsg = "thread busy"

	wrappers := buildWrappers()
	for _, tc := range wrappers {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// *JSONRPCError must be reachable.
			var rpcErr *JSONRPCError
			if !errors.As(tc.err, &rpcErr) {
				t.Fatalf("errors.As(%s, *JSONRPCError) = false, want true", tc.name)
			}

			// *AppServerRPCError must be reachable (except when the error IS a bare
			// *JSONRPCError returned by mapJSONRPCError for codes outside the -32000
			// range — none in our test set, but guard anyway).
			var appSrvRPCErr *AppServerRPCError
			if tc.name != "AppServerRPCError" {
				// Concrete wrappers all embed AppServerRPCError.
				if !errors.As(tc.err, &appSrvRPCErr) {
					t.Fatalf("errors.As(%s, *AppServerRPCError) = false, want true", tc.name)
				}
			} else {
				// The bare *AppServerRPCError itself.
				if !errors.As(tc.err, &appSrvRPCErr) {
					t.Fatalf("errors.As(AppServerRPCError, *AppServerRPCError) = false, want true")
				}
			}

			// *AppServerError must be reachable (deepest in the chain).
			var appSrvErr *AppServerError
			if !errors.As(tc.err, &appSrvErr) {
				t.Fatalf("errors.As(%s, *AppServerError) = false, want true", tc.name)
			}
			// The Message at every level must be the raw server text, never
			// the formatted "JSON-RPC error N: msg" prefix.
			if got := appSrvErr.Message; got != rawMsg {
				t.Fatalf("AppServerError.Message = %q, want %q", got, rawMsg)
			}
			if got := rpcErr.Message; got != rawMsg {
				t.Fatalf("JSONRPCError.Message = %q, want %q", got, rawMsg)
			}

			// errors.As must find the concrete type for each wrapper.
			switch tc.name {
			case "ParseError":
				var target *ParseError
				if !errors.As(tc.err, &target) {
					t.Fatal("errors.As(*ParseError) = false")
				}
			case "InvalidRequestError":
				var target *InvalidRequestError
				if !errors.As(tc.err, &target) {
					t.Fatal("errors.As(*InvalidRequestError) = false")
				}
			case "MethodNotFoundError":
				var target *MethodNotFoundError
				if !errors.As(tc.err, &target) {
					t.Fatal("errors.As(*MethodNotFoundError) = false")
				}
			case "InvalidParamsError":
				var target *InvalidParamsError
				if !errors.As(tc.err, &target) {
					t.Fatal("errors.As(*InvalidParamsError) = false")
				}
			case "InternalRPCError":
				var target *InternalRPCError
				if !errors.As(tc.err, &target) {
					t.Fatal("errors.As(*InternalRPCError) = false")
				}
			case "ServerBusyError":
				var target *ServerBusyError
				if !errors.As(tc.err, &target) {
					t.Fatal("errors.As(*ServerBusyError) = false")
				}
			case "RetryLimitExceededError":
				var target *RetryLimitExceededError
				if !errors.As(tc.err, &target) {
					t.Fatal("errors.As(*RetryLimitExceededError) = false")
				}
			case "AppServerRPCError":
				// Already tested above.
			}
		})
	}
}

// TestJSONRPCErrorMessageRaw verifies AC-2.5: AppServerError.Message holds
// the raw server text, not the "JSON-RPC error N: msg" formatted string.
func TestJSONRPCErrorMessageRaw(t *testing.T) {
	t.Parallel()

	const (
		code    = int64(-32000)
		rawMsg  = "thread busy"
		wantErr = "JSON-RPC error -32000: thread busy"
	)

	rpc := makeJSONRPCError(code, rawMsg, nil, "server_busy")

	if got := rpc.Message; got != rawMsg {
		t.Fatalf("JSONRPCError.Message = %q, want raw %q", got, rawMsg)
	}
	if got := rpc.Error(); got != wantErr {
		t.Fatalf("JSONRPCError.Error() = %q, want %q", got, wantErr)
	}

	// The embedded AppServerError.Error() returns the raw message too.
	var appSrvErr *AppServerError
	if !errors.As(rpc, &appSrvErr) {
		t.Fatal("errors.As(*AppServerError) = false")
	}
	if got := appSrvErr.Message; got != rawMsg {
		t.Fatalf("AppServerError.Message = %q, want raw %q", got, rawMsg)
	}
}

// TestAsJSONRPCErrorBoundedDepth verifies AC-2.6: asJSONRPCError caps its
// search at 32 levels and returns within a bounded time for a 64-deep chain.
// The wall-clock threshold is generous (5ms) to stay reliable under -race
// instrumentation while still catching any regression to unbounded recursion
// (which on a 64-deep chain would either deadlock or take seconds).
func TestAsJSONRPCErrorBoundedDepth(t *testing.T) {
	t.Parallel()

	// Build a 64-deep chain of fmt.Errorf("%w", ...) wrappers with no
	// *JSONRPCError anywhere — asJSONRPCError must give up and return nil.
	var chain error = errors.New("sentinel")
	for i := range 64 {
		chain = fmt.Errorf("level %d: %w", i, chain)
	}

	start := time.Now()
	result := asJSONRPCError(chain)
	elapsed := time.Since(start)

	if result != nil {
		t.Fatalf("asJSONRPCError(deep chain with no *JSONRPCError) = %v, want nil", result)
	}
	if elapsed > 5*time.Millisecond {
		t.Fatalf("asJSONRPCError took %v, want < 5ms (bounded depth check too slow)", elapsed)
	}

	// Sanity: a *JSONRPCError buried at depth 30 (within the 32-level cap)
	// must still be found.
	rpc := makeJSONRPCError(-32000, "buried", nil, "server_busy")
	var withRPC error = rpc
	for i := range 30 {
		withRPC = fmt.Errorf("wrap %d: %w", i, withRPC)
	}
	if got := asJSONRPCError(withRPC); got != rpc {
		t.Fatalf("asJSONRPCError(depth-30 chain) = %v, want the buried *JSONRPCError", got)
	}

	// A *JSONRPCError buried beyond depth 32 must NOT be found (cap enforced).
	var tooDeep error = rpc
	for i := range 33 {
		tooDeep = fmt.Errorf("wrap %d: %w", i, tooDeep)
	}
	if got := asJSONRPCError(tooDeep); got != nil {
		t.Fatalf("asJSONRPCError(depth-33+ chain) = %v, want nil (past cap)", got)
	}
}

// TestExportedJSONRPCErrorAccessor verifies that every concrete wrapper type
// exposes an exported JSONRPCError() method returning the underlying *JSONRPCError.
func TestExportedJSONRPCErrorAccessor(t *testing.T) {
	t.Parallel()

	const rawMsg = "exported accessor test"

	makeBase := func(kind string) *AppServerRPCError {
		return appServerRPCError(-32000, rawMsg, nil, kind)
	}

	cases := []struct {
		name   string
		getRPC func() *JSONRPCError
	}{
		{
			"ParseError",
			func() *JSONRPCError {
				e := &ParseError{AppServerRPCError: makeBase("parse_error")}
				return e.JSONRPCError()
			},
		},
		{
			"InvalidRequestError",
			func() *JSONRPCError {
				e := &InvalidRequestError{AppServerRPCError: makeBase("invalid_request")}
				return e.JSONRPCError()
			},
		},
		{
			"MethodNotFoundError",
			func() *JSONRPCError {
				e := &MethodNotFoundError{AppServerRPCError: makeBase("method_not_found")}
				return e.JSONRPCError()
			},
		},
		{
			"InvalidParamsError",
			func() *JSONRPCError {
				e := &InvalidParamsError{AppServerRPCError: makeBase("invalid_params")}
				return e.JSONRPCError()
			},
		},
		{
			"InternalRPCError",
			func() *JSONRPCError {
				e := &InternalRPCError{AppServerRPCError: makeBase("internal_error")}
				return e.JSONRPCError()
			},
		},
		{
			"ServerBusyError",
			func() *JSONRPCError {
				e := &ServerBusyError{AppServerRPCError: makeBase("server_busy")}
				return e.JSONRPCError()
			},
		},
		{
			"RetryLimitExceededError (promoted from ServerBusyError)",
			func() *JSONRPCError {
				e := &RetryLimitExceededError{
					ServerBusyError: &ServerBusyError{AppServerRPCError: makeBase("retry_limit_exceeded")},
				}
				return e.JSONRPCError()
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rpc := tc.getRPC()
			if rpc == nil {
				t.Fatalf("%s.JSONRPCError() = nil, want non-nil", tc.name)
			}
			if rpc.Message != rawMsg {
				t.Fatalf("%s.JSONRPCError().Message = %q, want %q", tc.name, rpc.Message, rawMsg)
			}
		})
	}
}
