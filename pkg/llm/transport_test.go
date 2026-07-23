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

package llm

import (
	"errors"
	"testing"
)

// TestStdioTransportZeroValue verifies that an unconfigured StdioTransport
// fails closed with the generic fallback error instead of panicking. The
// wired behavior (per-package error types, pipe round-trips) is covered by
// the SDK package transport tests.
func TestStdioTransportZeroValue(t *testing.T) {
	t.Parallel()

	tr := &StdioTransport{}
	if err := tr.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if err := tr.WriteJSON(t.Context(), []byte(`{}`)); !errors.Is(err, errStdioTransportClosed) {
		t.Fatalf("WriteJSON() error = %v, want errStdioTransportClosed", err)
	}
	if _, err := tr.ReadJSON(t.Context()); !errors.Is(err, errStdioTransportClosed) {
		t.Fatalf("ReadJSON() error = %v, want errStdioTransportClosed", err)
	}

	custom := errors.New("custom closed")
	tr = &StdioTransport{ClosedErr: func() error { return custom }}
	if err := tr.WriteJSON(t.Context(), []byte(`{}`)); !errors.Is(err, custom) {
		t.Fatalf("WriteJSON() with ClosedErr = %v, want custom error", err)
	}
}
