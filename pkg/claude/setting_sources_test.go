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

func TestSettingSource_ZeroValue(t *testing.T) {
	t.Parallel()
	var ss SettingSource
	if ss.Path != "" {
		t.Errorf("zero SettingSource.Path = %q, want empty", ss.Path)
	}
	if ss.URL != "" {
		t.Errorf("zero SettingSource.URL = %q, want empty", ss.URL)
	}
}

func TestSettingSource_Fields(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src      SettingSource
		wantPath string
		wantURL  string
	}{
		"success: path-based setting source": {
			src:      SettingSource{Path: "/etc/claude/settings.json"},
			wantPath: "/etc/claude/settings.json",
			wantURL:  "",
		},
		"success: URL-based setting source": {
			src:      SettingSource{URL: "https://example.com/settings.json"},
			wantPath: "",
			wantURL:  "https://example.com/settings.json",
		},
		"success: both path and URL set": {
			src:      SettingSource{Path: "/local/settings.json", URL: "https://remote/settings.json"},
			wantPath: "/local/settings.json",
			wantURL:  "https://remote/settings.json",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if tt.src.Path != tt.wantPath {
				t.Errorf("SettingSource.Path = %q, want %q", tt.src.Path, tt.wantPath)
			}
			if tt.src.URL != tt.wantURL {
				t.Errorf("SettingSource.URL = %q, want %q", tt.src.URL, tt.wantURL)
			}
		})
	}
}
