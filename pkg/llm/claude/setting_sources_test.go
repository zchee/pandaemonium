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

// TestSettingSource_Literals verifies the SettingSource enum values map to the
// exact wire literals the claude CLI expects (user|project|local).
func TestSettingSource_Literals(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src  SettingSource
		want string
	}{
		"user":    {SettingSourceUser, "user"},
		"project": {SettingSourceProject, "project"},
		"local":   {SettingSourceLocal, "local"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.src) != tt.want {
				t.Errorf("SettingSource = %q, want %q", string(tt.src), tt.want)
			}
		})
	}
}

// TestSettingSource_ZeroValue verifies the zero value is the empty string,
// which buildLaunchArgs skips.
func TestSettingSource_ZeroValue(t *testing.T) {
	t.Parallel()
	var ss SettingSource
	if ss != "" {
		t.Errorf("zero SettingSource = %q, want empty string", ss)
	}
}
