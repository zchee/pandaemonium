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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-json-experiment/json"
)

func traceDescriptor(env map[string]string, cwd string) mcpDescriptor {
	return mcpDescriptor{
		CommandName: "trace",
		Title:       "JSON CLI surface for OMX trace operations.",
		Tools: []mcpTool{
			{Name: "trace_timeline", Description: "Return chronological turn and mode transition trace entries."},
			{Name: "trace_summary", Description: "Return aggregate turn, mode, token, and timing statistics."},
		},
		Aliases: map[string]string{
			"timeline": "trace_timeline",
			"summary":  "trace_summary",
		},
		Handle: func(input map[string]any) (any, bool) {
			store := fileStore{env: env, cwd: cwd}
			return store.handleTrace(input)
		},
	}
}

func (s fileStore) handleTrace(input map[string]any) (any, bool) {
	root, err := s.workingDirectory(input)
	if err != nil {
		return errorPayload(err), true
	}
	logsDir := filepath.Join(root, ".omx", "logs")
	switch input["tool"] {
	case "trace_timeline":
		return traceTimeline(root, logsDir, input)
	case "trace_summary":
		return traceSummary(root, logsDir)
	default:
		return errorPayload(fmt.Errorf("unknown tool: %s", input["tool"])), true
	}
}

func traceTimeline(root, logsDir string, input map[string]any) (any, bool) {
	filter := stringField(input, "filter")
	if filter == "" {
		filter = "all"
	}
	if filter != "all" && filter != "turns" && filter != "modes" {
		return errorPayload(fmt.Errorf("filter must be one of all, turns, modes")), true
	}
	limit := maxInt(input["last"], 0, 10_000)
	entries := make([]map[string]any, 0)
	if filter != "modes" {
		turns, err := readTraceLogEntries(logsDir)
		if err != nil {
			return errorPayload(err), true
		}
		for _, turn := range turns {
			entries = append(entries, map[string]any{
				"timestamp":      stringField(turn, "timestamp"),
				"type":           "turn",
				"turn_type":      stringField(turn, "type"),
				"thread_id":      stringField(turn, "thread_id"),
				"input_preview":  stringField(turn, "input_preview"),
				"output_preview": stringField(turn, "output_preview"),
			})
		}
	}
	if filter != "turns" {
		modeEvents, err := readTraceModeEvents(root)
		if err != nil {
			return errorPayload(err), true
		}
		entries = append(entries, modeEvents...)
	}
	sort.Slice(entries, func(i, j int) bool {
		return stringField(entries[i], "timestamp") < stringField(entries[j], "timestamp")
	})
	total := len(entries)
	if limit > 0 && limit < len(entries) {
		entries = entries[len(entries)-limit:]
	}
	return map[string]any{"entryCount": len(entries), "totalAvailable": total, "filter": filter, "timeline": entries}, false
}

func traceSummary(root, logsDir string) (any, bool) {
	turns, err := readTraceLogEntries(logsDir)
	if err != nil {
		return errorPayload(err), true
	}
	byType := map[string]any{}
	var first, last string
	for _, turn := range turns {
		typeName := stringField(turn, "type")
		if typeName == "" {
			typeName = "unknown"
		}
		count, _ := byType[typeName].(int)
		byType[typeName] = count + 1
		stamp := stringField(turn, "timestamp")
		if stamp == "" {
			continue
		}
		if first == "" || stamp < first {
			first = stamp
		}
		if last == "" || stamp > last {
			last = stamp
		}
	}
	modeEvents, err := readTraceModeEvents(root)
	if err != nil {
		return errorPayload(err), true
	}
	modes := map[string]map[string]int{}
	for _, event := range modeEvents {
		mode := stringField(event, "mode")
		if mode == "" {
			continue
		}
		bucket := modes[mode]
		if bucket == nil {
			bucket = map[string]int{"starts": 0, "ends": 0}
			modes[mode] = bucket
		}
		switch stringField(event, "type") {
		case "mode_start":
			bucket["starts"]++
		case "mode_end":
			bucket["ends"]++
		}
	}
	duration := int64(0)
	if first != "" && last != "" {
		if start, err := time.Parse(time.RFC3339Nano, first); err == nil {
			if end, err := time.Parse(time.RFC3339Nano, last); err == nil {
				duration = end.Sub(start).Milliseconds()
			}
		}
	}
	metrics := any(map[string]any{"note": "No metrics file found"})
	if got, err := readJSONFile(filepath.Join(root, ".omx", "metrics.json")); err == nil {
		metrics = got
	}
	return map[string]any{
		"turns": map[string]any{
			"total":             len(turns),
			"byType":            byType,
			"firstAt":           nullableString(first),
			"lastAt":            nullableString(last),
			"durationMs":        duration,
			"durationFormatted": formatDuration(duration),
		},
		"modes":   modes,
		"metrics": metrics,
	}, false
}

func readTraceLogEntries(logsDir string) ([]map[string]any, error) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	out := make([]map[string]any, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "turns-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(logsDir, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(line), &got); err == nil {
				out = append(out, got)
			}
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return stringField(out[i], "timestamp") < stringField(out[j], "timestamp") })
	return out, nil
}

func readTraceModeEvents(root string) ([]map[string]any, error) {
	store := stateStore{env: map[string]string{}}
	scope, err := store.resolveStateScope(map[string]any{"workingDirectory": root})
	if err != nil {
		return nil, err
	}
	statuses, err := readStatusesFromDirs(store.readScopedStateDirs(scope), "")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(statuses)*2)
	for mode, status := range statuses {
		if started := stringField(status.Data, "started_at"); started != "" {
			out = append(out, map[string]any{"timestamp": started, "type": "mode_start", "mode": mode, "phase": stringField(status.Data, "current_phase"), "active": status.Active, "path": status.Path})
		}
		if completed := stringField(status.Data, "completed_at"); completed != "" {
			out = append(out, map[string]any{"timestamp": completed, "type": "mode_end", "mode": mode, "phase": stringField(status.Data, "current_phase"), "path": status.Path})
		}
	}
	sort.Slice(out, func(i, j int) bool { return stringField(out[i], "timestamp") < stringField(out[j], "timestamp") })
	return out, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func formatDuration(ms int64) string {
	if ms <= 0 {
		return "N/A"
	}
	seconds := ms / 1000
	return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
}
