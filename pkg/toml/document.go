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
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Document is a format-preserving TOML document.
//
// It stores the original raw bytes and records byte spans for parsed key/value
// entries. Untouched documents serialize byte-for-byte identically to the input;
// mutations rewrite only the affected value, line, or insertion point.
type Document struct {
	raw []byte

	entries []*documentEntry
	byPath  map[string]*documentEntry

	replacements map[string][]byte
	deletions    map[string]struct{}
	insertions   []documentInsertion
	pending      map[string]any
}

type documentEntry struct {
	path      []string
	pathKey   string
	keyRaw    []byte
	keySpan   [2]int
	value     any
	valueSet  bool
	valueRaw  []byte
	valueSpan [2]int
	lineSpan  [2]int
	kind      documentValueKind
}

type documentValueKind uint8

const (
	documentKindInvalid documentValueKind = iota
	documentKindString
	documentKindInteger
	documentKindFloat
	documentKindBool
	documentKindDatetime
	documentKindArray
	documentKindInlineTable
)

type documentInsertion struct {
	afterPath string
	path      []string
	value     any
}

type documentOp struct {
	start int
	end   int
	text  []byte
	order int
}

type spannedToken struct {
	Token
	span [2]int
}

type documentParser struct {
	data   []byte
	dec    *Decoder
	cursor int
}

// ParseDocument parses data into a format-preserving Document.
func ParseDocument(data []byte) (*Document, error) {
	raw := append([]byte(nil), data...)
	p := &documentParser{data: raw, dec: NewDecoderBytes(raw)}
	doc := &Document{
		raw:          raw,
		byPath:       make(map[string]*documentEntry),
		replacements: make(map[string][]byte),
		deletions:    make(map[string]struct{}),
		pending:      make(map[string]any),
	}
	currentPath := []string(nil)
	for {
		tok, err := p.readToken()
		if errors.Is(err, io.EOF) {
			return doc, nil
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
			currentPath = append([]string(nil), path...)
		case TokenKindArrayTableHeader:
			path, err := parseHeaderKey(tok.Bytes, true)
			if err != nil {
				return nil, err
			}
			currentPath = append([]string(nil), path...)
		case TokenKindKey:
			keyPath, err := parseDottedKey(tok.Bytes)
			if err != nil {
				return nil, err
			}
			valueRaw, valueSpan, kind, err := p.readValue()
			if err != nil {
				return nil, err
			}
			fullPath := make([]string, 0, len(currentPath)+len(keyPath))
			fullPath = append(fullPath, currentPath...)
			fullPath = append(fullPath, keyPath...)
			entry := &documentEntry{
				path:      fullPath,
				pathKey:   joinDocumentPath(fullPath),
				keyRaw:    append([]byte(nil), tok.Bytes...),
				keySpan:   tok.span,
				valueRaw:  append([]byte(nil), valueRaw...),
				valueSpan: valueSpan,
				lineSpan:  lineSpan(data, tok.span[0], valueSpan[1]),
				kind:      kind,
			}
			doc.entries = append(doc.entries, entry)
			doc.byPath[entry.pathKey] = entry
		default:
			if len(currentPath) == 0 {
				return nil, &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "unexpected token", Span: tok.span}
			}
		}
	}
}

// Raw returns the original document bytes.
func (d *Document) Raw() []byte {
	if d == nil {
		return nil
	}
	return d.raw
}

// Get returns the parsed value at path. Path accepts TOML dotted-key syntax,
// including quoted segments.
func (d *Document) Get(path string) (any, bool) {
	if d == nil {
		return nil, false
	}
	parts, err := parseDocumentPath(path)
	if err != nil {
		return nil, false
	}
	entry, ok := d.byPath[joinDocumentPath(parts)]
	if !ok {
		value, ok := d.pending[joinDocumentPath(parts)]
		if ok {
			return value, true
		}
		return nil, false
	}
	if _, deleted := d.deletions[entry.pathKey]; deleted {
		return nil, false
	}
	if !entry.valueSet {
		value, err := parseRawDocumentValue(entry.valueRaw)
		if err != nil {
			return nil, false
		}
		entry.value = value
		entry.valueSet = true
	}
	return entry.value, true
}

// Set replaces path with value. Existing values rewrite only their value span;
// new values are appended as canonical TOML key/value lines.
func (d *Document) Set(path string, value any) error {
	if d == nil {
		return errors.New("toml: nil Document")
	}
	parts, err := parseDocumentPath(path)
	if err != nil {
		return err
	}
	pathKey := joinDocumentPath(parts)
	repl, err := d.formatValueForPath(pathKey, value)
	if err != nil {
		return err
	}
	if entry, ok := d.byPath[pathKey]; ok {
		entry.value = value
		entry.valueSet = true
		d.replacements[pathKey] = repl
		delete(d.deletions, pathKey)
		delete(d.pending, pathKey)
		return nil
	}
	d.pending[pathKey] = value
	d.insertions = append(d.insertions, documentInsertion{path: parts, value: value})
	return nil
}

