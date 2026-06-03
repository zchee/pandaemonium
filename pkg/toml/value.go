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
	inTable := false
	declaredTables := make(map[string]string)
	dottedKeyTables := make(map[string]string)
	arrayTables := make(map[string]struct{})
	arrayTableEpochs := make(map[string]int)
	closedInlineTables := make(map[string]string)
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
			path, err := keyPath(key)
			if err != nil {
				return nil, err
			}
			pathKey := strings.Join(path, ".")
			encodedPathKey := encodedKeyPath(path)
			declContext := declarationContext(path, arrayTableEpochs)
			if _, ok := arrayTables[encodedPathKey]; ok {
				return nil, semanticSyntaxError(tok, "cannot redefine array table as table")
			}
			if existingContext, ok := dottedKeyTables[encodedPathKey]; ok && existingContext == declContext {
				return nil, semanticSyntaxError(tok, "cannot redefine dotted key table")
			}
			if closedInlinePrefix(path, closedInlineTables, arrayTableEpochs) {
				return nil, semanticSyntaxError(tok, "cannot extend inline table")
			}
			if existingContext, ok := declaredTables[encodedPathKey]; ok && existingContext == declContext {
				return nil, semanticSyntaxError(tok, "duplicate table")
			}
			if nextFilter, ok := filter.lookupPath(path); ok {
				next, err := ensureTable(root, path, tok)
				if err != nil {
					return nil, bindErrorPath(err, pathKey)
				}
				current = next
				currentPath = append(currentPath[:0], path...)
				currentFilter = nextFilter
				declaredTables[encodedPathKey] = declContext
			} else {
				current = nil
				currentPath = nil
				currentFilter = nil
			}
			inTable = true
		case TokenKindArrayTableHeader:
			key := trimHeaderKey(tok.Bytes, true)
			path, err := keyPath(key)
			if err != nil {
				return nil, err
			}
			pathKey := strings.Join(path, ".")
			encodedPathKey := encodedKeyPath(path)
			if _, ok := declaredTables[encodedPathKey]; ok {
				return nil, semanticSyntaxError(tok, "cannot redefine table as array table")
			}
			declContext := declarationContext(path, arrayTableEpochs)
			if existingContext, ok := dottedKeyTables[encodedPathKey]; ok && existingContext == declContext {
				return nil, semanticSyntaxError(tok, "cannot redefine dotted key table")
			}
			if closedInlinePrefix(path, closedInlineTables, arrayTableEpochs) {
				return nil, semanticSyntaxError(tok, "cannot extend inline table")
			}
			arrayTables[encodedPathKey] = struct{}{}
			arrayTableEpochs[encodedPathKey]++
			if nextFilter, ok := filter.lookupPath(path); ok {
				var next documentMap
				if len(path) == 1 {
					capacityHint := 0
					if _, exists := root[path[0]]; !exists {
						capacityHint = bytes.Count(data, tok.Bytes)
					}
					next, err = appendArrayTableKey(root, path[0], capacityHint, tok)
				} else {
					next, err = appendArrayTable(root, path, tok)
				}
				if err != nil {
					return nil, bindErrorPath(err, pathKey)
				}
				current = next
				currentPath = append(currentPath[:0], path...)
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
			key, err := keyPath(tok.Bytes)
			if err != nil {
				return nil, err
			}
			fullPath := append(append([]string(nil), currentPath...), key...)
			if len(key) > 1 {
				for i := len(currentPath) + 1; i < len(fullPath); i++ {
					prefix := encodedKeyPath(fullPath[:i])
					if _, ok := declaredTables[prefix]; ok {
						return nil, semanticSyntaxError(tok, "cannot extend explicit table with dotted key")
					}
					if existingContext, ok := closedInlineTables[prefix]; ok && existingContext == declarationContext(fullPath[:i], arrayTableEpochs) {
						return nil, semanticSyntaxError(tok, "cannot extend inline table")
					}
				}
			}
			value, err := parseNextValue(dec)
			if err != nil {
				return nil, err
			}
			if err := assignUnique(current, key, value, tok); err != nil {
				return nil, bindErrorPath(err, strings.Join(fullPath, "."))
			}
			markDottedKeyTables(dottedKeyTables, fullPath, len(currentPath), arrayTableEpochs)
			markClosedValuePaths(closedInlineTables, fullPath, value, arrayTableEpochs)
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
	depth := 1
	switch tok.Kind {
	case TokenKindArrayStart, TokenKindInlineTableStart:
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
		return parseIntegerLiteral(tok.Bytes)
	case TokenKindValueFloat:
		if f, ok := parseSpecialFloatLiteral(tok.Bytes); ok {
			return f, nil
		}
		text := normalizeNumericText(tok.Bytes, true)
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
	closedInlineTables := make(map[string]string)
	arrayTableEpochs := map[string]int(nil)
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
			if closedInlinePrefix(key, closedInlineTables, arrayTableEpochs) {
				return nil, semanticSyntaxError(tok, "cannot extend inline table")
			}
			v, err := parseNextValue(dec)
			if err != nil {
				return nil, err
			}
			if err := assignUnique(m, key, v, tok); err != nil {
				return nil, err
			}
			markClosedValuePaths(closedInlineTables, key, v, arrayTableEpochs)
		default:
			return nil, &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "expected inline table key", Span: [2]int{0, 1}}
		}
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
			if multiline && c == '\r' && i+1 < len(raw) && raw[i+1] == '\n' {
				b.WriteByte('\r')
				b.WriteByte('\n')
				i += 2
				continue
			}
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
		case 'e':
			b.WriteByte(0x1b)
			i += 2
		case '"':
			b.WriteByte('"')
			i += 2
		case '\\':
			b.WriteByte('\\')
			i += 2
		case 'x':
			r, err := parseUnicodeEscape(raw, i+2, 2)
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
			i += 4
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
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if multiline && c == '\r' && i+1 < len(raw) && raw[i+1] == '\n' {
			i++
			continue
		}
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
	if multiline && c == '\n' {
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
	raw = bytesTrimSpace(raw)
	if array {
		if len(raw) >= 4 {
			return bytesTrimSpace(raw[2 : len(raw)-2])
		}
		return nil
	}
	if len(raw) >= 2 {
		return bytesTrimSpace(raw[1 : len(raw)-1])
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
	raw = bytesTrimSpace(raw)
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
			if !isSimpleBareKey([]byte(part)) {
				return nil, &SyntaxError{Line: 1, Col: 1, Msg: "unexpected token in dotted key", Span: [2]int{0, origLen}}
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

func parseSpecialFloatLiteral(raw []byte) (float64, bool) {
	switch {
	case bytes.Equal(raw, nanLiteral), bytes.Equal(raw, posNanLiteral):
		return math.NaN(), true
	case bytes.Equal(raw, negNanLiteral):
		return math.Copysign(math.NaN(), -1), true
	case bytes.Equal(raw, infLiteral), bytes.Equal(raw, posInfLiteral):
		return math.Inf(1), true
	case bytes.Equal(raw, negInfLiteral):
		return math.Inf(-1), true
	default:
		return 0, false
	}
}

func parseIntegerLiteral(raw []byte) (int64, error) {
	text := normalizeNumericText(raw, false)
	sign := int64(1)
	if strings.HasPrefix(text, "+") {
		text = text[1:]
	} else if strings.HasPrefix(text, "-") {
		sign = -1
		text = text[1:]
	}
	base := 10
	if len(text) > 2 && text[0] == '0' {
		switch text[1] {
		case 'b', 'B':
			base = 2
			text = text[2:]
		case 'o', 'O':
			base = 8
			text = text[2:]
		case 'x', 'X':
			base = 16
			text = text[2:]
		}
	}
	u, err := strconv.ParseUint(text, base, 64)
	if err != nil {
		return 0, err
	}
	if sign < 0 {
		const minMagnitude = uint64(1) << 63
		if u > minMagnitude {
			return 0, strconv.ErrRange
		}
		if u == minMagnitude {
			return math.MinInt64, nil
		}
		return -int64(u), nil
	}
	if u > math.MaxInt64 {
		return 0, strconv.ErrRange
	}
	return int64(u), nil
}

func keyPath(raw []byte) ([]string, error) {
	if isSimpleBareKey(raw) {
		return []string{string(raw)}, nil
	}
	return parseDottedKey(raw)
}

func declarationContext(path []string, arrayTableEpochs map[string]int) string {
	for i := len(path); i > 0; i-- {
		prefix := encodedKeyPath(path[:i])
		if epoch := arrayTableEpochs[prefix]; epoch > 0 {
			return prefix + "#" + strconv.Itoa(epoch)
		}
	}
	return ""
}

func closedInlinePrefix(path []string, closed map[string]string, arrayTableEpochs map[string]int) bool {
	for i := 1; i <= len(path); i++ {
		prefix := path[:i]
		if existingContext, ok := closed[encodedKeyPath(prefix)]; ok && existingContext == declarationContext(prefix, arrayTableEpochs) {
			return true
		}
	}
	return false
}

func markClosedValuePaths(closed map[string]string, path []string, value any, arrayTableEpochs map[string]int) {
	key := encodedKeyPath(path)
	switch v := value.(type) {
	case documentMap:
		closed[key] = declarationContext(path, arrayTableEpochs)
		for childKey, child := range v {
			childPath := append(append([]string(nil), path...), childKey)
			markClosedValuePaths(closed, childPath, child, arrayTableEpochs)
		}
	case []any:
		closed[key] = declarationContext(path, arrayTableEpochs)
		for _, child := range v {
			if childMap, ok := child.(documentMap); ok {
				for childKey, grandchild := range childMap {
					childPath := append(append([]string(nil), path...), childKey)
					markClosedValuePaths(closed, childPath, grandchild, arrayTableEpochs)
				}
			}
		}
	}
}

func markDottedKeyTables(dotted map[string]string, fullPath []string, currentPathLen int, arrayTableEpochs map[string]int) {
	for i := currentPathLen + 1; i < len(fullPath); i++ {
		prefix := fullPath[:i]
		dotted[encodedKeyPath(prefix)] = declarationContext(prefix, arrayTableEpochs)
	}
}

func encodedKeyPath(path []string) string {
	var b strings.Builder
	for i, part := range path {
		if i > 0 {
			b.WriteByte(0)
		}
		b.WriteString(part)
	}
	return b.String()
}

func semanticSyntaxError(tok Token, msg string) *SyntaxError {
	return &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: msg, Span: [2]int{0, 1}}
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

func appendArrayTable(root documentMap, path []string, tok Token) (documentMap, error) {
	if len(path) == 0 {
		return root, nil
	}
	parent, err := ensureTable(root, path[:len(path)-1], tok)
	if err != nil {
		return nil, err
	}
	name := path[len(path)-1]
	table := newDocumentMap()
	existing, exists := parent[name]
	arr, _ := existing.([]any)
	if exists && arr == nil {
		return nil, semanticSyntaxError(tok, "cannot redefine value as array table")
	}
	arr = append(arr, table)
	parent[name] = arr
	return table, nil
}

func appendArrayTableKey(root documentMap, name string, capacityHint int, tok Token) (documentMap, error) {
	table := newDocumentMap()
	arr, _ := root[name].([]any)
	if arr == nil {
		if _, exists := root[name]; exists {
			return nil, semanticSyntaxError(tok, "cannot redefine value as array table")
		}
	}
	if arr == nil && capacityHint > 0 {
		arr = make([]any, 0, capacityHint)
	}
	arr = append(arr, table)
	root[name] = arr
	return table, nil
}

func assignUnique(root documentMap, path []string, value any, tok Token) error {
	cur := root
	for _, p := range path[:len(path)-1] {
		next, _ := cur[p].(documentMap)
		if next == nil {
			if _, exists := cur[p]; exists {
				return semanticSyntaxError(tok, "cannot redefine value as table")
			}
			next = newDocumentMap()
			cur[p] = next
		}
		cur = next
	}
	key := path[len(path)-1]
	if _, exists := cur[key]; exists {
		return semanticSyntaxError(tok, "duplicate key")
	}
	cur[key] = value
	return nil
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
