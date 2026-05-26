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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNotepadLifecycle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	longPriority := strings.Repeat("p", 550)
	_, _, err := runTestCommand(t, &recordingBackend{}, nil, []string{"notepad", "write-priority", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "content": longPriority}), "--json"})
	if err != nil {
		t.Fatalf("write priority: %v", err)
	}
	_, _, err = runTestCommand(t, &recordingBackend{}, nil, []string{"notepad", "write-working", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "content": "current task"}), "--json"})
	if err != nil {
		t.Fatalf("write working: %v", err)
	}
	_, _, err = runTestCommand(t, &recordingBackend{}, nil, []string{"notepad", "write-manual", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "content": "manual survives"}), "--json"})
	if err != nil {
		t.Fatalf("write manual: %v", err)
	}

	stdout, stderr, err := runTestCommand(t, &recordingBackend{}, nil, []string{"notepad", "read", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "section": "priority"}), "--json"})
	if err != nil {
		t.Fatalf("read priority: %v\nstderr=%s", err, stderr)
	}
	priority := decodeJSONMap(t, stdout)
	if got := len(priority["content"].(string)); got != 500 {
		t.Fatalf("priority content should be truncated to 500 runes, got %d", got)
	}

	old := time.Now().Add(-14 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	notePath := filepath.Join(root, ".omx", "notepad.md")
	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read notepad file: %v", err)
	}
	updated := strings.Replace(string(content), "## WORKING MEMORY", "## WORKING MEMORY\n["+old+"] stale entry", 1)
	if err := os.WriteFile(notePath, []byte(updated), 0o644); err != nil {
		t.Fatalf("inject stale entry: %v", err)
	}
	stdout, stderr, err = runTestCommand(t, &recordingBackend{}, nil, []string{"notepad", "prune", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "daysOld": 7}), "--json"})
	if err != nil {
		t.Fatalf("prune: %v\nstderr=%s", err, stderr)
	}
	prune := decodeJSONMap(t, stdout)
	if prune["pruned"] != float64(1) {
		t.Fatalf("expected one pruned entry: %#v", prune)
	}
}

func TestNativeCommandsDefaultToOptionsCwd(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	var stdout strings.Builder
	var stderr strings.Builder
	command := NewRootCommand(Options{Backend: &recordingBackend{}, Env: map[string]string{}, Cwd: root, Stdout: &stdout, Stderr: &stderr})
	command.SetArgs([]string{"notepad", "write-working", "--input", jsonInput(t, map[string]any{"content": "cwd default"}), "--json"})
	if err := command.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("notepad write with Options.Cwd failed: %v\nstderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".omx", "notepad.md")); err != nil {
		t.Fatalf("native command did not write under Options.Cwd: %v", err)
	}
}

func TestProjectMemoryLifecycle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, _, err := runTestCommand(t, &recordingBackend{}, nil, []string{"project-memory", "write", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "memory": map[string]any{"techStack": "Go"}}), "--json"})
	if err != nil {
		t.Fatalf("write memory: %v", err)
	}
	_, _, err = runTestCommand(t, &recordingBackend{}, nil, []string{"project-memory", "add-note", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "category": "build", "content": "go test ./..."}), "--json"})
	if err != nil {
		t.Fatalf("add note: %v", err)
	}
	_, _, err = runTestCommand(t, &recordingBackend{}, nil, []string{"project-memory", "add-directive", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "directive": "preserve vendor", "priority": "high"}), "--json"})
	if err != nil {
		t.Fatalf("add directive: %v", err)
	}
	stdout, stderr, err := runTestCommand(t, &recordingBackend{}, nil, []string{"project-memory", "read", "--input", jsonInput(t, map[string]any{"workingDirectory": root}), "--json"})
	if err != nil {
		t.Fatalf("read memory: %v\nstderr=%s", err, stderr)
	}
	memory := decodeJSONMap(t, stdout)
	if memory["techStack"] != "Go" {
		t.Fatalf("techStack mismatch: %#v", memory)
	}
	if len(memory["notes"].([]any)) != 1 || len(memory["directives"].([]any)) != 1 {
		t.Fatalf("memory append counts mismatch: %#v", memory)
	}
}

func TestTraceTimelineAndSummary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logsDir := filepath.Join(root, ".omx", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	log := `{"timestamp":"2026-05-26T18:00:00Z","type":"user","thread_id":"t1","input_preview":"start"}` + "\n" +
		`{"timestamp":"2026-05-26T18:01:00Z","type":"assistant","thread_id":"t1","output_preview":"done"}` + "\n"
	if err := os.WriteFile(filepath.Join(logsDir, "turns-1.jsonl"), []byte(log), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	store := stateStore{env: map[string]string{}}
	if payload, failed := store.stateWrite(map[string]any{"mode": "autopilot", "workingDirectory": root, "active": true, "current_phase": "run", "started_at": "2026-05-26T17:59:00Z"}); failed {
		t.Fatalf("write state: %#v", payload)
	}
	stdout, stderr, err := runTestCommand(t, &recordingBackend{}, nil, []string{"trace", "timeline", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "filter": "all"}), "--json"})
	if err != nil {
		t.Fatalf("trace timeline: %v\nstderr=%s", err, stderr)
	}
	timeline := decodeJSONMap(t, stdout)
	if timeline["entryCount"] != float64(3) {
		t.Fatalf("timeline entry count mismatch: %#v", timeline)
	}

	stdout, stderr, err = runTestCommand(t, &recordingBackend{}, nil, []string{"trace", "summary", "--input", jsonInput(t, map[string]any{"workingDirectory": root}), "--json"})
	if err != nil {
		t.Fatalf("trace summary: %v\nstderr=%s", err, stderr)
	}
	summary := decodeJSONMap(t, stdout)
	turns := summary["turns"].(map[string]any)
	if turns["total"] != float64(2) {
		t.Fatalf("summary turns mismatch: %#v", summary)
	}
}

