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
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestDecoderTokenStream_BasicKeyValue(t *testing.T) {
	dec := NewDecoderBytes([]byte("name = \"Alice\"\nactive = true\n[server]\nports = [80, 443]\n"))

	expects := []TokenKind{
		TokenKindKey,
		TokenKindValueString,
		TokenKindKey,
		TokenKindValueBool,
		TokenKindTableHeader,
		TokenKindKey,
		TokenKindArrayStart,
		TokenKindValueInteger,
		TokenKindValueInteger,
		TokenKindArrayEnd,
	}
	for i, wantKind := range expects {
		tok, err := dec.ReadToken()
		if err != nil {
			t.Fatalf("ReadToken #%d error = %v", i, err)
		}
		if got := tok.Kind; got != wantKind {
			t.Fatalf("ReadToken #%d kind=%q, want=%q", i, got, wantKind)
		}
	}
	if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
		t.Fatalf("ReadToken after stream = %v, want EOF", err)
	}
}

func TestHasBytePrefixForTripleQuotedStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		in    string
		quote byte
		want  bool
	}{
		{name: "basic string", in: "\"\"\"multi", quote: '"', want: true},
		{name: "literal string", in: "'''multi", quote: '\'', want: true},
		{name: "wrong quote", in: "\"\"\"multi", quote: '\'', want: false},
		{name: "too short", in: "''", quote: '\'', want: false},
		{name: "single quoted string", in: "'value'", quote: '\'', want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := hasBytePrefix([]byte(tc.in), tc.quote, tc.quote, tc.quote); got != tc.want {
				t.Fatalf("hasBytePrefix(%q, %q, %q, %q) = %v, want %v", tc.in, tc.quote, tc.quote, tc.quote, got, tc.want)
			}
		})
	}
}

func TestDecoderTokenStream_CommentAndHeaderAtLineStart(t *testing.T) {
	dec := NewDecoderBytes([]byte("# file\n[app]\n# row\nvalue = 1\n"))

	expects := []TokenKind{
		TokenKindComment,
		TokenKindTableHeader,
		TokenKindComment,
		TokenKindKey,
		TokenKindValueInteger,
	}
	for i, wantKind := range expects {
		tok, err := dec.ReadToken()
		if err != nil {
			t.Fatalf("ReadToken #%d error = %v", i, err)
		}
		if tok.Kind != wantKind {
			t.Fatalf("ReadToken #%d kind=%q, want=%q", i, tok.Kind, wantKind)
		}
	}
}

func TestDecoderCorpus_ValidFiles_Smoke(t *testing.T) {
	for _, rel := range []string{
		"pkg/toml/testdata/toml-rs/corpus/valid/ext/table/append-with-dotted-keys-1.toml",
	} {
		body := mustReadRepoFile(t, rel)
		if body == nil {
			// not fatal; helper already reports with context.
			t.Fatalf("fixture missing: %s", rel)
		}

		dec := NewDecoderBytes(body)
		if _, err := readAllTokens(dec); err != nil {
			t.Fatalf("NewDecoderBytes should parse %s without syntax errors, got %v", rel, err)
		}
	}
}

func TestDecoderTokenGolden_FixtureCorpus(t *testing.T) {
	base := "pkg/toml/testdata/tokens"
	entries := mustListTokenFixtures(t, base)
	if len(entries) == 0 {
		t.Fatal("expected at least one token fixture in testdata/tokens")
	}
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".toml")
		t.Run(entry.Name(), func(t *testing.T) {
			input := mustReadRepoFile(t, filepath.Join(base, entry.Name()))
			got := mustReadAllTokens(t, NewDecoderBytes(input))
			wantPath := filepath.Join(base, name+".tokens.golden")
			want := readTokenGolden(t, mustRepoPath(t, wantPath))
			if len(got) != len(want) {
				t.Fatalf("token count mismatch: got=%d want=%d", len(got), len(want))
			}
			for i := range got {
				if string(got[i].Bytes) != want[i].Text {
					t.Fatalf("token[%d] bytes mismatch: got=%q want=%q", i, got[i].Bytes, want[i].Text)
				}
				if got[i].Kind != tokenKindFromString(want[i].Kind) {
					t.Fatalf("token[%d] kind mismatch: got=%q want=%q", i, got[i].Kind, want[i].Kind)
				}
			}
		})
	}
}

