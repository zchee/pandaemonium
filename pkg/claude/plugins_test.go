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
	"testing"
)

func TestPlugin_ZeroValue(t *testing.T) {
	t.Parallel()
	var p Plugin
	if p.Name != "" {
		t.Errorf("zero Plugin.Name = %q, want empty", p.Name)
	}
	if p.Path != "" {
		t.Errorf("zero Plugin.Path = %q, want empty", p.Path)
	}
}

func TestPlugin_Fields(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		plugin   Plugin
		wantName string
		wantPath string
	}{
		"success: name and path set": {
			plugin:   Plugin{Name: "myplugin", Path: "/opt/plugins/myplugin"},
			wantName: "myplugin",
			wantPath: "/opt/plugins/myplugin",
		},
		"success: name only": {
			plugin:   Plugin{Name: "nameonly"},
			wantName: "nameonly",
			wantPath: "",
		},
		"success: path only": {
			plugin:   Plugin{Path: "/some/path"},
			wantName: "",
			wantPath: "/some/path",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if tt.plugin.Name != tt.wantName {
				t.Errorf("Plugin.Name = %q, want %q", tt.plugin.Name, tt.wantName)
			}
			if tt.plugin.Path != tt.wantPath {
				t.Errorf("Plugin.Path = %q, want %q", tt.plugin.Path, tt.wantPath)
			}
		})
	}
}
