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
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	controlModeEnterDCS = "\x1bP1000p"
	controlModeExitST   = "\x1b\\"
)

// BlockMarker is a tmux `%begin`, `%end`, or `%error` response marker.
type BlockMarker struct {
	// Time is the marker timestamp in seconds from the Unix epoch.
	Time time.Time
	// Command is tmux's unique command number for the response block.
	Command int
	// Flags are the marker flags reported by tmux.
	Flags int
}

// Response is one guarded tmux command response block.
type Response struct {
	// Begin is the opening `%begin` marker.
	Begin BlockMarker
	// End is the closing `%end` or `%error` marker.
	End BlockMarker
	// Lines are the command output lines between the guard markers.
	Lines []string
	// Error reports whether the block ended with `%error`.
	Error bool
}

type protocolMessageKind uint8

const (
	protocolMessageNone protocolMessageKind = iota
	protocolMessageResponse
	protocolMessageNotification
)

type protocolMessage struct {
	kind         protocolMessageKind
	response     Response
	notification Notification
}

// Event is one complete control-mode response or asynchronous notification.
//
// A zero Event means the parser accepted a line but has not completed a
// response block or notification yet.
type Event struct {
	// Response is set when a complete `%begin`/`%end` or `%begin`/`%error`
	// command response block has been read.
	Response *Response
	// Notification is set when a complete `%` notification has been read.
	Notification *Notification
}

// Parser incrementally parses tmux control-mode lines.
//
// Parser is reusable for callers that already manage a tmux control-mode
// transport and want the same response/notification splitting used by Client.
type Parser struct {
	parser protocolParser
}

// Feed parses one newline-free tmux control-mode line.
func (p *Parser) Feed(line string) (Event, error) {
	if p == nil {
		return Event{}, fmt.Errorf("tmux: nil parser")
	}
	message, err := p.parser.feed(line)
	if err != nil {
		return Event{}, err
	}
	switch message.kind {
	case protocolMessageResponse:
		response := message.response
		return Event{Response: &response}, nil
	case protocolMessageNotification:
		notification := message.notification
		return Event{Notification: &notification}, nil
	default:
		return Event{}, nil
	}
}

// Close reports whether the input ended in the middle of a response block.
func (p *Parser) Close() error {
	if p == nil {
		return nil
	}
	return p.parser.eof()
}

type protocolParser struct {
	active *responseBuilder
}

type responseBuilder struct {
	begin BlockMarker
	lines []string
}

func (p *protocolParser) feed(line string) (protocolMessage, error) {
	line, ok := normalizeControlLine(line)
	if !ok {
		return protocolMessage{}, nil
	}

	if p.active != nil {
		kind, marker, ok := parseTerminator(line)
		if ok && sameMarkerIdentity(p.active.begin, marker) {
			response := Response{
				Begin: p.active.begin,
				End:   marker,
				Lines: slices.Clone(p.active.lines),
				Error: kind == "%error",
			}
			p.active = nil
			return protocolMessage{kind: protocolMessageResponse, response: response}, nil
		}
		p.active.lines = append(p.active.lines, line)
		return protocolMessage{}, nil
	}

	marker, ok, err := parseBegin(line)
	if err != nil {
		return protocolMessage{}, &ProtocolError{Line: line, Err: err}
	}
	if ok {
		p.active = &responseBuilder{begin: marker}
		return protocolMessage{}, nil
	}
	if strings.HasPrefix(line, "%") {
		notification, err := ParseNotification(line)
		if err != nil {
			return protocolMessage{}, err
		}
		return protocolMessage{kind: protocolMessageNotification, notification: notification}, nil
	}
	return protocolMessage{}, &ProtocolError{Line: line, Err: fmt.Errorf("unexpected non-control line outside response block")}
}

func (p *protocolParser) eof() error {
	if p.active == nil {
		return nil
	}
	return &ProtocolError{Err: fmt.Errorf("%w after %%begin for command %d", io.ErrUnexpectedEOF, p.active.begin.Command)}
}

func normalizeControlLine(line string) (string, bool) {
	line = strings.ReplaceAll(line, controlModeEnterDCS, "")
	line = strings.ReplaceAll(line, controlModeExitST, "")
	if line == "" {
		return "", false
	}
	return line, true
}

func parseBegin(line string) (BlockMarker, bool, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] != "%begin" {
		return BlockMarker{}, false, nil
	}
	marker, err := parseMarker(line, "%begin")
	return marker, err == nil, err
}

func parseTerminator(line string) (rawMarker string, blockMarker BlockMarker, ok bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", BlockMarker{}, false
	}
	if fields[0] == "%end" {
		if len(fields) != 4 {
			return "", BlockMarker{}, false
		}
		marker, err := parseMarker(line, "%end")
		if err != nil {
			return "", BlockMarker{}, false
		}
		return "%end", marker, true
	}
	if fields[0] == "%error" {
		if len(fields) != 4 {
			return "", BlockMarker{}, false
		}
		marker, err := parseMarker(line, "%error")
		if err != nil {
			return "", BlockMarker{}, false
		}
		return "%error", marker, true
	}
	return "", BlockMarker{}, false
}

func parseMarker(line, prefix string) (BlockMarker, error) {
	fields := strings.Fields(line)
	if len(fields) != 4 || fields[0] != prefix {
		return BlockMarker{}, fmt.Errorf("malformed %s marker", prefix)
	}
	seconds, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return BlockMarker{}, fmt.Errorf("invalid marker time %q: %w", fields[1], err)
	}
	command, err := strconv.Atoi(fields[2])
	if err != nil {
		return BlockMarker{}, fmt.Errorf("invalid marker command %q: %w", fields[2], err)
	}
	flags, err := strconv.Atoi(fields[3])
	if err != nil {
		return BlockMarker{}, fmt.Errorf("invalid marker flags %q: %w", fields[3], err)
	}
	return BlockMarker{Time: time.Unix(seconds, 0), Command: command, Flags: flags}, nil
}

func sameMarkerIdentity(a, b BlockMarker) bool {
	return a.Command == b.Command && a.Time.Equal(b.Time)
}
