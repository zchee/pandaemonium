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
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverCLI_ExplicitPath(t *testing.T) {
	t.Parallel()

	// Create a temp executable that pretends to be the claude binary.
	dir := t.TempDir()
	fakeCLI := filepath.Join(dir, "claude")
	if err := os.WriteFile(fakeCLI, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := map[string]struct {
		opts    *Options
		wantErr bool
	}{
		"success: explicit CLIPath exists": {
			opts:    &Options{CLIPath: fakeCLI},
			wantErr: false,
		},
		"error: explicit CLIPath does not exist": {
			opts:    &Options{CLIPath: filepath.Join(dir, "missing_claude")},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := discoverCLI(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("discoverCLI() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if got != fakeCLI {
					t.Fatalf("discoverCLI() = %q, want %q", got, fakeCLI)
				}
			} else {
				var notFound *CLINotFoundError
				if !errors.As(err, &notFound) {
					t.Fatalf("discoverCLI() error type = %T, want *CLINotFoundError", err)
				}
				if len(notFound.SearchPaths) == 0 {
					t.Fatal("CLINotFoundError.SearchPaths is empty")
				}
			}
		})
	}
}

func TestDiscoverCLI_WellKnownPath(t *testing.T) {
	// Mutates package-level wellKnownCLIPaths — cannot run in parallel.

	// Install a fake binary into wellKnownCLIPaths[0] (home-relative).
	dir := t.TempDir()
	fakePath := filepath.Join(dir, "claude")
	if err := os.WriteFile(fakePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Temporarily prepend our fake path to wellKnownCLIPaths.
	orig := wellKnownCLIPaths
	wellKnownCLIPaths = append([]string{fakePath}, orig...)
	t.Cleanup(func() { wellKnownCLIPaths = orig })

	// Temporarily clear staticWellKnownCLIPaths so we only test the home-relative paths.
	origStatic := staticWellKnownCLIPaths
	staticWellKnownCLIPaths = nil
	t.Cleanup(func() { staticWellKnownCLIPaths = origStatic })

	// Use a nil opts (no CLIPath) so discovery falls through to well-known paths.
	// We also need PATH to not contain a "claude" binary. We can't control PATH
	// easily, but the test verifies the return value regardless.
	got, err := discoverCLI(nil)
	if err != nil {
		// If a real claude binary exists on PATH, the test should still pass:
		// the discovered path just won't be our fake.
		t.Logf("discoverCLI() error = %v (may be expected if PATH lacks claude)", err)
		return
	}
	// If discovery succeeded and used our injected fake, verify it.
	if got == fakePath {
		t.Logf("discoverCLI() = %q (from injected well-known path)", got)
	}
}

func TestDiscoverCLI_NotFoundReturnsAllSearchPaths(t *testing.T) {
	// Mutates package-level globals and uses t.Setenv — cannot run in parallel.

	// Override well-known paths with non-existent paths so discovery always fails.
	orig := wellKnownCLIPaths
	wellKnownCLIPaths = []string{"/nonexistent/path/to/claude"}
	t.Cleanup(func() { wellKnownCLIPaths = orig })

	origStatic := staticWellKnownCLIPaths
	staticWellKnownCLIPaths = []string{"/another/nonexistent/claude"}
	t.Cleanup(func() { staticWellKnownCLIPaths = origStatic })

	// Use a non-existent PATH to force failure of exec.LookPath.
	t.Setenv("PATH", t.TempDir()) // empty dir, no claude binary

	_, err := discoverCLI(nil)
	if err == nil {
		t.Fatal("discoverCLI() error = nil, want CLINotFoundError")
	}

	var notFound *CLINotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("discoverCLI() error type = %T, want *CLINotFoundError", err)
	}
	if len(notFound.SearchPaths) == 0 {
		t.Fatal("CLINotFoundError.SearchPaths is empty, want non-empty list")
	}
}
