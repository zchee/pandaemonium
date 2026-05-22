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

// ─── System prompt configuration ─────────────────────────────────────────────

// SystemPromptSource is the sealed interface implemented by every
// [Options].SystemPrompt variant. The unexported sentinel keeps the set closed
// to this package, enabling exhaustive type-switch coverage (the same idiom as
// [ThinkingConfig] and [PermissionResult]).
//
// A nil SystemPromptSource is valid and emits --system-prompt "" (matching
// upstream's system_prompt is None branch, subprocess_cli.py:227-228).
//
// Concrete types: [SystemPromptText], [SystemPromptFile], [SystemPromptPreset].
//
// Mirrors upstream system_prompt: str | SystemPromptFile | SystemPromptPreset
// (subprocess_cli.py:227-238).
type SystemPromptSource interface {
	isSystemPromptSource()
}

// SystemPromptText is an inline system-prompt string. It emits
// --system-prompt <text> (subprocess_cli.py:229-230). Use it where a bare
// string was previously assigned to Options.SystemPrompt:
//
//	Options{SystemPrompt: claude.SystemPromptText("You are a helpful assistant.")}
type SystemPromptText string

func (SystemPromptText) isSystemPromptSource() {}

// SystemPromptFile loads the system prompt from a file path. It emits
// --system-prompt-file <path> (subprocess_cli.py:233-234).
type SystemPromptFile struct {
	// Path is the filesystem path the CLI reads the system prompt from.
	Path string
}

func (SystemPromptFile) isSystemPromptSource() {}

// SystemPromptPreset appends to the CLI's built-in preset system prompt. It
// emits --append-system-prompt <append> (subprocess_cli.py:235-237),
// corresponding to upstream's {"type":"preset","append":...} object.
type SystemPromptPreset struct {
	// Append is the text appended to the preset system prompt.
	Append string
}

func (SystemPromptPreset) isSystemPromptSource() {}
