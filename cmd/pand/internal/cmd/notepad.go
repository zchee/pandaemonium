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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	notepadPrioritySection = "PRIORITY"
	notepadWorkingSection  = "WORKING MEMORY"
	notepadManualSection   = "MANUAL"
)

var notepadTimestampPattern = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2}T[\d:.]+(?:Z|[+-]\d{2}:\d{2})?)\]`)

func notepadDescriptor(env map[string]string, cwd string) mcpDescriptor {
	return mcpDescriptor{
		CommandName: "notepad",
		Title:       "JSON CLI surface for OMX notepad operations.",
		Tools: []mcpTool{
			{Name: "notepad_read", Description: "Read notepad content or a section."},
			{Name: "notepad_write_priority", Description: "Replace the Priority section, truncating to 500 characters."},
			{Name: "notepad_write_working", Description: "Append a timestamped Working Memory entry."},
			{Name: "notepad_write_manual", Description: "Append a Manual entry that is never pruned."},
			{Name: "notepad_prune", Description: "Prune Working Memory entries older than daysOld."},
			{Name: "notepad_stats", Description: "Return notepad size and section statistics."},
		},
		Aliases: map[string]string{
			"read":           "notepad_read",
			"write-priority": "notepad_write_priority",
			"write-working":  "notepad_write_working",
			"write-manual":   "notepad_write_manual",
			"prune":          "notepad_prune",
			"stats":          "notepad_stats",
		},
		Handle: func(input map[string]any) (any, bool) {
			store := fileStore{env: env, cwd: cwd}
			return store.handleNotepad(input)
		},
	}
}

func (s fileStore) handleNotepad(input map[string]any) (any, bool) {
	root, err := s.workingDirectory(input)
	if err != nil {
		return errorPayload(err), true
	}
	path := filepath.Join(root, ".omx", "notepad.md")
	switch input["tool"] {
	case "notepad_read":
		return notepadRead(path, stringField(input, "section")), false
	case "notepad_write_priority":
		content, err := requiredString(input, "content")
		if err != nil {
			return errorPayload(err), true
		}
		if len([]rune(content)) > 500 {
			content = string([]rune(content)[:500])
		}
		return notepadUpdate(path, func(existing string) string {
			return replaceMarkdownSection(existing, notepadPrioritySection, content)
		})
	case "notepad_write_working":
		content, err := requiredString(input, "content")
		if err != nil {
			return errorPayload(err), true
		}
		entry := "\n[" + nowISO() + "] " + content
		return notepadUpdate(path, func(existing string) string {
			return appendMarkdownSection(existing, notepadWorkingSection, entry)
		})
	case "notepad_write_manual":
		content, err := requiredString(input, "content")
		if err != nil {
			return errorPayload(err), true
		}
		return notepadUpdate(path, func(existing string) string {
			return appendMarkdownSection(existing, notepadManualSection, "\n"+content)
		})
	case "notepad_prune":
		return notepadPrune(path, input)
	case "notepad_stats":
		return notepadStats(path), false
	default:
		return errorPayload(fmt.Errorf("unknown tool: %s", input["tool"])), true
	}
}

func notepadRead(path, section string) map[string]any {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"exists": false, "content": ""}
		}
		return errorPayload(err)
	}
	text := string(content)
	if section != "" && section != "all" {
		return map[string]any{"section": section, "content": extractMarkdownSection(text, section)}
	}
	return map[string]any{"content": text}
}

func notepadUpdate(path string, update func(string) string) (any, bool) {
	existing, err := readTextFileOrEmpty(path)
	if err != nil {
		return errorPayload(err), true
	}
	if err := writeFileAtomic(path, []byte(update(existing))); err != nil {
		return errorPayload(err), true
	}
	return map[string]any{"success": true}, false
}

func notepadPrune(path string, input map[string]any) (any, bool) {
	contentRaw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"pruned": 0, "message": "No notepad file found"}, false
		}
		return errorPayload(err), true
	}
	days, err := parseNonNegativeInt(input["daysOld"], 7)
	if err != nil {
		return errorPayload(err), true
	}
	content := string(contentRaw)
	working := extractMarkdownSection(content, notepadWorkingSection)
	if working == "" {
		return map[string]any{"pruned": 0, "message": "No working memory entries found"}, false
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	kept := make([]string, 0)
	pruned := 0
	for line := range strings.SplitSeq(working, "\n") {
		match := notepadTimestampPattern.FindStringSubmatch(line)
		if match != nil {
			stamp, err := time.Parse(time.RFC3339Nano, match[1])
			if err == nil && stamp.Before(cutoff) {
				pruned++
				continue
			}
		}
		kept = append(kept, line)
	}
	if pruned > 0 {
		updated := replaceMarkdownSection(content, notepadWorkingSection, strings.Join(kept, "\n"))
		if err := writeFileAtomic(path, []byte(updated)); err != nil {
			return errorPayload(err), true
		}
	}
	remaining := 0
	for _, line := range kept {
		if notepadTimestampPattern.MatchString(line) {
			remaining++
		}
	}
	return map[string]any{"pruned": pruned, "remaining": remaining}, false
}

func notepadStats(path string) map[string]any {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"exists": false, "size": 0, "entryCount": 0, "oldestEntry": nil}
		}
		return errorPayload(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return errorPayload(err)
	}
	text := string(content)
	working := extractMarkdownSection(text, notepadWorkingSection)
	timestamps := make([]string, 0)
	for line := range strings.SplitSeq(working, "\n") {
		match := notepadTimestampPattern.FindStringSubmatch(line)
		if match != nil {
			timestamps = append(timestamps, match[1])
		}
	}
	manualLines := 0
	for line := range strings.SplitSeq(extractMarkdownSection(text, notepadManualSection), "\n") {
		if strings.TrimSpace(line) != "" {
			manualLines++
		}
	}
	var oldest any
	var newest any
	if len(timestamps) > 0 {
		oldest = timestamps[0]
		newest = timestamps[len(timestamps)-1]
	}
	return map[string]any{
		"exists":      true,
		"size":        info.Size(),
		"entryCount":  len(timestamps),
		"oldestEntry": oldest,
		"newestEntry": newest,
		"sections": map[string]any{
			"priority": len(extractMarkdownSection(text, notepadPrioritySection)),
			"working":  len(timestamps),
			"manual":   manualLines,
		},
	}
}

func extractMarkdownSection(content, section string) string {
	header := "## " + strings.ToUpper(section)
	idx := strings.Index(content, header)
	if idx < 0 {
		return ""
	}
	next := strings.Index(content[idx+len(header):], "\n## ")
	if next < 0 {
		return strings.TrimSpace(content[idx+len(header):])
	}
	return strings.TrimSpace(content[idx+len(header) : idx+len(header)+next])
}

func replaceMarkdownSection(content, section, newContent string) string {
	header := "## " + section
	idx := strings.Index(content, header)
	if idx < 0 {
		return strings.TrimRight(content, "\n") + "\n\n" + header + "\n" + newContent + "\n"
	}
	nextRelative := strings.Index(content[idx+len(header):], "\n## ")
	if nextRelative < 0 {
		return content[:idx] + header + "\n" + newContent + "\n"
	}
	next := idx + len(header) + nextRelative
	return content[:idx] + header + "\n" + newContent + "\n" + content[next:]
}

func appendMarkdownSection(content, section, entry string) string {
	header := "## " + section
	idx := strings.Index(content, header)
	if idx < 0 {
		return strings.TrimRight(content, "\n") + "\n\n" + header + entry + "\n"
	}
	nextRelative := strings.Index(content[idx+len(header):], "\n## ")
	if nextRelative < 0 {
		return content + entry
	}
	next := idx + len(header) + nextRelative
	return content[:next] + entry + content[next:]
}
