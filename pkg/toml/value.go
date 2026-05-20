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
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type documentMap map[string]any

const documentMapHint = 4

var (
	trueLiteral   = []byte("true")
	falseLiteral  = []byte("false")
	infLiteral    = []byte("inf")
	posInfLiteral = []byte("+inf")
	negInfLiteral = []byte("-inf")
	nanLiteral    = []byte("nan")
	posNanLiteral = []byte("+nan")
	negNanLiteral = []byte("-nan")
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
	currentPath := []string(nil)
	currentFilter := filter
	meta := newDocumentMeta()
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
					path := []string{string(key)}
					current, err = declareTable(root, path, meta, tok)
					if err != nil {
						return nil, err
					}
					currentPath = path
					currentFilter = nextFilter
				} else {
					current = nil
					currentPath = nil
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
				current, err = declareTable(root, path, meta, tok)
				if err != nil {
					return nil, err
				}
				currentPath = path
				currentFilter = nextFilter
			} else {
				current = nil
				currentPath = nil
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
					path := []string{name}
					current, err = appendArrayTablePath(root, path, capacityHint, meta, tok)
					if err != nil {
						return nil, err
					}
					currentPath = path
					currentFilter = nextFilter
				} else {
					current = nil
					currentPath = nil
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
				current, err = appendArrayTablePath(root, path, 0, meta, tok)
				if err != nil {
					return nil, err
				}
				currentPath = path
				currentFilter = nextFilter
			} else {
				current = nil
				currentPath = nil
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
				if err := assignUnique(current, currentPath, key, value, meta, tok); err != nil {
					return nil, err
				}
				continue
			}
			value, err := parseNextValue(dec)
			if err != nil {
				return nil, err
			}
			if _, exists := current[string(tok.Bytes)]; exists {
				return nil, semanticSyntaxError(tok, "duplicate key")
			}
			name := string(tok.Bytes)
			current[name] = value
			if isInlineTableValue(value) {
				meta.inlineTables[pathKey(appendPath(currentPath, name))] = struct{}{}
			}
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
		text := normalizeNumericText(tok.Bytes, true)
		switch text {
		case "nan", "+nan", "-nan":
			return math.NaN(), nil
		case "inf", "+inf":
			return math.Inf(1), nil
		case "-inf":
			return math.Inf(-1), nil
		}
		return strconv.ParseFloat(text, 64)
	case TokenKindValueBool:
		switch {
		case bytes.Equal(tok.Bytes, trueLiteral):
			return true, nil
		case bytes.Equal(tok.Bytes, falseLiteral):
			return false, nil
		default:
			return nil, &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "malformed boolean", Span: [2]int{0, len(tok.Bytes)}}
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
		if tok.Kind == TokenKindComment {
			continue
		}
		switch tok.Kind {
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
	meta := newDocumentMeta()
	for {
		tok, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}
		if tok.Kind == TokenKindComment {
			continue
		}
		switch tok.Kind {
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
			if err := assignUnique(m, nil, key, v, meta, tok); err != nil {
				return nil, err
			}
		default:
			return nil, &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "expected inline table key", Span: [2]int{0, 1}}
		}
	}
}

func skipLayoutAndComments(dec *Decoder) error {
	for {
		start := dec.off
		dec.skipSpaces()
		if dec.off != start {
			continue
		}
		if dec.off < len(dec.buf) && dec.buf[dec.off] == '#' {
			if _, err := dec.scanComment(); err != nil {
				return err
			}
			continue
		}
		return nil
	}
}

func parseStringValue(raw []byte) (string, error) {
	if len(raw) >= 6 && raw[0] == '\'' && raw[1] == '\'' && raw[2] == '\'' &&
		raw[len(raw)-1] == '\'' && raw[len(raw)-2] == '\'' && raw[len(raw)-3] == '\'' {
		body := trimInitialMultilineStringNewline(raw[3 : len(raw)-3])
		if idx := prohibitedStringControlIndex(body, true); idx >= 0 {
			return "", stringControlError(body, idx)
		}
		return string(body), nil
	}
	if len(raw) >= 6 && raw[0] == '"' && raw[1] == '"' && raw[2] == '"' &&
		raw[len(raw)-1] == '"' && raw[len(raw)-2] == '"' && raw[len(raw)-3] == '"' {
		return parseBasicStringBody(trimInitialMultilineStringNewline(raw[3:len(raw)-3]), true)
	}
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		body := raw[1 : len(raw)-1]
		if idx := prohibitedStringControlIndex(body, false); idx >= 0 {
			return "", stringControlError(body, idx)
		}
		return string(body), nil
	}
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return parseBasicStringBody(raw[1:len(raw)-1], false)
	}
	return "", &SyntaxError{Line: 1, Col: 1, Msg: "malformed string", Span: [2]int{0, len(raw)}}
}

func trimInitialMultilineStringNewline(raw []byte) []byte {
	if len(raw) >= 2 && raw[0] == '\r' && raw[1] == '\n' {
		return raw[2:]
	}
	if len(raw) >= 1 && raw[0] == '\n' {
		return raw[1:]
	}
	return raw
}

