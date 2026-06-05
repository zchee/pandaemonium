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

package codex_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type codexBinaryProbe struct {
	path    string
	version string
	err     error
}

func findCodexBinaryForGeneratedProvenance(ctx context.Context, expectedVersion string) (string, []codexBinaryProbe) {
	probes := make([]codexBinaryProbe, 0)
	seen := make(map[string]struct{})
	for _, candidate := range codexBinaryCandidates() {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		version, err := readCodexBinaryVersion(ctx, candidate)
		probe := codexBinaryProbe{path: candidate, version: version, err: err}
		probes = append(probes, probe)
		if err == nil && version == expectedVersion {
			return candidate, probes
		}
	}
	return "", probes
}

func codexBinaryCandidates() []string {
	candidates := make([]string, 0, 16)
	if path, err := exec.LookPath("codex"); err == nil {
		candidates = append(candidates, path)
		if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
			candidates = append(candidates, resolved)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		patterns := []string{
			filepath.Join(home, ".config", "codex", "packages", "standalone", "releases", "*", "bin", "codex"),
			filepath.Join(home, ".config", "codex", "packages", "standalone", "releases", "*", "codex"),
		}
		for _, pattern := range patterns {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				continue
			}
			candidates = append(candidates, matches...)
		}
	}
	return candidates
}

func readCodexBinaryVersion(ctx context.Context, codexBin string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, codexBin, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0]), nil
}

func formatCodexBinaryProbes(probes []codexBinaryProbe) string {
	if len(probes) == 0 {
		return "none"
	}
	lines := make([]string, 0, len(probes))
	for _, probe := range probes {
		if probe.err != nil {
			lines = append(lines, fmt.Sprintf("%s: %v", probe.path, probe.err))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", probe.path, probe.version))
	}
	return strings.Join(lines, "\n")
}
