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

package claude

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorTaxonomy(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err     error
		wantMsg string
	}{
		"success: CLINotFoundError single path": {
			err:     &CLINotFoundError{SearchPaths: []string{"/usr/bin/claude"}},
			wantMsg: "/usr/bin/claude",
		},
		"success: CLINotFoundError multiple paths": {
			err:     &CLINotFoundError{SearchPaths: []string{"claude (PATH)", "/opt/homebrew/bin/claude"}},
			wantMsg: "claude (PATH)",
		},
		"success: CLIConnectionError": {
			err:     &CLIConnectionError{Message: "connection refused"},
			wantMsg: "connection refused",
		},
		"success: ProcessError with stderr tail": {
			err:     &ProcessError{ExitCode: 1, StderrTail: "fatal: ENOENT"},
			wantMsg: "fatal: ENOENT",
		},
		"success: ProcessError without stderr tail": {
			err:     &ProcessError{ExitCode: 2},
			wantMsg: "code 2",
		},
		"success: CLIJSONDecodeError": {
			err:     &CLIJSONDecodeError{Line: []byte("{invalid"), Offset: 10},
			wantMsg: "offset 10",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			msg := tt.err.Error()
			if msg == "" {
				t.Fatal("Error() returned empty string")
			}
			if !strings.Contains(msg, tt.wantMsg) {
				t.Fatalf("Error() = %q, want contain %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestErrorTaxonomyErrorsAs(t *testing.T) {
	t.Parallel()

	t.Run("CLINotFoundError", func(t *testing.T) {
		t.Parallel()
		err := error(&CLINotFoundError{SearchPaths: []string{"/a"}})
		var target *CLINotFoundError
		if !errors.As(err, &target) {
			t.Fatal("errors.As(*CLINotFoundError) = false, want true")
		}
		if len(target.SearchPaths) != 1 || target.SearchPaths[0] != "/a" {
			t.Fatalf("target.SearchPaths = %v, want [/a]", target.SearchPaths)
		}
	})

	t.Run("CLIConnectionError", func(t *testing.T) {
		t.Parallel()
		err := error(&CLIConnectionError{Message: "EOF"})
		var target *CLIConnectionError
		if !errors.As(err, &target) {
			t.Fatal("errors.As(*CLIConnectionError) = false, want true")
		}
		if target.Message != "EOF" {
			t.Fatalf("target.Message = %q, want EOF", target.Message)
		}
	})

	t.Run("ProcessError", func(t *testing.T) {
		t.Parallel()
		err := error(&ProcessError{ExitCode: 127, StderrTail: "not found"})
		var target *ProcessError
		if !errors.As(err, &target) {
			t.Fatal("errors.As(*ProcessError) = false, want true")
		}
		if target.ExitCode != 127 {
			t.Fatalf("target.ExitCode = %d, want 127", target.ExitCode)
		}
		if target.StderrTail != "not found" {
			t.Fatalf("target.StderrTail = %q, want not found", target.StderrTail)
		}
	})

	t.Run("CLIJSONDecodeError", func(t *testing.T) {
		t.Parallel()
		raw := []byte("bad json")
		err := error(&CLIJSONDecodeError{Line: raw, Offset: 42})
		var target *CLIJSONDecodeError
		if !errors.As(err, &target) {
			t.Fatal("errors.As(*CLIJSONDecodeError) = false, want true")
		}
		if target.Offset != 42 {
			t.Fatalf("target.Offset = %d, want 42", target.Offset)
		}
		if string(target.Line) != "bad json" {
			t.Fatalf("target.Line = %q, want bad json", target.Line)
		}
	})
}

func TestErrorTaxonomyImplementsErrorInterface(t *testing.T) {
	t.Parallel()

	errs := []Error{
		&CLINotFoundError{SearchPaths: []string{"/bin/claude"}},
		&CLIConnectionError{Message: "test"},
		&ProcessError{ExitCode: 1},
		&CLIJSONDecodeError{Line: []byte("x"), Offset: 0},
	}

	for _, e := range errs {
		if e.Error() == "" {
			t.Errorf("%T.Error() = empty, want non-empty", e)
		}
	}
}
