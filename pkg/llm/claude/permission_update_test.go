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
	"testing"
)

// TestPermissionUpdateDestination_Literals verifies wire literals match
// upstream PermissionUpdateDestination (types.py:105-107).
func TestPermissionUpdateDestination_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		d    PermissionUpdateDestination
		want string
	}{
		"user":    {PermissionUpdateDestinationUserSettings, "userSettings"},
		"project": {PermissionUpdateDestinationProjectSettings, "projectSettings"},
		"local":   {PermissionUpdateDestinationLocalSettings, "localSettings"},
		"session": {PermissionUpdateDestinationSession, "session"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.d) != tt.want {
				t.Errorf("destination = %q, want %q", string(tt.d), tt.want)
			}
		})
	}
}

// TestPermissionBehavior_Literals verifies wire literals match upstream
// PermissionBehavior (types.py:109).
func TestPermissionBehavior_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		b    PermissionBehavior
		want string
	}{
		"allow": {PermissionBehaviorAllow, "allow"},
		"deny":  {PermissionBehaviorDeny, "deny"},
		"ask":   {PermissionBehaviorAsk, "ask"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.b) != tt.want {
				t.Errorf("behavior = %q, want %q", string(tt.b), tt.want)
			}
		})
	}
}

// TestPermissionUpdateType_Literals verifies wire literals match upstream
// PermissionUpdate.type (types.py:124-131).
func TestPermissionUpdateType_Literals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		typ  PermissionUpdateType
		want string
	}{
		"addRules":          {PermissionUpdateTypeAddRules, "addRules"},
		"replaceRules":      {PermissionUpdateTypeReplaceRules, "replaceRules"},
		"removeRules":       {PermissionUpdateTypeRemoveRules, "removeRules"},
		"setMode":           {PermissionUpdateTypeSetMode, "setMode"},
		"addDirectories":    {PermissionUpdateTypeAddDirectories, "addDirectories"},
		"removeDirectories": {PermissionUpdateTypeRemoveDirectories, "removeDirectories"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if string(tt.typ) != tt.want {
				t.Errorf("type = %q, want %q", string(tt.typ), tt.want)
			}
		})
	}
}

