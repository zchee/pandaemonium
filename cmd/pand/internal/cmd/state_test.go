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
	"testing"
)

func TestStateWriteStatusAndCancel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	env := map[string]string{"PATH": "/tmp/bin"}
	input := map[string]any{
		"mode":             "autopilot",
		"workingDirectory": root,
		"state": map[string]any{
			"active":        true,
			"current_phase": "ultragoal",
			"started_at":    "2026-05-26T18:00:00Z",
		},
	}
	stdout, stderr, err := runTestCommand(t, &recordingBackend{}, env, []string{"state", "write", "--input", jsonInput(t, input), "--json"})
	if err != nil {
		t.Fatalf("state write failed: %v\nstderr=%s", err, stderr)
	}
	writePayload := decodeJSONMap(t, stdout)
	if writePayload["success"] != true {
		t.Fatalf("state write did not report success: %#v", writePayload)
	}

	stdout, stderr, err = runTestCommand(t, &recordingBackend{}, env, []string{"status", "--input", jsonInput(t, map[string]any{"workingDirectory": root}), "--json"})
	if err != nil {
		t.Fatalf("status failed: %v\nstderr=%s", err, stderr)
	}
	statusPayload := decodeJSONMap(t, stdout)
	statuses := statusPayload["statuses"].(map[string]any)
	autopilot := statuses["autopilot"].(map[string]any)
	if autopilot["active"] != true || autopilot["phase"] != "ultragoal" {
		t.Fatalf("autopilot status mismatch: %#v", autopilot)
	}

	stdout, stderr, err = runTestCommand(t, &recordingBackend{}, env, []string{"cancel", "--input", jsonInput(t, map[string]any{"workingDirectory": root}), "--json"})
	if err != nil {
		t.Fatalf("cancel failed: %v\nstderr=%s", err, stderr)
	}
	cancelPayload := decodeJSONMap(t, stdout)
	if cancelPayload["cancelled"] != float64(1) {
		t.Fatalf("cancelled count mismatch: %#v", cancelPayload)
	}
	if _, err := os.Stat(filepath.Join(root, ".omx", "state", "autopilot-state.json")); !os.IsNotExist(err) {
		t.Fatalf("autopilot state should be removed after cancel, stat err=%v", err)
	}
}

func TestStateExplicitSessionFallbackRules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := stateStore{env: map[string]string{}}
	baseInput := map[string]any{"mode": "autopilot", "workingDirectory": root, "active": true, "current_phase": "root"}
	if payload, failed := store.stateWrite(baseInput); failed {
		t.Fatalf("write root state failed: %#v", payload)
	}

	missingSessionRead, failed := store.stateRead(map[string]any{"mode": "autopilot", "workingDirectory": root, "session_id": "missing"})
	if failed {
		t.Fatalf("missing explicit session read failed: %#v", missingSessionRead)
	}
	if got := stringField(missingSessionRead.(map[string]any), "current_phase"); got != "root" {
		t.Fatalf("missing explicit session should fall back to root, got phase %q", got)
	}

	if err := os.MkdirAll(filepath.Join(root, ".omx", "state", "sessions", "present"), 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}
	presentSessionRead, failed := store.stateRead(map[string]any{"mode": "autopilot", "workingDirectory": root, "session_id": "present"})
	if failed {
		t.Fatalf("present explicit session read failed: %#v", presentSessionRead)
	}
	presentPayload := presentSessionRead.(map[string]any)
	if presentPayload["exists"] != false {
		t.Fatalf("present explicit session should not fall back to root: %#v", presentPayload)
	}
}

func TestStateReadRejectsUnsafeModes(t *testing.T) {
	t.Parallel()

	store := stateStore{env: map[string]string{}}
	payload, failed := store.stateRead(map[string]any{"mode": "../../secrets", "workingDirectory": t.TempDir()})
	if !failed {
		t.Fatalf("unsafe mode should fail, got %#v", payload)
	}
}

func TestStateWriteRejectsMalformedExistingState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	statePath := filepath.Join(root, ".omx", "state", "autopilot-state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	original := []byte("{bad json")
	if err := os.WriteFile(statePath, original, 0o644); err != nil {
		t.Fatalf("write malformed state: %v", err)
	}
	store := stateStore{env: map[string]string{}}
	payload, failed := store.stateWrite(map[string]any{"mode": "autopilot", "workingDirectory": root, "active": true})
	if !failed {
		t.Fatalf("malformed state should fail, got %#v", payload)
	}
	preserved, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read preserved state: %v", err)
	}
	if string(preserved) != string(original) {
		t.Fatalf("malformed state bytes were not preserved: %q", preserved)
	}
}

func TestStateWriteSurfacesSkillActiveSyncFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillPath := filepath.Join(root, ".omx", "state", "skill-active-state.json")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write malformed skill-active: %v", err)
	}
	store := stateStore{env: map[string]string{}}
	payload, failed := store.stateWrite(map[string]any{"mode": "autopilot", "workingDirectory": root, "active": true})
	if !failed {
		t.Fatalf("skill-active sync failure should fail, got %#v", payload)
	}
}
