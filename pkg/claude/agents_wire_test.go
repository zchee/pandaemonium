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
	"context"
	"reflect"
	"testing"
)

// TestAgentsWire_MCPServerConfigsInlineOnly verifies that an AgentDefinition
// with only inline-config entries (no name-strings) emits each as a
// {serverName: <config>} dict on the wire — the dict variant of upstream's
// list[str | dict] shape.
func TestAgentsWire_MCPServerConfigsInlineOnly(t *testing.T) {
	t.Parallel()
	cp := newControlProtocol(&Options{
		Agents: []AgentDefinition{{
			Name: "subagent",
			MCPServerConfigs: map[string]MCPServer{
				"local-fs": &MCPStdioServerConfig{
					MCPName: "local-fs",
					Command: "mcp-fs",
					Args:    []string{"--root", "/srv"},
				},
			},
		}},
	}, func(_ context.Context, _ []byte) error { return nil })

	got := cp.agentsWire()["subagent"].(map[string]any)
	mcp, ok := got["mcpServers"].([]any)
	if !ok || len(mcp) != 1 {
		t.Fatalf("mcpServers = %v, want one inline-config entry", got["mcpServers"])
	}
	entry, ok := mcp[0].(map[string]any)
	if !ok {
		t.Fatalf("mcp[0] = %T, want map[string]any (dict variant)", mcp[0])
	}
	cfg, ok := entry["local-fs"].(map[string]any)
	if !ok {
		t.Fatalf("entry = %v, want {local-fs: {...}}", entry)
	}
	if cfg["type"] != "stdio" || cfg["command"] != "mcp-fs" {
		t.Errorf("inline config = %v, want stdio mcp-fs", cfg)
	}
}

// TestAgentsWire_MCPServersMixedShape pins the merge contract: when both
// MCPServers (name strings) and MCPServerConfigs (inline configs) are set,
// string entries appear first in their slice order, then inline-config
// entries in sorted-key order.
func TestAgentsWire_MCPServersMixedShape(t *testing.T) {
	t.Parallel()
	cp := newControlProtocol(&Options{
		Agents: []AgentDefinition{{
			Name:       "mixed",
			MCPServers: []string{"named-a", "named-b"},
			MCPServerConfigs: map[string]MCPServer{
				"zeta": &MCPSSEServerConfig{MCPName: "zeta", URL: "https://z.example"},
				"beta": &MCPHTTPServerConfig{MCPName: "beta", URL: "https://b.example"},
			},
		}},
	}, func(_ context.Context, _ []byte) error { return nil })

	got := cp.agentsWire()["mixed"].(map[string]any)
	mcp, ok := got["mcpServers"].([]any)
	if !ok || len(mcp) != 4 {
		t.Fatalf("mcpServers len = %d, want 4 (2 named + 2 inline); got=%v", len(mcp), got["mcpServers"])
	}
	// Names in original slice order.
	if mcp[0] != "named-a" {
		t.Errorf("mcp[0] = %v, want named-a (string in slice order)", mcp[0])
	}
	if mcp[1] != "named-b" {
		t.Errorf("mcp[1] = %v, want named-b", mcp[1])
	}
	// Inline configs in sorted-key order: beta < zeta.
	beta, ok := mcp[2].(map[string]any)
	if !ok || beta["beta"] == nil {
		t.Errorf("mcp[2] = %v, want {beta: ...} (inline configs sorted by key)", mcp[2])
	}
	zeta, ok := mcp[3].(map[string]any)
	if !ok || zeta["zeta"] == nil {
		t.Errorf("mcp[3] = %v, want {zeta: ...}", mcp[3])
	}
	// Verify the inner config types survived.
	betaCfg := beta["beta"].(map[string]any)
	if betaCfg["type"] != "http" {
		t.Errorf("beta inline config type = %v, want http", betaCfg["type"])
	}
	zetaCfg := zeta["zeta"].(map[string]any)
	if zetaCfg["type"] != "sse" {
		t.Errorf("zeta inline config type = %v, want sse", zetaCfg["type"])
	}
}

// TestAgentsWire_MCPServersBothEmpty verifies that when neither MCPServers
// nor MCPServerConfigs is populated, the mcpServers wire key is omitted
// entirely.
func TestAgentsWire_MCPServersBothEmpty(t *testing.T) {
	t.Parallel()
	cp := newControlProtocol(&Options{
		Agents: []AgentDefinition{{Name: "empty", Description: "d"}},
	}, func(_ context.Context, _ []byte) error { return nil })

	got := cp.agentsWire()["empty"].(map[string]any)
	if _, has := got["mcpServers"]; has {
		t.Errorf("mcpServers wire key present with no MCPServers/MCPServerConfigs: %v", got)
	}
}

// TestMergeMCPServersWire_SortDeterministic verifies the inline-config sort
// is stable across iterations — pins that map iteration randomness cannot
// leak into the wire payload.
func TestMergeMCPServersWire_SortDeterministic(t *testing.T) {
	t.Parallel()
	configs := map[string]MCPServer{
		"gamma": &MCPStdioServerConfig{MCPName: "gamma", Command: "g"},
		"alpha": &MCPStdioServerConfig{MCPName: "alpha", Command: "a"},
		"beta":  &MCPStdioServerConfig{MCPName: "beta", Command: "b"},
	}
	want := []any{
		map[string]any{"alpha": map[string]any{"type": "stdio", "command": "a"}},
		map[string]any{"beta": map[string]any{"type": "stdio", "command": "b"}},
		map[string]any{"gamma": map[string]any{"type": "stdio", "command": "g"}},
	}
	// Run the merge many times — any single non-sorted iteration would fail.
	for i := 0; i < 50; i++ {
		got := mergeMCPServersWire(nil, configs)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("iteration %d: got = %v, want sorted alpha/beta/gamma", i, got)
		}
	}
}
