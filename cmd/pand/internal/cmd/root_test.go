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
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-json-experiment/json"
	gocmp "github.com/google/go-cmp/cmp"
)

type backendCall struct {
	Args []string
	Env  map[string]string
	Dir  string
}

type recordingBackend struct {
	calls []backendCall
	err   error
}

func (b *recordingBackend) Run(_ context.Context, req BackendRequest) error {
	b.calls = append(b.calls, backendCall{Args: append([]string(nil), req.Args...), Env: cloneStringMap(req.Env), Dir: req.Dir})
	return b.err
}

func TestRootCommandDelegatesRuntimeCommands(t *testing.T) {
	t.Parallel()

	backend := &recordingBackend{}
	stdout, stderr, err := runTestCommand(t, backend, map[string]string{"PATH": "/tmp/bin"}, []string{"team", "--flag", "value"})
	if err != nil {
		t.Fatalf("pand team returned error: %v\nstderr=%s", err, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("delegated command wrote unexpected output stdout=%q stderr=%q", stdout, stderr)
	}
	want := []backendCall{{Args: []string{"team", "--flag", "value"}, Env: map[string]string{"PATH": "/tmp/bin"}}}
	if diff := gocmp.Diff(want, backend.calls); diff != "" {
		t.Fatalf("backend calls mismatch (-want +got):\n%s", diff)
	}
}

func TestRootCommandKeepsNativeCommandsLocal(t *testing.T) {
	t.Parallel()

	backend := &recordingBackend{}
	stdout, stderr, err := runTestCommand(t, backend, map[string]string{"PATH": "/tmp/bin"}, []string{"version"})
	if err != nil {
		t.Fatalf("pand version returned error: %v\nstderr=%s", err, stderr)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("native version delegated unexpectedly: %#v", backend.calls)
	}
	if stdout == "" {
		t.Fatalf("version command produced no stdout")
	}
}

func runTestCommand(t *testing.T, backend Backend, env map[string]string, args []string) (string, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := NewRootCommand(Options{Backend: backend, Env: env, Stdout: &stdout, Stderr: &stderr})
	command.SetArgs(args)
	err := command.ExecuteContext(t.Context())
	return stdout.String(), stderr.String(), err
}

func jsonInput(t *testing.T, input map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return string(raw)
}

func decodeJSONMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal JSON %q: %v", raw, err)
	}
	return got
}

func decodeJSONAny(t *testing.T, raw string) any {
	t.Helper()
	var got any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal JSON %q: %v", raw, err)
	}
	return got
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func TestRootHelpCommandIsNative(t *testing.T) {
	t.Parallel()

	backend := &recordingBackend{}
	stdout, stderr, err := runTestCommand(t, backend, map[string]string{"PATH": "/tmp/bin"}, []string{"help"})
	if err != nil {
		t.Fatalf("pand help returned error: %v\nstderr=%s", err, stderr)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("help delegated unexpectedly: %#v", backend.calls)
	}
	for _, want := range []string{"state", "notepad", "project-memory", "team"} {
		if !bytes.Contains([]byte(stdout), []byte(want)) {
			t.Fatalf("help output missing %q:\n%s", want, stdout)
		}
	}
}

func TestRootHelpFlagIsNative(t *testing.T) {
	t.Parallel()

	backend := &recordingBackend{}
	stdout, stderr, err := runTestCommand(t, backend, map[string]string{"PATH": "/tmp/bin"}, []string{"--help"})
	if err != nil {
		t.Fatalf("pand --help returned error: %v\nstderr=%s", err, stderr)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("--help delegated unexpectedly: %#v", backend.calls)
	}
	if !bytes.Contains([]byte(stdout), []byte("pand ports")) {
		t.Fatalf("help output missing root description:\n%s", stdout)
	}
}

func TestRootFlagsDelegateToBackend(t *testing.T) {
	t.Parallel()

	backend := &recordingBackend{}
	_, stderr, err := runTestCommand(t, backend, map[string]string{"PATH": "/tmp/bin"}, []string{"--yolo", "--dry-run"})
	if err != nil {
		t.Fatalf("pand root flags returned error: %v\nstderr=%s", err, stderr)
	}
	want := []backendCall{{Args: []string{"--yolo", "--dry-run"}, Env: map[string]string{"PATH": "/tmp/bin"}}}
	if diff := gocmp.Diff(want, backend.calls); diff != "" {
		t.Fatalf("backend calls mismatch (-want +got):\n%s", diff)
	}
}

func TestSystemBackendPropagatesExitCode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	backendPath := filepath.Join(root, "omx")
	if err := os.WriteFile(backendPath, []byte("#!/bin/sh\necho backend stderr >&2\nexit 7\n"), 0o755); err != nil {
		t.Fatalf("write fake backend: %v", err)
	}
	var stderr bytes.Buffer
	err := SystemBackend{}.Run(t.Context(), BackendRequest{Args: []string{"team"}, Env: map[string]string{"PAND_OMX_BINARY": backendPath}, Stderr: &stderr})
	var exitErr ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T %[1]v", err)
	}
	if exitErr.Code() != 7 {
		t.Fatalf("exit code mismatch: %d", exitErr.Code())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("backend stderr")) {
		t.Fatalf("stderr was not propagated: %q", stderr.String())
	}
}