// InsertAfter inserts path after afterPath using canonical TOML key/value
// syntax. afterPath must identify an existing, non-deleted entry.
func (d *Document) InsertAfter(afterPath, path string, value any) error {
	if d == nil {
		return errors.New("toml: nil Document")
	}
	afterParts, err := parseDocumentPath(afterPath)
	if err != nil {
		return err
	}
	afterKey := joinDocumentPath(afterParts)
	if _, ok := d.byPath[afterKey]; !ok {
		return fmt.Errorf("toml: insert anchor %q not found", afterPath)
	}
	if _, deleted := d.deletions[afterKey]; deleted {
		return fmt.Errorf("toml: insert anchor %q deleted", afterPath)
	}
	parts, err := parseDocumentPath(path)
	if err != nil {
		return err
	}
	d.pending[joinDocumentPath(parts)] = value
	d.insertions = append(d.insertions, documentInsertion{afterPath: afterKey, path: parts, value: value})
	return nil
}

// Delete removes the key/value line at path.
func (d *Document) Delete(path string) error {
	if d == nil {
		return errors.New("toml: nil Document")
	}
	parts, err := parseDocumentPath(path)
	if err != nil {
		return err
	}
	pathKey := joinDocumentPath(parts)
	if _, ok := d.byPath[pathKey]; !ok {
		if _, ok := d.pending[pathKey]; ok {
			delete(d.pending, pathKey)
			return nil
		}
		return fmt.Errorf("toml: path %q not found", path)
	}
	d.deletions[pathKey] = struct{}{}
	delete(d.replacements, pathKey)
	return nil
}

// Bytes serializes the document, preserving untouched bytes exactly.
func (d *Document) Bytes() []byte {
	if d == nil {
		return nil
	}
	if len(d.replacements) == 0 && len(d.deletions) == 0 && len(d.insertions) == 0 {
		return d.raw
	}
	ops := make([]documentOp, 0, len(d.replacements)+len(d.deletions)+len(d.insertions))
	order := 0
	for _, entry := range d.entries {
		if _, ok := d.deletions[entry.pathKey]; ok {
			ops = append(ops, documentOp{start: entry.lineSpan[0], end: entry.lineSpan[1], order: order})
			order++
			continue
		}
		if repl, ok := d.replacements[entry.pathKey]; ok {
			ops = append(ops, documentOp{start: entry.valueSpan[0], end: entry.valueSpan[1], text: repl, order: order})
			order++
		}
	}
	for _, ins := range d.insertions {
		value, ok := d.pending[joinDocumentPath(ins.path)]
		if !ok {
			continue
		}
		text, err := formatDocumentKeyValue(ins.path, value)
		if err != nil {
			continue
		}
		if ins.afterPath == "" {
			prefix := []byte(nil)
			if len(d.raw) > 0 && !bytes.HasSuffix(d.raw, []byte("\n")) {
				prefix = []byte("\n")
			}
			ops = append(ops, documentOp{start: len(d.raw), end: len(d.raw), text: append(prefix, text...), order: order})
			order++
			continue
		}
		entry := d.byPath[ins.afterPath]
		if entry == nil {
			continue
		}
		pos := entry.lineSpan[1]
		ops = append(ops, documentOp{start: pos, end: pos, text: text, order: order})
		order++
	}
	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].start == ops[j].start {
			if ops[i].end == ops[j].end {
				return ops[i].order < ops[j].order
			}
			return ops[i].end < ops[j].end
		}
		return ops[i].start < ops[j].start
	})
	out := make([]byte, 0, len(d.raw)+documentOpsDelta(ops))
	cursor := 0
	for _, op := range ops {
		if op.start < cursor || op.start < 0 || op.end < op.start || op.end > len(d.raw) {
			continue
		}
		out = append(out, d.raw[cursor:op.start]...)
		out = append(out, op.text...)
		cursor = op.end
	}
	out = append(out, d.raw[cursor:]...)
	return out
}

func (p *documentParser) readToken() (spannedToken, error) {
	tok, err := p.dec.ReadToken()
	if err != nil {
		return spannedToken{}, err
	}
	start := p.findTokenStart(tok.Bytes)
	end := start + len(tok.Bytes)
	p.cursor = end
	return spannedToken{Token: tok, span: [2]int{start, end}}, nil
}

