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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// maxSSELineLength caps one SSE line to defend against a misbehaving stream;
// legitimate opencode events are far smaller than this.
const maxSSELineLength = 16 << 20

// sseFrame is one parsed server-sent event.
//
// OpenCode only populates data lines (the event id rides inside the JSON
// payload), but the parser tolerates the full wire format: event:/id: fields,
// comment lines, multi-line data, and CRLF or LF line endings.
type sseFrame struct {
	event string
	id    string
	data  []byte
}

// sseScanner incrementally parses text/event-stream frames from r.
type sseScanner struct {
	reader *bufio.Reader
}

func newSSEScanner(r io.Reader) *sseScanner {
	return &sseScanner{reader: bufio.NewReader(r)}
}

// readLine reads one line, tolerating LF and CRLF endings, bounded by
// maxSSELineLength. It returns the line without its terminator. err is
// io.EOF only when the stream ended before any byte of a new line.
func (s *sseScanner) readLine() (string, error) {
	line, err := s.reader.ReadString('\n')
	if len(line) > maxSSELineLength {
		return "", fmt.Errorf("opencode: SSE line exceeds %d bytes", maxSSELineLength)
	}
	if err != nil {
		if errors.Is(err, io.EOF) && line != "" {
			// Final unterminated line: deliver it; the next call reports EOF.
			return strings.TrimSuffix(line, "\r"), nil
		}
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, nil
}

// next returns the next complete frame. Frames without data (heartbeat
// comments, stray field-only records) are skipped. io.EOF signals a cleanly
// closed stream.
func (s *sseScanner) next() (sseFrame, error) {
	var frame sseFrame
	var data []string
	haveData := false

	for {
		line, err := s.readLine()
		if err != nil {
			if errors.Is(err, io.EOF) && haveData {
				frame.data = []byte(strings.Join(data, "\n"))
				return frame, nil
			}
			return sseFrame{}, err
		}

		if line == "" {
			// Dispatch boundary.
			if !haveData {
				frame = sseFrame{}
				continue
			}
			frame.data = []byte(strings.Join(data, "\n"))
			return frame, nil
		}
		if strings.HasPrefix(line, ":") {
			continue // comment / keepalive
		}

		field, value, cut := strings.Cut(line, ":")
		if cut {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "data":
			data = append(data, value)
			haveData = true
		case "event":
			frame.event = value
		case "id":
			frame.id = value
		default:
			// Unknown fields (including "retry") are tolerated and ignored.
		}
	}
}

// dialEvents opens GET /event as a text/event-stream. The request binds to
// ctx (the bus lifetime — closing it terminates the stream read); dial
// success additionally requires HTTP 200 with a text/event-stream content
// type. The first server.connected handshake is enforced by the bus, which
// bounds it with the dial deadline.
func dialEvents(ctx context.Context, httpClient *http.Client, baseURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/event", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("opencode: build event stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-store")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencode: GET /event: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		return nil, mapHTTPError(resp.StatusCode, http.MethodGet, "/event", body)
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "text/event-stream" {
		resp.Body.Close()
		return nil, fmt.Errorf("opencode: GET /event: unexpected content type %q", resp.Header.Get("Content-Type"))
	}
	return resp.Body, nil
}
