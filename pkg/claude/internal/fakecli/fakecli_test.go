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

package fakecli_test

import (
	"context"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zchee/pandaemonium/pkg/claude/internal/fakecli"
)

func TestNew_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	f := fakecli.New(t, nil)
	if f == nil {
		t.Fatal("New() = nil, want *FakeCLI")
	}
}

func TestFakeCLI_WriteJSON_RecordsPayload(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		payloads [][]byte
		want     [][]byte
	}{
		"success: single write is recorded": {
			payloads: [][]byte{[]byte(`{"type":"ping"}`)},
			want:     [][]byte{[]byte(`{"type":"ping"}`)},
		},
		"success: multiple writes are recorded in order": {
			payloads: [][]byte{
				[]byte(`{"id":1}`),
				[]byte(`{"id":2}`),
				[]byte(`{"id":3}`),
			},
			want: [][]byte{
				[]byte(`{"id":1}`),
				[]byte(`{"id":2}`),
				[]byte(`{"id":3}`),
			},
		},
		"success: nil script does not block WriteJSON": {
			payloads: [][]byte{[]byte(`{}`)},
			want:     [][]byte{[]byte(`{}`)},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := fakecli.New(t, nil) // empty script

			ctx := t.Context()
			for _, p := range tt.payloads {
				if err := f.WriteJSON(ctx, p); err != nil {
					t.Fatalf("WriteJSON(%q) = %v, want nil", p, err)
				}
			}

			got := f.Written()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Written() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFakeCLI_WriteJSON_EnqueuesFrameLines(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		script []fakecli.Frame
		writes int
		want   []string // lines expected from ReadJSON in order
	}{
		"success: single frame single line": {
			script: []fakecli.Frame{
				{Lines: []string{`{"type":"text"}`}},
			},
			writes: 1,
			want:   []string{`{"type":"text"}` + "\n"},
		},
		"success: single frame multiple lines": {
			script: []fakecli.Frame{
				{Lines: []string{`{"seq":1}`, `{"seq":2}`, `{"seq":3}`}},
			},
			writes: 1,
			want: []string{
				`{"seq":1}` + "\n",
				`{"seq":2}` + "\n",
				`{"seq":3}` + "\n",
			},
		},
		"success: multiple frames each enqueue own lines": {
			script: []fakecli.Frame{
				{Lines: []string{`{"frame":0}`}},
				{Lines: []string{`{"frame":1}`}},
			},
			writes: 2,
			want: []string{
				`{"frame":0}` + "\n",
				`{"frame":1}` + "\n",
			},
		},
		"success: extra writes beyond script are no-ops for lines": {
			script: []fakecli.Frame{
				{Lines: []string{`{"only":true}`}},
			},
			writes: 3, // only first write enqueues a frame
			want:   []string{`{"only":true}` + "\n"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := fakecli.New(t, tt.script)
			ctx := t.Context()

			for range tt.writes {
				if err := f.WriteJSON(ctx, []byte(`{}`)); err != nil {
					t.Fatalf("WriteJSON() = %v, want nil", err)
				}
			}

			var got []string
			for range len(tt.want) {
				line, err := f.ReadJSON(ctx)
				if err != nil {
					t.Fatalf("ReadJSON() = %v, want nil", err)
				}
				got = append(got, string(line))
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ReadJSON lines mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFakeCLI_ReadJSON_BlocksUntilClose(t *testing.T) {
	t.Parallel()

	f := fakecli.New(t, nil) // empty script — ReadJSON will block

	done := make(chan error, 1)
	go func() {
		_, err := f.ReadJSON(t.Context())
		done <- err
	}()

	// Close the FakeCLI; the blocking ReadJSON must return io.EOF.
	if err := f.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	if err := <-done; !isEOF(err) {
		t.Fatalf("ReadJSON() after Close = %v, want io.EOF", err)
	}
}

func TestFakeCLI_ReadJSON_ContextCancellation(t *testing.T) {
	t.Parallel()

	f := fakecli.New(t, nil)

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)
	go func() {
		_, err := f.ReadJSON(ctx)
		done <- err
	}()

	cancel()

	if err := <-done; err == nil {
		t.Fatal("ReadJSON() after cancel = nil, want context error")
	}
}

func TestFakeCLI_WriteJSON_AfterClose_ReturnsError(t *testing.T) {
	t.Parallel()

	f := fakecli.New(t, nil)
	if err := f.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	err := f.WriteJSON(t.Context(), []byte(`{}`))
	if err == nil {
		t.Fatal("WriteJSON() after Close = nil, want error")
	}
}

func TestFakeCLI_Close_IsIdempotent(t *testing.T) {
	t.Parallel()

	f := fakecli.New(t, nil)
	for range 3 {
		if err := f.Close(); err != nil {
			t.Fatalf("Close() = %v, want nil", err)
		}
	}
}

func TestFakeCLI_FrameIdx_AdvancesPerWrite(t *testing.T) {
	t.Parallel()

	script := []fakecli.Frame{
		{Lines: []string{`{"a":1}`}},
		{Lines: []string{`{"b":2}`}},
	}
	f := fakecli.New(t, script)
	ctx := t.Context()

	if got := f.FrameIdx(); got != 0 {
		t.Fatalf("FrameIdx() before writes = %d, want 0", got)
	}

	_ = f.WriteJSON(ctx, []byte(`{}`))
	if got := f.FrameIdx(); got != 1 {
		t.Fatalf("FrameIdx() after 1 write = %d, want 1", got)
	}

	_ = f.WriteJSON(ctx, []byte(`{}`))
	if got := f.FrameIdx(); got != 2 {
		t.Fatalf("FrameIdx() after 2 writes = %d, want 2", got)
	}

	// Beyond script length — FrameIdx must not exceed len(script).
	_ = f.WriteJSON(ctx, []byte(`{}`))
	if got := f.FrameIdx(); got != 2 {
		t.Fatalf("FrameIdx() after excess write = %d, want 2 (capped at script length)", got)
	}
}

func TestFakeCLI_Written_IsSnapshot(t *testing.T) {
	t.Parallel()

	f := fakecli.New(t, nil)
	ctx := t.Context()

	p1 := []byte(`{"seq":1}`)
	p2 := []byte(`{"seq":2}`)
	_ = f.WriteJSON(ctx, p1)
	_ = f.WriteJSON(ctx, p2)

	snap1 := f.Written()
	_ = f.WriteJSON(ctx, []byte(`{"seq":3}`))
	snap2 := f.Written()

	if len(snap1) != 2 {
		t.Fatalf("Written() before 3rd write = %d payloads, want 2", len(snap1))
	}
	if len(snap2) != 3 {
		t.Fatalf("Written() after 3rd write = %d payloads, want 3", len(snap2))
	}

	// Mutate snap1; snap2 must be independent.
	snap1[0][0] = 'X'
	if snap2[0][0] != '{' {
		t.Error("Written() snapshots share underlying memory (not independent copies)")
	}
}

// isEOF returns true if err is io.EOF or wraps it.
func isEOF(err error) bool {
	return err == io.EOF
}
