// Copyright 2026 The omxx Authors.
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

package codexappserver

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	json "github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// AppServerError is the base error for SDK failures.
type AppServerError struct {
	Message string
}

func (e *AppServerError) Error() string { return e.Message }

// TransportClosedError is returned when the app-server stdio transport closes.
type TransportClosedError struct {
	Message string
}

func (e *TransportClosedError) Error() string { return e.Message }

// JSONRPCError is a JSON-RPC error response from the app-server.
type JSONRPCError struct {
	Code    int64
	Message string
	Data    jsontext.Value
	Kind    string
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// IsServerBusy reports whether err is retryable server overload.
func IsServerBusy(err error) bool {
	var rpcErr *JSONRPCError
	if !errors.As(err, &rpcErr) {
		return false
	}
	return rpcErr.Kind == "server_busy" || rpcErr.Kind == "retry_limit_exceeded" || isServerOverloaded(rpcErr.Data)
}

// IsRetryLimitExceeded reports whether err indicates an exhausted retry budget.
func IsRetryLimitExceeded(err error) bool {
	var rpcErr *JSONRPCError
	return errors.As(err, &rpcErr) && rpcErr.Kind == "retry_limit_exceeded"
}

func mapJSONRPCError(code int64, message string, data jsontext.Value) error {
	kind := "jsonrpc"
	switch code {
	case -32700:
		kind = "parse_error"
	case -32600:
		kind = "invalid_request"
	case -32601:
		kind = "method_not_found"
	case -32602:
		kind = "invalid_params"
	case -32603:
		kind = "internal_error"
	default:
		if code >= -32099 && code <= -32000 {
			kind = "app_server_rpc"
		}
	}
	if isServerOverloaded(data) {
		kind = "server_busy"
	}
	if containsRetryLimitText(message) {
		kind = "retry_limit_exceeded"
	}
	return &JSONRPCError{Code: code, Message: message, Data: data, Kind: kind}
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
