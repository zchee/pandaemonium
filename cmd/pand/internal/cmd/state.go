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

package cmd

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var strictReadableModes = map[string]struct{}{
	"autopilot":      {},
	"autoresearch":   {},
	"team":           {},
	"ralph":          {},
	"ultrawork":      {},
	"ultraqa":        {},
	"ralplan":        {},
	"deep-interview": {},
	"skill-active":   {},
}

type stateStore struct {
	env map[string]string
	cwd string
}

type resolvedStateScope struct {
	Source     string
	SessionID  string
	StateDir   string
	BaseDir    string
	WorkingDir string
}

func newStateCommand(env map[string]string, cwd string) *cobra.Command {
	store := stateStore{env: env, cwd: cwd}
	command := &cobra.Command{
		Use:                "state <read|write|clear|list-active|get-status>",
		Short:              "Read and write OMX mode state",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return store.runStateCommand(cmd, args)
		},
	}
	return command
}

func (s stateStore) runStateCommand(cmd *cobra.Command, args []string) error {
	operation := args[0]
	input, compact, err := parseCommandInput(args[1:])
	if err != nil {
		return err
	}
	var result any
	var isError bool
	switch operation {
	case "read":
		result, isError = s.stateRead(input)
	case "write":
		result, isError = s.stateWrite(input)
	case "clear":
		result, isError = s.stateClear(input)
	case "list-active":
		result, isError = s.stateListActive(input)
	case "get-status":
		result, isError = s.stateGetStatus(input)
	case "--help", "-h", "help":
		return cmd.Help()
	default:
		return fmt.Errorf("unknown state subcommand: %s", operation)
	}
	out := cmd.OutOrStdout()
	if isError {
		out = cmd.ErrOrStderr()
	}
	if err := writeJSON(out, result, compact); err != nil {
		return err
	}
	if isError {
		return fmt.Errorf("state %s failed", operation)
	}
	return nil
}

func newStatusCommand(env map[string]string, cwd string) *cobra.Command {
	store := stateStore{env: env, cwd: cwd}
	return &cobra.Command{
		Use:                "status",
		Short:              "Show active OMX mode status",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			input, compact, err := parseCommandInput(args)
			if err != nil {
				return err
			}
			payload, isError := store.stateGetStatus(input)
			if compact || boolField(input, "json") {
				if err := writeJSON(cmd.OutOrStdout(), payload, true); err != nil {
					return err
				}
			} else {
				statuses, _ := payload.(map[string]any)["statuses"].(map[string]any)
				if len(statuses) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No active modes.")
				} else {
					keys := make([]string, 0, len(statuses))
					for key := range statuses {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					for _, key := range keys {
						status, _ := statuses[key].(map[string]any)
						fmt.Fprintf(cmd.OutOrStdout(), "%s: %s (phase: %s)\n", key, activeText(status["active"]), safeDisplay(status["phase"], "n/a"))
					}
				}
			}
			if isError {
				return fmt.Errorf("status failed")
			}
			return nil
		},
	}
}

func newCancelCommand(env map[string]string, cwd string) *cobra.Command {
	store := stateStore{env: env, cwd: cwd}
	return &cobra.Command{
		Use:                "cancel",
		Short:              "Cancel active OMX modes by clearing state",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			input, compact, err := parseCommandInput(args)
			if err != nil {
				return err
			}
			activePayload, isError := store.stateListActive(input)
			if isError {
				_ = writeJSON(cmd.ErrOrStderr(), activePayload, compact)
				return fmt.Errorf("cancel failed")
			}
			rawActiveModes, _ := activePayload.(map[string]any)["active_modes"].([]string)
			activeModes := make([]string, 0, len(rawActiveModes))
			for _, mode := range rawActiveModes {
				if mode != "skill-active" {
					activeModes = append(activeModes, mode)
				}
			}
			cleared := make([]any, 0, len(activeModes))
			for _, mode := range activeModes {
				clearInput := cloneMap(input)
				clearInput["mode"] = mode
				payload, failed := store.stateClear(clearInput)
				if failed {
					_ = writeJSON(cmd.ErrOrStderr(), payload, compact)
					return fmt.Errorf("cancel %s failed", mode)
				}
				cleared = append(cleared, payload)
			}
			payload := map[string]any{"cancelled": len(cleared), "modes": activeModes, "cleared": cleared}
			if err := writeJSON(cmd.OutOrStdout(), payload, compact); err != nil {
				return err
			}
			return nil
		},
	}
}

