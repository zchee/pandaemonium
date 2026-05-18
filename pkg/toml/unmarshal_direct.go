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
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/zchee/pandaemonium/pkg/toml/internal/reflectcache"
)

var errDirectUnknownField = errors.New("toml: direct destination field is unknown")

var directStructEligibilityCache sync.Map // map[reflect.Type]bool

type directAssignment struct {
	name      string
	rawName   []byte
	dst       reflect.Value
	mapTarget bool
}

// directRawPath keeps common dotted bare-key paths on the stack. It is used
// only while binding the current input buffer, so its byte slices must not be
// stored beyond the active Unmarshal call.
type directRawPath struct {
	stack [8][]byte
	extra [][]byte
	n     int
}

type directPathState struct {
	raw        directRawPath
	text       []string
	valid      bool
	arrayIndex int
}

func directStructUnmarshalEligible(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	if t == reflect.TypeFor[time.Time]() ||
		t == reflect.TypeFor[LocalDateTime]() ||
		t == reflect.TypeFor[LocalDate]() ||
		t == reflect.TypeFor[LocalTime]() {
		return false
	}
	if cached, ok := directStructEligibilityCache.Load(t); ok {
		return cached.(bool)
	}
	eligible := !directTypeContainsMap(t, make(map[reflect.Type]bool))
	if cached, loaded := directStructEligibilityCache.LoadOrStore(t, eligible); loaded {
		return cached.(bool)
	}
	return eligible
}

func directTypeContainsMap(t reflect.Type, seen map[reflect.Type]bool) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeFor[time.Time]() ||
		t == reflect.TypeFor[LocalDateTime]() ||
		t == reflect.TypeFor[LocalDate]() ||
		t == reflect.TypeFor[LocalTime]() {
		return false
	}
	switch t.Kind() {
	case reflect.Map:
		return true
	case reflect.Struct:
		if seen[t] {
			return false
		}
		seen[t] = true
		info, err := reflectcache.Lookup(t)
		if err != nil {
			return true
		}
		for _, field := range info.Fields {
			if directTypeContainsMap(field.Type, seen) {
				return true
			}
		}
		return false
	case reflect.Slice, reflect.Array:
		return directTypeContainsMap(t.Elem(), seen)
	default:
		return false
	}
}

// bindDocumentDirect decodes the token stream directly into dst. It avoids the
// intermediate documentMap used by generic callers while preserving the same
// Decoder.ReadToken source of truth as the rest of the facade.
func bindDocumentDirect(data []byte, dst reflect.Value, opts []Option, cfg bindConfig) error {
	dec := NewDecoderBytes(data, opts...)
	current := dst
	currentPath := directPathState{}
	for {
		tok, err := dec.ReadToken()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch tok.Kind {
		case TokenKindComment:
			continue
		case TokenKindTableHeader:
			if path, ok := parseDirectHeaderPath(tok.Bytes, false); ok {
				next, err := directTableRaw(dst, path)
				if err != nil {
					return err
				}
				current = next
				currentPath = directPathState{raw: path, valid: true, arrayIndex: -1}
				continue
			}
			path, err := parseHeaderKey(tok.Bytes, false)
			if err != nil {
				return err
			}
			next, err := directTable(dst, path)
			if err != nil {
				return err
			}
			current = next
			currentPath = directPathState{text: path, valid: true, arrayIndex: -1}
		case TokenKindArrayTableHeader:
			if path, ok := parseDirectHeaderPath(tok.Bytes, true); ok {
				next, index, err := directArrayTableRaw(dst, path)
				if err != nil {
					return err
				}
				current = next
				currentPath = directPathState{raw: path, valid: true, arrayIndex: index}
				continue
			}
			path, err := parseHeaderKey(tok.Bytes, true)
			if err != nil {
				return err
			}
			next, index, err := directArrayTable(dst, path)
			if err != nil {
				return err
			}
			current = next
			currentPath = directPathState{text: path, valid: true, arrayIndex: index}
		case TokenKindKey:
			if isSimpleBareKey(tok.Bytes) {
				target, ok, err := directAssignmentForKey(current, tok.Bytes)
				if err != nil {
					return err
				}
				if !ok {
					if err := skipNextValue(dec); err != nil {
						return err
					}
					continue
				}
				value, err := parseNextValue(dec)
				if err != nil {
					return err
				}
				if err := target.assign(value, cfg); err != nil {
					return bindErrorPath(err, currentPath.string())
				}
				continue
			}
			if path, ok := parseDirectRawPath(tok.Bytes); ok {
				ok, err := directCanAssignRaw(current, path)
				if err != nil {
					return err
				}
				if !ok {
					if err := skipNextValue(dec); err != nil {
						return err
					}
					continue
				}
				value, err := parseNextValue(dec)
				if err != nil {
					return err
				}
				if err := directAssignRaw(current, path, value, cfg); err != nil {
					return bindErrorPath(err, currentPath.string())
				}
				continue
			}
			key, err := parseDottedKey(tok.Bytes)
			if err != nil {
				return err
			}
			ok, err := directCanAssign(current, key)
			if err != nil {
				return err
			}
			if !ok {
				if err := skipNextValue(dec); err != nil {
					return err
				}
				continue
			}
			value, err := parseNextValue(dec)
			if err != nil {
				return err
			}
			if err := directAssign(current, key, value, cfg); err != nil {
				return bindErrorPath(err, currentPath.string())
			}
		default:
			return &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "unexpected token", Span: [2]int{0, 1}}
		}
	}
}

