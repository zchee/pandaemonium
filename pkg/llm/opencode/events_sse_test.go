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

package opencode

import (
	"errors"
	"io"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
)

// chunkedReader feeds its payload n bytes at a time, forcing partial frames
// at every buffer boundary.
type chunkedReader struct {
	payload []byte
	offset  int
	chunk   int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.payload) {
		return 0, io.EOF
	}
	n := min(r.chunk, len(r.payload)-r.offset, len(p))
	copy(p, r.payload[r.offset:r.offset+n])
	r.offset += n
	return n, nil
}

func collectFrames(t *testing.T, r io.Reader) []sseFrame {
	t.Helper()
	scanner := newSSEScanner(r)
	var frames []sseFrame
	for {
		frame, err := scanner.next()
		if errors.Is(err, io.EOF) {
			return frames
		}
		if err != nil {
			t.Fatalf("scanner.next: %v", err)
		}
		frames = append(frames, frame)
	}
}

func TestSSEScanner(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  []sseFrame
	}{
		"success: single data frame": {
			input: "data: {\"type\":\"server.connected\"}\n\n",
			want:  []sseFrame{{data: []byte(`{"type":"server.connected"}`)}},
		},
		"success: multiple frames in order": {
			input: "data: one\n\ndata: two\n\n",
			want:  []sseFrame{{data: []byte("one")}, {data: []byte("two")}},
		},
		"success: multi-line data joined with newline": {
			input: "data: {\"a\":\ndata: 1}\n\n",
			want:  []sseFrame{{data: []byte("{\"a\":\n1}")}},
		},
		"success: comment and heartbeat lines skipped": {
			input: ": keepalive\n\n: another comment\ndata: real\n\n",
			want:  []sseFrame{{data: []byte("real")}},
		},
		"success: CRLF line endings tolerated": {
			input: "data: crlf-frame\r\n\r\n",
			want:  []sseFrame{{data: []byte("crlf-frame")}},
		},
		"success: event and id fields captured": {
			input: "event: message\nid: 42\ndata: payload\n\n",
			want:  []sseFrame{{event: "message", id: "42", data: []byte("payload")}},
		},
		"success: data without space after colon": {
			input: "data:nospace\n\n",
			want:  []sseFrame{{data: []byte("nospace")}},
		},
		"success: unknown fields ignored": {
			input: "retry: 1000\nweird: field\ndata: kept\n\n",
			want:  []sseFrame{{data: []byte("kept")}},
		},
		"success: empty data line preserved in join": {
			input: "data: first\ndata:\ndata: third\n\n",
			want:  []sseFrame{{data: []byte("first\n\nthird")}},
		},
		"success: final unterminated frame delivered at EOF": {
			input: "data: tail-frame\n",
			want:  []sseFrame{{data: []byte("tail-frame")}},
		},
		"success: field-only frame without data skipped": {
			input: "event: orphan\n\ndata: kept\n\n",
			want:  []sseFrame{{data: []byte("kept")}},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := collectFrames(t, strings.NewReader(tt.input))
			if diff := gocmp.Diff(tt.want, got, gocmp.AllowUnexported(sseFrame{})); diff != "" {
				t.Errorf("frames mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSSEScannerPartialFramesAtEveryBoundary(t *testing.T) {
	t.Parallel()

	payload := ": comment\r\nevent: e1\ndata: {\"type\":\"message.part.updated\",\ndata: \"properties\":{\"sessionID\":\"ses_1\"}}\r\n\r\ndata: {\"type\":\"session.idle\",\"properties\":{\"sessionID\":\"ses_1\"}}\n\n"
	want := collectFrames(t, strings.NewReader(payload))

	for chunk := 1; chunk <= len(payload); chunk++ {
		got := collectFrames(t, &chunkedReader{payload: []byte(payload), chunk: chunk})
		if diff := gocmp.Diff(want, got, gocmp.AllowUnexported(sseFrame{})); diff != "" {
			t.Fatalf("chunk size %d: frames mismatch (-want +got):\n%s", chunk, diff)
		}
	}
}

func TestSSEScannerLongLine(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 1<<20) // 1MiB single line, within the cap
	frames := collectFrames(t, strings.NewReader("data: "+long+"\n\n"))
	if len(frames) != 1 || string(frames[0].data) != long {
		t.Fatalf("long line not preserved (got %d frames)", len(frames))
	}

	scanner := newSSEScanner(strings.NewReader("data: " + strings.Repeat("y", maxSSELineLength+1) + "\n\n"))
	if _, err := scanner.next(); err == nil {
		t.Fatal("over-cap line must error, not OOM")
	}
}
