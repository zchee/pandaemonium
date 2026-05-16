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

package toml_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

var benchOnlyCompetitorPackages = []string{
	"github.com/BurntSushi/toml",
	"github.com/pelletier/go-toml/v2",
}

func TestCompetitorDependenciesAreBenchOnly(t *testing.T) {
	t.Parallel()

	repoRoot := mustRepoPath(t, ".")
	for _, tc := range []struct {
		name          string
		args          []string
		wantPresent   bool
		wantExactPkgs []string
	}{
		{
			name: "production package graph excludes competitors",
			args: []string{"list", "-deps", "./pkg/toml/..."},
		},
		{
			name: "normal test graph excludes competitors",
			args: []string{"list", "-deps", "-test", "./pkg/toml/..."},
		},
		{
			name:          "bench-tag test graph includes competitors",
			args:          []string{"list", "-deps", "-test", "-tags=bench", "./pkg/toml/..."},
			wantPresent:   true,
			wantExactPkgs: benchOnlyCompetitorPackages,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deps := goListDeps(t, repoRoot, tc.args...)
			if tc.wantPresent {
				assertDependencyGraphContains(t, deps, tc.wantExactPkgs)
				return
			}
			assertDependencyGraphExcludes(t, deps, benchOnlyCompetitorPackages)
		})
	}
}

func goListDeps(t *testing.T, dir string, args ...string) []string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("go %s timed out after 30s\n%s", strings.Join(args, " "), out)
	}
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}

	return strings.Fields(string(out))
}

func assertDependencyGraphContains(t *testing.T, deps []string, want []string) {
	t.Helper()

	have := dependencySet(deps)
	for _, pkg := range want {
		if !have[pkg] {
			t.Fatalf("dependency graph is missing %s; competitor deps = %v", pkg, competitorDeps(deps))
		}
	}
}

func assertDependencyGraphExcludes(t *testing.T, deps []string, competitors []string) {
	t.Helper()

	if got := competitorDeps(deps); len(got) != 0 {
		t.Fatalf("dependency graph includes bench-only competitors: %v", got)
	}

	have := dependencySet(deps)
	for _, pkg := range competitors {
		if have[pkg] {
			t.Fatalf("dependency graph includes bench-only competitor root %s", pkg)
		}
	}
}

func dependencySet(deps []string) map[string]bool {
	set := make(map[string]bool, len(deps))
	for _, dep := range deps {
		set[dep] = true
	}
	return set
}

func competitorDeps(deps []string) []string {
	var got []string
	for _, dep := range deps {
		if strings.Contains(dep, "BurntSushi") || strings.Contains(dep, "pelletier") {
			got = append(got, dep)
		}
	}
	return got
}
