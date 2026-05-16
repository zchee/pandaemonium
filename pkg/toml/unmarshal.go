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
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/zchee/pandaemonium/pkg/toml/internal/reflectcache"
)

var textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()

// Unmarshal decodes a TOML document into dst.
func Unmarshal(data []byte, dst any) error {
	return unmarshalWithOptions(data, dst, UnmarshalOptions{})
}

func unmarshalWithOptions(data []byte, dst any, opts UnmarshalOptions) error {
	cfg := bindConfigFromOptions(opts.DecoderOptions)
	if dst == nil {
		return &TypeMismatchError{Want: "non-nil pointer", Got: "nil"}
	}
	if u, ok := dst.(UnmarshalerFrom); ok {
		return u.UnmarshalTOMLFrom(NewDecoderBytes(data, opts.DecoderOptions...))
	}
	root, err := parseDocument(data, opts.DecoderOptions)
	if err != nil {
		return err
	}
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return &TypeMismatchError{Want: "non-nil pointer", Got: v.Kind().String()}
	}
	return bindValue(v.Elem(), root, "", cfg)
}

type bindConfig struct {
	localAsUTC bool
}

func bindConfigFromOptions(opts []Option) bindConfig {
	decoder := &Decoder{}
	for _, opt := range opts {
		opt(decoder)
	}
	return bindConfig{localAsUTC: decoder.localAsUTC}
}

func bindValue(dst reflect.Value, src any, path string, cfg bindConfig) error {
	if !dst.CanSet() {
		return nil
	}
	if dst.Kind() == reflect.Pointer {
		if src == nil {
			return nil
		}
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return bindValue(dst.Elem(), src, path, cfg)
	}
	if dst.CanAddr() && dst.Addr().Type().Implements(textUnmarshalerType) {
		text, ok := src.(string)
		if !ok {
			if tm, ok := src.(encoding.TextMarshaler); ok {
				b, err := tm.MarshalText()
				if err != nil {
					return err
				}
				text = string(b)
				ok = true
			}
		}
		if ok {
			return dst.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(text))
		}
	}
	if dst.Type() == reflect.TypeFor[time.Time]() {
		switch v := src.(type) {
		case time.Time:
			dst.Set(reflect.ValueOf(v))
			return nil
		case LocalDateTime, LocalDate, LocalTime:
			if !cfg.localAsUTC {
				return &LocalTimeIntoTimeError{Kind: TokenKindValueDatetime}
			}
			switch x := v.(type) {
			case LocalDateTime:
				dst.Set(reflect.ValueOf(time.Date(x.Year, time.Month(x.Month), x.Day, x.Hour, x.Minute, x.Second, x.Nanosecond, time.UTC)))
			case LocalDate:
				dst.Set(reflect.ValueOf(time.Date(x.Year, time.Month(x.Month), x.Day, 0, 0, 0, 0, time.UTC)))
			case LocalTime:
				dst.Set(reflect.ValueOf(time.Date(0, time.January, 1, x.Hour, x.Minute, x.Second, x.Nanosecond, time.UTC)))
			}
			return nil
		}
	}
	switch dst.Kind() {
	case reflect.Interface:
		dst.Set(reflect.ValueOf(src))
		return nil
	case reflect.Struct:
		m, ok := asDocumentMap(src)
		if !ok {
			return mismatch(path, dst.Type(), src)
		}
		info, err := reflectcache.Lookup(dst.Type())
		if err != nil {
			return normalizeReflectcacheError(err)
		}
		for name, raw := range m {
			field, ok := lookupStructField(info, name)
			if !ok {
				continue
			}
			if err := bindValue(dst.FieldByIndex(field.Index), raw, joinPath(path, name), cfg); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		m, ok := asDocumentMap(src)
		if !ok {
			return mismatch(path, dst.Type(), src)
		}
		if dst.Type().Key().Kind() != reflect.String {
			return &UnsupportedTypeError{Type: dst.Type().String()}
		}
		if dst.IsNil() {
			dst.Set(reflect.MakeMapWithSize(dst.Type(), len(m)))
		}
		for k, raw := range m {
			kv := reflect.ValueOf(k).Convert(dst.Type().Key())
			ev := reflect.New(dst.Type().Elem()).Elem()
			if err := bindValue(ev, raw, joinPath(path, k), cfg); err != nil {
				return err
			}
			dst.SetMapIndex(kv, ev)
		}
		return nil
	case reflect.Slice:
		items, ok := src.([]any)
		if !ok {
			return mismatch(path, dst.Type(), src)
		}
		out := reflect.MakeSlice(dst.Type(), len(items), len(items))
		for i, raw := range items {
			if err := bindValue(out.Index(i), raw, fmt.Sprintf("%s[%d]", path, i), cfg); err != nil {
				return err
			}
		}
		dst.Set(out)
		return nil
	case reflect.Array:
		items, ok := src.([]any)
		if !ok || len(items) != dst.Len() {
			return mismatch(path, dst.Type(), src)
		}
		for i, raw := range items {
			if err := bindValue(dst.Index(i), raw, fmt.Sprintf("%s[%d]", path, i), cfg); err != nil {
				return err
			}
		}
		return nil
	case reflect.String:
		s, ok := src.(string)
		if !ok {
			return mismatch(path, dst.Type(), src)
		}
		dst.SetString(s)
		return nil
	case reflect.Bool:
		b, ok := src.(bool)
		if !ok {
			return mismatch(path, dst.Type(), src)
		}
		dst.SetBool(b)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, ok := int64Value(src)
		if !ok || dst.OverflowInt(i) {
			return mismatch(path, dst.Type(), src)
		}
		dst.SetInt(i)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		i, ok := int64Value(src)
		if !ok || i < 0 || dst.OverflowUint(uint64(i)) {
			return mismatch(path, dst.Type(), src)
		}
		dst.SetUint(uint64(i))
		return nil
	case reflect.Float32, reflect.Float64:
		f, ok := float64Value(src)
		if !ok || dst.OverflowFloat(f) {
			return mismatch(path, dst.Type(), src)
		}
		dst.SetFloat(f)
		return nil
	default:
		return &UnsupportedTypeError{Type: dst.Type().String()}
	}
}