func parseBasicStringBody(raw []byte, multiline bool) (string, error) {
	if bytes.IndexByte(raw, '\\') < 0 {
		if idx := prohibitedStringControlIndex(raw, multiline); idx >= 0 {
			return "", stringControlError(raw, idx)
		}
		return string(raw), nil
	}
	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); {
		c := raw[i]
		if c != '\\' {
			if prohibitedStringControl(c, multiline) {
				return "", stringControlError(raw, i)
			}
			b.WriteByte(c)
			i++
			continue
		}
		if multiline {
			if next, ok := skipEscapedMultilineStringWhitespace(raw, i+1); ok {
				i = next
				continue
			}
		}
		if i+1 >= len(raw) {
			return "", &SyntaxError{Line: 1, Col: i + 1, Msg: "unterminated string escape", Span: [2]int{i, len(raw)}}
		}
		next := raw[i+1]
		switch next {
		case 'b':
			b.WriteByte('\b')
			i += 2
		case 't':
			b.WriteByte('\t')
			i += 2
		case 'n':
			b.WriteByte('\n')
			i += 2
		case 'f':
			b.WriteByte('\f')
			i += 2
		case 'r':
			b.WriteByte('\r')
			i += 2
		case '"':
			b.WriteByte('"')
			i += 2
		case '\\':
			b.WriteByte('\\')
			i += 2
		case 'u', 'U':
			digits := 4
			if next == 'U' {
				digits = 8
			}
			r, err := parseUnicodeEscape(raw, i+2, digits)
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
			i += 2 + digits
		default:
			return "", &SyntaxError{Line: 1, Col: i + 1, Msg: "invalid string escape", Span: [2]int{i, min(i+2, len(raw))}}
		}
	}
	return b.String(), nil
}

func skipEscapedMultilineStringWhitespace(raw []byte, i int) (int, bool) {
	j := i
	for j < len(raw) && (raw[j] == ' ' || raw[j] == '\t') {
		j++
	}
	switch {
	case j < len(raw) && raw[j] == '\n':
		j++
	case j+1 < len(raw) && raw[j] == '\r' && raw[j+1] == '\n':
		j += 2
	default:
		return i, false
	}
	for j < len(raw) {
		switch raw[j] {
		case ' ', '\t', '\n':
			j++
		case '\r':
			if j+1 < len(raw) && raw[j+1] == '\n' {
				j += 2
				continue
			}
			return j, true
		default:
			return j, true
		}
	}
	return j, true
}

func parseUnicodeEscape(raw []byte, start, digits int) (rune, error) {
	end := start + digits
	if end > len(raw) {
		return 0, &SyntaxError{Line: 1, Col: start + 1, Msg: "short unicode escape", Span: [2]int{start - 2, len(raw)}}
	}
	for _, c := range raw[start:end] {
		if !isHexDigit(c) {
			return 0, &SyntaxError{Line: 1, Col: start + 1, Msg: "invalid unicode escape", Span: [2]int{start - 2, end}}
		}
	}
	v, err := strconv.ParseInt(string(raw[start:end]), 16, 32)
	if err != nil {
		return 0, err
	}
	r := rune(v)
	if !utf8.ValidRune(r) {
		return 0, &SyntaxError{Line: 1, Col: start + 1, Msg: "invalid unicode scalar", Span: [2]int{start - 2, end}}
	}
	return r, nil
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f'
}

func prohibitedStringControlIndex(raw []byte, multiline bool) int {
	for i, c := range raw {
		if prohibitedStringControl(c, multiline) {
			return i
		}
	}
	return -1
}

func prohibitedStringControl(c byte, multiline bool) bool {
	if c == '\t' {
		return false
	}
	if multiline && (c == '\n' || c == '\r') {
		return false
	}
	return c < 0x20 || c == 0x7f
}