func TestDecoderTokenGolden_InvalidUTF8Fixture(t *testing.T) {
	input := mustReadRepoFile(t, "pkg/toml/testdata/tokens/invalid-utf8.toml")
	dec := NewDecoderBytes(input)
	if _, err := readAllTokens(dec); err == nil {
		t.Fatalf("expected invalid-utf8 fixture to fail")
	}
}

func TestDecoderTokenGolden_ReaderAndBytesParityForFixtureCorpus(t *testing.T) {
	base := "pkg/toml/testdata/tokens"
	entries := mustListTokenFixtures(t, base)
	if len(entries) == 0 {
		t.Fatal("expected at least one token fixture in testdata/tokens")
	}
	for _, entry := range entries {
		path := filepath.Join(base, entry.Name())
		t.Run(entry.Name(), func(t *testing.T) {
			input := mustReadRepoFile(t, path)
			gotKinds, err := readTokenKinds(NewDecoderBytes(input))
			if err != nil {
				t.Fatalf("bytes constructor failed: %v", err)
			}
			gotReaderKinds, err := readTokenKinds(NewDecoder(strings.NewReader(string(input))))
			if err != nil {
				t.Fatalf("reader constructor failed: %v", err)
			}
			if len(gotKinds) != len(gotReaderKinds) {
				t.Fatalf("token kind count mismatch: bytes=%d reader=%d", len(gotKinds), len(gotReaderKinds))
			}
			for i := range gotKinds {
				if gotKinds[i].Kind != gotReaderKinds[i].Kind {
					t.Fatalf("token[%d] mismatch: bytes=%q reader=%q", i, gotKinds[i].Kind, gotReaderKinds[i].Kind)
				}
			}
		})
	}
}

func TestDecoderCorpus_InvalidFiles_RejectSyntax(t *testing.T) {
	for _, rel := range []string{
		"pkg/toml/testdata/toml-rs/corpus/invalid/ext/keys/ml_literal.toml",
		"pkg/toml/testdata/toml-rs/corpus/invalid/ext/not-toml/deb.toml",
		"pkg/toml/testdata/toml-rs/corpus/invalid/ext/string/bad-escape-6.toml",
		"pkg/toml/testdata/toml-rs/corpus/invalid/ext/string/no-close-recovery-02.toml",
		"pkg/toml/testdata/toml-rs/corpus/invalid/ext/table/quoted-unclosed-2.toml",
		"pkg/toml/testdata/toml-rs/corpus/invalid/ext/table/newline-1.toml",
		"pkg/toml/testdata/toml-rs/corpus/invalid/ext/table/value.toml",
	} {
		body := mustReadRepoFile(t, rel)
		dec := NewDecoderBytes(body)
		if _, err := readAllTokens(dec); err == nil {
			t.Fatalf("expected %s to fail", rel)
		}
	}
}

func TestDecoderSyntaxErrorLineColumn(t *testing.T) {
	dec := NewDecoderBytes(mustReadRepoFile(t, "pkg/toml/testdata/toml-rs/corpus/invalid/ext/string/no-close-recovery-02.toml"))
	if tok, err := dec.ReadToken(); err != nil || tok.Kind != TokenKindKey {
		t.Fatalf("first ReadToken = %v, %v; want key before value error", tok, err)
	}
	_, err := dec.ReadToken()
	if err == nil {
		t.Fatalf("ReadToken expected syntax error")
	}
	se, ok := err.(*SyntaxError)
	if !ok {
		t.Fatalf("error type=%T, want *SyntaxError", err)
	}
	if se.Line != 1 {
		t.Fatalf("error line = %d, want 1", se.Line)
	}
	if se.Col != 5 {
		t.Fatalf("error col = %d, want 5", se.Col)
	}
}

