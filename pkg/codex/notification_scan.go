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

package codex

import (
	"github.com/go-json-experiment/json"
)

func notificationTurnID(notif Notification) string {
	if turnID, ok := scanNotificationTurnID(notif.Params); ok {
		return turnID
	}
	return decodeNotificationTurnID(notif)
}

type scannedTurn struct {
	id      string
	turnID  string
	turnID2 string
}

type notificationJSONScanner struct {
	data []byte
	pos  int
}

func scanNotificationTurnID(params []byte) (string, bool) {
	scanner := notificationJSONScanner{data: params}
	scanner.skipSpace()
	if scanner.consumeLiteral("null") {
		scanner.skipSpace()
		return "", scanner.done()
	}
	if !scanner.consumeByte('{') {
		return "", false
	}

	var turnID, turnID2 string
	var turn scannedTurn
	for {
		scanner.skipSpace()
		if scanner.consumeByte('}') {
			break
		}
		key, escaped, ok := scanner.readString()
		if !ok || escaped {
			return "", false
		}
		scanner.skipSpace()
		if !scanner.consumeByte(':') {
			return "", false
		}
		switch {
		case bytesEqualString(key, "turnId"):
			value, ok := scanner.readSimpleString()
			if !ok {
				return "", false
			}
			turnID = value
		case bytesEqualString(key, "turn_id"):
			value, ok := scanner.readSimpleString()
			if !ok {
				return "", false
			}
			turnID2 = value
		case bytesEqualString(key, "turn"):
			value, ok := scanner.readTurnObject()
			if !ok {
				return "", false
			}
			turn = value
		default:
			if !scanner.skipValue() {
				return "", false
			}
		}
		scanner.skipSpace()
		if scanner.consumeByte('}') {
			break
		}
		if !scanner.consumeByte(',') {
			return "", false
		}
	}
	scanner.skipSpace()
	if !scanner.done() {
		return "", false
	}
	if turnID != "" {
		return turnID, true
	}
	if turnID2 != "" {
		return turnID2, true
	}
	if turn.turnID != "" {
		return turn.turnID, true
	}
	if turn.turnID2 != "" {
		return turn.turnID2, true
	}
	return turn.id, true
}

func (s *notificationJSONScanner) readTurnObject() (scannedTurn, bool) {
	s.skipSpace()
	if s.consumeLiteral("null") {
		return scannedTurn{}, true
	}
	if !s.consumeByte('{') {
		return scannedTurn{}, false
	}

	var turn scannedTurn
	for {
		s.skipSpace()
		if s.consumeByte('}') {
			return turn, true
		}
		key, escaped, ok := s.readString()
		if !ok || escaped {
			return scannedTurn{}, false
		}
		s.skipSpace()
		if !s.consumeByte(':') {
			return scannedTurn{}, false
		}
		switch {
		case bytesEqualString(key, "id"):
			value, ok := s.readSimpleString()
			if !ok {
				return scannedTurn{}, false
			}
			turn.id = value
		case bytesEqualString(key, "turnId"):
			value, ok := s.readSimpleString()
			if !ok {
				return scannedTurn{}, false
			}
			turn.turnID = value
		case bytesEqualString(key, "turn_id"):
			value, ok := s.readSimpleString()
			if !ok {
				return scannedTurn{}, false
			}
			turn.turnID2 = value
		default:
			if !s.skipValue() {
				return scannedTurn{}, false
			}
		}
		s.skipSpace()
		if s.consumeByte('}') {
			return turn, true
		}
		if !s.consumeByte(',') {
			return scannedTurn{}, false
		}
	}
}

func (s *notificationJSONScanner) skipValue() bool {
	s.skipSpace()
	if s.done() {
		return false
	}
	switch s.data[s.pos] {
	case '"':
		_, _, ok := s.readString()
		return ok
	case '{':
		return s.skipObject()
	case '[':
		return s.skipArray()
	case 't':
		return s.consumeLiteral("true")
	case 'f':
		return s.consumeLiteral("false")
	case 'n':
		return s.consumeLiteral("null")
	default:
		return s.skipNumber()
	}
}

func (s *notificationJSONScanner) skipObject() bool {
	if !s.consumeByte('{') {
		return false
	}
	for {
		s.skipSpace()
		if s.consumeByte('}') {
			return true
		}
		if _, _, ok := s.readString(); !ok {
			return false
		}
		s.skipSpace()
		if !s.consumeByte(':') {
			return false
		}
		if !s.skipValue() {
			return false
		}
		s.skipSpace()
		if s.consumeByte('}') {
			return true
		}
		if !s.consumeByte(',') {
			return false
		}
	}
}

