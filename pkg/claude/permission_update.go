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

// PermissionUpdateDestination identifies where a [PermissionUpdate] is
// applied. Mirrors upstream PermissionUpdateDestination (types.py:105).
type PermissionUpdateDestination string

const (
	// PermissionUpdateDestinationUserSettings writes to the per-user
	// settings layer.
	PermissionUpdateDestinationUserSettings PermissionUpdateDestination = "userSettings"

	// PermissionUpdateDestinationProjectSettings writes to the project
	// settings layer (.claude/ in the project tree).
	PermissionUpdateDestinationProjectSettings PermissionUpdateDestination = "projectSettings"

	// PermissionUpdateDestinationLocalSettings writes to the per-machine
	// settings layer (.claude/settings.local.json).
	PermissionUpdateDestinationLocalSettings PermissionUpdateDestination = "localSettings"

	// PermissionUpdateDestinationSession applies the update for the
	// duration of the current session only.
	PermissionUpdateDestinationSession PermissionUpdateDestination = "session"
)

// PermissionBehavior is the verdict carried by an addRules/replaceRules/
// removeRules [PermissionUpdate]. Mirrors upstream PermissionBehavior
// (types.py:109).
type PermissionBehavior string

const (
	// PermissionBehaviorAllow permits matching tool calls.
	PermissionBehaviorAllow PermissionBehavior = "allow"

	// PermissionBehaviorDeny blocks matching tool calls.
	PermissionBehaviorDeny PermissionBehavior = "deny"

	// PermissionBehaviorAsk forces an interactive prompt for matching tool
	// calls.
	PermissionBehaviorAsk PermissionBehavior = "ask"
)

// PermissionUpdateType identifies the variant of a [PermissionUpdate]. The
// emitted wire shape depends on the type (see [PermissionUpdate.ToWire]):
// rules-based variants carry rules + behavior, setMode carries mode,
// directory variants carry directories.
type PermissionUpdateType string

const (
	// PermissionUpdateTypeAddRules appends rules with the carried behavior.
	PermissionUpdateTypeAddRules PermissionUpdateType = "addRules"

	// PermissionUpdateTypeReplaceRules replaces the matching ruleset with
	// the carried rules and behavior.
	PermissionUpdateTypeReplaceRules PermissionUpdateType = "replaceRules"

	// PermissionUpdateTypeRemoveRules removes the carried rules at the
	// carried behavior.
	PermissionUpdateTypeRemoveRules PermissionUpdateType = "removeRules"

	// PermissionUpdateTypeSetMode sets the active [PermissionMode] for the
	// session.
	PermissionUpdateTypeSetMode PermissionUpdateType = "setMode"

	// PermissionUpdateTypeAddDirectories grants access to the carried
	// directories.
	PermissionUpdateTypeAddDirectories PermissionUpdateType = "addDirectories"

	// PermissionUpdateTypeRemoveDirectories revokes access to the carried
	// directories.
	PermissionUpdateTypeRemoveDirectories PermissionUpdateType = "removeDirectories"
)

// PermissionRuleValue is a single permission rule. Mirrors upstream
// PermissionRuleValue (types.py:112). The wire field names are camelCase
// (`toolName`, `ruleContent`); the Go struct uses Go-conventional names and
// the ToWire/wireToRule helpers translate.
type PermissionRuleValue struct {
	// ToolName is the tool the rule applies to (wire key: toolName).
	ToolName string

	// RuleContent is the rule body. Empty means no extra constraint
	// (wire key: ruleContent, optional).
	RuleContent string
}

