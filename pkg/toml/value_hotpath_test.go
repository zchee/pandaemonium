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
	"math"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func TestParseStringValueHotPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "literal", input: "'demo'", want: "demo"},
		{name: "basic", input: "\"demo\"", want: "demo"},
		{name: "literal multiline", input: "'''demo'''", want: "demo"},
		{name: "basic multiline", input: "\"\"\"demo\"\"\"", want: "demo"},
		{name: "escaped", input: "\"demo\\nline\"", want: "demo\nline"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseStringValue([]byte(tc.input))
			if err != nil {
				t.Fatalf("parseStringValue(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("parseStringValue(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseDottedKeyHotPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "bare segments", input: "fruit.apple.taste", want: []string{"fruit", "apple", "taste"}},
		{name: "quoted segment", input: "fruit.\"apple pie\".taste", want: []string{"fruit", "apple pie", "taste"}},
		{name: "literal segment", input: "fruit.'apple pie'.taste", want: []string{"fruit", "apple pie", "taste"}},
		{name: "spaced", input: "  fruit . apple  ", want: []string{"fruit", "apple"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDottedKey([]byte(tc.input))
			if err != nil {
				t.Fatalf("parseDottedKey(%q) error = %v", tc.input, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseDottedKey(%q) = %#v, want %#v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseDirectRawPathHotPaths(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  string
		ok    bool
	}{
		"success: bare dotted": {
			input: "fruit.variety",
			want:  "fruit.variety",
			ok:    true,
		},
		"success: bare with trim": {
			input: "  table.subtable  ",
			want:  "table.subtable",
			ok:    true,
		},
		"fallback: quoted": {
			input: `fruit."apple pie"`,
		},
		"fallback: spaced separator": {
			input: "fruit . variety",
		},
		"fallback: empty segment": {
			input: "fruit..variety",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseDirectRawPath([]byte(tc.input))
			if ok != tc.ok {
				t.Fatalf("parseDirectRawPath(%q) ok = %v, want %v", tc.input, ok, tc.ok)
			}
			if !ok {
				return
			}
			if got.string() != tc.want {
				t.Fatalf("parseDirectRawPath(%q).string() = %q, want %q", tc.input, got.string(), tc.want)
			}
		})
	}
}

func TestDirectAssignmentForKeyAvoidsSuccessAllocation(t *testing.T) {
	type sample struct {
		Count int
	}

	dst := reflect.ValueOf(&sample{}).Elem()
	raw := []byte("count")
	if _, ok, err := directAssignmentForKey(dst, raw); err != nil || !ok {
		t.Fatalf("directAssignmentForKey() = ok %v, err %v; want ok true", ok, err)
	}

	valid := true
	allocs := testing.AllocsPerRun(1000, func() {
		target, ok, err := directAssignmentForKey(dst, raw)
		if err != nil || !ok || !target.dst.IsValid() {
			valid = false
		}
	})
	if !valid {
		t.Fatal("directAssignmentForKey() did not return a valid target")
	}
	if checkptrInstrumented() {
		t.Skipf("checkptr instrumentation changes unsafe lookup allocation count: got %v", allocs)
	}
	if allocs != 0 {
		t.Fatalf("directAssignmentForKey() allocs = %v, want 0", allocs)
	}
}

func TestDirectAssignmentForKeyInfoReusesCurrentStructMetadata(t *testing.T) {
	type sample struct {
		Count int
	}

	dst := reflect.ValueOf(&sample{}).Elem()
	info, err := directStructInfo(dst)
	if err != nil {
		t.Fatalf("directStructInfo() error = %v", err)
	}
	raw := []byte("count")
	target, ok, err := directAssignmentForKeyInfo(dst, info, raw)
	if err != nil || !ok {
		t.Fatalf("directAssignmentForKeyInfo() = ok %v, err %v; want ok true", ok, err)
	}
	if target.dst.Kind() != reflect.Int {
		t.Fatalf("directAssignmentForKeyInfo() dst kind = %s, want int", target.dst.Kind())
	}
}

func TestDirectDestinationRawResolvesFinalStructField(t *testing.T) {
	type sample struct {
		Fruit struct {
			Physical struct {
				Color string
			}
		}
	}

	dst := reflect.ValueOf(&sample{}).Elem()
	path, ok := parseDirectRawPath([]byte("fruit.physical.color"))
	if !ok {
		t.Fatal("parseDirectRawPath() failed for bare dotted path")
	}
	target, valueKind, ok, err := directDestinationRaw(dst, path)
	if err != nil || !ok {
		t.Fatalf("directDestinationRaw() = ok %v, err %v; want ok true", ok, err)
	}
	if !target.IsValid() || target.Kind() != reflect.String {
		t.Fatalf("directDestinationRaw() target = %#v, want string field", target)
	}
	if valueKind != directValueString {
		t.Fatalf("directDestinationRaw() valueKind = %v, want directValueString", valueKind)
	}
}

func TestDirectBindTypedTokenScalarDestinations(t *testing.T) {
	type sample struct {
		Name     string
		Enabled  bool
		Count    int16
		Unsigned uint16
		Ratio    float32
		When     time.Time
	}

	tests := map[string]struct {
		field string
		kind  directValueKind
		token Token
		check func(t *testing.T, got sample)
	}{
		"success: string": {
			field: "Name",
			kind:  directValueString,
			token: Token{Kind: TokenKindValueString, Bytes: []byte(`"demo"`)},
			check: func(t *testing.T, got sample) {
				t.Helper()
				if got.Name != "demo" {
					t.Fatalf("Name = %q, want demo", got.Name)
				}
			},
		},
		"success: bool": {
			field: "Enabled",
			kind:  directValueBool,
			token: Token{Kind: TokenKindValueBool, Bytes: []byte("true")},
			check: func(t *testing.T, got sample) {
				t.Helper()
				if !got.Enabled {
					t.Fatal("Enabled = false, want true")
				}
			},
		},
		"success: int": {
			field: "Count",
			kind:  directValueInt,
			token: Token{Kind: TokenKindValueInteger, Bytes: []byte("12")},
			check: func(t *testing.T, got sample) {
				t.Helper()
				if got.Count != 12 {
					t.Fatalf("Count = %d, want 12", got.Count)
				}
			},
		},
		"success: uint": {
			field: "Unsigned",
			kind:  directValueUint,
			token: Token{Kind: TokenKindValueInteger, Bytes: []byte("34")},
			check: func(t *testing.T, got sample) {
				t.Helper()
				if got.Unsigned != 34 {
					t.Fatalf("Unsigned = %d, want 34", got.Unsigned)
				}
			},
		},
		"success: float": {
			field: "Ratio",
			kind:  directValueFloat,
			token: Token{Kind: TokenKindValueFloat, Bytes: []byte("1.25")},
			check: func(t *testing.T, got sample) {
				t.Helper()
				if got.Ratio != 1.25 {
					t.Fatalf("Ratio = %v, want 1.25", got.Ratio)
				}
			},
		},
		"success: time": {
			field: "When",
			kind:  directValueTime,
			token: Token{Kind: TokenKindValueDatetime, Bytes: []byte("2026-05-17T03:04:05Z")},
			check: func(t *testing.T, got sample) {
				t.Helper()
				if got.When.Format(time.RFC3339) != "2026-05-17T03:04:05Z" {
					t.Fatalf("When = %s, want 2026-05-17T03:04:05Z", got.When.Format(time.RFC3339Nano))
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got sample
			field := reflect.ValueOf(&got).Elem().FieldByName(tc.field)
			if err := directBindTypedToken(nil, tc.token, field, tc.kind, bindConfig{}); err != nil {
				t.Fatalf("directBindTypedToken() error = %v", err)
			}
			tc.check(t, got)
		})
	}
}

func TestDirectBindTypedTokenLocalTimeUTCOption(t *testing.T) {
	type sample struct {
		When time.Time
	}

	tok := Token{Kind: TokenKindValueDatetime, Bytes: []byte("2026-05-17T03:04:05")}
	var rejected sample
	err := directBindTypedToken(nil, tok, reflect.ValueOf(&rejected).Elem().FieldByName("When"), directValueTime, bindConfig{})
	var localErr *LocalTimeIntoTimeError
	if !errors.As(err, &localErr) {
		t.Fatalf("directBindTypedToken(local datetime) error = %T(%v), want LocalTimeIntoTimeError", err, err)
	}

	var got sample
	if err := directBindTypedToken(nil, tok, reflect.ValueOf(&got).Elem().FieldByName("When"), directValueTime, bindConfig{localAsUTC: true}); err != nil {
		t.Fatalf("directBindTypedToken(localAsUTC) error = %v", err)
	}
	want := time.Date(2026, time.May, 17, 3, 4, 5, 0, time.UTC)
	if !got.When.Equal(want) || got.When.Location() != time.UTC {
		t.Fatalf("When = %s (%s), want %s UTC", got.When, got.When.Location(), want)
	}

	var noSeconds sample
	noSecondsTok := Token{Kind: TokenKindValueDatetime, Bytes: []byte("2026-05-17T03:04")}
	if err := directBindTypedToken(nil, noSecondsTok, reflect.ValueOf(&noSeconds).Elem().FieldByName("When"), directValueTime, bindConfig{localAsUTC: true}); err != nil {
		t.Fatalf("directBindTypedToken(no-seconds) error = %v", err)
	}
	want = time.Date(2026, time.May, 17, 3, 4, 0, 0, time.UTC)
	if !noSeconds.When.Equal(want) || noSeconds.When.Location() != time.UTC {
		t.Fatalf("When = %s (%s), want %s UTC", noSeconds.When, noSeconds.When.Location(), want)
	}
}

func TestDirectBindTypedTokenRejectsCapitalizedBool(t *testing.T) {
	type sample struct {
		Enabled bool
	}

	var got sample
	err := directBindTypedToken(nil, Token{Kind: TokenKindValueBool, Bytes: []byte("TRUE"), Line: 1, Col: 1}, reflect.ValueOf(&got).Elem().FieldByName("Enabled"), directValueBool, bindConfig{})
	if err == nil {
		t.Fatal("directBindTypedToken(TRUE) error = nil, want syntax error")
	}
}

func checkptrInstrumented() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, setting := range info.Settings {
		if setting.Key == "-gcflags" && strings.Contains(setting.Value, "checkptr") {
			return true
		}
	}
	return false
}

func TestParseValueTokenNormalizationHotPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tok  Token
		want any
	}{
		{name: "integer underscores", tok: Token{Kind: TokenKindValueInteger, Bytes: []byte("1_2_3")}, want: int64(123)},
		{name: "float normalization", tok: Token{Kind: TokenKindValueFloat, Bytes: []byte("1_2.3E+4")}, want: 123000.0},
		{name: "bool true", tok: Token{Kind: TokenKindValueBool, Bytes: []byte("true")}, want: true},
		{name: "bool false", tok: Token{Kind: TokenKindValueBool, Bytes: []byte("false")}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseValueToken(nil, tc.tok)
			if err != nil {
				t.Fatalf("parseValueToken(%q) error = %v", tc.tok.Bytes, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseValueToken(%q) = %#v, want %#v", tc.tok.Bytes, got, tc.want)
			}
		})
	}
}

func TestParseValueTokenRejectsCapitalizedBool(t *testing.T) {
	t.Parallel()

	_, err := parseValueToken(nil, Token{Kind: TokenKindValueBool, Bytes: []byte("TRUE"), Line: 1, Col: 1})
	if err == nil {
		t.Fatal("parseValueToken(TRUE) error = nil, want syntax error")
	}
}

func TestParseValueTokenSpecialFloatLiterals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tok  Token
		want float64
	}{
		{name: "positive inf", tok: Token{Kind: TokenKindValueFloat, Bytes: []byte("inf")}, want: math.Inf(1)},
		{name: "negative inf", tok: Token{Kind: TokenKindValueFloat, Bytes: []byte("-inf")}, want: math.Inf(-1)},
		{name: "nan", tok: Token{Kind: TokenKindValueFloat, Bytes: []byte("nan")}, want: math.Float64frombits(0x7ff8000000000000)},
		{name: "negative nan", tok: Token{Kind: TokenKindValueFloat, Bytes: []byte("-nan")}, want: math.Float64frombits(0xfff8000000000000)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseValueToken(nil, tc.tok)
			if err != nil {
				t.Fatalf("parseValueToken(%q) error = %v", tc.tok.Bytes, err)
			}
			gotF, ok := got.(float64)
			if !ok {
				t.Fatalf("parseValueToken(%q) = %T, want float64", tc.tok.Bytes, got)
			}
			if math.IsNaN(tc.want) {
				if !math.IsNaN(gotF) || math.Signbit(gotF) != math.Signbit(tc.want) {
					t.Fatalf("parseValueToken(%q) = %v, want signed NaN", tc.tok.Bytes, gotF)
				}
				return
			}
			if gotF != tc.want {
				t.Fatalf("parseValueToken(%q) = %v, want %v", tc.tok.Bytes, gotF, tc.want)
			}
		})
	}
}
