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

//go:build bench
// +build bench

// Package smoketest is the Phase 2.5 throwaway parser throughput harness.
package smoketest

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/zchee/pandaemonium/pkg/toml"
)

// Unmarshal is a deliberately small, no-cache reflection shim over
// toml.Decoder.ReadToken. It exists only to test Phase 2.5 throughput
// trajectory before the real Phase 4 facade and reflectcache land.
func Unmarshal(data []byte, dst any) error {
	root, err := rootStruct(dst)
	if err != nil {
		return err
	}

	dec := toml.NewDecoderBytes(data)
	cur := root
	var pendingKey string
	var arrayField reflect.Value
	for {
		tok, err := dec.ReadToken()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		switch tok.Kind {
		case toml.TokenKindComment:
			continue
		case toml.TokenKindTableHeader:
			cur = descendTable(root, headerName(tok.Bytes, false))
			pendingKey = ""
			arrayField = reflect.Value{}
		case toml.TokenKindArrayTableHeader:
			cur = appendArrayTable(root, headerName(tok.Bytes, true))
			pendingKey = ""
			arrayField = reflect.Value{}
		case toml.TokenKindKey:
			pendingKey = keyName(tok.Bytes)
			arrayField = reflect.Value{}
		case toml.TokenKindArrayStart:
			arrayField = fieldByTOMLName(cur, pendingKey)
		case toml.TokenKindArrayEnd:
			arrayField = reflect.Value{}
		case toml.TokenKindValueString, toml.TokenKindValueInteger,
			toml.TokenKindValueFloat, toml.TokenKindValueBool,
			toml.TokenKindValueDatetime:
			if arrayField.IsValid() {
				if err := appendScalar(arrayField, tok); err != nil {
					return err
				}
				continue
			}
			if err := setScalar(fieldByTOMLName(cur, pendingKey), tok); err != nil {
				return err
			}
		case toml.TokenKindInlineTableStart, toml.TokenKindInlineTableEnd:
			// Cargo.lock does not need inline table materialization for the
			// Phase 2.5 representative struct; keep scanning for trajectory.
		}
	}
}

func rootStruct(dst any) (reflect.Value, error) {
	if dst == nil {
		return reflect.Value{}, fmt.Errorf("smoketest: nil destination")
	}
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return reflect.Value{}, fmt.Errorf("smoketest: destination must be non-nil pointer")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("smoketest: destination must point to struct")
	}
	return v, nil
}

func headerName(raw []byte, array bool) string {
	s := strings.TrimSpace(string(raw))
	if array {
		s = strings.TrimPrefix(strings.TrimSuffix(s, "]]"), "[[")
	} else {
		s = strings.TrimPrefix(strings.TrimSuffix(s, "]"), "[")
	}
	return keyName([]byte(s))
}

func keyName(raw []byte) string {
	parts := strings.Split(strings.TrimSpace(string(raw)), ".")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) >= 2 && (p[0] == '\'' || p[0] == '"') {
			if unq, err := strconv.Unquote(p); err == nil {
				p = unq
			}
		}
		parts[i] = p
	}
	return strings.Join(parts, ".")
}

func descendTable(root reflect.Value, name string) reflect.Value {
	cur := root
	for _, part := range strings.Split(name, ".") {
		field := fieldByTOMLName(cur, part)
		if !field.IsValid() {
			return reflect.Value{}
		}
		cur = indirect(field)
		if !cur.IsValid() || cur.Kind() != reflect.Struct {
			return reflect.Value{}
		}
	}
	return cur
}

func appendArrayTable(root reflect.Value, name string) reflect.Value {
	parts := strings.Split(name, ".")
	parent := root
	for _, part := range parts[:len(parts)-1] {
		parent = descendTable(parent, part)
		if !parent.IsValid() {
			return reflect.Value{}
		}
	}
	field := fieldByTOMLName(parent, parts[len(parts)-1])
	if !field.IsValid() || field.Kind() != reflect.Slice {
		return reflect.Value{}
	}
	elem := reflect.New(field.Type().Elem()).Elem()
	field.Set(reflect.Append(field, elem))
	return field.Index(field.Len() - 1)
}

func fieldByTOMLName(v reflect.Value, name string) reflect.Value {
	if !v.IsValid() || name == "" {
		return reflect.Value{}
	}
	v = indirect(v)
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	for i := range v.NumField() {
		fieldInfo := v.Type().Field(i)
		if fieldInfo.PkgPath != "" {
			continue
		}
		want := fieldInfo.Tag.Get("toml")
		if comma := strings.IndexByte(want, ','); comma >= 0 {
			want = want[:comma]
		}
		if want == "" {
			want = fieldInfo.Name
		}
		if strings.EqualFold(want, name) {
			return v.Field(i)
		}
	}
	return reflect.Value{}
}

func indirect(v reflect.Value) reflect.Value {
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	return v
}

func appendScalar(field reflect.Value, tok toml.Token) error {
	if !field.IsValid() || field.Kind() != reflect.Slice {
		return nil
	}
	elem := reflect.New(field.Type().Elem()).Elem()
	if err := setScalar(elem, tok); err != nil {
		return err
	}
	field.Set(reflect.Append(field, elem))
	return nil
}

func setScalar(field reflect.Value, tok toml.Token) error {
	if !field.IsValid() || !field.CanSet() {
		return nil
	}
	field = indirect(field)
	switch field.Kind() {
	case reflect.String:
		field.SetString(tokenString(tok))
	case reflect.Bool:
		v, err := strconv.ParseBool(string(tok.Bytes))
		if err != nil {
			return err
		}
		field.SetBool(v)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(strings.ReplaceAll(string(tok.Bytes), "_", ""), 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(strings.ReplaceAll(string(tok.Bytes), "_", ""), 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(v)
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(strings.ReplaceAll(string(tok.Bytes), "_", ""), field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(v)
	}
	return nil
}

func tokenString(tok toml.Token) string {
	s := string(tok.Bytes)
	if tok.Kind == toml.TokenKindValueString && len(s) >= 2 {
		if unq, err := strconv.Unquote(s); err == nil {
			return unq
		}
	}
	return s
}
