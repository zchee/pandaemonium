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
	"os"
	"os/exec"
	"path/filepath"
)

// wellKnownCLIPaths are the installation directories searched (in order) after
// exec.LookPath("claude") fails. Mirrors the Python SDK's hard-coded list.
var wellKnownCLIPaths = []string{
	// Populated at package init from os.UserHomeDir.
}

func init() {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	wellKnownCLIPaths = []string{
		filepath.Join(home, ".claude", "local", "claude"),
	}
}

// staticWellKnownCLIPaths holds paths that do not depend on the home directory.
var staticWellKnownCLIPaths = []string{
	"/opt/homebrew/bin/claude",
	"/usr/local/bin/claude",
}

// discoverCLI resolves the claude CLI binary path from opts.
//
// Discovery order (AC6):
//  1. opts.CLIPath — if non-empty, the binary must exist at that exact path.
//     Returns CLINotFoundError immediately if it does not.
//  2. exec.LookPath("claude") — searches the directories in PATH.
//  3. Well-known install directories (in order):
//     ~/.claude/local/claude, /opt/homebrew/bin/claude, /usr/local/bin/claude.
//
// Returns CLINotFoundError with every searched path if all fail.
func discoverCLI(opts *Options) (string, error) {
	// 1. Explicit override: fail fast if the binary is not at that path.
	if opts != nil && opts.CLIPath != "" {
		if _, err := os.Stat(opts.CLIPath); err == nil {
			return opts.CLIPath, nil
		}
		return "", &CLINotFoundError{SearchPaths: []string{opts.CLIPath}}
	}

	var searched []string

	// 2. PATH lookup.
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}
	searched = append(searched, "claude (PATH)")

	// 3. Well-known home-relative paths.
	for _, p := range wellKnownCLIPaths {
		searched = append(searched, p)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// 4. Well-known static paths.
	for _, p := range staticWellKnownCLIPaths {
		searched = append(searched, p)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", &CLINotFoundError{SearchPaths: searched}
}
