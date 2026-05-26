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
)

func projectMemoryDescriptor(env map[string]string, cwd string) mcpDescriptor {
	return mcpDescriptor{
		CommandName: "project-memory",
		Title:       "JSON CLI surface for OMX project-memory operations.",
		Tools: []mcpTool{
			{Name: "project_memory_read", Description: "Read full project memory or a section."},
			{Name: "project_memory_write", Description: "Write or merge project memory."},
			{Name: "project_memory_add_note", Description: "Append a categorized project-memory note."},
			{Name: "project_memory_add_directive", Description: "Append a persistent project-memory directive."},
		},
		Aliases: map[string]string{
			"read":          "project_memory_read",
			"write":         "project_memory_write",
			"add-note":      "project_memory_add_note",
			"add-directive": "project_memory_add_directive",
		},
		Handle: func(input map[string]any) (any, bool) {
			store := fileStore{env: env, cwd: cwd}
			return store.handleProjectMemory(input)
		},
	}
}

func (s fileStore) handleProjectMemory(input map[string]any) (any, bool) {
	root, err := s.workingDirectory(input)
	if err != nil {
		return errorPayload(err), true
	}
	path := filepath.Join(root, ".omx", "project-memory.json")
	switch input["tool"] {
	case "project_memory_read":
		return projectMemoryRead(path, stringField(input, "section"))
	case "project_memory_write":
		memory, ok := input["memory"].(map[string]any)
		if !ok || memory == nil {
			return errorPayload(fmt.Errorf("memory must be a JSON object")), true
		}
		return projectMemoryWrite(path, memory, boolField(input, "merge"))
	case "project_memory_add_note":
		category, err := requiredString(input, "category")
		if err != nil {
			return errorPayload(err), true
		}
		content, err := requiredString(input, "content")
		if err != nil {
			return errorPayload(err), true
		}
		return projectMemoryAppend(path, "notes", map[string]any{"category": category, "content": content, "timestamp": nowISO()})
	case "project_memory_add_directive":
		directive, err := requiredString(input, "directive")
		if err != nil {
			return errorPayload(err), true
		}
		priority := stringField(input, "priority")
		if priority == "" {
			priority = "normal"
		}
		entry := map[string]any{"directive": directive, "priority": priority, "timestamp": nowISO()}
		if context := stringField(input, "context"); context != "" {
			entry["context"] = context
		}
		return projectMemoryAppend(path, "directives", entry)
	default:
		return errorPayload(fmt.Errorf("unknown tool: %s", input["tool"])), true
	}
}

func projectMemoryRead(path, section string) (any, bool) {
	data, exists, err := readProjectMemoryFile(path)
	if err != nil {
		return errorPayload(err), true
	}
	if !exists {
		return map[string]any{"exists": false}, false
	}
	if section != "" && section != "all" {
		if value, ok := data[section]; ok {
			return value, false
		}
	}
	return data, false
}

func projectMemoryWrite(path string, memory map[string]any, merge bool) (any, bool) {
	existing, exists, err := readProjectMemoryFile(path)
	if err != nil {
		return errorPayload(err), true
	}
	out := cloneMap(memory)
	if merge && exists {
		out = cloneMap(existing)
		maps.Copy(out, memory)
	}
	if err := writeJSONFileAtomic(path, out); err != nil {
		return errorPayload(err), true
	}
	return map[string]any{"success": true}, false
}

func projectMemoryAppend(path, key string, entry map[string]any) (any, bool) {
	data, _, err := readProjectMemoryFile(path)
	if err != nil {
		return errorPayload(err), true
	}
	list := append(toAnySlice(data[key]), entry)
	data[key] = list
	if err := writeJSONFileAtomic(path, data); err != nil {
		return errorPayload(err), true
	}
	field := "noteCount"
	if key == "directives" {
		field = "directiveCount"
	}
	return map[string]any{"success": true, field: len(list)}, false
}

func readProjectMemoryFile(path string) (map[string]any, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, false, nil
		}
		return nil, false, err
	}
	data, err := readJSONFile(path)
	if err != nil {
		return nil, true, fmt.Errorf("read project memory %s: %w", path, err)
	}
	return data, true, nil
}
