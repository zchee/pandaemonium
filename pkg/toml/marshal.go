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
	"encoding"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zchee/pandaemonium/pkg/toml/internal/reflectcache"
)

var textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()

// Marshal encodes v as a TOML document.
func Marshal(v any) ([]byte, error) {
	return marshalWithOptions(v, MarshalOptions{})
}

func marshalWithOptions(v any, _ MarshalOptions) ([]byte, error) {
	if m, ok := v.(MarshalerTo); ok {
		var buf bytes.Buffer
		if err := m.MarshalTOMLTo(NewEncoder(&buf)); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	var buf bytes.Buffer
	if err := encodeDocument(&buf, reflect.ValueOf(v), nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeDocument(buf *bytes.Buffer, v reflect.Value, path []string) error {
	m, err := valueMap(v)
	if err != nil {
		return err
	}
	keys := sortedKeys(m)
	for _, k := range keys {
		if isTableLike(reflect.ValueOf(m[k])) {
			continue
		}
		if err := writeKeyValue(buf, k, reflect.ValueOf(m[k])); err != nil {
			return err
		}
	}
	for _, k := range keys {
		rv := reflect.ValueOf(m[k])
		if !isTableLike(rv) {
			continue
		}
		if isArrayOfTables(rv) {
			items := indirectValue(rv)
			for i := range items.Len() {
				buf.WriteByte('\n')
				writeHeader(buf, append(path, k), true)
				if err := encodeDocument(buf, items.Index(i), append(path, k)); err != nil {
					return err
				}
			}
			continue
		}
		buf.WriteByte('\n')
		writeHeader(buf, append(path, k), false)
		if err := encodeDocument(buf, rv, append(path, k)); err != nil {
			return err
		}
	}
	return nil
}

func writeKeyValue(buf *bytes.Buffer, key string, v reflect.Value) error {
	buf.WriteString(formatKey(key))
	buf.WriteString(" = ")
	if err := writeValue(buf, v); err != nil {
		return err
	}
	buf.WriteByte('\n')
	return nil
}

func writeHeader(buf *bytes.Buffer, path []string, array bool) {
	if array {
		buf.WriteString("[[")
	} else {
		buf.WriteByte('[')
	}
	for i, p := range path {
		if i > 0 {
			buf.WriteByte('.')
		}
		buf.WriteString(formatKey(p))
	}
	if array {
		buf.WriteString("]]\n")
	} else {
		buf.WriteString("]\n")
	}
}

func writeValue(buf *bytes.Buffer, v reflect.Value) error {
	v = indirectValue(v)
	if !v.IsValid() {
		return &UnsupportedTypeError{Type: "nil"}
	}
	if v.CanInterface() {
		switch x := v.Interface().(type) {
		case time.Time:
			buf.WriteString(x.Format(time.RFC3339Nano))
			return nil
		case LocalDateTime:
			buf.WriteString(x.String())
			return nil
		case LocalDate:
			buf.WriteString(x.String())
			return nil
		case LocalTime:
			buf.WriteString(x.String())
			return nil
		}
		if v.Type().Implements(textMarshalerType) {
			text, err := v.Interface().(encoding.TextMarshaler).MarshalText()
			if err != nil {
				return err
			}
			buf.WriteString(strconv.Quote(string(text)))
			return nil
		}
	}
	switch v.Kind() {
	case reflect.String:
		buf.WriteString(strconv.Quote(v.String()))
		return nil
	case reflect.Bool:
		buf.WriteString(strconv.FormatBool(v.Bool()))
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		buf.WriteString(strconv.FormatInt(v.Int(), 10))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		buf.WriteString(strconv.FormatUint(v.Uint(), 10))
		return nil
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		switch {
		case math.IsInf(f, 1):
			buf.WriteString("inf")
		case math.IsInf(f, -1):
			buf.WriteString("-inf")
		case math.IsNaN(f):
			buf.WriteString("nan")
		default:
			buf.WriteString(strconv.FormatFloat(f, 'g', -1, v.Type().Bits()))
		}
		return nil
	case reflect.Slice, reflect.Array:
		buf.WriteByte('[')
		for i := range v.Len() {
			if i > 0 {
				buf.WriteString(", ")
			}
			if err := writeValue(buf, v.Index(i)); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	case reflect.Map, reflect.Struct:
		m, err := valueMap(v)
		if err != nil {
			return err
		}
		buf.WriteString("{ ")
		keys := sortedKeys(m)
		for i, k := range keys {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(formatKey(k))
			buf.WriteString(" = ")
			if err := writeValue(buf, reflect.ValueOf(m[k])); err != nil {
				return err
			}
		}
		buf.WriteString(" }")
		return nil
	case reflect.Interface:
		if v.IsNil() {
			return &UnsupportedTypeError{Type: "nil interface"}
		}
		return writeValue(buf, v.Elem())
	default:
		return &UnsupportedTypeError{Type: v.Type().String()}
	}
}

func valueMap(v reflect.Value) (map[string]any, error) {
	v = indirectValue(v)
	if !v.IsValid() {
		return nil, &UnsupportedTypeError{Type: "nil"}
	}
	switch v.Kind() {
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return nil, &UnsupportedTypeError{Type: v.Type().String()}
		}
		m := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			m[iter.Key().String()] = iter.Value().Interface()
		}
		return m, nil
	case reflect.Struct:
		info, err := reflectcache.Lookup(v.Type())
		if err != nil {
			return nil, normalizeReflectcacheError(err)
		}
		m := make(map[string]any, len(info.Fields))
		for _, f := range info.Fields {
			fv := v.FieldByIndex(f.Index)
			if f.OmitZero && fv.IsZero() {
				continue
			}
			if !fv.CanInterface() {
				continue
			}
			if isNilValue(fv) {
				continue
			}
			m[f.Name] = fv.Interface()
		}
		return m, nil
	default:
		return nil, &UnsupportedTypeError{Type: v.Type().String()}
	}
}

func indirectValue(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func isNilValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isTableLike(v reflect.Value) bool {
	v = indirectValue(v)
	if !v.IsValid() || isScalarSpecial(v) {
		return false
	}
	if isArrayOfTables(v) {
		return true
	}
	return v.Kind() == reflect.Struct || v.Kind() == reflect.Map
}

func isScalarSpecial(v reflect.Value) bool {
	if !v.IsValid() || !v.CanInterface() {
		return false
	}
	switch v.Interface().(type) {
	case time.Time, LocalDateTime, LocalDate, LocalTime:
		return true
	}
	return v.Type().Implements(textMarshalerType)
}

func isArrayOfTables(v reflect.Value) bool {
	v = indirectValue(v)
	if !v.IsValid() || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) || v.Len() == 0 {
		return false
	}
	e := indirectValue(v.Index(0))
	return e.IsValid() && !isScalarSpecial(e) && (e.Kind() == reflect.Struct || e.Kind() == reflect.Map)
}

func formatKey(key string) string {
	if key == "" {
		return strconv.Quote(key)
	}
	for _, r := range key {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return strconv.Quote(key)
	}
	return key
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}
