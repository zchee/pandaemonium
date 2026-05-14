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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestArtifactWorkflowPortGenerationHasSingleGoGenerateEntrypoint(t *testing.T) {
	repoRoot := artifactWorkflowRepoRoot(t)
	generatePath := filepath.Join(repoRoot, "pkg", "codex", "generate.go")
	body, err := os.ReadFile(generatePath)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", generatePath, err)
	}

	var directives []string
	for line := range strings.SplitSeq(string(body), "\n") {
		if after, ok := strings.CutPrefix(line, "//go:generate "); ok {
			directives = append(directives, after)
		}
	}
	if len(directives) != 1 {
		t.Fatalf("go:generate directives = %#v, want exactly one protocol generation entrypoint", directives)
	}

	directive := directives[0]
	for _, want := range []string{
		"go run ./internal/cmd/generate-protocol-types",
		"https://raw.githubusercontent.com/openai/codex/refs/tags/rust-v0.131.0-alpha.9/codex-rs/app-server-protocol/schema/json/codex_app_server_protocol.v2.schemas.json",
		"-out ./protocol_gen.go",
		"-package codex",
	} {
		if !strings.Contains(directive, want) {
			t.Fatalf("go:generate directive missing %q:\n%s", want, directive)
		}
	}
}

func TestArtifactWorkflowPortSDKHasNoCheckedInRuntimeBinaries(t *testing.T) {
	packageRoot := filepath.Join(artifactWorkflowRepoRoot(t), "pkg", "codex")
	var offenders []string
	err := filepath.WalkDir(packageRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == "bin" {
				offenders = append(offenders, artifactWorkflowRel(t, packageRoot, path)+"/")
			}
			return nil
		}
		if artifactWorkflowLooksLikeRuntimeBinary(name) {
			offenders = append(offenders, artifactWorkflowRel(t, packageRoot, path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("filepath.WalkDir(%s) error = %v", packageRoot, err)
	}
	if len(offenders) != 0 {
		t.Fatalf("checked-in runtime binary artifacts under pkg/codex = %v, want none", offenders)
	}
}

func TestArtifactWorkflowPortMissingCodexBinaryRequiresOverride(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-codex-binary")
	client := codex.NewClient(&codex.Config{CodexBin: missing}, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	t.Cleanup(cancel)

	err := client.Start(ctx)
	if err == nil {
		t.Fatal("Client.Start() error = nil, want missing binary error")
	}
	if !strings.Contains(err.Error(), "codex binary not found") && !strings.Contains(err.Error(), "locate codex binary") {
		t.Fatalf("Client.Start() error = %v, want codex binary resolution failure", err)
	}
}

func TestArtifactWorkflowPortLaunchArgsOverrideTakesPriority(t *testing.T) {
	cfg := helperCodexConfig(t, "lifecycle_inputs")
	cfg.CodexBin = filepath.Join(t.TempDir(), "missing-codex-binary")
	client := codex.NewClient(cfg, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Client.Start() with LaunchArgsOverride and missing CodexBin error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Client.Close() error = %v", err)
		}
	})
	metadata, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Client.Initialize() error = %v", err)
	}
	if metadata.ServerInfo == nil || metadata.ServerInfo.Name != "codex-test" || metadata.ServerInfo.Version != "1.2.3" {
		t.Fatalf("Initialize() metadata = %#v, want helper app-server metadata", metadata)
	}
}

func artifactWorkflowRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func artifactWorkflowRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("filepath.Rel(%s, %s) error = %v", root, path, err)
	}
	return filepath.ToSlash(rel)
}

func artifactWorkflowLooksLikeRuntimeBinary(name string) bool {
	switch name {
	case "codex", "codex.exe":
		return true
	}
	for _, suffix := range []string{".dll", ".dylib", ".so"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