// TestPermissionUpdate_ToWire exercises the discriminated field emission per
// upstream PermissionUpdate.to_dict (types.py:138-172). For each Type the
// wire object must carry exactly the right keys — mismatched fields on the
// Go struct are silently dropped.
func TestPermissionUpdate_ToWire(t *testing.T) {
	t.Parallel()

	t.Run("addRules emits rules+behavior with toolName/ruleContent renames", func(t *testing.T) {
		t.Parallel()
		u := PermissionUpdate{
			Type:        PermissionUpdateTypeAddRules,
			Rules:       []PermissionRuleValue{{ToolName: "Bash", RuleContent: "echo *"}},
			Behavior:    PermissionBehaviorAllow,
			Destination: PermissionUpdateDestinationSession,
			Mode:        PermissionModePlan, // must be dropped — wrong variant
			Directories: []string{"/tmp"},   // must be dropped — wrong variant
		}
		got := u.ToWire()
		want := map[string]any{
			"type":        "addRules",
			"destination": "session",
			"behavior":    "allow",
			"rules": []map[string]any{
				{"toolName": "Bash", "ruleContent": "echo *"},
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("addRules wire:\n got: %#v\nwant: %#v", got, want)
		}
	})

	t.Run("setMode emits mode only", func(t *testing.T) {
		t.Parallel()
		u := PermissionUpdate{
			Type:        PermissionUpdateTypeSetMode,
			Mode:        PermissionModeAcceptEdits,
			Destination: PermissionUpdateDestinationUserSettings,
			Rules:       []PermissionRuleValue{{ToolName: "X"}}, // must drop
			Behavior:    PermissionBehaviorDeny,                 // must drop
			Directories: []string{"/x"},                         // must drop
		}
		got := u.ToWire()
		want := map[string]any{
			"type":        "setMode",
			"destination": "userSettings",
			"mode":        "acceptEdits",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("setMode wire:\n got: %#v\nwant: %#v", got, want)
		}
	})

	t.Run("addDirectories emits directories only", func(t *testing.T) {
		t.Parallel()
		u := PermissionUpdate{
			Type:        PermissionUpdateTypeAddDirectories,
			Directories: []string{"/work/src", "/work/docs"},
			Destination: PermissionUpdateDestinationProjectSettings,
			Rules:       []PermissionRuleValue{{ToolName: "X"}}, // must drop
			Behavior:    PermissionBehaviorAsk,                  // must drop
			Mode:        PermissionModePlan,                     // must drop
		}
		got := u.ToWire()
		want := map[string]any{
			"type":        "addDirectories",
			"destination": "projectSettings",
			"directories": []string{"/work/src", "/work/docs"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("addDirectories wire:\n got: %#v\nwant: %#v", got, want)
		}
	})

	t.Run("destination omitted when empty", func(t *testing.T) {
		t.Parallel()
		u := PermissionUpdate{
			Type: PermissionUpdateTypeSetMode,
			Mode: PermissionModeAuto,
		}
		got := u.ToWire()
		if _, has := got["destination"]; has {
			t.Errorf("destination must be omitted when empty: %v", got)
		}
	})

	t.Run("removeRules with nil Rules omits the rules key", func(t *testing.T) {
		t.Parallel()
		u := PermissionUpdate{Type: PermissionUpdateTypeRemoveRules, Behavior: PermissionBehaviorDeny}
		got := u.ToWire()
		if _, has := got["rules"]; has {
			t.Errorf("nil Rules must omit rules key: %v", got)
		}
		if got["behavior"] != "deny" {
			t.Errorf("behavior = %v, want deny", got["behavior"])
		}
	})
}

// TestPermissionUpdate_FromWire is the inverse pin: a wire-shaped dict must
// decode back into the equivalent Go struct, including the camelCase rename.
func TestPermissionUpdate_FromWire(t *testing.T) {
	t.Parallel()

	t.Run("addRules round-trip", func(t *testing.T) {
		t.Parallel()
		in := map[string]any{
			"type":        "addRules",
			"destination": "localSettings",
			"behavior":    "deny",
			"rules": []any{
				map[string]any{"toolName": "WebFetch", "ruleContent": "http://*"},
			},
		}
		got := PermissionUpdateFromWire(in)
		want := PermissionUpdate{
			Type:        PermissionUpdateTypeAddRules,
			Destination: PermissionUpdateDestinationLocalSettings,
			Behavior:    PermissionBehaviorDeny,
			Rules:       []PermissionRuleValue{{ToolName: "WebFetch", RuleContent: "http://*"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("fromWire:\n got: %#v\nwant: %#v", got, want)
		}
	})

	t.Run("setMode round-trip", func(t *testing.T) {
		t.Parallel()
		in := map[string]any{"type": "setMode", "mode": "plan", "destination": "session"}
		got := PermissionUpdateFromWire(in)
		want := PermissionUpdate{
			Type:        PermissionUpdateTypeSetMode,
			Mode:        PermissionModePlan,
			Destination: PermissionUpdateDestinationSession,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("fromWire setMode:\n got: %#v\nwant: %#v", got, want)
		}
	})

	t.Run("addDirectories round-trip", func(t *testing.T) {
		t.Parallel()
		in := map[string]any{
			"type":        "addDirectories",
			"directories": []any{"/a", "/b"},
		}
		got := PermissionUpdateFromWire(in)
		want := PermissionUpdate{
			Type:        PermissionUpdateTypeAddDirectories,
			Directories: []string{"/a", "/b"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("fromWire addDirectories:\n got: %#v\nwant: %#v", got, want)
		}
	})
}
