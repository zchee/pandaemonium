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
	"slices"
	"strings"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// AppServerError is the base error for SDK failures.
type AppServerError struct {
	Message string
}

func (e *AppServerError) Error() string { return e.Message }

// JSONRPCError is a JSON-RPC error response from the app-server.
// The embedded AppServerError.Message field holds the raw server text.
// Error() formats "JSON-RPC error <code>: <message>" at call time.
type JSONRPCError struct {
	AppServerError
	Code int64
	Data jsontext.Value
	Kind string
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// Unwrap exposes the embedded AppServerError so that errors.As traversal
// can locate *AppServerError in the chain.
func (e *JSONRPCError) Unwrap() error { return &e.AppServerError }

// TransportClosedError is returned when the app-server stdio transport closes.
type TransportClosedError struct {
	Message string
}

func (e *TransportClosedError) Error() string { return e.Message }

// NotificationDroppedError is returned by a turn notification consumer when one
// or more notifications were silently dropped due to per-turn queue overflow.
// The error is surfaced exactly once per drop event and the counter resets after
// delivery. The router is NOT torn down; subsequent calls to next succeed normally.
type NotificationDroppedError struct {
	TurnID  string
	Dropped int
}

func (e *NotificationDroppedError) Error() string {
	return fmt.Sprintf("turn %s: %d notification(s) dropped due to queue overflow", e.TurnID, e.Dropped)
}

// AppServerRPCError is a server-side JSON-RPC error.
type AppServerRPCError struct {
	*JSONRPCError
}

// Unwrap makes *JSONRPCError reachable via errors.As / errors.Unwrap.
func (e *AppServerRPCError) Unwrap() error {
	if e == nil || e.JSONRPCError == nil {
		return nil
	}
	return e.JSONRPCError
}

// ParseError indicates invalid JSON syntax or non-conformant request payload.
type ParseError struct {
	*AppServerRPCError
}

// JSONRPCError returns the underlying *JSONRPCError.
func (e *ParseError) JSONRPCError() *JSONRPCError {
	if e == nil || e.AppServerRPCError == nil {
		return nil
	}
	return e.AppServerRPCError.JSONRPCError
}

// Unwrap makes *AppServerRPCError (and its chain) reachable via errors.As.
func (e *ParseError) Unwrap() error { return e.AppServerRPCError }

// InvalidRequestError indicates invalid JSON-RPC request structure.
type InvalidRequestError struct {
	*AppServerRPCError
}

// JSONRPCError returns the underlying *JSONRPCError.
func (e *InvalidRequestError) JSONRPCError() *JSONRPCError {
	if e == nil || e.AppServerRPCError == nil {
		return nil
	}
	return e.AppServerRPCError.JSONRPCError
}

// Unwrap makes *AppServerRPCError (and its chain) reachable via errors.As.
func (e *InvalidRequestError) Unwrap() error { return e.AppServerRPCError }

// MethodNotFoundError indicates an unknown JSON-RPC method call.
type MethodNotFoundError struct {
	*AppServerRPCError
}

// JSONRPCError returns the underlying *JSONRPCError.
func (e *MethodNotFoundError) JSONRPCError() *JSONRPCError {
	if e == nil || e.AppServerRPCError == nil {
		return nil
	}
	return e.AppServerRPCError.JSONRPCError
}

// Unwrap makes *AppServerRPCError (and its chain) reachable via errors.As.
func (e *MethodNotFoundError) Unwrap() error { return e.AppServerRPCError }

// InvalidParamsError indicates malformed or invalid request params.
type InvalidParamsError struct {
	*AppServerRPCError
}

// JSONRPCError returns the underlying *JSONRPCError.
func (e *InvalidParamsError) JSONRPCError() *JSONRPCError {
	if e == nil || e.AppServerRPCError == nil {
		return nil
	}
	return e.AppServerRPCError.JSONRPCError
}

// Unwrap makes *AppServerRPCError (and its chain) reachable via errors.As.
func (e *InvalidParamsError) Unwrap() error { return e.AppServerRPCError }

// InternalRPCError indicates internal server JSON-RPC failure.
type InternalRPCError struct {
	*AppServerRPCError
}

// JSONRPCError returns the underlying *JSONRPCError.
func (e *InternalRPCError) JSONRPCError() *JSONRPCError {
	if e == nil || e.AppServerRPCError == nil {
		return nil
	}
	return e.AppServerRPCError.JSONRPCError
}

// Unwrap makes *AppServerRPCError (and its chain) reachable via errors.As.
func (e *InternalRPCError) Unwrap() error { return e.AppServerRPCError }

// ServerBusyError indicates server-overload style retriable app-server errors.
type ServerBusyError struct {
	*AppServerRPCError
}

// JSONRPCError returns the underlying *JSONRPCError.
func (e *ServerBusyError) JSONRPCError() *JSONRPCError {
	if e == nil || e.AppServerRPCError == nil {
		return nil
	}
	return e.AppServerRPCError.JSONRPCError
}

// Unwrap makes *AppServerRPCError (and its chain) reachable via errors.As.
func (e *ServerBusyError) Unwrap() error { return e.AppServerRPCError }

// RetryLimitExceededError indicates app-server overload retries were exhausted.
type RetryLimitExceededError struct {
	*ServerBusyError
}

// Unwrap makes *ServerBusyError (and its chain) reachable via errors.As.
func (e *RetryLimitExceededError) Unwrap() error { return e.ServerBusyError }

// TurnFailedError is returned by collectRunResult when a turn ends with
// Status=failed. Unwrap maps the embedded CodexErrorInfo to the SDK error
// hierarchy when present, enabling errors.As to reach *ServerBusyError,
// *RetryLimitExceededError, etc. without inspecting the raw TurnError.
type TurnFailedError struct {
	TurnID string
	Status TurnStatus
	Err    *TurnError
}

func (e *TurnFailedError) Error() string {
	if e.Err != nil && e.Err.Message != "" {
		return fmt.Sprintf("turn %s failed: %s", e.TurnID, e.Err.Message)
	}
	return fmt.Sprintf("turn %s failed: %s", e.TurnID, e.Status)
}

// Unwrap maps CodexErrorInfo to a typed SDK error so that errors.As can
// traverse through *TurnFailedError to reach *ServerBusyError,
// *RetryLimitExceededError, etc. Returns nil when Err is nil or CodexErrorInfo
// is absent, preserving the invariant that *TurnFailedError is always findable
// via errors.As even when no further chain is present.
func (e *TurnFailedError) Unwrap() error {
	if e.Err == nil || e.Err.CodexErrorInfo == nil {
		return nil
	}
	// Marshal the concrete CodexErrorInfo value (e.g. CodexErrorInfoValue
	// "serverOverloaded") to JSON so that mapJSONRPCError's isServerOverloaded
	// check can classify it into the correct typed wrapper.
	data, err := json.Marshal(*e.Err.CodexErrorInfo)
	if err != nil {
		return nil
	}
	return mapJSONRPCError(-32000, e.Err.Message, jsontext.Value(data))
}

// IsRetryLimitExceeded reports whether err indicates an exhausted retry budget.
func IsRetryLimitExceeded(err error) bool {
	if _, ok := errors.AsType[*RetryLimitExceededError](err); ok {
		return true
	}
	rpcErr := asJSONRPCError(err)
	return rpcErr != nil && rpcErr.Kind == "retry_limit_exceeded"
}

// IsRetryableError reports whether err should be retried for overload-style behavior.
func IsRetryableError(err error) bool {
	rpcErr := asJSONRPCError(err)
	if rpcErr == nil {
		return false
	}
	return rpcErr.Kind == "server_busy" ||
		rpcErr.Kind == "retry_limit_exceeded" ||
		isServerOverloaded(rpcErr.Data)
}

// asJSONRPCError walks the error chain (up to 32 levels) and returns the first
// *JSONRPCError found, or nil if none is present in the chain.
func asJSONRPCError(err error) *JSONRPCError {
	for range 32 {
		if err == nil {
			return nil
		}
		if rpcErr, ok := err.(*JSONRPCError); ok {
			return rpcErr
		}
		// Concrete wrapper types (ParseError, ServerBusyError, etc.) expose an
		// exported JSONRPCError() accessor; use it as a fast-path shortcut.
		if carrier, ok := err.(interface{ JSONRPCError() *JSONRPCError }); ok {
			return carrier.JSONRPCError()
		}
		err = errors.Unwrap(err)
	}
	return nil
}

func makeJSONRPCError(code int64, message string, data jsontext.Value, kind string) *JSONRPCError {
	return &JSONRPCError{
		AppServerError: AppServerError{Message: message},
		Code:           code,
		Data:           data,
		Kind:           kind,
	}
}

func appServerRPCError(code int64, message string, data jsontext.Value, kind string) *AppServerRPCError {
	return &AppServerRPCError{JSONRPCError: makeJSONRPCError(code, message, data, kind)}
}

func mapJSONRPCError(code int64, message string, data jsontext.Value) error {
	overloaded := isServerOverloaded(data)
	retryLimit := containsRetryLimitText(message)

	switch code {
	case -32700:
		return &ParseError{AppServerRPCError: appServerRPCError(code, message, data, "parse_error")}
	case -32600:
		return &InvalidRequestError{AppServerRPCError: appServerRPCError(code, message, data, "invalid_request")}
	case -32601:
		return &MethodNotFoundError{AppServerRPCError: appServerRPCError(code, message, data, "method_not_found")}
	case -32602:
		return &InvalidParamsError{AppServerRPCError: appServerRPCError(code, message, data, "invalid_params")}
	case -32603:
		return &InternalRPCError{AppServerRPCError: appServerRPCError(code, message, data, "internal_error")}
	}

	if code >= -32099 && code <= -32000 {
		switch {
		case retryLimit:
			return &RetryLimitExceededError{ServerBusyError: &ServerBusyError{AppServerRPCError: appServerRPCError(code, message, data, "retry_limit_exceeded")}}
		case overloaded:
			return &ServerBusyError{AppServerRPCError: appServerRPCError(code, message, data, "server_busy")}
		default:
			return appServerRPCError(code, message, data, "app_server_rpc")
		}
	}

	kind := "jsonrpc"
	if retryLimit {
		kind = "retry_limit_exceeded"
	}
	return makeJSONRPCError(code, message, data, kind)
}

func containsRetryLimitText(message string) bool {
	lowered := strings.ToLower(message)
	return strings.Contains(lowered, "retry limit") || strings.Contains(lowered, "too many failed attempts")
}

func isServerOverloaded(data jsontext.Value) bool {
	if len(data) == 0 || string(data) == "null" {
		return false
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return false
	}
	return containsOverloadMarker(value)
}

func containsOverloadMarker(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.EqualFold(typed, "server_overloaded") || strings.EqualFold(typed, "serverOverloaded")

	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(key, "codex_error_info") || strings.EqualFold(key, "codexErrorInfo") || strings.EqualFold(key, "errorInfo") {
				if containsOverloadMarker(child) {
					return true
				}
			}
			if containsOverloadMarker(child) {
				return true
			}
		}

	case []any:
		if slices.ContainsFunc(typed, containsOverloadMarker) {
			return true
		}

	case jsontext.Value:
		return isServerOverloaded(typed)
	}

	return false
}