func asDocumentMap(v any) (documentMap, bool) {
	switch m := v.(type) {
	case documentMap:
		return m, true
	case map[string]any:
		return documentMap(m), true
	default:
		return nil, false
	}
}

func int64Value(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	default:
		return 0, false
	}
}

func float64Value(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	default:
		return 0, false
	}
}

func mismatch(path string, typ reflect.Type, src any) error {
	return &TypeMismatchError{Path: path, Want: typ.String(), Got: fmt.Sprintf("%T", src)}
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func lookupStructField(info *reflectcache.TypeInfo, name string) (reflectcache.Field, bool) {
	if field, ok := info.ByName[name]; ok {
		return field, true
	}
	lower := strings.ToLower(name)
	if field, ok := info.ByName[lower]; ok {
		return field, true
	}
	for _, field := range info.Fields {
		if strings.EqualFold(field.Name, name) {
			return field, true
		}
	}
	return reflectcache.Field{}, false
}

func normalizeReflectcacheError(err error) error {
	var tag *reflectcache.InvalidTagOptionError
	if errors.As(err, &tag) {
		return &TagOptionError{Struct: tag.Struct.String(), Field: tag.Field, Option: tag.Option}
	}
	return err
}

func parseIntText(text string, bits int) (reflect.Value, error) {
	i, err := strconv.ParseInt(strings.ReplaceAll(text, "_", ""), 10, bits)
	if err != nil {
		return reflect.Value{}, err
	}
	return reflect.ValueOf(i), nil
}