func TestDecoderValueKindAndValidation(t *testing.T) {
	t.Parallel()

	valid := []struct {
		name string
		in   string
		kind TokenKind
	}{
		{name: "boolean", in: "a = true", kind: TokenKindValueBool},
		{name: "integer", in: "a = +12_34", kind: TokenKindValueInteger},
		{name: "float", in: "a = 12.34e+1", kind: TokenKindValueFloat},
		{name: "datetime-z", in: "a = 1987-07-05T17:45:00Z", kind: TokenKindValueDatetime},
		{name: "date", in: "a = 1979-05-27", kind: TokenKindValueDatetime},
		{name: "time", in: "a = 07:32:00", kind: TokenKindValueDatetime},
	}

	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dec := NewDecoderBytes([]byte(tc.in))
			if _, err := dec.ReadToken(); err != nil {
				t.Fatalf("ReadToken key = %v", err)
			}
			tok, err := dec.ReadToken()
			if err != nil {
				t.Fatalf("ReadToken value = %v", err)
			}
			if tok.Kind != tc.kind {
				t.Fatalf("ReadToken kind = %q, want %q", tok.Kind, tc.kind)
			}
		})
	}

	invalid := []string{
		"a = 2025-1-1",             // invalid date format
		"a = 2025-01-01T24:00:00Z", // invalid hour
		"a = 1e+",
		"a = 1_",
		"a = 18446744073709551616",
	}
	for _, in := range invalid {
		t.Run("invalid: "+in, func(t *testing.T) {
			t.Parallel()
			dec := NewDecoderBytes([]byte(in))
			for i := range 2 {
				if _, err := dec.ReadToken(); err != nil {
					// key token should not fail for these cases.
					if i == 0 {
						t.Fatalf("first token key parse = %v", err)
					}
					break
				}
				if i == 1 {
					t.Fatalf("expected syntax error for %q", in)
				}
			}
		})
	}
}

func TestDecoderErrorStateIsSticky(t *testing.T) {
	dec := NewDecoderBytes([]byte("broken = [\n"))
	tok, err := dec.ReadToken()
	if err != nil || tok.Kind != TokenKindKey {
		t.Fatalf("ReadToken key = %v, %v", tok, err)
	}
	tok, err = dec.ReadToken()
	if err != nil || tok.Kind != TokenKindArrayStart {
		t.Fatalf("ReadToken array start = %v, %v", tok, err)
	}
	_, err1 := dec.ReadToken()
	if err1 == nil {
		t.Fatalf("expected parse error from malformed array open")
	}
	_, err2 := dec.ReadToken()
	if err2 == nil {
		t.Fatalf("expected sticky error on repeated ReadToken calls")
	}
	if !errors.Is(err1, err2) {
		t.Fatalf("sticky error mismatch: first=%v second=%v", err1, err2)
	}
}

func TestDecoderReaderAndBytesConstructorsMatch(t *testing.T) {
	input := []byte("title = \"Toml\"\na = 1\n")
	wantKinds, err := readAllTokens(NewDecoderBytes(input))
	if err != nil {
		t.Fatalf("ReadToken with NewDecoderBytes = %v", err)
	}

	gotKinds, err := readAllTokens(NewDecoder(strings.NewReader(string(input))))
	if err != nil {
		t.Fatalf("ReadToken with NewDecoder = %v", err)
	}
	if len(wantKinds) != len(gotKinds) {
		t.Fatalf("token count mismatch: bytes=%d reader=%d", len(wantKinds), len(gotKinds))
	}
	for i := range wantKinds {
		if wantKinds[i].Kind != gotKinds[i].Kind {
			t.Fatalf("token[%d] mismatch: bytes=%q reader=%q", i, wantKinds[i].Kind, gotKinds[i].Kind)
		}
	}
}

func TestDecoderEOF(t *testing.T) {
	dec := NewDecoderBytes([]byte{})
	if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF on empty input, got %v", err)
	}
}

func TestDecoderKeyLimitError(t *testing.T) {
	tooLong := strings.Repeat("a", MaxKeyLength+1)
	dec := NewDecoderBytes([]byte(tooLong+" = 1\n"), WithLimits(Limits{MaxKeyLength: MaxKeyLength}))
	_, err := dec.ReadToken()
	if err == nil {
		t.Fatalf("ReadToken expected limit error")
	}
	if !strings.Contains(err.Error(), "MaxKeyLength") {
		t.Fatalf("error=%v, want MaxKeyLength mention", err)
	}
}

