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
	"testing"
)

func TestTokenStream_GoldenAppendWithDottedKeys(t *testing.T) {
	body := mustReadRepoFile(t, "pkg/toml/testdata/toml-rs/corpus/valid/ext/table/append-with-dotted-keys-1.toml")
	tokens, err := readAllTokens(NewDecoderBytes(body))
	if err != nil {
		t.Fatalf("readAllTokens returned %v", err)
	}

	want := []Token{
		{Kind: TokenKindTableHeader, Bytes: []byte("[fruit.apple.texture]")},
		{Kind: TokenKindKey, Bytes: []byte("smooth")},
		{Kind: TokenKindValueBool, Bytes: []byte("true")},
		{Kind: TokenKindTableHeader, Bytes: []byte("[fruit]")},
		{Kind: TokenKindKey, Bytes: []byte("apple.color")},
		{Kind: TokenKindValueString, Bytes: []byte("\"red\"")},
		{Kind: TokenKindKey, Bytes: []byte("apple.taste.sweet")},
		{Kind: TokenKindValueBool, Bytes: []byte("true")},
	}
	if len(tokens) != len(want) {
		t.Fatalf("token count mismatch: got=%d want=%d", len(tokens), len(want))
	}
	for i := range want {
		got := tokens[i]
		if got.Kind != want[i].Kind {
			t.Fatalf("token[%d] kind mismatch: got=%q want=%q", i, got.Kind, want[i].Kind)
		}
		if string(got.Bytes) != string(want[i].Bytes) {
			t.Fatalf("token[%d] bytes mismatch: got=%q want=%q", i, got.Bytes, want[i].Bytes)
		}
	}
}
