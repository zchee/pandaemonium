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

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type pathResolver struct {
	env map[string]string
	cwd string
}

func (r pathResolver) workingDirectory(raw string) (string, error) {
	if strings.Contains(raw, "\x00") {
		return "", fmt.Errorf("workingDirectory contains a NUL byte")
	}
	if strings.TrimSpace(raw) == "" {
		if strings.TrimSpace(r.cwd) != "" {
			raw = r.cwd
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				return "", err
			}
			raw = cwd
		}
	}
	if strings.Contains(raw, "\x00") {
		return "", fmt.Errorf("workingDirectory contains a NUL byte")
	}
	resolved, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	return r.enforceWorkingDirectoryPolicy(resolved)
}

func (r pathResolver) enforceWorkingDirectoryPolicy(path string) (string, error) {
	rootsRaw := r.env["OMX_MCP_WORKDIR_ROOTS"]
	if strings.TrimSpace(rootsRaw) == "" {
		return path, nil
	}
	candidate := canonicalizePath(path)
	for _, rawRoot := range filepath.SplitList(rootsRaw) {
		if strings.TrimSpace(rawRoot) == "" {
			continue
		}
		if strings.Contains(rawRoot, "\x00") {
			return "", fmt.Errorf("OMX_MCP_WORKDIR_ROOTS contains an invalid root with a NUL byte")
		}
		rootAbs, err := filepath.Abs(rawRoot)
		if err != nil {
			return "", err
		}
		root := canonicalizePath(rootAbs)
		rel, err := filepath.Rel(root, candidate)
		if err == nil && (rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("workingDirectory %q is outside allowed roots (OMX_MCP_WORKDIR_ROOTS)", candidate)
}

func (r pathResolver) baseStateDir(wd string) (string, error) {
	if override := strings.TrimSpace(r.env["OMX_TEAM_STATE_ROOT"]); override != "" {
		return r.workingDirectory(override)
	}
	if override := strings.TrimSpace(r.env["OMX_ROOT"]); override != "" {
		root, err := r.workingDirectory(override)
		if err != nil {
			return "", err
		}
		return filepath.Join(root, ".omx", "state"), nil
	}
	if override := strings.TrimSpace(r.env["OMX_STATE_ROOT"]); override != "" {
		root, err := r.workingDirectory(override)
		if err != nil {
			return "", err
		}
		return filepath.Join(root, ".omx", "state"), nil
	}
	return filepath.Join(wd, ".omx", "state"), nil
}

func canonicalizePath(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return filepath.Clean(path)
}