func (s *notificationJSONScanner) skipArray() bool {
	if !s.consumeByte('[') {
		return false
	}
	for {
		s.skipSpace()
		if s.consumeByte(']') {
			return true
		}
		if !s.skipValue() {
			return false
		}
		s.skipSpace()
		if s.consumeByte(']') {
			return true
		}
		if !s.consumeByte(',') {
			return false
		}
	}
}

func (s *notificationJSONScanner) skipNumber() bool {
	start := s.pos
	if s.pos < len(s.data) && s.data[s.pos] == '-' {
		s.pos++
	}
	if s.pos >= len(s.data) {
		return false
	}
	if s.data[s.pos] == '0' {
		s.pos++
	} else if isDigitOneToNine(s.data[s.pos]) {
		s.pos++
		for s.pos < len(s.data) && isDigit(s.data[s.pos]) {
			s.pos++
		}
	} else {
		return false
	}
	if s.pos < len(s.data) && s.data[s.pos] == '.' {
		s.pos++
		if s.pos >= len(s.data) || !isDigit(s.data[s.pos]) {
			return false
		}
		for s.pos < len(s.data) && isDigit(s.data[s.pos]) {
			s.pos++
		}
	}
	if s.pos < len(s.data) && (s.data[s.pos] == 'e' || s.data[s.pos] == 'E') {
		s.pos++
		if s.pos < len(s.data) && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
			s.pos++
		}
		if s.pos >= len(s.data) || !isDigit(s.data[s.pos]) {
			return false
		}
		for s.pos < len(s.data) && isDigit(s.data[s.pos]) {
			s.pos++
		}
	}
	return s.pos > start
}

// readSimpleString reads a non-escaped JSON string and returns an allocating
// copy of the value. The returned string is safe to use beyond the lifetime of
// the underlying Notification.Params slice.
func (s *notificationJSONScanner) readSimpleString() (string, bool) {
	value, escaped, ok := s.readString()
	if !ok || escaped {
		return "", false
	}
	return string(value), true
}

func (s *notificationJSONScanner) readString() ([]byte, bool, bool) {
	if !s.consumeByte('"') {
		return nil, false, false
	}
	start := s.pos
	escaped := false
	for s.pos < len(s.data) {
		switch c := s.data[s.pos]; c {
		case '"':
			value := s.data[start:s.pos]
			s.pos++
			return value, escaped, true
		case '\\':
			escaped = true
			s.pos++
			if s.pos >= len(s.data) {
				return nil, false, false
			}
			switch s.data[s.pos] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				s.pos++
			case 'u':
				s.pos++
				for range 4 {
					if s.pos >= len(s.data) || !isHexDigit(s.data[s.pos]) {
						return nil, false, false
					}
					s.pos++
				}
			default:
				return nil, false, false
			}
		default:
			if c < 0x20 {
				return nil, false, false
			}
			s.pos++
		}
	}
	return nil, false, false
}

func (s *notificationJSONScanner) skipSpace() {
	for s.pos < len(s.data) {
		switch s.data[s.pos] {
		case ' ', '\n', '\r', '\t':
			s.pos++
		default:
			return
		}
	}
}

func (s *notificationJSONScanner) consumeByte(c byte) bool {
	if s.pos >= len(s.data) || s.data[s.pos] != c {
		return false
	}
	s.pos++
	return true
}

func (s *notificationJSONScanner) consumeLiteral(literal string) bool {
	if len(s.data)-s.pos < len(literal) {
		return false
	}
	for i := range len(literal) {
		if s.data[s.pos+i] != literal[i] {
			return false
		}
	}
	s.pos += len(literal)
	return true
}

func (s *notificationJSONScanner) done() bool {
	return s.pos == len(s.data)
}

func bytesEqualString(value []byte, want string) bool {
	if len(value) != len(want) {
		return false
	}
	for i := range value {
		if value[i] != want[i] {
			return false
		}
	}
	return true
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isDigitOneToNine(c byte) bool {
	return c >= '1' && c <= '9'
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func decodeNotificationTurnID(notif Notification) string {
	var envelope struct {
		TurnID  string `json:"turnId"`
		TurnID2 string `json:"turn_id"`
		Turn    *struct {
			ID      string `json:"id"`
			TurnID  string `json:"turnId"`
			TurnID2 string `json:"turn_id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(notif.Params, &envelope); err != nil {
		return ""
	}
	if envelope.TurnID != "" {
		return envelope.TurnID
	}
	if envelope.TurnID2 != "" {
		return envelope.TurnID2
	}
	if envelope.Turn != nil {
		if envelope.Turn.TurnID != "" {
			return envelope.Turn.TurnID
		}
		if envelope.Turn.TurnID2 != "" {
			return envelope.Turn.TurnID2
		}
		return envelope.Turn.ID
	}
	return ""
}
