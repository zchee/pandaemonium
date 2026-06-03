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
	"strings"
	"testing"
)

func TestAppendBoundedLineAndTail(t *testing.T) {
	t.Parallel()

	lines := []string{"one", "two"}
	lines = AppendBoundedLine(lines, "three", 3)
	lines = AppendBoundedLine(lines, "four", 3)
	if got, want := strings.Join(lines, ","), "two,three,four"; got != want {
		t.Fatalf("AppendBoundedLine() = %q, want %q", got, want)
	}
	if got, want := Tail(lines, 2), "three\nfour"; got != want {
		t.Fatalf("Tail() = %q, want %q", got, want)
	}
}

func TestDrainLines(t *testing.T) {
	t.Parallel()

	var lines []string
	DrainLines(strings.NewReader("one\ntwo\n"), func(line string) {
		lines = AppendBoundedLine(lines, line, 4)
	})
	if got, want := strings.Join(lines, ","), "one,two"; got != want {
		t.Fatalf("DrainLines() = %q, want %q", got, want)
	}
}
