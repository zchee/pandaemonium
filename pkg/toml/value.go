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
	"bytes"
	"errors"
	"io"
	"strconv"
	"sync"
)

type documentMap map[string]any

const documentMapHint = 4

var (
	trueLiteral  = []byte("true")
	falseLiteral = []byte("false")
)

var documentMapPool = sync.Pool{
	New: func() any {
		return make(documentMap, documentMapHint)
	},
}

func parseDocument(data []byte, opts []Option, filter *decodeFilter) (documentMap, error) {
	dec := NewDecoderBytes(data, opts...)
	root := newDocumentMap()
	current := root
	currentFilter := filter
	inTable := false
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
			key := trimHeaderKey(tok.Bytes, false)
			if isSimpleBareKey(key) {
				if nextFilter, ok := filter.lookup(key); ok {
					current = ensureTableKey(root, string(key))
					currentFilter = nextFilter
				} else {
					current = nil
					currentFilter = nil
				}
				inTable = true
				continue
			}
			path, err := parseDottedKey(key)
			if err != nil {
				return nil, err
			}
			if nextFilter, ok := filter.lookupPath(path); ok {
				current = ensureTable(root, path)
				currentFilter = nextFilter
			} else {
				current = nil
				currentFilter = nil
			}
			inTable = true
		case TokenKindArrayTableHeader:
			key := trimHeaderKey(tok.Bytes, true)
			if isSimpleBareKey(key) {
				if nextFilter, ok := filter.lookup(key); ok {
					name := string(key)
					capacityHint := 0
					if _, exists := root[name]; !exists {
						capacityHint = bytes.Count(data, tok.Bytes)
					}
					current = appendArrayTableKey(root, name, capacityHint)
					currentFilter = nextFilter
				} else {
					current = nil
					currentFilter = nil
				}
				inTable = true
				continue
			}
			path, err := parseDottedKey(key)
			if err != nil {
				return nil, err
			}
			if nextFilter, ok := filter.lookupPath(path); ok {
				current = appendArrayTable(root, path)
				currentFilter = nextFilter
			} else {
				current = nil
				currentFilter = nil
			}
			inTable = true
		case TokenKindKey:
			if current == nil {
				if err := skipNextValue(dec); err != nil {
					return nil, err
				}
				continue
			}
			if _, ok := currentFilter.lookup(tok.Bytes); !ok {
				if err := skipNextValue(dec); err != nil {
					return nil, err
				}
				continue
			}
			if !isSimpleBareKey(tok.Bytes) {
				key, err := parseDottedKey(tok.Bytes)
				if err != nil {
					return nil, err
				}
				value, err := parseNextValue(dec)
				if err != nil {
					return nil, err
				}
				assign(current, key, value)
				continue
			}
			value, err := parseNextValue(dec)
			if err != nil {
				return nil, err
			}
			current[string(tok.Bytes)] = value
		default:
			if !inTable {
				return nil, &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "unexpected token", Span: [2]int{0, 1}}
			}
		}
	}
}

func skipNextValue(dec *Decoder) error {
	for {
		tok, err := dec.ReadToken()
		if err != nil {
			return err
		}
		if tok.Kind == TokenKindComment {
			continue
		}
		return skipValueToken(dec, tok)
	}
}

func skipValueToken(dec *Decoder, tok Token) error {
	depth := 0
	switch tok.Kind {
	case TokenKindArrayStart, TokenKindInlineTableStart:
		depth = 1
	case TokenKindValueString, TokenKindValueInteger, TokenKindValueFloat, TokenKindValueBool, TokenKindValueDatetime:
		return nil
	default:
		return &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "expected value", Span: [2]int{0, 1}}
	}
	for depth > 0 {
		next, err := dec.ReadToken()
		if err != nil {
			return err
		}
		switch next.Kind {
		case TokenKindArrayStart, TokenKindInlineTableStart:
			depth++
		case TokenKindArrayEnd, TokenKindInlineTableEnd:
			depth--
		}
	}
	return nil
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
		return strconv.ParseInt(normalizeNumericText(tok.Bytes, false), 0, 64)
	case TokenKindValueFloat:
		return strconv.ParseFloat(normalizeNumericText(tok.Bytes, true), 64)
	case TokenKindValueBool:
		switch {
		case bytes.EqualFold(tok.Bytes, trueLiteral):
			return true, nil
		case bytes.EqualFold(tok.Bytes, falseLiteral):
			return false, nil
		default:
			return strconv.ParseBool(string(tok.Bytes))
		}
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
	values := make([]any, 0, documentMapHint)
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
	m := newDocumentMap()
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
	if len(raw) >= 6 && raw[0] == '\'' && raw[1] == '\'' && raw[2] == '\'' &&
		raw[len(raw)-1] == '\'' && raw[len(raw)-2] == '\'' && raw[len(raw)-3] == '\'' {
		return string(raw[3 : len(raw)-3]), nil
	}
	if len(raw) >= 6 && raw[0] == '"' && raw[1] == '"' && raw[2] == '"' &&
		raw[len(raw)-1] == '"' && raw[len(raw)-2] == '"' && raw[len(raw)-3] == '"' {
		return string(raw[3 : len(raw)-3]), nil
	}
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		return string(raw[1 : len(raw)-1]), nil
	}
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' && bytes.IndexByte(raw[1:len(raw)-1], '\\') < 0 {
		return string(raw[1 : len(raw)-1]), nil
	}
	return strconv.Unquote(string(raw))
}