func stringControlError(raw []byte, idx int) error {
	return &SyntaxError{
		Line: 1,
		Col:  idx + 1,
		Msg:  fmt.Sprintf("unescaped control character 0x%02x in string", raw[idx]),
		Span: [2]int{idx, idx + 1},
	}
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
			if part == "" {
				return nil, &SyntaxError{Line: 1, Col: 1, Msg: "empty key segment", Span: [2]int{0, origLen}}
			}
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
		if len(bytes.TrimLeft(raw, " \t")) == 0 {
			return nil, &SyntaxError{Line: 1, Col: 1, Msg: "empty key segment", Span: [2]int{0, origLen}}
		}
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

type documentMeta struct {
	declaredTables map[string]struct{}
	dottedTables   map[string]struct{}
	arrayTables    map[string]struct{}
	inlineTables   map[string]struct{}
}

func newDocumentMeta() *documentMeta {
	return &documentMeta{
		declaredTables: make(map[string]struct{}),
		dottedTables:   make(map[string]struct{}),
		arrayTables:    make(map[string]struct{}),
		inlineTables:   make(map[string]struct{}),
	}
}

func declareTable(root documentMap, path []string, meta *documentMeta, tok Token) (documentMap, error) {
	if meta.hasInlineAncestor(path) {
		return nil, semanticSyntaxError(tok, "cannot extend inline table")
	}
	key := pathKey(path)
	if _, ok := meta.declaredTables[key]; ok && !meta.hasArrayAncestor(path) {
		return nil, semanticSyntaxError(tok, "duplicate table")
	}
	if _, ok := meta.dottedTables[key]; ok {
		return nil, semanticSyntaxError(tok, "cannot redefine dotted-key table")
	}
	if _, ok := meta.arrayTables[key]; ok {
		return nil, semanticSyntaxError(tok, "cannot redefine array table")
	}
	table, err := ensureTable(root, path, tok)
	if err != nil {
		return nil, err
	}
	meta.declaredTables[key] = struct{}{}
	return table, nil
}

func appendArrayTablePath(root documentMap, path []string, capacityHint int, meta *documentMeta, tok Token) (documentMap, error) {
	if len(path) == 0 {
		return root, nil
	}
	if meta.hasInlineAncestor(path) {
		return nil, semanticSyntaxError(tok, "cannot extend inline table")
	}
	key := pathKey(path)
	if _, ok := meta.declaredTables[key]; ok {
		return nil, semanticSyntaxError(tok, "cannot redefine table as array table")
	}
	if _, ok := meta.dottedTables[key]; ok {
		return nil, semanticSyntaxError(tok, "cannot redefine dotted-key table as array table")
	}
	parent, err := ensureTable(root, path[:len(path)-1], tok)
	if err != nil {
		return nil, err
	}
	name := path[len(path)-1]
	if existing, ok := parent[name]; ok {
		arr, ok := existing.([]any)
		if !ok {
			return nil, semanticSyntaxError(tok, "cannot redefine table as array table")
		}
		table := newDocumentMap()
		parent[name] = append(arr, table)
		meta.arrayTables[key] = struct{}{}
		return table, nil
	}
	table := newDocumentMap()
	arr := []any{table}
	if capacityHint > 0 {
		arr = make([]any, 0, capacityHint)
		arr = append(arr, table)
	}
	parent[name] = arr
	meta.arrayTables[key] = struct{}{}
	return table, nil
}

func ensureTable(root documentMap, path []string, tok Token) (documentMap, error) {
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
			if _, exists := cur[p]; exists {
				return nil, semanticSyntaxError(tok, "cannot redefine value as table")
			}
			next = newDocumentMap()
			cur[p] = next
		}
		cur = next
	}
	return cur, nil
}

func assignUnique(root documentMap, basePath, path []string, value any, meta *documentMeta, tok Token) error {
	cur := root
	fullPath := appendDocumentPath(basePath, path)
	for i, p := range path[:len(path)-1] {
		prefix := fullPath[:len(basePath)+i+1]
		if _, ok := meta.inlineTables[pathKey(prefix)]; ok {
			return semanticSyntaxError(tok, "cannot extend inline table")
		}
		if len(prefix) > len(basePath) {
			if _, ok := meta.declaredTables[pathKey(prefix)]; ok {
				return semanticSyntaxError(tok, "cannot extend explicit table with dotted key")
			}
		}
		next, _ := cur[p].(documentMap)
		if next == nil {
			if arr, ok := cur[p].([]any); ok && len(arr) > 0 {
				if last, ok := arr[len(arr)-1].(documentMap); ok {
					next = last
				}
			}
		}
		if next == nil {
			if _, exists := cur[p]; exists {
				return semanticSyntaxError(tok, "cannot redefine value as table")
			}
			next = newDocumentMap()
			cur[p] = next
		}
		meta.dottedTables[pathKey(prefix)] = struct{}{}
		cur = next
	}
	name := path[len(path)-1]
	if _, exists := cur[name]; exists {
		return semanticSyntaxError(tok, "duplicate key")
	}
	cur[name] = value
	if isInlineTableValue(value) {
		meta.inlineTables[pathKey(fullPath)] = struct{}{}
	}
	return nil
}

func (m *documentMeta) hasArrayAncestor(path []string) bool {
	for i := 1; i < len(path); i++ {
		if _, ok := m.arrayTables[pathKey(path[:i])]; ok {
			return true
		}
	}
	return false
}

func (m *documentMeta) hasInlineAncestor(path []string) bool {
	for i := 1; i <= len(path); i++ {
		if _, ok := m.inlineTables[pathKey(path[:i])]; ok {
			return true
		}
	}
	return false
}

func isInlineTableValue(value any) bool {
	switch v := value.(type) {
	case documentMap:
		return true
	case []any:
		for _, item := range v {
			if isInlineTableValue(item) {
				return true
			}
		}
	}
	return false
}

func appendDocumentPath(base, rel []string) []string {
	out := make([]string, 0, len(base)+len(rel))
	out = append(out, base...)
	out = append(out, rel...)
	return out
}

func pathKey(path []string) string {
	return strings.Join(path, "\x00")
}

func semanticSyntaxError(tok Token, msg string) *SyntaxError {
	return &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: msg, Span: [2]int{0, 1}}
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