func TestDecoderDocumentSizeLimit(t *testing.T) {
	dec := NewDecoderBytes([]byte("name = \"Alice\""), WithLimits(Limits{MaxDocumentSize: 4}))
	_, err := dec.ReadToken()
	if err == nil {
		t.Fatalf("ReadToken expected document size limit error")
	}
	if !strings.Contains(err.Error(), "MaxDocumentSize") {
		t.Fatalf("error=%v, want MaxDocumentSize mention", err)
	}
}

func mustListTokenFixtures(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(mustRepoPath(t, dir))
	if err != nil {
		t.Fatalf("os.ReadDir(%q) error = %v", dir, err)
	}
	var fixtures []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".toml" {
			continue
		}
		if strings.HasSuffix(e.Name(), ".tokens.golden") {
			continue
		}
		if e.Name() == "invalid-utf8.toml" {
			continue
		}
		fixtures = append(fixtures, e)
	}
	sort.Slice(fixtures, func(i, j int) bool {
		return fixtures[i].Name() < fixtures[j].Name()
	})
	return fixtures
}

func readTokenKinds(dec *Decoder) ([]Token, error) {
	return readAllTokens(dec)
}

func mustReadAllTokens(t testing.TB, dec *Decoder) []Token {
	t.Helper()
	tokens, err := readAllTokens(dec)
	if err != nil {
		t.Fatalf("read token stream = %v", err)
	}
	return tokens
}

type tokenGolden struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

func readTokenGolden(t testing.TB, path string) []tokenGolden {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open(%s) error = %v", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	got := make([]tokenGolden, 0)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var item tokenGolden
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			t.Fatalf("invalid golden format %s:%d: %v", path, lineNo, err)
		}
		got = append(got, item)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan golden file %s error = %v", path, err)
	}
	return got
}

func tokenKindFromString(name string) TokenKind {
	switch name {
	case "Invalid":
		return TokenKindInvalid
	case "TableHeader":
		return TokenKindTableHeader
	case "ArrayTableHeader":
		return TokenKindArrayTableHeader
	case "Key":
		return TokenKindKey
	case "ValueString":
		return TokenKindValueString
	case "ValueInteger":
		return TokenKindValueInteger
	case "ValueFloat":
		return TokenKindValueFloat
	case "ValueBool":
		return TokenKindValueBool
	case "ValueDatetime":
		return TokenKindValueDatetime
	case "ArrayStart":
		return TokenKindArrayStart
	case "ArrayEnd":
		return TokenKindArrayEnd
	case "InlineTableStart":
		return TokenKindInlineTableStart
	case "InlineTableEnd":
		return TokenKindInlineTableEnd
	case "Comment":
		return TokenKindComment
	default:
		panic(fmt.Sprintf("unsupported token kind in fixture: %q", name))
	}
}

func readAllTokens(dec *Decoder) ([]Token, error) {
	tokens := make([]Token, 0)
	for {
		tok, err := dec.ReadToken()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return tokens, nil
			}
			return nil, err
		}
		tokens = append(tokens, tok)
	}
}

func mustReadRepoFile(t testing.TB, rel string) []byte {
	t.Helper()
	path := mustRepoPath(t, rel)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", path, err)
	}
	return body
}

func mustRepoPath(t testing.TB, rel string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(repoRoot, filepath.FromSlash(rel))
}

func TestHasBytePrefix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		raw  []byte
		want []byte
		ok   bool
	}{
		{name: "triple double", raw: []byte("\"\"\"value"), want: []byte{'"', '"', '"'}, ok: true},
		{name: "triple literal", raw: []byte("'''value"), want: []byte{'\'', '\'', '\''}, ok: true},
		{name: "too short", raw: []byte("\"\""), want: []byte{'"', '"', '"'}, ok: false},
		{name: "mismatch", raw: []byte("\"'\""), want: []byte{'"', '"', '"'}, ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := hasBytePrefix(tc.raw, tc.want...); got != tc.ok {
				t.Fatalf("hasBytePrefix(%q, %q...) = %v, want %v", tc.raw, tc.want, got, tc.ok)
			}
		})
	}
}
