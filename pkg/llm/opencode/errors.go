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

package opencode

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-json-experiment/json"
)

// Turn-level error names this package branches on; the full name taxonomy
// carried in AssistantMessage.Error.Name is documented on MessageError.
const (
	errorNameUnknown        = "UnknownError"
	errorNameMessageAborted = "MessageAbortedError"
)

// APIError is an OpenCode HTTP error response. Error() reports the method,
// path, status, and the server's named error envelope; it never includes
// credentials (auth rides the Authorization header, never the URL).
type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Name       string // server error name, e.g. "NotFoundError", "BadRequest"
	Message    string // server error message (data.message)
	Body       []byte // raw response body (may be empty)
}

func (e *APIError) Error() string {
	name := e.Name
	if name == "" {
		name = http.StatusText(e.StatusCode)
	}
	if e.Message == "" {
		return fmt.Sprintf("opencode: %s %s: %d %s", e.Method, e.Path, e.StatusCode, name)
	}
	return fmt.Sprintf("opencode: %s %s: %d %s: %s", e.Method, e.Path, e.StatusCode, name, e.Message)
}

// NotFoundError is an HTTP 404 (unknown session, message, or permission).
type NotFoundError struct {
	*APIError
}

// Unwrap makes *APIError reachable via errors.As / errors.Unwrap.
func (e *NotFoundError) Unwrap() error { return e.APIError }

// ServerBusyError is an HTTP 429 or 503 overload response; it is retryable.
type ServerBusyError struct {
	*APIError
}

// Unwrap makes *APIError reachable via errors.As / errors.Unwrap.
func (e *ServerBusyError) Unwrap() error { return e.APIError }

// retryable marks ServerBusyError for RetryOnOverload.
func (e *ServerBusyError) retryable() {}

// MessageAbortedError reports a turn that ended because it was interrupted
// (TurnHandle.Interrupt, POST /session/{id}/abort, or a server-side abort).
type MessageAbortedError struct {
	SessionID string
	MessageID string
	Message   string
}

func (e *MessageAbortedError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("opencode: turn aborted on session %s: %s", e.SessionID, e.Message)
	}
	return fmt.Sprintf("opencode: turn aborted on session %s", e.SessionID)
}

// TurnFailedError reports a turn that ended with a named turn-level error
// (AssistantMessage.Error or a session.error event). Err carries the named
// error; MessageID is empty when the failure arrived as a session.error
// event before an assistant message was observed.
type TurnFailedError struct {
	SessionID string
	MessageID string
	Err       MessageError
}

func (e *TurnFailedError) Error() string {
	if message := e.Err.Message(); message != "" {
		return fmt.Sprintf("opencode: turn failed on session %s: %s: %s", e.SessionID, e.Err.Name, message)
	}
	return fmt.Sprintf("opencode: turn failed on session %s: %s", e.SessionID, e.Err.Name)
}

// TransportClosedError is returned when the shared SSE event bus is closed:
// the client shut down, or auto-reconnect exhausted its RetryConfig budget.
type TransportClosedError struct {
	Message string
}

func (e *TransportClosedError) Error() string { return e.Message }

// IsRetryableError reports whether err should be retried for overload-style
// behavior (HTTP 429/503).
func IsRetryableError(err error) bool {
	var busy *ServerBusyError
	return errors.As(err, &busy)
}

// IsNotFound reports whether err is an HTTP 404 from the server.
func IsNotFound(err error) bool {
	var notFound *NotFoundError
	return errors.As(err, &notFound)
}

// mapHTTPError converts a non-2xx HTTP response into the typed error
// hierarchy. body is the (possibly empty) response body; the OpenCode error
// envelope {"name": ..., "data": {"message": ...}} is decoded best-effort.
func mapHTTPError(statusCode int, method, path string, body []byte) error {
	apiErr := &APIError{
		StatusCode: statusCode,
		Method:     method,
		Path:       path,
		Body:       body,
	}
	if len(body) > 0 {
		var envelope errorEnvelope
		if err := json.Unmarshal(body, &envelope); err == nil {
			apiErr.Name = envelope.Name
			apiErr.Message = envelope.Data.Message
		}
	}

	switch statusCode {
	case http.StatusNotFound:
		return &NotFoundError{APIError: apiErr}
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return &ServerBusyError{APIError: apiErr}
	default:
		return apiErr
	}
}

// mapTurnError converts a turn-level MessageError into the typed error
// hierarchy: MessageAbortedError for aborted turns, TurnFailedError for
// every other named failure.
func mapTurnError(sessionID, messageID string, turnErr MessageError) error {
	if turnErr.Name == errorNameMessageAborted {
		return &MessageAbortedError{
			SessionID: sessionID,
			MessageID: messageID,
			Message:   turnErr.Message(),
		}
	}
	return &TurnFailedError{
		SessionID: sessionID,
		MessageID: messageID,
		Err:       turnErr,
	}
}
