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
	"reflect"
	"strings"
	"testing"

	"github.com/go-json-experiment/json"
)

// ── compile-time interface satisfaction ──────────────────────────────────────

var (
	_ MCPServer = (*MCPStdioServerConfig)(nil)
	_ MCPServer = (*MCPSSEServerConfig)(nil)
	_ MCPServer = (*MCPHTTPServerConfig)(nil)
)

// TestMCPStdioServerConfig_ConfigForCLI verifies the wire shape matches
// upstream McpStdioServerConfig (types.py:602-609) exactly.
func TestMCPStdioServerConfig_ConfigForCLI(t *testing.T) {
	t.Parallel()

	t.Run("full payload", func(t *testing.T) {
		t.Parallel()
		cfg := &MCPStdioServerConfig{
			MCPName: "fs",
			Command: "mcp-fs",
			Args:    []string{"--root", "/srv"},
			Env:     map[string]string{"TOKEN": "abc"},
		}
		got := cfg.configForCLI()
		want := map[string]any{
			"type":    "stdio",
			"command": "mcp-fs",
			"args":    []string{"--root", "/srv"},
			"env":     map[string]string{"TOKEN": "abc"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("configForCLI:\n got: %#v\nwant: %#v", got, want)
		}
	})

	t.Run("nil args/env omit keys", func(t *testing.T) {
		t.Parallel()
		cfg := &MCPStdioServerConfig{MCPName: "fs", Command: "mcp-fs"}
		got := cfg.configForCLI()
		if _, has := got["args"]; has {
			t.Errorf("nil Args must omit args key: %v", got)
		}
		if _, has := got["env"]; has {
			t.Errorf("nil Env must omit env key: %v", got)
		}
		if got["type"] != "stdio" || got["command"] != "mcp-fs" {
			t.Errorf("minimal stdio = %v, want type=stdio command=mcp-fs", got)
		}
	})

	t.Run("empty args/env slice omit keys", func(t *testing.T) {
		t.Parallel()
		cfg := &MCPStdioServerConfig{
			MCPName: "fs",
			Command: "mcp-fs",
			Args:    []string{},
			Env:     map[string]string{},
		}
		got := cfg.configForCLI()
		if _, has := got["args"]; has {
			t.Errorf("empty Args must omit args key: %v", got)
		}
		if _, has := got["env"]; has {
			t.Errorf("empty Env must omit env key: %v", got)
		}
	})
}

// TestMCPSSEServerConfig_ConfigForCLI verifies the wire shape matches
// upstream McpSSEServerConfig (types.py:611-617). Note the type literal
// "sse" must NOT collide with HTTP's "http".
func TestMCPSSEServerConfig_ConfigForCLI(t *testing.T) {
	t.Parallel()
	cfg := &MCPSSEServerConfig{
		MCPName: "events",
		URL:     "https://example.com/sse",
		Headers: map[string]string{"X-Trace": "abc"},
	}
	got := cfg.configForCLI()
	want := map[string]any{
		"type":    "sse",
		"url":     "https://example.com/sse",
		"headers": map[string]string{"X-Trace": "abc"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SSE configForCLI:\n got: %#v\nwant: %#v", got, want)
	}

	cfg2 := &MCPSSEServerConfig{MCPName: "events", URL: "https://example.com/sse"}
	got2 := cfg2.configForCLI()
	if _, has := got2["headers"]; has {
		t.Errorf("nil Headers must omit key: %v", got2)
	}
}

// TestMCPHTTPServerConfig_ConfigForCLI verifies the wire shape matches
// upstream McpHttpServerConfig (types.py:619-625). The type literal must
// be "http", NOT "sse" or "https" — easy typo to make.
func TestMCPHTTPServerConfig_ConfigForCLI(t *testing.T) {
	t.Parallel()
	cfg := &MCPHTTPServerConfig{
		MCPName: "tools",
		URL:     "https://api.example.com/mcp",
		Headers: map[string]string{"Authorization": "Bearer xyz"},
	}
	got := cfg.configForCLI()
	want := map[string]any{
		"type":    "http",
		"url":     "https://api.example.com/mcp",
		"headers": map[string]string{"Authorization": "Bearer xyz"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("HTTP configForCLI:\n got: %#v\nwant: %#v", got, want)
	}
}

// TestMCPExternalServer_TypeDiscriminators is the discriminator test the
// advisor flagged: HTTP and SSE share field shapes (URL, Headers) so a typo
// like "sse" leaking into HTTP's configForCLI would silently misroute. This
// test pins them apart explicitly.
func TestMCPExternalServer_TypeDiscriminators(t *testing.T) {
	t.Parallel()
	if (&MCPStdioServerConfig{Command: "x"}).configForCLI()["type"] != "stdio" {
		t.Error("stdio must emit type=stdio")
	}
	if (&MCPSSEServerConfig{URL: "x"}).configForCLI()["type"] != "sse" {
		t.Error("sse must emit type=sse")
	}
	if (&MCPHTTPServerConfig{URL: "x"}).configForCLI()["type"] != "http" {
		t.Error("http must emit type=http (NOT sse, NOT https)")
	}
}

// TestMCPExternalServer_Metadata verifies the [MCPServer] interface methods
// that aren't configForCLI.
func TestMCPExternalServer_Metadata(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		s        MCPServer
		wantName string
		wantMode MCPServerMode
	}{
		"stdio": {&MCPStdioServerConfig{MCPName: "s", Command: "c"}, "s", MCPServerModeStdio},
		"sse":   {&MCPSSEServerConfig{MCPName: "e", URL: "u"}, "e", MCPServerModeSSE},
		"http":  {&MCPHTTPServerConfig{MCPName: "h", URL: "u"}, "h", MCPServerModeHTTP},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tt.s.Name(); got != tt.wantName {
				t.Errorf("Name = %q, want %q", got, tt.wantName)
			}
			if got := tt.s.Mode(); got != tt.wantMode {
				t.Errorf("Mode = %q, want %q", got, tt.wantMode)
			}
			if got := tt.s.Version(); got != "" {
				t.Errorf("Version = %q, want empty (external servers carry no SDK-side version)", got)
			}
			if err := tt.s.Close(); err != nil {
				t.Errorf("Close = %v, want nil (external servers hold no SDK-side resources)", err)
			}
		})
	}
}

