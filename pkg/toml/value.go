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

package toml

import (
	"errors"
	"io"
	"strconv"
	"strings"
)

type documentMap map[string]any

func parseDocument(data []byte, opts []Option) (documentMap, error) {
	dec := NewDecoderBytes(data, opts...)
	root := documentMap{}
	current := root
	var currentPath []string
	for {
		tok, err := dec.ReadToken()
		if errors.Is(err, io.EOF) {
			return root, nil
		}
		if err != nil {
			return nil, err
		}
		switch tok.Kind {
		case TokenKindComment:
			continue
		case TokenKindTableHeader:
			path, err := parseHeaderKey(tok.Bytes, false)
			if err != nil {
				return nil, err
			}
			currentPath = path
			current = ensureTable(root, path)
		case TokenKindArrayTableHeader:
			path, err := parseHeaderKey(tok.Bytes, true)
			if err != nil {
				return nil, err
			}
			currentPath = path
			current = appendArrayTable(root, path)
		case TokenKindKey:
			key, err := parseDottedKey(tok.Bytes)
			if err != nil {
				return nil, err
			}
			value, err := parseNextValue(dec)
			if err != nil {
				return nil, err
			}
			assign(current, key, value)
		default:
			if len(currentPath) == 0 {
				return nil, &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "unexpected token", Span: [2]int{0, 1}}
			}
		}
	}
}

func parseNextValue(dec *Decoder) (any, error) {
	for {
		tok, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}
		if tok.Kind == TokenKindComment {
			continue
		}
		return parseValueToken(dec, tok)
	}
}

func parseValueToken(dec *Decoder, tok Token) (any, error) {
	switch tok.Kind {
	case TokenKindValueString:
		return parseStringValue(tok.Bytes)
	case TokenKindValueInteger:
		clean := strings.ReplaceAll(string(tok.Bytes), "_", "")
		return strconv.ParseInt(clean, 0, 64)
	case TokenKindValueFloat:
		clean := strings.ReplaceAll(strings.ToLower(string(tok.Bytes)), "_", "")
		return strconv.ParseFloat(clean, 64)
	case TokenKindValueBool:
		return strconv.ParseBool(strings.ToLower(string(tok.Bytes)))
	case TokenKindValueDatetime:
		v, _, err := parseDateTimeValue(tok.Bytes)
		return v, err
	case TokenKindArrayStart:
		return parseArrayValue(dec)
	case TokenKindInlineTableStart:
		return parseInlineTableValue(dec)
	default:
		return nil, &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "expected value", Span: [2]int{0, 1}}
	}
}

func parseArrayValue(dec *Decoder) ([]any, error) {
	var values []any
	for {
		tok, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}
		switch tok.Kind {
		case TokenKindComment:
			continue
		case TokenKindArrayEnd:
			return values, nil
		default:
			v, err := parseValueToken(dec, tok)
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		}
	}
}

func parseInlineTableValue(dec *Decoder) (documentMap, error) {
	m := documentMap{}
	for {
		tok, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}
		switch tok.Kind {
		case TokenKindComment:
			continue
		case TokenKindInlineTableEnd:
			return m, nil
		case TokenKindKey:
			key, err := parseDottedKey(tok.Bytes)
			if err != nil {
				return nil, err
			}
			v, err := parseNextValue(dec)
			if err != nil {
				return nil, err
			}
			assign(m, key, v)
		default:
			return nil, &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "expected inline table key", Span: [2]int{0, 1}}
		}
	}
}

func parseStringValue(raw []byte) (string, error) {
	s := string(raw)
	if strings.HasPrefix(s, "'''") && strings.HasSuffix(s, "'''") {
		return s[3 : len(s)-3], nil
	}
	if strings.HasPrefix(s, "\"\"\"") && strings.HasSuffix(s, "\"\"\"") {
		return s[3 : len(s)-3], nil
	}
	if strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") {
		return s[1 : len(s)-1], nil
	}
	return strconv.Unquote(s)
}

func parseHeaderKey(raw []byte, array bool) ([]string, error) {
	s := strings.TrimSpace(string(raw))
	if array {
		s = strings.TrimPrefix(strings.TrimSuffix(s, "]]"), "[[")
	} else {
		s = strings.TrimPrefix(strings.TrimSuffix(s, "]"), "[")
	}
	return parseDottedKey([]byte(s))
}

func parseDottedKey(raw []byte) ([]string, error) {
	s := strings.TrimSpace(string(raw))
	var parts []string
	for len(s) > 0 {
		s = strings.TrimLeft(s, " \t")
		if s == "" {
			break
		}
		var part string
		if s[0] == '\'' || s[0] == '"' {
			q := s[0]
			end := 1
			for end < len(s) {
				if s[end] == '\\' && q == '"' {
					end += 2
					continue
				}
				if s[end] == q {
					break
				}
				end++
			}
			if end >= len(s) {
				return nil, &SyntaxError{Line: 1, Col: 1, Msg: "unterminated quoted key", Span: [2]int{0, len(raw)}}
			}
			v, err := parseStringValue([]byte(s[:end+1]))
			if err != nil {
				return nil, err
			}
			part = v
			s = s[end+1:]
		} else {
			end := strings.IndexAny(s, ". \t")
			if end < 0 {
				part, s = s, ""
			} else {
				part, s = s[:end], s[end:]
			}
		}
		if part == "" {
			return nil, &SyntaxError{Line: 1, Col: 1, Msg: "empty key segment", Span: [2]int{0, len(raw)}}
		}
		parts = append(parts, part)
		s = strings.TrimLeft(s, " \t")
		if s == "" {
			break
		}
		if s[0] != '.' {
			return nil, &SyntaxError{Line: 1, Col: 1, Msg: "unexpected token in dotted key", Span: [2]int{0, len(raw)}}
		}
		s = s[1:]
	}
	if len(parts) == 0 {
		return nil, &SyntaxError{Line: 1, Col: 1, Msg: "empty key", Span: [2]int{0, len(raw)}}
	}
	return parts, nil
}

func ensureTable(root documentMap, path []string) documentMap {
	cur := root
	for _, p := range path {
		next, _ := cur[p].(documentMap)
		if next == nil {
			if arr, ok := cur[p].([]any); ok && len(arr) > 0 {
				if last, ok := arr[len(arr)-1].(documentMap); ok {
					next = last
				}
			}
		}
		if next == nil {
			next = documentMap{}
			cur[p] = next
		}
		cur = next
	}
	return cur
}

func appendArrayTable(root documentMap, path []string) documentMap {
	if len(path) == 0 {
		return root
	}
	parent := ensureTable(root, path[:len(path)-1])
	name := path[len(path)-1]
	table := documentMap{}
	arr, _ := parent[name].([]any)
	arr = append(arr, table)
	parent[name] = arr
	return table
}

func assign(root documentMap, path []string, value any) {
	cur := root
	for _, p := range path[:len(path)-1] {
		next, _ := cur[p].(documentMap)
		if next == nil {
			next = documentMap{}
			cur[p] = next
		}
		cur = next
	}
	cur[path[len(path)-1]] = value
}
