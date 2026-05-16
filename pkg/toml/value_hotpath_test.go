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
	"reflect"
	"testing"
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

func TestParseValueTokenNormalizationHotPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tok  Token
		want any
	}{
		{name: "integer underscores", tok: Token{Kind: TokenKindValueInteger, Bytes: []byte("1_2_3")}, want: int64(123)},
		{name: "float normalization", tok: Token{Kind: TokenKindValueFloat, Bytes: []byte("1_2.3E+4")}, want: 123000.0},
		{name: "bool true", tok: Token{Kind: TokenKindValueBool, Bytes: []byte("TRUE")}, want: true},
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