// PermissionUpdate is one entry in the permission_updates list sent on the
// control protocol (and accepted as a CLI suggestion alongside CanUseTool
// requests). Mirrors upstream PermissionUpdate (types.py:120).
//
// The Type field discriminates which other fields are carried on the wire:
//
//   - AddRules/ReplaceRules/RemoveRules: Rules + Behavior
//   - SetMode: Mode
//   - AddDirectories/RemoveDirectories: Directories
//
// Destination is always emitted when set, regardless of Type. Use [PermissionUpdate.ToWire]
// to produce the wire object the CLI expects; [PermissionUpdateFromWire]
// inverts it. The two together preserve the only round-trip parity that
// matters for this type — the Go struct intentionally carries every variant's
// fields to mirror upstream's dataclass shape, and the wire layer prunes them
// per-Type.
type PermissionUpdate struct {
	// Type discriminates the wire variant. Required.
	Type PermissionUpdateType

	// Rules carries the rule list for the addRules/replaceRules/removeRules
	// variants. Ignored for other types.
	Rules []PermissionRuleValue

	// Behavior carries the verdict for the rules-based variants. Ignored for
	// other types. The zero value (empty string) omits the wire key.
	Behavior PermissionBehavior

	// Mode carries the [PermissionMode] for the setMode variant. Ignored for
	// other types. The zero value (empty string) omits the wire key.
	Mode PermissionMode

	// Directories carries the directory list for the addDirectories/
	// removeDirectories variants. Ignored for other types.
	Directories []string

	// Destination is always emitted when set. The zero value omits it.
	Destination PermissionUpdateDestination
}

// ToWire converts a [PermissionUpdate] into the dict shape upstream
// `PermissionUpdate.to_dict` produces (types.py:138-172). The Type field
// drives which optional keys are included; mismatched fields are silently
// dropped, matching upstream behavior.
func (u PermissionUpdate) ToWire() map[string]any {
	out := map[string]any{
		"type": string(u.Type),
	}
	if u.Destination != "" {
		out["destination"] = string(u.Destination)
	}

	switch u.Type {
	case PermissionUpdateTypeAddRules, PermissionUpdateTypeReplaceRules, PermissionUpdateTypeRemoveRules:
		if u.Rules != nil {
			rules := make([]map[string]any, len(u.Rules))
			for i, r := range u.Rules {
				rules[i] = map[string]any{
					"toolName":    r.ToolName,
					"ruleContent": r.RuleContent,
				}
			}
			out["rules"] = rules
		}
		if u.Behavior != "" {
			out["behavior"] = string(u.Behavior)
		}
	case PermissionUpdateTypeSetMode:
		if u.Mode != "" {
			out["mode"] = string(u.Mode)
		}
	case PermissionUpdateTypeAddDirectories, PermissionUpdateTypeRemoveDirectories:
		if u.Directories != nil {
			out["directories"] = u.Directories
		}
	}
	return out
}

// PermissionUpdateFromWire is the inverse of [PermissionUpdate.ToWire],
// mirroring upstream `PermissionUpdate.from_dict` (types.py:174-193). Missing
// optional keys decode to zero values; the result of round-tripping a
// well-formed wire object back through ToWire is the original object modulo
// nil-vs-empty-slice distinctions.
func PermissionUpdateFromWire(data map[string]any) PermissionUpdate {
	u := PermissionUpdate{}
	if v, ok := data["type"].(string); ok {
		u.Type = PermissionUpdateType(v)
	}
	if v, ok := data["destination"].(string); ok {
		u.Destination = PermissionUpdateDestination(v)
	}
	if v, ok := data["behavior"].(string); ok {
		u.Behavior = PermissionBehavior(v)
	}
	if v, ok := data["mode"].(string); ok {
		u.Mode = PermissionMode(v)
	}
	if v, ok := data["directories"].([]any); ok {
		dirs := make([]string, 0, len(v))
		for _, d := range v {
			if s, ok := d.(string); ok {
				dirs = append(dirs, s)
			}
		}
		u.Directories = dirs
	}
	if v, ok := data["rules"].([]any); ok {
		rules := make([]PermissionRuleValue, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			r := PermissionRuleValue{}
			if tn, ok := m["toolName"].(string); ok {
				r.ToolName = tn
			}
			if rc, ok := m["ruleContent"].(string); ok {
				r.RuleContent = rc
			}
			rules = append(rules, r)
		}
		u.Rules = rules
	}
	return u
}