func parseCommandInput(args []string) (map[string]any, bool, error) {
	input := map[string]any{}
	compact := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			compact = true
		case arg == "--input":
			if i+1 >= len(args) {
				return nil, false, fmt.Errorf("missing JSON value after --input")
			}
			parsed, err := parseJSONObject(args[i+1])
			if err != nil {
				return nil, false, err
			}
			input = parsed
			i++
		case strings.HasPrefix(arg, "--input="):
			parsed, err := parseJSONObject(strings.TrimPrefix(arg, "--input="))
			if err != nil {
				return nil, false, err
			}
			input = parsed
		case arg == "--help" || arg == "-h" || arg == "help":
			input["help"] = true
		default:
			return nil, false, fmt.Errorf("unknown argument: %s", arg)
		}
	}
	return input, compact, nil
}

func (s stateStore) stateRead(input map[string]any) (any, bool) {
	mode, err := validateSafeSegment("mode", input["mode"])
	if err != nil {
		return errorPayload(err), true
	}
	if _, ok := strictReadableModes[mode]; !ok {
		return errorPayload(fmt.Errorf("mode must be one of: %s", strings.Join(strictModeNames(), ", "))), true
	}
	scope, err := s.resolveStateScope(input)
	if err != nil {
		return errorPayload(err), true
	}
	for _, path := range s.readScopedStatePaths(scope, mode) {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		data, err := readJSONFile(path)
		if err != nil {
			return errorPayload(err), true
		}
		return data, false
	}
	return map[string]any{"exists": false, "mode": mode}, false
}

func (s stateStore) stateWrite(input map[string]any) (any, bool) {
	mode, err := validateSafeSegment("mode", input["mode"])
	if err != nil {
		return errorPayload(err), true
	}
	scope, err := s.resolveWriteScope(input)
	if err != nil {
		return errorPayload(err), true
	}
	path := statePath(scope.StateDir, mode)
	existing := map[string]any{}
	if _, err := os.Stat(path); err == nil {
		if existing, err = readJSONFile(path); err != nil {
			return errorPayload(fmt.Errorf("read existing state %s: %w", path, err)), true
		}
	} else if !os.IsNotExist(err) {
		return errorPayload(err), true
	}
	merged := cloneMap(existing)
	for key, value := range input {
		switch key {
		case "mode", "workingDirectory", "session_id", "state":
			continue
		default:
			merged[key] = value
		}
	}
	if custom, ok := input["state"].(map[string]any); ok {
		maps.Copy(merged, custom)
	}
	merged["mode"] = mode
	if scope.SessionID != "" {
		merged["session_id"] = scope.SessionID
	}
	if _, ok := merged["updated_at"]; !ok {
		merged["updated_at"] = nowISO()
	}
	if err := writeJSONFileAtomic(path, merged); err != nil {
		return errorPayload(err), true
	}
	if mode != "skill-active" {
		if err := s.syncSkillActive(scope, mode, merged); err != nil {
			return errorPayload(err), true
		}
	}
	return map[string]any{"success": true, "mode": mode, "path": path}, false
}

func (s stateStore) stateClear(input map[string]any) (any, bool) {
	mode, err := validateSafeSegment("mode", input["mode"])
	if err != nil {
		return errorPayload(err), true
	}
	scope, err := s.resolveWriteScope(input)
	if err != nil {
		return errorPayload(err), true
	}
	if boolField(input, "all_sessions") {
		paths, err := allScopedStatePaths(scope.BaseDir, mode)
		if err != nil {
			return errorPayload(err), true
		}
		removed := make([]string, 0, len(paths))
		for _, path := range paths {
			if _, err := os.Stat(path); err != nil {
				continue
			}
			if err := os.Remove(path); err != nil {
				return errorPayload(err), true
			}
			removed = append(removed, path)
		}
		if mode != "skill-active" {
			if err := s.clearSkillActive(scope, mode, true); err != nil {
				return errorPayload(err), true
			}
		}
		return map[string]any{"cleared": true, "mode": mode, "all_sessions": true, "removed": len(removed), "paths": removed, "warning": "all_sessions clears global and session-scoped state files"}, false
	}
	path := statePath(scope.StateDir, mode)
	rootPath := statePath(scope.BaseDir, mode)
	if scope.SessionID != "" {
		if _, err := os.Stat(rootPath); err == nil {
			cleared := map[string]any{"mode": mode, "active": false, "current_phase": "cleared", "updated_at": nowISO(), "completed_at": nowISO(), "session_id": scope.SessionID}
			if err := writeJSONFileAtomic(path, cleared); err != nil {
				return errorPayload(err), true
			}
		} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return errorPayload(err), true
		}
	} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return errorPayload(err), true
	}
	if mode != "skill-active" {
		if err := s.clearSkillActive(scope, mode, false); err != nil {
			return errorPayload(err), true
		}
	}
	return map[string]any{"cleared": true, "mode": mode, "path": path}, false
}