func TestBuildLaunchArgs_MCPConfigDeterministic(t *testing.T) {
	t.Parallel()

	opts := &Options{MCPServers: []MCPServer{
		&MCPStdioServerConfig{MCPName: "zeta", Command: "z", Env: map[string]string{"B": "2", "A": "1"}},
		&MCPStdioServerConfig{MCPName: "alpha", Command: "a", Env: map[string]string{"D": "4", "C": "3"}},
	}}
	var first string
	for i := range 20 {
		args := mustLaunchArgs(t, "/bin/claude", opts, "")
		cfg := argValue(args, "--mcp-config")
		if cfg == "" {
			t.Fatalf("iteration %d: --mcp-config missing: %v", i, args)
		}
		if first == "" {
			first = cfg
			continue
		}
		if cfg != first {
			t.Fatalf("iteration %d: --mcp-config changed\nfirst: %s\n  got: %s", i, first, cfg)
		}
	}
	want := `{"mcpServers":{"alpha":{"command":"a","env":{"C":"3","D":"4"},"type":"stdio"},"zeta":{"command":"z","env":{"A":"1","B":"2"},"type":"stdio"}}}`
	if first != want {
		t.Fatalf("--mcp-config = %s\nwant %s", first, want)
	}
}

// TestBuildLaunchArgs_MCPServersMixed verifies the launch wiring builds
// --mcp-config with every variant alongside the existing in-process server,
// each emitting its own wire shape under the mcpServers map.
func TestBuildLaunchArgs_MCPServersMixed(t *testing.T) {
	t.Parallel()
	opts := &Options{
		MCPServers: []MCPServer{
			&MCPStdioServerConfig{MCPName: "fs", Command: "mcp-fs", Args: []string{"--root", "/srv"}},
			&MCPSSEServerConfig{MCPName: "events", URL: "https://example.com/sse"},
			&MCPHTTPServerConfig{MCPName: "tools", URL: "https://api.example.com/mcp"},
			NewSDKMCPServer("my-tools", "1.0.0"),
		},
	}
	args := mustLaunchArgs(t, "/bin/claude", opts, "")
	cfg := argValue(args, "--mcp-config")
	if cfg == "" {
		t.Fatalf("--mcp-config missing: %v", args)
	}

	var payload struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(cfg), &payload); err != nil {
		t.Fatalf("decode --mcp-config: %v\nvalue=%s", err, cfg)
	}

	checks := map[string]struct {
		typ string
		key string // key whose presence confirms the variant
	}{
		"fs":       {"stdio", "command"},
		"events":   {"sse", "url"},
		"tools":    {"http", "url"},
		"my-tools": {"sdk", "name"},
	}
	for name, want := range checks {
		entry, ok := payload.MCPServers[name]
		if !ok {
			t.Errorf("--mcp-config missing server %q: %v", name, payload.MCPServers)
			continue
		}
		if entry["type"] != want.typ {
			t.Errorf("server %q type = %v, want %q", name, entry["type"], want.typ)
		}
		if _, has := entry[want.key]; !has {
			t.Errorf("server %q missing key %q: %v", name, want.key, entry)
		}
	}

	// HTTP and SSE share URL/Headers shape; pin that the type literal differs.
	if strings.Contains(cfg, `"type":"https"`) {
		t.Errorf("--mcp-config contains type=https (likely typo): %s", cfg)
	}
}
