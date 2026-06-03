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

package llm

import (
	"bufio"
	"io"
	"strings"
)

// AppendBoundedLine appends line to lines while retaining at most max entries.
func AppendBoundedLine(lines []string, line string, max int) []string {
	if max <= 0 {
		return lines[:0]
	}
	if len(lines) < max {
		return append(lines, line)
	}
	copy(lines, lines[1:])
	lines[max-1] = line
	return lines
}

// DrainLines scans r line-by-line and calls appendLine for each stderr line.
//
// If scanning itself fails, the synthetic line "stderr read error: <err>" is
// appended through the same callback.
func DrainLines(r io.Reader, appendLine func(string)) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		appendLine(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		appendLine("stderr read error: " + err.Error())
	}
}

// Tail joins the final limit entries in lines with newlines.
func Tail(lines []string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	if limit > len(lines) {
		limit = len(lines)
	}
	return strings.Join(lines[len(lines)-limit:], "\n")
}