func (s stateStore) stateListActive(input map[string]any) (any, bool) {
	scope, err := s.resolveActiveScope(input)
	if err != nil {
		return errorPayload(err), true
	}
	statuses, err := readStatusesFromDirs([]string{scope.StateDir}, "")
	if err != nil {
		return errorPayload(err), true
	}
	active := make([]string, 0, len(statuses))
	for mode, status := range statuses {
		if status.Active {
			active = append(active, mode)
		}
	}
	sort.Strings(active)
	return map[string]any{"active_modes": active}, false
}

func (s stateStore) stateGetStatus(input map[string]any) (any, bool) {
	scope, err := s.resolveStateScope(input)
	if err != nil {
		return errorPayload(err), true
	}
	mode := ""
	if raw := strings.TrimSpace(stringField(input, "mode")); raw != "" {
		mode, err = validateSafeSegment("mode", raw)
		if err != nil {
			return errorPayload(err), true
		}
	}
	statuses, err := readStatusesFromDirs(s.readScopedStateDirs(scope), mode)
	if err != nil {
		return errorPayload(err), true
	}
	out := make(map[string]any, len(statuses))
	for name, status := range statuses {
		out[name] = status.toMap()
	}
	return map[string]any{"statuses": out}, false
}

func (s stateStore) resolveStateScope(input map[string]any) (resolvedStateScope, error) {
	wd, baseDir, err := s.resolveWorkingAndBase(input)
	if err != nil {
		return resolvedStateScope{}, err
	}
	if sessionID, ok, err := optionalSafeSegment("session_id", input["session_id"]); err != nil {
		return resolvedStateScope{}, err
	} else if ok {
		return resolvedStateScope{Source: "explicit", SessionID: sessionID, StateDir: filepath.Join(baseDir, "sessions", sessionID), BaseDir: baseDir, WorkingDir: wd}, nil
	}
	if sessionID := s.currentSessionID(baseDir, wd); sessionID != "" {
		return resolvedStateScope{Source: "session", SessionID: sessionID, StateDir: filepath.Join(baseDir, "sessions", sessionID), BaseDir: baseDir, WorkingDir: wd}, nil
	}
	return resolvedStateScope{Source: "root", StateDir: baseDir, BaseDir: baseDir, WorkingDir: wd}, nil
}

func (s stateStore) resolveWriteScope(input map[string]any) (resolvedStateScope, error) {
	scope, err := s.resolveStateScope(input)
	if err != nil {
		return resolvedStateScope{}, err
	}
	if err := os.MkdirAll(scope.StateDir, 0o755); err != nil {
		return resolvedStateScope{}, err
	}
	return scope, nil
}

func (s stateStore) resolveActiveScope(input map[string]any) (resolvedStateScope, error) {
	return s.resolveStateScope(input)
}

func (s stateStore) resolveWorkingAndBase(input map[string]any) (string, string, error) {
	resolver := pathResolver{env: s.env, cwd: s.cwd}
	wd, err := resolver.workingDirectory(stringField(input, "workingDirectory"))
	if err != nil {
		return "", "", err
	}
	baseDir, err := resolver.baseStateDir(wd)
	if err != nil {
		return "", "", err
	}
	return wd, baseDir, nil
}

func (s stateStore) currentSessionID(baseDir, wd string) string {
	for _, key := range []string{"OMX_SESSION_ID", "CODEX_SESSION_ID", "SESSION_ID"} {
		candidate := strings.TrimSpace(s.env[key])
		if candidate == "" {
			continue
		}
		if _, err := validateSafeSegment("session_id", candidate); err == nil {
			if _, err := os.Stat(filepath.Join(baseDir, "sessions", candidate)); err == nil {
				return candidate
			}
		}
	}
	state, err := readJSONFile(filepath.Join(baseDir, "session.json"))
	if err != nil {
		return ""
	}
	if cwd := stringField(state, "cwd"); cwd != "" {
		if abs, err := filepath.Abs(cwd); err == nil && filepath.Clean(abs) != filepath.Clean(wd) {
			return ""
		}
	}
	candidate := stringField(state, "session_id")
	if _, err := validateSafeSegment("session_id", candidate); err == nil {
		return candidate
	}
	return ""
}

func (s stateStore) readScopedStateDirs(scope resolvedStateScope) []string {
	if scope.Source == "root" {
		return []string{scope.BaseDir}
	}
	if scope.Source == "explicit" {
		if _, err := os.Stat(scope.StateDir); err == nil {
			return []string{scope.StateDir}
		}
	}
	return []string{scope.StateDir, scope.BaseDir}
}

