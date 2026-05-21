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

// TestPermissionMode_Literals verifies the PermissionMode constants serialize
// to the exact wire literals the claude CLI expects, matching upstream
// PermissionMode = Literal["default", "acceptEdits", "plan",
// "bypassPermissions", "dontAsk", "auto"] (types.py:23-25).
func TestPermissionMode_Literals(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mode PermissionMode
		want string
	}{
		"default":           {PermissionModeDefault, "default"},
		"acceptEdits":       {PermissionModeAcceptEdits, "acceptEdits"},
		"plan":              {PermissionModePlan, "plan"},
		"bypassPermissions": {PermissionModeBypassPermissions, "bypassPermissions"},
		"dontAsk":           {PermissionModeDontAsk, "dontAsk"},
		"auto":              {PermissionModeAuto, "auto"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.mode) != tt.want {
				t.Errorf("PermissionMode = %q, want %q", string(tt.mode), tt.want)
			}
		})
	}
}

// TestPermissionMode_ZeroValue verifies the zero value is the empty string,
// which buildLaunchArgs skips so the CLI uses its configured default.
func TestPermissionMode_ZeroValue(t *testing.T) {
	t.Parallel()
	var pm PermissionMode
	if pm != "" {
		t.Errorf("zero PermissionMode = %q, want empty string", pm)
	}
}