func (p *documentParser) readValue() ([]byte, [2]int, documentValueKind, error) {
	for {
		tok, err := p.readToken()
		if err != nil {
			return nil, [2]int{}, documentKindInvalid, err
		}
		if tok.Kind == TokenKindComment {
			continue
		}
		start := tok.span[0]
		end := tok.span[1]
		switch tok.Kind {
		case TokenKindArrayStart, TokenKindInlineTableStart:
			wantArray := tok.Kind == TokenKindArrayStart
			arrayDepth, inlineDepth := 0, 0
			if wantArray {
				arrayDepth = 1
			} else {
				inlineDepth = 1
			}
			for arrayDepth > 0 || inlineDepth > 0 {
				next, err := p.readToken()
				if err != nil {
					return nil, [2]int{}, documentKindInvalid, err
				}
				end = next.span[1]
				switch next.Kind {
				case TokenKindArrayStart:
					arrayDepth++
				case TokenKindArrayEnd:
					arrayDepth--
				case TokenKindInlineTableStart:
					inlineDepth++
				case TokenKindInlineTableEnd:
					inlineDepth--
				}
			}
			raw := p.data[start:end]
			kind := documentKindArray
			if !wantArray {
				kind = documentKindInlineTable
			}
			return raw, [2]int{start, end}, kind, nil
		default:
			raw := p.data[start:end]
			return raw, [2]int{start, end}, documentKindFromToken(tok.Kind), nil
		}
	}
}

func (p *documentParser) findTokenStart(token []byte) int {
	if len(token) == 0 {
		return p.cursor
	}
	if p.cursor > len(p.data) {
		return len(p.data)
	}
	if idx := bytes.Index(p.data[p.cursor:], token); idx >= 0 {
		return p.cursor + idx
	}
	return len(p.data)
}

func parseRawDocumentValue(raw []byte) (any, error) {
	buf := make([]byte, 0, len(raw)+4)
	buf = append(buf, 'x', ' ', '=', ' ')
	buf = append(buf, raw...)
	dec := NewDecoderBytes(buf)
	if tok, err := dec.ReadToken(); err != nil || tok.Kind != TokenKindKey {
		if err != nil {
			return nil, err
		}
		return nil, &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "expected key", Span: [2]int{0, len(buf)}}
	}
	return parseNextValue(dec)
}

func documentKindFromToken(kind TokenKind) documentValueKind {
	switch kind {
	case TokenKindValueString:
		return documentKindString
	case TokenKindValueInteger:
		return documentKindInteger
	case TokenKindValueFloat:
		return documentKindFloat
	case TokenKindValueBool:
		return documentKindBool
	case TokenKindValueDatetime:
		return documentKindDatetime
	case TokenKindArrayStart:
		return documentKindArray
	case TokenKindInlineTableStart:
		return documentKindInlineTable
	default:
		return documentKindInvalid
	}
}

func (d *Document) formatValueForPath(pathKey string, value any) ([]byte, error) {
	entry := d.byPath[pathKey]
	if entry == nil {
		return formatDocumentValue(value)
	}
	if entry.kind == documentKindForGoValue(value) {
		switch entry.kind {
		case documentKindString:
			if s, ok := value.(string); ok {
				return formatStringLike(entry.valueRaw, s), nil
			}
		case documentKindBool, documentKindInteger, documentKindFloat, documentKindDatetime:
			return formatDocumentValue(value)
		}
	}
	return formatDocumentValue(value)
}

func documentKindForGoValue(value any) documentValueKind {
	switch value.(type) {
	case string:
		return documentKindString
	case bool:
		return documentKindBool
	case float32, float64:
		return documentKindFloat
	case time.Time, LocalDateTime, LocalDate, LocalTime:
		return documentKindDatetime
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return documentKindInvalid
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return documentKindInteger
	case reflect.Slice, reflect.Array:
		return documentKindArray
	case reflect.Map, reflect.Struct:
		return documentKindInlineTable
	default:
		return documentKindInvalid
	}
}

func formatDocumentKeyValue(path []string, value any) ([]byte, error) {
	valueBytes, err := formatDocumentValue(value)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	for i, part := range path {
		if i > 0 {
			buf.WriteByte('.')
		}
		buf.WriteString(formatKey(part))
	}
	buf.WriteString(" = ")
	buf.Write(valueBytes)
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func formatDocumentValue(value any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeValue(&buf, reflect.ValueOf(value)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatStringLike(existing []byte, value string) []byte {
	trimmed := bytes.TrimSpace(existing)
	if len(trimmed) >= 2 && trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'' &&
		!strings.ContainsAny(value, "'\n\r") {
		return []byte("'" + value + "'")
	}
	return []byte(strconv.Quote(value))
}

func parseDocumentPath(path string) ([]string, error) {
	return parseDottedKey([]byte(path))
}

func joinDocumentPath(parts []string) string {
	return strings.Join(parts, "\x00")
}

func lineSpan(data []byte, start, end int) [2]int {
	lineStart := start
	for lineStart > 0 && data[lineStart-1] != '\n' && data[lineStart-1] != '\r' {
		lineStart--
	}
	lineEnd := end
	for lineEnd < len(data) {
		b := data[lineEnd]
		lineEnd++
		if b == '\n' {
			break
		}
		if b == '\r' {
			if lineEnd < len(data) && data[lineEnd] == '\n' {
				lineEnd++
			}
			break
		}
	}
	return [2]int{lineStart, lineEnd}
}

func documentOpsDelta(ops []documentOp) int {
	delta := 0
	for _, op := range ops {
		delta += len(op.text) - (op.end - op.start)
	}
	if delta < 0 {
		return 0
	}
	return delta
}