func parseHeaderKey(raw []byte, array bool) ([]string, error) {
	return parseDottedKey(trimHeaderKey(raw, array))
}

func trimHeaderKey(raw []byte, array bool) []byte {
	raw = bytes.TrimSpace(raw)
	if array {
		if len(raw) >= 4 {
			return bytes.TrimSpace(raw[2 : len(raw)-2])
		}
		return nil
	}
	if len(raw) >= 2 {
		return bytes.TrimSpace(raw[1 : len(raw)-1])
	}
	return nil
}

func isSimpleBareKey(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	for _, c := range raw {
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

func parseDottedKey(raw []byte) ([]string, error) {
	origLen := len(raw)
	raw = bytes.TrimSpace(raw)
	parts := make([]string, 0, bytes.Count(raw, []byte("."))+1)
	for len(raw) > 0 {
		raw = bytes.TrimLeft(raw, " \t")
		if len(raw) == 0 {
			break
		}
		var part string
		if raw[0] == '\'' || raw[0] == '"' {
			q := raw[0]
			end := 1
			for end < len(raw) {
				if raw[end] == '\\' && q == '"' {
					end += 2
					continue
				}
				if raw[end] == q {
					break
				}
				end++
			}
			if end >= len(raw) {
				return nil, &SyntaxError{Line: 1, Col: 1, Msg: "unterminated quoted key", Span: [2]int{0, origLen}}
			}
			v, err := parseStringValue(raw[:end+1])
			if err != nil {
				return nil, err
			}
			part = v
			raw = raw[end+1:]
		} else {
			end := bytes.IndexAny(raw, ". \t")
			if end < 0 {
				part = string(raw)
				raw = raw[:0]
			} else {
				part = string(raw[:end])
				raw = raw[end:]
			}
		}
		if part == "" {
			return nil, &SyntaxError{Line: 1, Col: 1, Msg: "empty key segment", Span: [2]int{0, origLen}}
		}
		parts = append(parts, part)
		raw = bytes.TrimLeft(raw, " \t")
		if len(raw) == 0 {
			break
		}
		if raw[0] != '.' {
			return nil, &SyntaxError{Line: 1, Col: 1, Msg: "unexpected token in dotted key", Span: [2]int{0, origLen}}
		}
		raw = raw[1:]
	}
	if len(parts) == 0 {
		return nil, &SyntaxError{Line: 1, Col: 1, Msg: "empty key", Span: [2]int{0, origLen}}
	}
	return parts, nil
}

func normalizeNumericText(raw []byte, lower bool) string {
	needsCopy := false
	for _, b := range raw {
		if b == '_' {
			needsCopy = true
			break
		}
		if lower && 'A' <= b && b <= 'Z' {
			needsCopy = true
			break
		}
	}
	if !needsCopy {
		return string(raw)
	}
	buf := make([]byte, 0, len(raw))
	for _, b := range raw {
		if b == '_' {
			continue
		}
		if lower && 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		buf = append(buf, b)
	}
	return string(buf)
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
			next = newDocumentMap()
			cur[p] = next
		}
		cur = next
	}
	return cur
}

func ensureTableKey(root documentMap, name string) documentMap {
	next, _ := root[name].(documentMap)
	if next == nil {
		if arr, ok := root[name].([]any); ok && len(arr) > 0 {
			if last, ok := arr[len(arr)-1].(documentMap); ok {
				next = last
			}
		}
	}
	if next == nil {
		next = newDocumentMap()
		root[name] = next
	}
	return next
}

func appendArrayTable(root documentMap, path []string) documentMap {
	if len(path) == 0 {
		return root
	}
	parent := ensureTable(root, path[:len(path)-1])
	name := path[len(path)-1]
	table := newDocumentMap()
	arr, _ := parent[name].([]any)
	arr = append(arr, table)
	parent[name] = arr
	return table
}

func appendArrayTableKey(root documentMap, name string, capacityHint int) documentMap {
	table := newDocumentMap()
	arr, _ := root[name].([]any)
	if arr == nil && capacityHint > 0 {
		arr = make([]any, 0, capacityHint)
	}
	arr = append(arr, table)
	root[name] = arr
	return table
}

func assign(root documentMap, path []string, value any) {
	cur := root
	for _, p := range path[:len(path)-1] {
		next, _ := cur[p].(documentMap)
		if next == nil {
			next = newDocumentMap()
			cur[p] = next
		}
		cur = next
	}
	cur[path[len(path)-1]] = value
}

func newDocumentMap() documentMap {
	m := documentMapPool.Get().(documentMap)
	clear(m)
	return m
}

func recycleDocument(v any) {
	switch x := v.(type) {
	case documentMap:
		for _, child := range x {
			recycleDocument(child)
		}
		clear(x)
		documentMapPool.Put(x)
	case []any:
		for _, child := range x {
			recycleDocument(child)
		}
	}
}

func bytesContains(raw []byte, needle byte) bool {
	for _, c := range raw {
		if c == needle {
			return true
		}
	}
	return false
}

func bytesTrimSpace(raw []byte) []byte {
	for len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t' || raw[0] == '\n' || raw[0] == '\r') {
		raw = raw[1:]
	}
	for len(raw) > 0 {
		c := raw[len(raw)-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		raw = raw[:len(raw)-1]
	}
	return raw
}
