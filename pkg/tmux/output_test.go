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
	"bytes"
	"strings"
	"testing"
)

func TestDecodeOutputValue(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		value   string
		want    []byte
		wantErr string
	}{
		"success: carriage return and newline": {
			value: `hello\015\012`,
			want:  []byte("hello\r\n"),
		},
		"success: escaped backslash": {
			value: `path\134name`,
			want:  []byte(`path\name`),
		},
		"success: terminal escape bytes": {
			value: `\033[31mred\033[0m`,
			want:  []byte("\x1b[31mred\x1b[0m"),
		},
		"success: non utf8 bytes preserved": {
			value: `bin\377ary`,
			want:  []byte{'b', 'i', 'n', 0xff, 'a', 'r', 'y'},
		},
		"success: large payload": {
			value: strings.Repeat("a", 8192) + `\012`,
			want:  append(bytes.Repeat([]byte{'a'}, 8192), '\n'),
		},
		"error: incomplete escape": {
			value:   `bad\01`,
			wantErr: "incomplete octal escape",
		},
		"error: invalid digit": {
			value:   `bad\09x`,
			wantErr: "invalid octal digit",
		},
		"error: out of range": {
			value:   `bad\777`,
			wantErr: "out of range",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := DecodeOutputValue(tt.value)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("DecodeOutputValue() error = %v, want nil", err)
				}
				if !bytes.Equal(got, tt.want) {
					t.Fatalf("DecodeOutputValue() = %q, want %q", got, tt.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("DecodeOutputValue() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestOutputNotificationText(t *testing.T) {
	t.Parallel()
	text, err := (OutputNotification{Value: `hello\012`}).Text()
	if err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	if text != "hello\n" {
		t.Fatalf("Text() = %q, want %q", text, "hello\n")
	}
	if _, err := (OutputNotification{Value: `bad\377`}).Text(); err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("Text() invalid UTF-8 error = %v, want valid UTF-8", err)
	}
}

func TestOutputNotificationTextLossyKeepsPartialDecode(t *testing.T) {
	t.Parallel()
	// Valid prefix followed by an incomplete escape; lossy decoder must return
	// the bytes decoded before the error rather than collapsing to "".
	got := (OutputNotification{Value: `ok\01`}).TextLossy()
	if got != "ok" {
		t.Fatalf("TextLossy() = %q, want %q", got, "ok")
	}
}
