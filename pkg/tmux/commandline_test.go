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

package tmux

import (
	"strings"
	"testing"
)

func TestCommandLineString(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		line    CommandLine
		want    string
		wantErr string
	}{
		"success: bare arguments": {
			line: NewCommandLine(DisplayMessage, RawArg("-p"), StringArg("#{session_name}")),
			want: "display-message -p '#{session_name}'",
		},
		"success: spaces are quoted": {
			line: NewCommandLine(DisplayMessage, StringArg("hello world")),
			want: "display-message 'hello world'",
		},
		"success: semicolon stays argument content": {
			line: NewCommandLine(DisplayMessage, StringArg("a;b")),
			want: "display-message 'a;b'",
		},
		"success: double quotes inside single quotes": {
			line: NewCommandLine(DisplayMessage, StringArg(`say "hi"`)),
			want: `display-message 'say "hi"'`,
		},
		"success: single quote switches to double quotes": {
			line: NewCommandLine(DisplayMessage, StringArg("can't")),
			want: `display-message "can't"`,
		},
		"success: backslash is escaped in double quotes": {
			line: NewCommandLine(DisplayMessage, StringArg(`can't\stop`)),
			want: `display-message "can't\\stop"`,
		},
		"success: unicode is quoted when needed": {
			line: NewCommandLine(DisplayMessage, StringArg("hello 😀")),
			want: "display-message 'hello 😀'",
		},
		"success: raw argument is passed through": {
			line: NewCommandLine(RefreshClient, RawArg("-f"), RawArg("pause-after=30")),
			want: "refresh-client -f pause-after=30",
		},
		"error: empty command": {
			line:    NewCommandLine(Command("")),
			wantErr: "must not be empty",
		},
		"error: command token with space": {
			line:    NewCommandLine(Command("display message")),
			wantErr: "plain token",
		},
		"error: argument newline": {
			line:    NewCommandLine(DisplayMessage, StringArg("bad\n")),
			wantErr: "contains a newline",
		},
		"error: raw argument empty": {
			line:    NewCommandLine(DisplayMessage, RawArg(" ")),
			wantErr: "raw argument must not be empty",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.line.String()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("String() error = %v, want nil", err)
				}
				if got != tt.want {
					t.Fatalf("String() = %q, want %q", got, tt.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("String() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRawLine(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		line    string
		wantErr string
	}{
		"success: command sequence syntax without newline": {line: "display-message -p ok ; list-panes"},
		"error: blank line": {line: "  ", wantErr: "must not be empty"},
		"error: newline":    {line: "display-message\nlist-panes", wantErr: "contains a newline"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := validateRawLine(tt.line)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRawLine() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateRawLine() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCommandLineInvalidUTF8(t *testing.T) {
	t.Parallel()
	bad := string([]byte{0xff})
	if _, err := NewCommandLine(Command(bad)).String(); err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("invalid command UTF-8 error = %v, want valid UTF-8", err)
	}
	if _, err := NewCommandLine(DisplayMessage, StringArg(bad)).String(); err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("invalid argument UTF-8 error = %v, want valid UTF-8", err)
	}
}
