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
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

var (
	errClosed      = errors.New("closed")
	errWriteFailed = errors.New("write failed")
)

func TestWriteJSONLine(t *testing.T) {
	t.Parallel()

	data := []byte(`{"id":1}`)
	var out bytes.Buffer
	if err := WriteJSONLine(&out, data, func() error { return errClosed }, func(err error) error { return err }); err != nil {
		t.Fatalf("WriteJSONLine() error = %v", err)
	}
	if got, want := out.String(), `{"id":1}`+"\n"; got != want {
		t.Fatalf("WriteJSONLine() wrote %q, want %q", got, want)
	}
	data[0] = '['
	if got, want := out.String(), `{"id":1}`+"\n"; got != want {
		t.Fatalf("WriteJSONLine() aliased input, got %q, want %q", got, want)
	}

	if err := WriteJSONLine(nil, data, func() error { return errClosed }, func(err error) error { return err }); !errors.Is(err, errClosed) {
		t.Fatalf("WriteJSONLine(nil) error = %v, want %v", err, errClosed)
	}
	if err := WriteJSONLine(failWriter{}, data, func() error { return errClosed }, func(err error) error { return err }); !errors.Is(err, errWriteFailed) {
		t.Fatalf("WriteJSONLine(failing) error = %v, want %v", err, errWriteFailed)
	}
}

func TestReadJSONLine(t *testing.T) {
	t.Parallel()

	reader := bufio.NewReader(bytes.NewBufferString("{}\n"))
	line, err := ReadJSONLine(reader, func() error { return errClosed })
	if err != nil {
		t.Fatalf("ReadJSONLine() error = %v", err)
	}
	if got, want := string(line), "{}\n"; got != want {
		t.Fatalf("ReadJSONLine() = %q, want %q", got, want)
	}
	if _, err := ReadJSONLine(bufio.NewReader(bytes.NewBufferString("partial")), func() error { return errClosed }); !errors.Is(err, io.EOF) {
		t.Fatalf("ReadJSONLine(partial EOF) error = %v, want io.EOF", err)
	}
	if _, err := ReadJSONLine(nil, func() error { return errClosed }); !errors.Is(err, errClosed) {
		t.Fatalf("ReadJSONLine(nil) error = %v, want %v", err, errClosed)
	}
}

func TestReadJSONLineContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := ReadJSONLineContext(ctx, bufio.NewReader(bytes.NewBuffer(nil)), func() error { return errClosed })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadJSONLineContext() error = %v, want context.Canceled", err)
	}
}