func parseDirectHeaderPath(raw []byte, array bool) (directRawPath, bool) {
	return parseDirectRawPath(trimHeaderKey(raw, array))
}

// parseDirectRawPath accepts the allocation-free subset used by most decoded
// keys and table headers. Quoted or whitespace-heavy paths deliberately return
// ok=false so the full parser remains the compatibility path.
func parseDirectRawPath(raw []byte) (directRawPath, bool) {
	raw = bytes.TrimSpace(raw)
	var path directRawPath
	for len(raw) > 0 {
		end := 0
		for end < len(raw) && raw[end] != '.' {
			if !isDirectBareKeyByte(raw[end]) {
				return directRawPath{}, false
			}
			end++
		}
		if end == 0 {
			return directRawPath{}, false
		}
		path.append(raw[:end])
		if end == len(raw) {
			return path, true
		}
		raw = raw[end+1:]
	}
	return directRawPath{}, false
}

func isDirectBareKeyByte(c byte) bool {
	return c >= 'A' && c <= 'Z' ||
		c >= 'a' && c <= 'z' ||
		c >= '0' && c <= '9' ||
		c == '_' ||
		c == '-'
}

func (p *directRawPath) append(part []byte) {
	if p.n < len(p.stack) {
		p.stack[p.n] = part
	} else {
		p.extra = append(p.extra, part)
	}
	p.n++
}

func (p directRawPath) len() int {
	return p.n
}

func (p directRawPath) part(i int) []byte {
	if i < len(p.stack) {
		return p.stack[i]
	}
	return p.extra[i-len(p.stack)]
}

func (p directRawPath) stringAt(i int) string {
	return string(p.part(i))
}

func (p directRawPath) string() string {
	switch p.n {
	case 0:
		return ""
	case 1:
		return p.stringAt(0)
	}
	var b strings.Builder
	for i := range p.n {
		if i > 0 {
			b.WriteByte('.')
		}
		b.Write(p.part(i))
	}
	return b.String()
}

func (s directPathState) string() string {
	if !s.valid {
		return ""
	}
	base := s.stringNoIndex()
	if s.arrayIndex < 0 {
		return base
	}
	var buf [32]byte
	b := buf[:0]
	b = append(b, base...)
	b = append(b, '[')
	b = strconv.AppendInt(b, int64(s.arrayIndex), 10)
	b = append(b, ']')
	return string(b)
}

func (s directPathState) stringNoIndex() string {
	if len(s.text) > 0 {
		return strings.Join(s.text, ".")
	}
	return s.raw.string()
}

func directAssignmentForKey(root reflect.Value, raw []byte) (directAssignment, bool, error) {
	if !root.IsValid() {
		return directAssignment{}, false, nil
	}
	cur := directWritableValue(root)
	switch cur.Kind() {
	case reflect.Struct:
		info, err := reflectcache.Lookup(cur.Type())
		if err != nil {
			return directAssignment{}, false, normalizeReflectcacheError(err)
		}
		field, ok := lookupStructFieldBytes(info, raw)
		if !ok {
			return directAssignment{}, false, nil
		}
		return directAssignment{rawName: raw, dst: cur.FieldByIndex(field.Index)}, true, nil
	case reflect.Map:
		if cur.Type().Key().Kind() != reflect.String {
			return directAssignment{}, false, &UnsupportedTypeError{Type: cur.Type().String()}
		}
		if cur.IsNil() {
			cur.Set(reflect.MakeMap(cur.Type()))
		}
		return directAssignment{name: string(raw), dst: cur, mapTarget: true}, true, nil
	default:
		return directAssignment{}, false, nil
	}
}

