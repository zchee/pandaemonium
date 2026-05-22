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
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zchee/pandaemonium/pkg/toml/internal/reflectcache"
)

var textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()

const maxPooledStringKeys = 1024

var stringKeysPool sync.Pool

type marshalEntry struct {
	name  string
	value reflect.Value
}

// Marshal encodes v as a TOML document.
func Marshal(v any) ([]byte, error) {
	return marshalWithOptions(v, MarshalOptions{})
}

func marshalWithOptions(v any, opts MarshalOptions) ([]byte, error) {
	var buf bytes.Buffer
	if err := marshalToBuffer(&buf, v, opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func marshalToBuffer(buf *bytes.Buffer, v any, opts MarshalOptions) error {
	if m, ok := v.(MarshalerTo); ok {
		return m.MarshalTOMLTo(NewEncoder(buf, opts))
	}
	if m, ok := v.(map[string]any); ok {
		return encodeAnyMapDocument(buf, m, nil)
	}
	return encodeDocument(buf, reflect.ValueOf(v), nil)
}

func encodeDocument(buf *bytes.Buffer, v reflect.Value, path []string) error {
	v = indirectValue(v)
	if !v.IsValid() {
		return &UnsupportedTypeError{Type: "nil"}
	}
	switch v.Kind() {
	case reflect.Map:
		return encodeMapDocument(buf, v, path)
	case reflect.Struct:
		return encodeStructDocument(buf, v, path)
	default:
		return &UnsupportedTypeError{Type: v.Type().String()}
	}
}

func encodeStructDocument(buf *bytes.Buffer, v reflect.Value, path []string) error {
	info, err := reflectcache.Lookup(v.Type())
	if err != nil {
		return normalizeReflectcacheError(err)
	}
	if info.HasDuplicateNames {
		entries, err := structMarshalEntries(v)
		if err != nil {
			return err
		}
		return encodeEntriesDocument(buf, entries, path)
	}
	for _, field := range info.MarshalFields {
		value, ok := marshalFieldValue(v, field)
		if !ok || isTableLike(value) {
			continue
		}
		if err := writeKeyValue(buf, field.Name, value); err != nil {
			return err
		}
	}
	for _, field := range info.MarshalFields {
		value, ok := marshalFieldValue(v, field)
		if !ok || !isTableLike(value) {
			continue
		}
		if isArrayOfTables(value) {
			items := indirectValue(value)
			for i := range items.Len() {
				buf.WriteByte('\n')
				nextPath := appendPath(path, field.Name)
				writeHeader(buf, nextPath, true)
				if err := encodeDocument(buf, items.Index(i), nextPath); err != nil {
					return err
				}
			}
			continue
		}
		buf.WriteByte('\n')
		nextPath := appendPath(path, field.Name)
		writeHeader(buf, nextPath, false)
		if err := encodeDocument(buf, value, nextPath); err != nil {
			return err
		}
	}
	return nil
}

func encodeEntriesDocument(buf *bytes.Buffer, entries []marshalEntry, path []string) error {
	for _, entry := range entries {
		if isTableLike(entry.value) {
			continue
		}
		if err := writeKeyValue(buf, entry.name, entry.value); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if !isTableLike(entry.value) {
			continue
		}
		if isArrayOfTables(entry.value) {
			items := indirectValue(entry.value)
			for i := range items.Len() {
				buf.WriteByte('\n')
				nextPath := appendPath(path, entry.name)
				writeHeader(buf, nextPath, true)
				if err := encodeDocument(buf, items.Index(i), nextPath); err != nil {
					return err
				}
			}
			continue
		}
		buf.WriteByte('\n')
		nextPath := appendPath(path, entry.name)
		writeHeader(buf, nextPath, false)
		if err := encodeDocument(buf, entry.value, nextPath); err != nil {
			return err
		}
	}
	return nil
}

func encodeMapDocument(buf *bytes.Buffer, v reflect.Value, path []string) error {
	keys, err := sortedMapKeys(v)
	if err != nil {
		return err
	}
	for _, key := range keys {
		value := v.MapIndex(key)
		if isTableLike(value) {
			continue
		}
		if err := writeKeyValue(buf, key.String(), value); err != nil {
			return err
		}
	}
	for _, key := range keys {
		value := v.MapIndex(key)
		if !isTableLike(value) {
			continue
		}
		if isArrayOfTables(value) {
			items := indirectValue(value)
			for i := range items.Len() {
				buf.WriteByte('\n')
				nextPath := appendPath(path, key.String())
				writeHeader(buf, nextPath, true)
				if err := encodeDocument(buf, items.Index(i), nextPath); err != nil {
					return err
				}
			}
			continue
		}
		buf.WriteByte('\n')
		nextPath := appendPath(path, key.String())
		writeHeader(buf, nextPath, false)
		if err := encodeDocument(buf, value, nextPath); err != nil {
			return err
		}
	}
	return nil
}

func encodeAnyMapDocument(buf *bytes.Buffer, m map[string]any, path []string) error {
	keys := sortedStringKeys(m)
	defer recycleStringKeys(keys)
	for _, key := range keys {
		value := m[key]
		if isTableLikeAny(value) {
			continue
		}
		if err := writeKeyValueAny(buf, key, value); err != nil {
			return err
		}
	}
	for _, key := range keys {
		value := m[key]
		if !isTableLikeAny(value) {
			continue
		}
		if isArrayOfTablesAny(value) {
			items := value.([]any)
			for i := range items {
				buf.WriteByte('\n')
				nextPath := appendPath(path, key)
				writeHeader(buf, nextPath, true)
				if err := encodeAnyDocument(buf, items[i], nextPath); err != nil {
					return err
				}
			}
			continue
		}
		buf.WriteByte('\n')
		nextPath := appendPath(path, key)
		writeHeader(buf, nextPath, false)
		if err := encodeAnyDocument(buf, value, nextPath); err != nil {
			return err
		}
	}
	return nil
}

func encodeAnyDocument(buf *bytes.Buffer, value any, path []string) error {
	switch x := value.(type) {
	case map[string]any:
		return encodeAnyMapDocument(buf, x, path)
	case documentMap:
		return encodeAnyMapDocument(buf, map[string]any(x), path)
	default:
		return encodeDocument(buf, reflect.ValueOf(value), path)
	}
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

func writeKeyValueAny(buf *bytes.Buffer, key string, value any) error {
	buf.WriteString(formatKey(key))
	buf.WriteString(" = ")
	if err := writeAnyValue(buf, value); err != nil {
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
		writeFloat(buf, v.Float(), v.Type().Bits())
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
		return writeInlineTable(buf, v)
	case reflect.Interface:
		if v.IsNil() {
			return &UnsupportedTypeError{Type: "nil interface"}
		}
		return writeValue(buf, v.Elem())
	default:
		return &UnsupportedTypeError{Type: v.Type().String()}
	}
}

func writeAnyValue(buf *bytes.Buffer, value any) error {
	switch x := value.(type) {
	case nil:
		return &UnsupportedTypeError{Type: "nil"}
	case string:
		buf.WriteString(strconv.Quote(x))
		return nil
	case bool:
		buf.WriteString(strconv.FormatBool(x))
		return nil
	case int:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
		return nil
	case int8:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
		return nil
	case int16:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
		return nil
	case int32:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
		return nil
	case int64:
		buf.WriteString(strconv.FormatInt(x, 10))
		return nil
	case uint:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
		return nil
	case uint8:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
		return nil
	case uint16:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
		return nil
	case uint32:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
		return nil
	case uint64:
		buf.WriteString(strconv.FormatUint(x, 10))
		return nil
	case uintptr:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
		return nil
	case float32:
		writeFloat(buf, float64(x), 32)
		return nil
	case float64:
		writeFloat(buf, x, 64)
		return nil
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
	case encoding.TextMarshaler:
		text, err := x.MarshalText()
		if err != nil {
			return err
		}
		buf.WriteString(strconv.Quote(string(text)))
		return nil
	case []any:
		buf.WriteByte('[')
		for i, item := range x {
			if i > 0 {
				buf.WriteString(", ")
			}
			if err := writeAnyValue(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	case map[string]any:
		return writeInlineAnyMap(buf, x)
	case documentMap:
		return writeInlineAnyMap(buf, map[string]any(x))
	default:
		return writeValue(buf, reflect.ValueOf(value))
	}
}

func writeFloat(buf *bytes.Buffer, value float64, bitSize int) {
	switch {
	case math.IsInf(value, 1):
		buf.WriteString("inf")
	case math.IsInf(value, -1):
		buf.WriteString("-inf")
	case math.IsNaN(value):
		if math.Signbit(value) {
			buf.WriteString("-nan")
		} else {
			buf.WriteString("nan")
		}
	default:
		text := strconv.FormatFloat(value, 'g', -1, bitSize)
		if !strings.ContainsAny(text, ".eE") {
			text += ".0"
		}
		buf.WriteString(text)
	}
}

func writeInlineTable(buf *bytes.Buffer, v reflect.Value) error {
	v = indirectValue(v)
	if !v.IsValid() {
		return &UnsupportedTypeError{Type: "nil"}
	}
	buf.WriteString("{ ")
	switch v.Kind() {
	case reflect.Map:
		keys, err := sortedMapKeys(v)
		if err != nil {
			return err
		}
		for i, key := range keys {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(formatKey(key.String()))
			buf.WriteString(" = ")
			if err := writeValue(buf, v.MapIndex(key)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		if err := writeInlineStructTable(buf, v); err != nil {
			return err
		}
	default:
		return &UnsupportedTypeError{Type: v.Type().String()}
	}
	buf.WriteString(" }")
	return nil
}

func writeInlineAnyMap(buf *bytes.Buffer, m map[string]any) error {
	buf.WriteString("{ ")
	keys := sortedStringKeys(m)
	defer recycleStringKeys(keys)
	for i, key := range keys {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(formatKey(key))
		buf.WriteString(" = ")
		if err := writeAnyValue(buf, m[key]); err != nil {
			return err
		}
	}
	buf.WriteString(" }")
	return nil
}

func writeInlineStructTable(buf *bytes.Buffer, v reflect.Value) error {
	info, err := reflectcache.Lookup(v.Type())
	if err != nil {
		return normalizeReflectcacheError(err)
	}
	if info.HasDuplicateNames {
		entries, err := structMarshalEntries(v)
		if err != nil {
			return err
		}
		for i, entry := range entries {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(formatKey(entry.name))
			buf.WriteString(" = ")
			if err := writeValue(buf, entry.value); err != nil {
				return err
			}
		}
		return nil
	}
	first := true
	for _, field := range info.MarshalFields {
		value, ok := marshalFieldValue(v, field)
		if !ok {
			continue
		}
		if !first {
			buf.WriteString(", ")
		}
		first = false
		buf.WriteString(formatKey(field.Name))
		buf.WriteString(" = ")
		if err := writeValue(buf, value); err != nil {
			return err
		}
	}
	return nil
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

func structMarshalEntries(v reflect.Value) ([]marshalEntry, error) {
	info, err := reflectcache.Lookup(v.Type())
	if err != nil {
		return nil, normalizeReflectcacheError(err)
	}
	entries := make([]marshalEntry, 0, len(info.Fields))
	for _, f := range info.Fields {
		fv, ok := marshalFieldValue(v, f)
		if !ok {
			continue
		}
		entry := marshalEntry{name: f.Name, value: fv}
		if i := findMarshalEntry(entries, f.Name); i >= 0 {
			entries[i] = entry
			continue
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})
	return entries, nil
}

func marshalFieldValue(v reflect.Value, field reflectcache.Field) (reflect.Value, bool) {
	fv := v.FieldByIndex(field.Index)
	if field.OmitZero && fv.IsZero() {
		return reflect.Value{}, false
	}
	if !fv.CanInterface() {
		return reflect.Value{}, false
	}
	if isNilValue(fv) {
		return reflect.Value{}, false
	}
	return fv, true
}

func findMarshalEntry(entries []marshalEntry, name string) int {
	for i, entry := range entries {
		if entry.name == name {
			return i
		}
	}
	return -1
}

func sortedMapKeys(v reflect.Value) ([]reflect.Value, error) {
	if v.Type().Key().Kind() != reflect.String {
		return nil, &UnsupportedTypeError{Type: v.Type().String()}
	}
	keys := v.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].String() < keys[j].String()
	})
	return keys, nil
}

func sortedStringKeys(m map[string]any) []string {
	keysp, _ := stringKeysPool.Get().(*[]string)
	var keys []string
	if keysp != nil {
		keys = *keysp
	}
	if cap(keys) < len(m) {
		keys = make([]string, 0, len(m))
	} else {
		keys = keys[:0]
	}
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func recycleStringKeys(keys []string) {
	if cap(keys) > maxPooledStringKeys {
		return
	}
	clear(keys)
	keys = keys[:0]
	stringKeysPool.Put(&keys)
}

func appendPath(path []string, key string) []string {
	next := make([]string, len(path)+1)
	copy(next, path)
	next[len(path)] = key
	return next
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

func isTableLikeAny(value any) bool {
	switch x := value.(type) {
	case nil:
		return false
	case time.Time, LocalDateTime, LocalDate, LocalTime, encoding.TextMarshaler:
		return false
	case map[string]any, documentMap:
		return true
	case []any:
		return isArrayOfTablesAny(x)
	default:
		return isTableLike(reflect.ValueOf(value))
	}
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

func isArrayOfTablesAny(value any) bool {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return false
	}
	allMaps := true
	allEmpty := true
	hasNestedTable := false
	for _, item := range items {
		switch x := item.(type) {
		case map[string]any:
			if len(x) != 0 {
				allEmpty = false
				if containsNonEmptyTableLikeValueAny(x) {
					hasNestedTable = true
				}
			}
		case documentMap:
			if len(x) != 0 {
				allEmpty = false
				if containsNonEmptyTableLikeValueAny(map[string]any(x)) {
					hasNestedTable = true
				}
			}
		default:
			allMaps = false
		}
		if !allMaps {
			return false
		}
	}
	return allEmpty || hasNestedTable
}

func containsNonEmptyTableLikeValueAny(v map[string]any) bool {
	for _, child := range v {
		if hasNonEmptyTableLikeDescendantAny(child) {
			return true
		}
	}
	return false
}

func hasNonEmptyTableLikeDescendantAny(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			return false
		}
		return containsNonEmptyTableLikeValueAny(x)
	case documentMap:
		if len(x) == 0 {
			return false
		}
		return containsNonEmptyTableLikeValueAny(map[string]any(x))
	case []any:
		return slices.ContainsFunc(x, hasNonEmptyTableLikeDescendantAny)
	default:
		return false
	}
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
