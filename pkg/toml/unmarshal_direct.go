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

	"github.com/zchee/pandaemonium/pkg/toml/internal/reflectcache"
)

var errDirectUnknownField = errors.New("toml: direct destination field is unknown")

// bindDocumentDirect decodes the token stream directly into dst. It avoids the
// intermediate documentMap used by generic callers while preserving the same
// Decoder.ReadToken source of truth as the rest of the facade.
func bindDocumentDirect(data []byte, dst reflect.Value, opts []Option, cfg bindConfig) error {
	dec := NewDecoderBytes(data, opts...)
	current := dst
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
		case TokenKindArrayTableHeader:
			path, err := parseHeaderKey(tok.Bytes, true)
			if err != nil {
				return err
			}
			next, err := directArrayTable(dst, path)
			if err != nil {
				return err
			}
			current = next
		case TokenKindKey:
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
				return err
			}
		default:
			return &SyntaxError{Line: tok.Line, Col: tok.Col, Msg: "unexpected token", Span: [2]int{0, 1}}
		}
	}
}

func directTable(root reflect.Value, path []string) (reflect.Value, error) {
	cur := root
	for _, name := range path {
		next, err := directField(cur, name)
		if errors.Is(err, errDirectUnknownField) {
			return reflect.Value{}, nil
		}
		if err != nil {
			return reflect.Value{}, err
		}
		cur = directWritableValue(next)
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

func directArrayTable(root reflect.Value, path []string) (reflect.Value, error) {
	if len(path) == 0 {
		return root, nil
	}
	parent, err := directTable(root, path[:len(path)-1])
	if err != nil {
		return reflect.Value{}, err
	}
	if !parent.IsValid() {
		return reflect.Value{}, nil
	}
	slot, err := directField(parent, path[len(path)-1])
	if errors.Is(err, errDirectUnknownField) {
		return reflect.Value{}, nil
	}
	if err != nil {
		return reflect.Value{}, err
	}
	slot = directWritableValue(slot)
	if slot.Kind() != reflect.Slice {
		return reflect.Value{}, bindErrorPath(mismatch(slot.Type(), []any{}), path[len(path)-1])
	}
	elem := reflect.New(slot.Type().Elem()).Elem()
	slot.Set(reflect.Append(slot, elem))
	return directWritableValue(slot.Index(slot.Len() - 1)), nil
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
		return directAssignMap(cur, path[len(path)-1], value, cfg)
	}
	dst, err := directField(cur, path[len(path)-1])
	if errors.Is(err, errDirectUnknownField) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := bindValue(dst, value, cfg); err != nil {
		return bindErrorPath(err, path[len(path)-1])
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

func directAssignMap(dst reflect.Value, name string, value any, cfg bindConfig) error {
	if dst.Type().Key().Kind() != reflect.String {
		return &UnsupportedTypeError{Type: dst.Type().String()}
	}
	if dst.IsNil() {
		dst.Set(reflect.MakeMap(dst.Type()))
	}
	elem := reflect.New(dst.Type().Elem()).Elem()
	if err := bindValue(elem, value, cfg); err != nil {
		return bindErrorPath(err, name)
	}
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