func lookupStructFieldBytes(info *reflectcache.TypeInfo, name []byte) (reflectcache.Field, bool) {
	if field, ok := info.ByName[unsafeString(name)]; ok {
		return field, true
	}
	for _, field := range info.Fields {
		if asciiEqualFoldStringBytes(field.Name, name) {
			return field, true
		}
	}
	return reflectcache.Field{}, false
}

func asciiEqualFoldStringBytes(s string, b []byte) bool {
	if len(s) != len(b) {
		return false
	}
	for i := range s {
		if asciiFoldByte(s[i]) != asciiFoldByte(b[i]) {
			return false
		}
	}
	return true
}

func asciiFoldByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func unsafeString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// The result is used only for immediate map lookups against cached struct
	// metadata. Callers must not store it because it aliases the input buffer.
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func (a directAssignment) assign(value any, cfg bindConfig) error {
	if a.mapTarget {
		elem := reflect.New(a.dst.Type().Elem()).Elem()
		if err := bindValue(elem, value, cfg); err != nil {
			return bindErrorPath(err, a.pathName())
		}
		a.dst.SetMapIndex(reflect.ValueOf(a.name).Convert(a.dst.Type().Key()), elem)
		return nil
	}
	if err := bindValue(a.dst, value, cfg); err != nil {
		return bindErrorPath(err, a.pathName())
	}
	return nil
}

func (a directAssignment) pathName() string {
	if a.name != "" {
		return a.name
	}
	return string(a.rawName)
}

func directTableRaw(root reflect.Value, path directRawPath) (reflect.Value, error) {
	return directTableRawN(root, path, path.len())
}

func directTableRawN(root reflect.Value, path directRawPath, n int) (reflect.Value, error) {
	cur := root
	for i := 0; i < n; i++ {
		cur = directWritableValue(cur)
		if cur.IsValid() && cur.Kind() == reflect.Slice {
			if cur.Len() == 0 {
				return reflect.Value{}, nil
			}
			cur = directWritableValue(cur.Index(cur.Len() - 1))
		}
		next, err := directFieldRaw(cur, path.part(i))
		if errors.Is(err, errDirectUnknownField) {
			return reflect.Value{}, nil
		}
		if err != nil {
			return reflect.Value{}, err
		}
		cur = directWritableValue(next)
	}
	if cur = directWritableValue(cur); cur.IsValid() && cur.Kind() == reflect.Slice {
		if cur.Len() == 0 {
			return reflect.Value{}, nil
		}
		cur = directWritableValue(cur.Index(cur.Len() - 1))
	}
	return cur, nil
}

func directTable(root reflect.Value, path []string) (reflect.Value, error) {
	cur := root
	for _, name := range path {
		cur = directWritableValue(cur)
		if cur.IsValid() && cur.Kind() == reflect.Slice {
			if cur.Len() == 0 {
				return reflect.Value{}, nil
			}
			cur = directWritableValue(cur.Index(cur.Len() - 1))
		}
		next, err := directField(cur, name)
		if errors.Is(err, errDirectUnknownField) {
			return reflect.Value{}, nil
		}
		if err != nil {
			return reflect.Value{}, err
		}
		cur = directWritableValue(next)
	}
	if cur = directWritableValue(cur); cur.IsValid() && cur.Kind() == reflect.Slice {
		if cur.Len() == 0 {
			return reflect.Value{}, nil
		}
		cur = directWritableValue(cur.Index(cur.Len() - 1))
	}
	return cur, nil
}

func directCanAssignRaw(root reflect.Value, path directRawPath) (bool, error) {
	if !root.IsValid() {
		return false, nil
	}
	cur := root
	for i := 0; i < path.len(); i++ {
		cur = directWritableValue(cur)
		switch cur.Kind() {
		case reflect.Struct:
			info, err := reflectcache.Lookup(cur.Type())
			if err != nil {
				return false, normalizeReflectcacheError(err)
			}
			field, ok := lookupStructFieldBytes(info, path.part(i))
			if !ok {
				return false, nil
			}
			cur = cur.FieldByIndex(field.Index)
		case reflect.Map:
			return true, nil
		default:
			return false, nil
		}
	}
	return true, nil
}

