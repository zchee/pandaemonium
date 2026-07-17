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
	"strings"
	"testing"
)

func TestMapHTTPError(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status        int
		body          string
		wantType      func(error) bool
		wantRetryable bool
		wantNotFound  bool
		wantName      string
		wantMessage   string
	}{
		"error: 404 maps to NotFoundError": {
			status: 404,
			body:   `{"name":"NotFoundError","data":{"message":"Session not found: ses_x"}}`,
			wantType: func(err error) bool {
				var target *NotFoundError
				return errors.As(err, &target)
			},
			wantNotFound: true,
			wantName:     "NotFoundError",
			wantMessage:  "Session not found: ses_x",
		},
		"error: 429 maps to retryable ServerBusyError": {
			status: 429,
			body:   `{"name":"UnknownError","data":{"message":"overloaded"}}`,
			wantType: func(err error) bool {
				var target *ServerBusyError
				return errors.As(err, &target)
			},
			wantRetryable: true,
			wantName:      "UnknownError",
			wantMessage:   "overloaded",
		},
		"error: 503 maps to retryable ServerBusyError": {
			status: 503,
			body:   ``,
			wantType: func(err error) bool {
				var target *ServerBusyError
				return errors.As(err, &target)
			},
			wantRetryable: true,
		},
		"error: 400 maps to bare APIError with envelope": {
			status: 400,
			body:   `{"name":"BadRequest","data":{"message":"Missing key\n  at [\"parts\"]","kind":"Payload"}}`,
			wantType: func(err error) bool {
				var busy *ServerBusyError
				var notFound *NotFoundError
				var api *APIError
				return !errors.As(err, &busy) && !errors.As(err, &notFound) && errors.As(err, &api)
			},
			wantName:    "BadRequest",
			wantMessage: "Missing key\n  at [\"parts\"]",
		},
		"error: 500 with UnknownError envelope": {
			status: 500,
			body:   `{"name":"UnknownError","data":{"message":"Unexpected server error.","ref":"err_x"}}`,
			wantType: func(err error) bool {
				var api *APIError
				return errors.As(err, &api)
			},
			wantName:    "UnknownError",
			wantMessage: "Unexpected server error.",
		},
		"error: non-JSON body tolerated": {
			status: 502,
			body:   `upstream exploded`,
			wantType: func(err error) bool {
				var api *APIError
				return errors.As(err, &api)
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := mapHTTPError(tt.status, "POST", "/session/ses_x/message", []byte(tt.body))
			if err == nil {
				t.Fatal("mapHTTPError returned nil")
			}
			if !tt.wantType(err) {
				t.Fatalf("error type mismatch: %T %v", err, err)
			}
			if got := IsRetryableError(err); got != tt.wantRetryable {
				t.Errorf("IsRetryableError = %t, want %t (err: %v)", got, tt.wantRetryable, err)
			}
			if got := IsNotFound(err); got != tt.wantNotFound {
				t.Errorf("IsNotFound = %t, want %t (err: %v)", got, tt.wantNotFound, err)
			}

			var api *APIError
			if !errors.As(err, &api) {
				t.Fatalf("APIError not reachable in chain of %T", err)
			}
			if api.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", api.StatusCode, tt.status)
			}
			if api.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", api.Name, tt.wantName)
			}
			if api.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", api.Message, tt.wantMessage)
			}
			if !strings.Contains(err.Error(), "POST /session/ses_x/message") {
				t.Errorf("Error() missing method/path context: %q", err.Error())
			}
		})
	}
}

func TestMapTurnError(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		turnErr     MessageError
		wantAborted bool
		wantText    string
	}{
		"error: MessageAbortedError maps to MessageAbortedError": {
			turnErr:     MessageError{Name: "MessageAbortedError", Data: []byte(`{"message":"user hit stop"}`)},
			wantAborted: true,
			wantText:    "user hit stop",
		},
		"error: ProviderAuthError maps to TurnFailedError": {
			turnErr:  MessageError{Name: "ProviderAuthError", Data: []byte(`{"message":"bad key"}`)},
			wantText: "ProviderAuthError",
		},
		"error: name without data still readable": {
			turnErr:  MessageError{Name: "ContextOverflowError"},
			wantText: "ContextOverflowError",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := mapTurnError("ses_x", "msg_y", tt.turnErr)
			var aborted *MessageAbortedError
			if gotAborted := errors.As(err, &aborted); gotAborted != tt.wantAborted {
				t.Fatalf("MessageAbortedError match = %t, want %t (err: %v)", gotAborted, tt.wantAborted, err)
			}
			if !tt.wantAborted {
				var failed *TurnFailedError
				if !errors.As(err, &failed) {
					t.Fatalf("TurnFailedError not reachable in %T", err)
				}
				if failed.SessionID != "ses_x" || failed.MessageID != "msg_y" {
					t.Errorf("scope = (%q, %q), want (ses_x, msg_y)", failed.SessionID, failed.MessageID)
				}
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("Error() = %q, want substring %q", err.Error(), tt.wantText)
			}
		})
	}
}

func TestTransportClosedErrorNotRetryable(t *testing.T) {
	t.Parallel()

	err := &TransportClosedError{Message: "opencode event bus closed"}
	if IsRetryableError(err) {
		t.Error("TransportClosedError must not be retryable")
	}
	if IsNotFound(err) {
		t.Error("TransportClosedError must not match IsNotFound")
	}
}
