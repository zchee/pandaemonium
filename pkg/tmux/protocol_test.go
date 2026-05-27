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
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

func TestProtocolParserResponses(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		lines   []string
		want    Response
		wantErr string
	}{
		"success: empty response": {
			lines: []string{"%begin 1578920019 258 0", "%end 1578920019 258 0"},
			want:  Response{Begin: marker(1578920019, 258, 0), End: marker(1578920019, 258, 0)},
		},
		"success: multiline response": {
			lines: []string{"%begin 1578922740 269 1", "one", "%not-a-notification inside output", "two", "%end 1578922740 269 1"},
			want:  Response{Begin: marker(1578922740, 269, 1), End: marker(1578922740, 269, 1), Lines: []string{"one", "%not-a-notification inside output", "two"}},
		},
		"success: command error response": {
			lines: []string{"%begin 1578923149 270 1", "parse error", "%error 1578923149 270 1"},
			want:  Response{Begin: marker(1578923149, 270, 1), End: marker(1578923149, 270, 1), Lines: []string{"parse error"}, Error: true},
		},
		"success: fake end payload does not terminate": {
			lines: []string{"%begin 1 2 1", "%end payload", "%end 1 2 1"},
			want:  Response{Begin: marker(1, 2, 1), End: marker(1, 2, 1), Lines: []string{"%end payload"}},
		},
		"success: malformed end-like payload does not terminate": {
			lines: []string{"%begin 1 2 1", "%end a b c", "%end 1 2 1"},
			want:  Response{Begin: marker(1, 2, 1), End: marker(1, 2, 1), Lines: []string{"%end a b c"}},
		},
		"success: malformed error-like payload does not terminate": {
			lines: []string{"%begin 1 2 1", "%error one two three", "%end 1 2 1"},
			want:  Response{Begin: marker(1, 2, 1), End: marker(1, 2, 1), Lines: []string{"%error one two three"}},
		},
		"error: mismatched end marker": {
			lines:   []string{"%begin 1 2 1", "%end 1 3 1"},
			wantErr: "does not match active command",
		},
		"error: malformed begin marker": {
			lines:   []string{"%begin one 2 1"},
			wantErr: "invalid marker time",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			parser := &protocolParser{}
			var got Response
			for _, line := range tt.lines {
				msg, err := parser.feed(line)
				if err != nil {
					if tt.wantErr == "" || !strings.Contains(err.Error(), tt.wantErr) {
						t.Fatalf("feed(%q) error = %v, want containing %q", line, err, tt.wantErr)
					}
					return
				}
				if msg.kind == protocolMessageResponse {
					got = msg.response
				}
			}
			if tt.wantErr != "" {
				t.Fatalf("feed() error = nil, want containing %q", tt.wantErr)
			}
			assertResponse(t, got, tt.want)
		})
	}
}

func TestProtocolParserNotificationsAndControlSequences(t *testing.T) {
	t.Parallel()
	parser := &protocolParser{}
	if msg, err := parser.feed("\x1bP1000p%output %1 hello"); err != nil {
		t.Fatalf("feed() error = %v", err)
	} else if msg.kind != protocolMessageNotification || msg.notification.Kind != NotificationOutput {
		t.Fatalf("feed() message = %#v, want output notification", msg)
	}
	if msg, err := parser.feed("\x1b\\"); err != nil {
		t.Fatalf("feed(ST) error = %v", err)
	} else if msg.kind != protocolMessageNone {
		t.Fatalf("feed(ST) message = %#v, want none", msg)
	}
	if msg, err := parser.feed("%beginning future notification"); err != nil {
		t.Fatalf("feed(beginning) error = %v", err)
	} else if msg.kind != protocolMessageNotification || msg.notification.Kind != "%beginning" {
		t.Fatalf("feed(beginning) message = %#v, want notification", msg)
	}
	if _, err := parser.feed("stray line"); err == nil || !strings.Contains(err.Error(), "unexpected non-control") {
		t.Fatalf("feed(stray) error = %v, want unexpected non-control", err)
	}
}

func TestProtocolParserEOFMidBlock(t *testing.T) {
	t.Parallel()
	parser := &protocolParser{}
	if _, err := parser.feed("%begin 1 2 1"); err != nil {
		t.Fatalf("feed(begin) error = %v", err)
	}
	err := parser.eof()
	if err == nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("eof() error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func marker(seconds int64, command, flags int) BlockMarker {
	m, err := parseMarker("%begin "+strconvFormat(seconds)+" "+strconvFormat(int64(command))+" "+strconvFormat(int64(flags)), "%begin")
	if err != nil {
		panic(err)
	}
	return m
}

func strconvFormat(v int64) string { return strconv.FormatInt(v, 10) }

func assertResponse(t *testing.T, got, want Response) {
	t.Helper()
	if !got.Begin.Time.Equal(want.Begin.Time) || got.Begin.Command != want.Begin.Command || got.Begin.Flags != want.Begin.Flags ||
		!got.End.Time.Equal(want.End.Time) || got.End.Command != want.End.Command || got.End.Flags != want.End.Flags ||
		got.Error != want.Error || strings.Join(got.Lines, "\x00") != strings.Join(want.Lines, "\x00") {
		t.Fatalf("response = %#v, want %#v", got, want)
	}
}