func (s stateStore) readScopedStatePaths(scope resolvedStateScope, mode string) []string {
	dirs := s.readScopedStateDirs(scope)
	paths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		paths = append(paths, statePath(dir, mode))
	}
	return paths
}

type modeStatus struct {
	Active bool
	Phase  string
	Path   string
	Data   map[string]any
}

func (s modeStatus) toMap() map[string]any {
	return map[string]any{"active": s.Active, "phase": s.Phase, "path": s.Path, "data": s.Data}
}

func readStatusesFromDirs(dirs []string, onlyMode string) (map[string]modeStatus, error) {
	statuses := map[string]modeStatus{}
	for _, dir := range slices.Backward(dirs) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), "-state.json") {
				continue
			}
			mode := strings.TrimSuffix(entry.Name(), "-state.json")
			if onlyMode != "" && mode != onlyMode {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			data, err := readJSONFile(path)
			if err != nil {
				statuses[mode] = modeStatus{Path: path, Data: map[string]any{"error": "malformed state file"}}
				continue
			}
			statuses[mode] = modeStatus{Active: data["active"] == true, Phase: stringField(data, "current_phase"), Path: path, Data: data}
		}
	}
	return statuses, nil
}

func allScopedStatePaths(baseDir, mode string) ([]string, error) {
	paths := []string{statePath(baseDir, mode)}
	sessionsRoot := filepath.Join(baseDir, "sessions")
	entries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return paths, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := validateSafeSegment("session_id", entry.Name()); err != nil {
			continue
		}
		paths = append(paths, statePath(filepath.Join(sessionsRoot, entry.Name()), mode))
	}
	return paths, nil
}

func statePath(dir, mode string) string {
	return filepath.Join(dir, mode+"-state.json")
}

func (s stateStore) syncSkillActive(scope resolvedStateScope, mode string, data map[string]any) error {
	path := statePath(scope.StateDir, "skill-active")
	state := map[string]any{}
	if existing, err := readJSONFile(path); err == nil {
		state = existing
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read skill-active state %s: %w", path, err)
	}
	entries := activeEntries(state["active_skills"])
	key := mode + "::" + scope.SessionID
	if data["active"] == true {
		entries[key] = map[string]any{"skill": mode, "phase": stringField(data, "current_phase"), "active": true, "session_id": scope.SessionID, "updated_at": nowISO()}
	} else {
		delete(entries, key)
	}
	list := make([]any, 0, len(entries))
	for _, entry := range entries {
		list = append(list, entry)
	}
	state["active"] = len(list) > 0
	state["skill"] = mode
	state["phase"] = stringField(data, "current_phase")
	state["active_skills"] = list
	state["updated_at"] = nowISO()
	return writeJSONFileAtomic(path, state)
}

func (s stateStore) clearSkillActive(scope resolvedStateScope, mode string, allSessions bool) error {
	paths := []string{statePath(scope.StateDir, "skill-active")}
	if allSessions {
		all, err := allScopedStatePaths(scope.BaseDir, "skill-active")
		if err != nil {
			return err
		}
		paths = all
	}
	for _, path := range paths {
		state, err := readJSONFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read skill-active state %s: %w", path, err)
		}
		entries := activeEntries(state["active_skills"])
		for key, entry := range entries {
			if entry["skill"] == mode {
				delete(entries, key)
			}
		}
		list := make([]any, 0, len(entries))
		for _, entry := range entries {
			list = append(list, entry)
		}
		state["active"] = len(list) > 0
		state["active_skills"] = list
		state["updated_at"] = nowISO()
		if err := writeJSONFileAtomic(path, state); err != nil {
			return err
		}
	}
	return nil
}

func activeEntries(raw any) map[string]map[string]any {
	out := map[string]map[string]any{}
	items, ok := raw.([]any)
	if !ok {
		return out
	}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		skill := stringField(entry, "skill")
		if skill == "" || entry["active"] == false {
			continue
		}
		key := skill + "::" + stringField(entry, "session_id")
		out[key] = entry
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func errorPayload(err error) map[string]any {
	return map[string]any{"error": err.Error()}
}

func strictModeNames() []string {
	names := make([]string, 0, len(strictReadableModes))
	for name := range strictReadableModes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func activeText(value any) string {
	if value == true {
		return "ACTIVE"
	}
	return "inactive"
}

func safeDisplay(value any, fallback string) string {
	if s, ok := value.(string); ok && s != "" {
		return s
	}
	return fallback
}