func directCanAssign(root reflect.Value, path []string) (bool, error) {
	if !root.IsValid() {
		return false, nil
	}
	cur := root
	for _, name := range path {
		cur = directWritableValue(cur)
		switch cur.Kind() {
		case reflect.Struct:
			info, err := reflectcache.Lookup(cur.Type())
			if err != nil {
				return false, normalizeReflectcacheError(err)
			}
			field, ok := lookupStructField(info, name)
			if !ok {
				return false, nil
			}
			cur = cur.FieldByIndex(field.Index)
		case reflect.Map:
			return true, nil
		default:
			return false, nil
		}
	}
	return true, nil
}

func directArrayTableRaw(root reflect.Value, path directRawPath) (reflect.Value, int, error) {
	if path.len() == 0 {
		return root, -1, nil
	}
	parent, err := directTableRawN(root, path, path.len()-1)
	if err != nil {
		return reflect.Value{}, -1, err
	}
	if !parent.IsValid() {
		return reflect.Value{}, -1, nil
	}
	slot, err := directFieldRaw(parent, path.part(path.len()-1))
	if errors.Is(err, errDirectUnknownField) {
		return reflect.Value{}, -1, nil
	}
	if err != nil {
		return reflect.Value{}, -1, err
	}
	slot = directWritableValue(slot)
	if slot.Kind() != reflect.Slice {
		return reflect.Value{}, -1, bindDirectRawErrorPath(mismatch(slot.Type(), []any{}), path)
	}
	elem := reflect.New(slot.Type().Elem()).Elem()
	index := slot.Len()
	slot.Set(reflect.Append(slot, elem))
	return directWritableValue(slot.Index(index)), index, nil
}

func directArrayTable(root reflect.Value, path []string) (reflect.Value, int, error) {
	if len(path) == 0 {
		return root, -1, nil
	}
	parent, err := directTable(root, path[:len(path)-1])
	if err != nil {
		return reflect.Value{}, -1, err
	}
	if !parent.IsValid() {
		return reflect.Value{}, -1, nil
	}
	slot, err := directField(parent, path[len(path)-1])
	if errors.Is(err, errDirectUnknownField) {
		return reflect.Value{}, -1, nil
	}
	if err != nil {
		return reflect.Value{}, -1, err
	}
	slot = directWritableValue(slot)
	if slot.Kind() != reflect.Slice {
		return reflect.Value{}, -1, bindDirectErrorPath(mismatch(slot.Type(), []any{}), path)
	}
	elem := reflect.New(slot.Type().Elem()).Elem()
	index := slot.Len()
	slot.Set(reflect.Append(slot, elem))
	return directWritableValue(slot.Index(index)), index, nil
}

func directAssignRaw(root reflect.Value, path directRawPath, value any, cfg bindConfig) error {
	if !root.IsValid() {
		return nil
	}
	if path.len() == 0 {
		return &SyntaxError{Line: 1, Col: 1, Msg: "empty key", Span: [2]int{0, 0}}
	}
	cur := root
	for i := 0; i < path.len()-1; i++ {
		next, err := directFieldRaw(cur, path.part(i))
		if errors.Is(err, errDirectUnknownField) {
			return nil
		}
		if err != nil {
			return err
		}
		cur = directWritableValue(next)
	}
	cur = directWritableValue(cur)
	if cur.Kind() == reflect.Map {
		return directAssignMapRaw(cur, path, value, cfg)
	}
	dst, err := directFieldRaw(cur, path.part(path.len()-1))
	if errors.Is(err, errDirectUnknownField) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := bindValue(dst, value, cfg); err != nil {
		return bindDirectRawErrorPath(err, path)
	}
	return nil
}

func directAssign(root reflect.Value, path []string, value any, cfg bindConfig) error {
	if !root.IsValid() {
		return nil
	}
	if len(path) == 0 {
		return &SyntaxError{Line: 1, Col: 1, Msg: "empty key", Span: [2]int{0, 0}}
	}
	cur := root
	for _, name := range path[:len(path)-1] {
		next, err := directField(cur, name)
		if errors.Is(err, errDirectUnknownField) {
			return nil
		}
		if err != nil {
			return err
		}
		cur = directWritableValue(next)
	}
	cur = directWritableValue(cur)
	if cur.Kind() == reflect.Map {
		return directAssignMap(cur, path, value, cfg)
	}
	dst, err := directField(cur, path[len(path)-1])
	if errors.Is(err, errDirectUnknownField) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := bindValue(dst, value, cfg); err != nil {
		return bindDirectErrorPath(err, path)
	}
	return nil
}

