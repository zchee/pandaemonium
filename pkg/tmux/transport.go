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
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
)

type transport interface {
	io.Closer
	ReadLine(ctx context.Context) (string, error)
	WriteLine(ctx context.Context, line string) error
}

type stdioTransport struct {
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

var _ transport = (*stdioTransport)(nil)

func (t *stdioTransport) Close() error {
	if t == nil || t.stdin == nil {
		return nil
	}
	return t.stdin.Close()
}

func (t *stdioTransport) ReadLine(_ context.Context) (string, error) {
	if t == nil || t.stdout == nil {
		return "", ErrClosed
	}
	line, err := t.stdout.ReadString('\n')
	if line != "" {
		trimmed := trimLineEnding(line)
		if errors.Is(err, io.EOF) {
			return trimmed, io.EOF
		}
		if err != nil {
			return trimmed, err
		}
		return trimmed, nil
	}
	if errors.Is(err, io.EOF) {
		return "", io.EOF
	}
	return "", err
}

func (t *stdioTransport) WriteLine(_ context.Context, line string) error {
	if t == nil || t.stdin == nil {
		return ErrClosed
	}
	if _, err := io.WriteString(t.stdin, line+"\n"); err != nil {
		return err
	}
	return nil
}

func trimLineEnding(line string) string {
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line
}
