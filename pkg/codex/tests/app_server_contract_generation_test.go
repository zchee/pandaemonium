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
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const contractGenerationPinnedSchema = "https://raw.githubusercontent.com/openai/codex/refs/tags/rust-v0.131.0-alpha.9/codex-rs/app-server-protocol/schema/json/codex_app_server_protocol.v2.schemas.json"

func TestContractGenerationPortGeneratedFilesAreUpToDate(t *testing.T) {
	repoRoot := artifactWorkflowRepoRoot(t)
	packageRoot := filepath.Join(repoRoot, "pkg", "codex")
	checkedInPath := filepath.Join(packageRoot, "protocol_gen.go")
	generatedPath := filepath.Join(t.TempDir(), "protocol_gen.go")

	before, err := os.ReadFile(checkedInPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", checkedInPath, err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "go", "run", "./internal/cmd/generate-protocol-types",
		"-schema", contractGenerationPinnedSchema,
		"-out", generatedPath,
		"-package", "codex",
	)
	cmd.Dir = packageRoot
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=auto")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", strings.Join(cmd.Args, " "), err, output)
	}

	after, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", generatedPath, err)
	}
	if bytes.Equal(before, after) {
		return
	}

	diff := contractGenerationDiff(t, checkedInPath, generatedPath)
	t.Fatalf("Generated files drifted after regeneration from %s.\n%s", contractGenerationPinnedSchema, diff)
}

func contractGenerationDiff(t *testing.T, checkedInPath, generatedPath string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "diff", "-u",
		"--label", "pkg/codex/protocol_gen.go",
		"--label", "regenerated protocol_gen.go",
		checkedInPath,
		generatedPath,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return "diff unexpectedly reported no changes"
	}
	if len(output) == 0 {
		return "diff produced no output"
	}
	return contractGenerationBoundedOutput(string(output), 12_000)
}

func contractGenerationBoundedOutput(output string, maxBytes int) string {
	if len(output) <= maxBytes {
		return output
	}
	truncated := output[:maxBytes]
	if cut := strings.LastIndexByte(truncated, '\n'); cut > 0 {
		truncated = truncated[:cut+1]
	}
	return truncated + "\n... diff output truncated ...\n"
}
