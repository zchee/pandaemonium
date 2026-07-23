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
	"slices"
	"sync"
)

// LineBuffer is a mutex-guarded, bounded ring of output lines retaining at
// most the newest maxLines entries, dropping the oldest on overflow. It backs
// the per-process stderr/stdout diagnostic tails kept by the SDK packages.
type LineBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

// NewLineBuffer returns a LineBuffer retaining at most maxLines lines.
func NewLineBuffer(maxLines int) *LineBuffer {
	return &LineBuffer{max: maxLines}
}

// Append appends line, evicting the oldest entries beyond the buffer's bound.
func (b *LineBuffer) Append(line string) {
	b.mu.Lock()
	b.lines = AppendBoundedLine(b.lines, line, b.max)
	b.mu.Unlock()
}

// Tail returns the last n retained lines joined by newlines.
func (b *LineBuffer) Tail(n int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Tail(b.lines, n)
}

// Lines returns a snapshot of the retained lines.
func (b *LineBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.lines)
}
