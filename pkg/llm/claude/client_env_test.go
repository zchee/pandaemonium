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
	"strings"
	"testing"
)

// envMap parses a "KEY=VALUE" slice into a map for assertions.
func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			m[k] = v
		}
	}
	return m
}

func TestBuildSubprocessEnv(t *testing.T) {
	t.Parallel()

	inherited := []string{
		"PATH=/usr/bin",
		"HOME=/home/u",
		"CLAUDECODE=1",
		"TRACEPARENT=00-trace-span-01",
	}

	t.Run("drops CLAUDECODE", func(t *testing.T) {
		t.Parallel()
		env := envMap(buildSubprocessEnv(inherited, &Options{}, ""))
		if _, ok := env["CLAUDECODE"]; ok {
			t.Error("CLAUDECODE present, want dropped")
		}
	})

	t.Run("injects SDK identity", func(t *testing.T) {
		t.Parallel()
		env := envMap(buildSubprocessEnv(inherited, &Options{}, ""))
		if env["CLAUDE_CODE_ENTRYPOINT"] != sdkEntrypoint {
			t.Errorf("CLAUDE_CODE_ENTRYPOINT = %q, want %q", env["CLAUDE_CODE_ENTRYPOINT"], sdkEntrypoint)
		}
		if env["CLAUDE_AGENT_SDK_VERSION"] != sdkVersion {
			t.Errorf("CLAUDE_AGENT_SDK_VERSION = %q, want %q", env["CLAUDE_AGENT_SDK_VERSION"], sdkVersion)
		}
	})

	t.Run("preserves inherited values and trace context", func(t *testing.T) {
		t.Parallel()
		env := envMap(buildSubprocessEnv(inherited, &Options{}, ""))
		if env["PATH"] != "/usr/bin" {
			t.Errorf("PATH = %q, want /usr/bin", env["PATH"])
		}
		if env["TRACEPARENT"] != "00-trace-span-01" {
			t.Errorf("TRACEPARENT = %q, want it preserved", env["TRACEPARENT"])
		}
	})

	t.Run("sets PWD only when cwd is non-empty", func(t *testing.T) {
		t.Parallel()
		withCwd := envMap(buildSubprocessEnv(inherited, &Options{}, "/work/dir"))
		if withCwd["PWD"] != "/work/dir" {
			t.Errorf("PWD = %q, want /work/dir", withCwd["PWD"])
		}
		noCwd := envMap(buildSubprocessEnv(inherited, &Options{}, ""))
		if _, ok := noCwd["PWD"]; ok {
			t.Error("PWD present with empty cwd, want absent")
		}
	})

	t.Run("opts.Env overrides injected and inherited keys", func(t *testing.T) {
		t.Parallel()
		opts := &Options{Env: map[string]string{
			"PATH":                   "/custom/bin",
			"CLAUDE_CODE_ENTRYPOINT": "custom-entry",
			"MY_VAR":                 "x",
		}}
		env := envMap(buildSubprocessEnv(inherited, opts, "/work"))
		if env["PATH"] != "/custom/bin" {
			t.Errorf("PATH = %q, want user override /custom/bin", env["PATH"])
		}
		if env["CLAUDE_CODE_ENTRYPOINT"] != "custom-entry" {
			t.Errorf("CLAUDE_CODE_ENTRYPOINT = %q, want user override", env["CLAUDE_CODE_ENTRYPOINT"])
		}
		if env["MY_VAR"] != "x" {
			t.Errorf("MY_VAR = %q, want x", env["MY_VAR"])
		}
	})

	t.Run("no duplicate keys in output", func(t *testing.T) {
		t.Parallel()
		opts := &Options{Env: map[string]string{"PATH": "/custom"}}
		out := buildSubprocessEnv(inherited, opts, "/work")
		seen := make(map[string]bool)
		for _, kv := range out {
			k, _, _ := strings.Cut(kv, "=")
			if seen[k] {
				t.Errorf("duplicate key %q in env output", k)
			}
			seen[k] = true
		}
	})
}