func directFieldRaw(container reflect.Value, name []byte) (reflect.Value, error) {
	container = directWritableValue(container)
	switch container.Kind() {
	case reflect.Struct:
		info, err := reflectcache.Lookup(container.Type())
		if err != nil {
			return reflect.Value{}, normalizeReflectcacheError(err)
		}
		field, ok := lookupStructFieldBytes(info, name)
		if !ok {
			return reflect.Value{}, errDirectUnknownField
		}
		return container.FieldByIndex(field.Index), nil
	case reflect.Map:
		if container.Type().Key().Kind() != reflect.String {
			return reflect.Value{}, &UnsupportedTypeError{Type: container.Type().String()}
		}
		if container.IsNil() {
			container.Set(reflect.MakeMap(container.Type()))
		}
		elem := reflect.New(container.Type().Elem()).Elem()
		if elem.Kind() == reflect.Map {
			elem.Set(reflect.MakeMap(elem.Type()))
		}
		return elem, nil
	default:
		return reflect.Value{}, &UnsupportedTypeError{Type: container.Type().String()}
	}
}

func directField(container reflect.Value, name string) (reflect.Value, error) {
	container = directWritableValue(container)
	switch container.Kind() {
	case reflect.Struct:
		info, err := reflectcache.Lookup(container.Type())
		if err != nil {
			return reflect.Value{}, normalizeReflectcacheError(err)
		}
		field, ok := lookupStructField(info, name)
		if !ok {
			return reflect.Value{}, errDirectUnknownField
		}
		return container.FieldByIndex(field.Index), nil
	case reflect.Map:
		if container.Type().Key().Kind() != reflect.String {
			return reflect.Value{}, &UnsupportedTypeError{Type: container.Type().String()}
		}
		if container.IsNil() {
			container.Set(reflect.MakeMap(container.Type()))
		}
		elem := reflect.New(container.Type().Elem()).Elem()
		if elem.Kind() == reflect.Map {
			elem.Set(reflect.MakeMap(elem.Type()))
		}
		return elem, nil
	default:
		return reflect.Value{}, &UnsupportedTypeError{Type: container.Type().String()}
	}
}

func directAssignMapRaw(dst reflect.Value, path directRawPath, value any, cfg bindConfig) error {
	if dst.Type().Key().Kind() != reflect.String {
		return &UnsupportedTypeError{Type: dst.Type().String()}
	}
	if dst.IsNil() {
		dst.Set(reflect.MakeMap(dst.Type()))
	}
	elem := reflect.New(dst.Type().Elem()).Elem()
	if err := bindValue(elem, value, cfg); err != nil {
		return bindDirectRawErrorPath(err, path)
	}
	name := path.stringAt(path.len() - 1)
	dst.SetMapIndex(reflect.ValueOf(name).Convert(dst.Type().Key()), elem)
	return nil
}

func directAssignMap(dst reflect.Value, path []string, value any, cfg bindConfig) error {
	if dst.Type().Key().Kind() != reflect.String {
		return &UnsupportedTypeError{Type: dst.Type().String()}
	}
	if dst.IsNil() {
		dst.Set(reflect.MakeMap(dst.Type()))
	}
	elem := reflect.New(dst.Type().Elem()).Elem()
	if err := bindValue(elem, value, cfg); err != nil {
		return bindDirectErrorPath(err, path)
	}
	name := path[len(path)-1]
	dst.SetMapIndex(reflect.ValueOf(name).Convert(dst.Type().Key()), elem)
	return nil
}

func directWritableValue(v reflect.Value) reflect.Value {
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	return v
}

func bindDirectErrorPath(err error, path []string) error {
	for i := len(path) - 1; i >= 0; i-- {
		err = bindErrorPath(err, path[i])
	}
	return err
}

func bindDirectRawErrorPath(err error, path directRawPath) error {
	for i := path.len() - 1; i >= 0; i-- {
		err = bindErrorPath(err, path.stringAt(i))
	}
	return err
}
