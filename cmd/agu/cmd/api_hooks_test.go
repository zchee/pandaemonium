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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
)

func TestAPIHooksCommandWritesLogUnderConfiguredStateHome(t *testing.T) {
	tests := map[string]struct {
		input   string
		wantLog string
	}{
		"success: session start hook is logged": {
			input: `{"cwd":"/work","hook_event_name":"SessionStart","model":"gpt-5.5","permission_mode":"default","session_id":"sess-1","source":"startup","transcript_path":null}`,
			wantLog: `{
  "cwd": "/work",
  "hook_event_name": "SessionStart",
  "model": "gpt-5.5",
  "permission_mode": "default",
  "session_id": "sess-1",
  "source": "startup"
}
`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			stateHome := t.TempDir()
			var stderr bytes.Buffer
			var stdout bytes.Buffer

			cmd := NewRootCommand(Options{
				Env: map[string]string{
					"XDG_STATE_HOME": stateHome,
				},
				Stdin:  strings.NewReader(tt.input),
				Stdout: &stdout,
				Stderr: &stderr,
			})
			cmd.SetArgs([]string{"api", "hooks"})

			if err := cmd.ExecuteContext(t.Context()); err != nil {
				t.Fatalf("ExecuteContext() returned error: %v\nstderr:\n%s", err, stderr.String())
			}

			got, err := os.ReadFile(filepath.Join(stateHome, "agu", "hooks.log.jsonl"))
			if err != nil {
				t.Fatalf("ReadFile() returned error: %v", err)
			}
			if diff := gocmp.Diff(tt.wantLog, string(got)); diff != "" {
				t.Fatalf("hooks log mismatch (-want +got):\n%s", diff)
			}
			if diff := gocmp.Diff("", stdout.String()); diff != "" {
				t.Fatalf("stdout mismatch (-want +got):\n%s", diff)
			}
			if diff := gocmp.Diff("", stderr.String()); diff != "" {
				t.Fatalf("stderr mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
