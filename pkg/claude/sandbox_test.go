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
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-json-experiment/json"
)

// TestSandboxSettings_JSONTagsParity is the single discriminator that catches
// every struct-tag typo at once: a fully populated SandboxSettings must
// marshal to an object keyed exactly as upstream's TypedDicts
// (types.py:835,861,873). Adding a new field without the right `json:` tag
// will break this test.
func TestSandboxSettings_JSONTagsParity(t *testing.T) {
	t.Parallel()

	in := SandboxSettings{
		Enabled:                   true,
		AutoAllowBashIfSandboxed:  BoolPtr(false),
		ExcludedCommands:          []string{"git", "docker"},
		AllowUnsandboxedCommands:  BoolPtr(false),
		EnableWeakerNestedSandbox: true,
		Network: SandboxNetworkConfig{
			AllowedDomains:          []string{"a.example.com"},
			DeniedDomains:           []string{"b.example.com"},
			AllowManagedDomainsOnly: true,
			AllowUnixSockets:        []string{"/var/run/docker.sock"},
			AllowAllUnixSockets:     true,
			AllowLocalBinding:       true,
			AllowMachLookup:         []string{"com.example.*"},
			HTTPProxyPort:           8080,
			SocksProxyPort:          1080,
		},
		IgnoreViolations: SandboxIgnoreViolations{
			File:    []string{"/tmp/foo"},
			Network: []string{"flaky.host"},
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := map[string]any{
		"enabled":                   true,
		"autoAllowBashIfSandboxed":  false,
		"excludedCommands":          []any{"git", "docker"},
		"allowUnsandboxedCommands":  false,
		"enableWeakerNestedSandbox": true,
		"network": map[string]any{
			"allowedDomains":          []any{"a.example.com"},
			"deniedDomains":           []any{"b.example.com"},
			"allowManagedDomainsOnly": true,
			"allowUnixSockets":        []any{"/var/run/docker.sock"},
			"allowAllUnixSockets":     true,
			"allowLocalBinding":       true,
			"allowMachLookup":         []any{"com.example.*"},
			"httpProxyPort":           float64(8080),
			"socksProxyPort":          float64(1080),
		},
		"ignoreViolations": map[string]any{
			"file":    []any{"/tmp/foo"},
			"network": []any{"flaky.host"},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("SandboxSettings JSON mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// TestSandboxSettings_DefaultTrueBoolsOmitWhenNil verifies that nil *bool for
// the upstream-default-true fields produces no key on the wire (so the CLI's
// default applies), and that BoolPtr(false) does send an explicit false.
func TestSandboxSettings_DefaultTrueBoolsOmitWhenNil(t *testing.T) {
	t.Parallel()

	in := SandboxSettings{}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, has := got["autoAllowBashIfSandboxed"]; has {
		t.Errorf("zero-value SandboxSettings must omit autoAllowBashIfSandboxed: %v", got)
	}
	if _, has := got["allowUnsandboxedCommands"]; has {
		t.Errorf("zero-value SandboxSettings must omit allowUnsandboxedCommands: %v", got)
	}

	in2 := SandboxSettings{
		AutoAllowBashIfSandboxed: BoolPtr(false),
		AllowUnsandboxedCommands: BoolPtr(true),
	}
	data2, _ := json.Marshal(in2)
	var got2 map[string]any
	_ = json.Unmarshal(data2, &got2)
	if got2["autoAllowBashIfSandboxed"] != false {
		t.Errorf("BoolPtr(false) must send false; got %v", got2["autoAllowBashIfSandboxed"])
	}
	if got2["allowUnsandboxedCommands"] != true {
		t.Errorf("BoolPtr(true) must send true; got %v", got2["allowUnsandboxedCommands"])
	}
}

// TestBuildSettingsValue covers the upstream _build_settings_value branches
// (subprocess_cli.py:129-181).
func TestBuildSettingsValue(t *testing.T) {
	t.Parallel()

	t.Run("neither set returns empty", func(t *testing.T) {
		t.Parallel()
		v, err := buildSettingsValue(&Options{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if v != "" {
			t.Errorf("v = %q, want empty", v)
		}
	})

	t.Run("settings alone passes through verbatim", func(t *testing.T) {
		t.Parallel()
		path := "/etc/claude/settings.json"
		v, err := buildSettingsValue(&Options{Settings: path})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if v != path {
			t.Errorf("v = %q, want %q", v, path)
		}
	})

	t.Run("sandbox alone produces sandbox-only object", func(t *testing.T) {
		t.Parallel()
		v, err := buildSettingsValue(&Options{
			Sandbox: &SandboxSettings{Enabled: true},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(v), &obj); err != nil {
			t.Fatalf("unmarshal merged: %v\nvalue: %s", err, v)
		}
		if len(obj) != 1 {
			t.Errorf("sandbox-only object must have only 'sandbox' key: %v", obj)
		}
		sb, _ := obj["sandbox"].(map[string]any)
		if sb["enabled"] != true {
			t.Errorf("sandbox.enabled = %v, want true", sb["enabled"])
		}
	})

	t.Run("inline JSON merged with sandbox preserves keys", func(t *testing.T) {
		t.Parallel()
		v, err := buildSettingsValue(&Options{
			Settings: `{"theme": "dark", "fontSize": 14}`,
			Sandbox:  &SandboxSettings{Enabled: true},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(v), &obj); err != nil {
			t.Fatalf("unmarshal merged: %v", err)
		}
		if obj["theme"] != "dark" {
			t.Errorf("theme key lost: %v", obj)
		}
		if obj["fontSize"] != float64(14) {
			t.Errorf("fontSize key lost: %v", obj)
		}
		if _, ok := obj["sandbox"]; !ok {
			t.Errorf("sandbox key missing after merge: %v", obj)
		}
	})

	t.Run("existing settings file merged with sandbox", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		if err := os.WriteFile(path, []byte(`{"existing": "yes"}`), 0o600); err != nil {
			t.Fatalf("write tmp settings: %v", err)
		}
		v, err := buildSettingsValue(&Options{
			Settings: path,
			Sandbox:  &SandboxSettings{Enabled: true},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(v), &obj); err != nil {
			t.Fatalf("unmarshal merged: %v", err)
		}
		if obj["existing"] != "yes" {
			t.Errorf("settings file content not merged: %v", obj)
		}
		if _, ok := obj["sandbox"]; !ok {
			t.Errorf("sandbox not merged: %v", obj)
		}
	})

	t.Run("missing settings file falls through with sandbox only", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "does-not-exist.json")
		v, err := buildSettingsValue(&Options{
			Settings: path,
			Sandbox:  &SandboxSettings{Enabled: true},
		})
		if err != nil {
			t.Fatalf("err = %v, want nil (upstream warn-and-continue)", err)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(v), &obj); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(obj) != 1 {
			t.Errorf("missing-file fallback must produce sandbox-only object: %v", obj)
		}
		if _, ok := obj["sandbox"]; !ok {
			t.Errorf("sandbox missing in fallback object: %v", obj)
		}
	})

	t.Run("malformed inline JSON with sandbox errors", func(t *testing.T) {
		t.Parallel()
		_, err := buildSettingsValue(&Options{
			Settings: `{not valid json}`,
			Sandbox:  &SandboxSettings{Enabled: true},
		})
		if err == nil {
			t.Fatalf("expected error on malformed inline JSON")
		}
	})

	t.Run("malformed inline JSON without sandbox passes through", func(t *testing.T) {
		t.Parallel()
		s := `{not valid json}`
		v, err := buildSettingsValue(&Options{Settings: s})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if v != s {
			t.Errorf("without Sandbox, malformed literal must pass through; got %q", v)
		}
	})
}

// TestBuildLaunchArgs_Sandbox verifies the --settings flag emits the merged
// JSON when Options.Sandbox is set.
func TestBuildLaunchArgs_Sandbox(t *testing.T) {
	t.Parallel()
	const fakeCLI = "/usr/local/bin/claude"

	t.Run("sandbox alone emits merged --settings", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{
			Sandbox: &SandboxSettings{
				Enabled:          true,
				ExcludedCommands: []string{"docker"},
			},
		}, "")
		v := argValue(got, "--settings")
		if v == "" {
			t.Fatalf("--settings missing: %v", got)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(v), &obj); err != nil {
			t.Fatalf("--settings not JSON: %v\nvalue: %s", err, v)
		}
		sb, _ := obj["sandbox"].(map[string]any)
		if sb["enabled"] != true {
			t.Errorf("sandbox.enabled = %v, want true", sb["enabled"])
		}
		cmds, _ := sb["excludedCommands"].([]any)
		if len(cmds) != 1 || cmds[0] != "docker" {
			t.Errorf("excludedCommands = %v, want [docker]", cmds)
		}
	})

	t.Run("settings path alone passes through verbatim", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{Settings: "/etc/claude.json"}, "")
		if v := argValue(got, "--settings"); v != "/etc/claude.json" {
			t.Errorf("--settings = %q, want /etc/claude.json", v)
		}
	})

	t.Run("neither set omits --settings", func(t *testing.T) {
		t.Parallel()
		got := mustLaunchArgs(t, fakeCLI, &Options{}, "")
		if extraArgIndex(got, "--settings") != -1 {
			t.Errorf("unset Settings+Sandbox must omit --settings: %v", got)
		}
	})
}

// TestEffectiveToolsAndSources_CarryoverCopies verifies the M9 carryover fix:
// effectiveToolsAndSources must not return the caller's SettingSources slice
// directly (any append-to-effSources would otherwise mutate it).
func TestEffectiveToolsAndSources_CarryoverCopies(t *testing.T) {
	t.Parallel()
	caller := make([]SettingSource, 1, 4) // spare capacity to make aliasing observable
	caller[0] = SettingSourceUser
	opts := &Options{SettingSources: caller}
	_, sources := effectiveToolsAndSources(opts)
	sources = append(sources, SettingSourceProject)
	if len(caller) != 1 || caller[0] != SettingSourceUser {
		t.Errorf("caller's SettingSources mutated by effectiveToolsAndSources append: %v", caller)
	}
}