func TestWikiLifecycleAndLegacyFallback(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	addInput := map[string]any{"workingDirectory": root, "title": "Routing Decision", "content": "Use [[State Store]] for mode state.", "tags": []any{"routing", "state"}, "category": "decision"}
	_, _, err := runTestCommand(t, &recordingBackend{}, nil, []string{"wiki", "add", "--input", jsonInput(t, addInput), "--json"})
	if err != nil {
		t.Fatalf("wiki add: %v", err)
	}
	_, _, err = runTestCommand(t, &recordingBackend{}, nil, []string{"wiki", "add", "--input", jsonInput(t, addInput), "--json"})
	if err == nil {
		t.Fatalf("duplicate wiki add should fail")
	}
	stdout, stderr, err := runTestCommand(t, &recordingBackend{}, nil, []string{"wiki", "query", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "query": "routing", "limit": 5}), "--json"})
	if err != nil {
		t.Fatalf("wiki query: %v\nstderr=%s", err, stderr)
	}
	matches := decodeJSONAny(t, stdout).([]any)
	if len(matches) != 1 {
		t.Fatalf("expected one wiki query result, got %#v", matches)
	}
	stdout, stderr, err = runTestCommand(t, &recordingBackend{}, nil, []string{"wiki", "lint", "--input", jsonInput(t, map[string]any{"workingDirectory": root}), "--json"})
	if err != nil {
		t.Fatalf("wiki lint: %v\nstderr=%s", err, stderr)
	}
	lint := decodeJSONMap(t, stdout)
	if lint["stats"] == nil {
		t.Fatalf("lint missing stats: %#v", lint)
	}

	legacyRoot := t.TempDir()
	legacyDir := filepath.Join(legacyRoot, ".omx", "wiki")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy wiki: %v", err)
	}
	legacyPage := wikiPage{Filename: "legacy.md", Frontmatter: map[string]any{"title": "Legacy", "tags": []string{"old"}, "created": "2026-01-01T00:00:00Z", "updated": "2026-01-01T00:00:00Z", "sources": []string{}, "links": []string{}, "category": "reference", "confidence": "medium"}, Content: "\n# Legacy\n\nOld page.\n"}
	if err := os.WriteFile(filepath.Join(legacyDir, "legacy.md"), []byte(serializeWikiPage(legacyPage)), 0o644); err != nil {
		t.Fatalf("write legacy page: %v", err)
	}
	stdout, stderr, err = runTestCommand(t, &recordingBackend{}, nil, []string{"wiki", "list", "--input", jsonInput(t, map[string]any{"workingDirectory": legacyRoot}), "--json"})
	if err != nil {
		t.Fatalf("wiki legacy list: %v\nstderr=%s", err, stderr)
	}
	legacyList := decodeJSONMap(t, stdout)
	if len(legacyList["pages"].([]any)) != 1 {
		t.Fatalf("legacy fallback did not list page: %#v", legacyList)
	}
}

func TestProjectMemoryRejectsMalformedExistingFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, ".omx", "project-memory.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir memory dir: %v", err)
	}
	original := []byte("{bad json")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("write malformed memory: %v", err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "read", args: []string{"project-memory", "read", "--input", jsonInput(t, map[string]any{"workingDirectory": root}), "--json"}},
		{name: "replace", args: []string{"project-memory", "write", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "memory": map[string]any{"new": "value"}}), "--json"}},
		{name: "merge", args: []string{"project-memory", "write", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "merge": true, "memory": map[string]any{"new": "value"}}), "--json"}},
		{name: "append note", args: []string{"project-memory", "add-note", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "category": "build", "content": "note"}), "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runTestCommand(t, &recordingBackend{}, nil, tc.args)
			if err == nil {
				t.Fatalf("malformed project memory should fail")
			}
			preserved, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read preserved memory: %v", err)
			}
			if string(preserved) != string(original) {
				t.Fatalf("malformed memory bytes were not preserved: %q", preserved)
			}
		})
	}
}

func TestStoreWorkingDirectoryAllowlistRejectsOutsideRoot(t *testing.T) {
	t.Parallel()

	allowed := t.TempDir()
	outside := t.TempDir()
	_, _, err := runTestCommand(t, &recordingBackend{}, map[string]string{"OMX_MCP_WORKDIR_ROOTS": allowed}, []string{"notepad", "read", "--input", jsonInput(t, map[string]any{"workingDirectory": outside}), "--json"})
	if err == nil {
		t.Fatalf("outside workingDirectory should fail")
	}
}

func TestWikiRejectsPageTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, _, err := runTestCommand(t, &recordingBackend{}, nil, []string{"wiki", "read", "--input", jsonInput(t, map[string]any{"workingDirectory": root, "page": "../secret"}), "--json"})
	if err == nil {
		t.Fatalf("wiki traversal read should fail")
	}
}
