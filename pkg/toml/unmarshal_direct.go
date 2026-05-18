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
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/zchee/pandaemonium/pkg/toml/internal/reflectcache"
)

var errDirectUnknownField = errors.New("toml: direct destination field is unknown")

var directStructEligibilityCache sync.Map // map[reflect.Type]bool

type directAssignment struct {
	name      string
	dst       reflect.Value
	mapTarget bool
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
	currentPath := []string(nil)
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
			path, err := parseHeaderKey(tok.Bytes, false)
			if err != nil {
				return err
			}
			next, err := directTable(dst, path)
			if err != nil {
				return err
			}
			current = next
			currentPath = path
		case TokenKindArrayTableHeader:
			path, err := parseHeaderKey(tok.Bytes, true)
			if err != nil {
				return err
			}
			next, index, err := directArrayTable(dst, path)
			if err != nil {
				return err
			}
			current = next
			currentPath = directArrayTablePath(path, index)
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
					return bindErrorPath(err, currentPathString(currentPath))
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
				return bindDirectErrorPath(err, currentPath)
			}
		default:
			return &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "unexpected token", Span: [2]int{0, 1}}
		}
	}
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
		name := string(raw)
		field, ok := lookupStructField(info, name)
		if !ok {
			return directAssignment{}, false, nil
		}
		return directAssignment{name: name, dst: cur.FieldByIndex(field.Index)}, true, nil
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

func (a directAssignment) assign(value any, cfg bindConfig) error {
	if a.mapTarget {
		elem := reflect.New(a.dst.Type().Elem()).Elem()
		if err := bindValue(elem, value, cfg); err != nil {
			return bindErrorPath(err, a.name)
		}
		a.dst.SetMapIndex(reflect.ValueOf(a.name).Convert(a.dst.Type().Key()), elem)
		return nil
	}
	if err := bindValue(a.dst, value, cfg); err != nil {
		return bindErrorPath(err, a.name)
	}
	return nil
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

func directArrayTablePath(path []string, index int) []string {
	if len(path) == 0 || index < 0 {
		return nil
	}
	next := append([]string(nil), path...)
	next[len(next)-1] += indexPath(index)
	return next
}

func bindDirectErrorPath(err error, path []string) error {
	for i := len(path) - 1; i >= 0; i-- {
		err = bindErrorPath(err, path[i])
	}
	return err
}

func currentPathString(path []string) string {
	switch len(path) {
	case 0:
		return ""
	case 1:
		return path[0]
	default:
		return strings.Join(path, ".")
	}
}
