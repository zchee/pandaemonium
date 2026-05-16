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
	"errors"
	"io"
	"strings"
	"testing"
)

func TestParseDocumentBytesRoundTrip(t *testing.T) {
	t.Parallel()

	for _, rel := range []string{
		"pkg/toml/testdata/corpus/cargo.lock",
		"pkg/toml/testdata/tokens/basic.toml",
		"pkg/toml/testdata/tokens/comments.toml",
		"pkg/toml/testdata/toml-rs/corpus/valid/ext/table/append-with-dotted-keys-1.toml",
		"pkg/toml/testdata/toml-test/valid/array/array.toml",
	} {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			body := mustReadRepoFile(t, rel)
			doc, err := ParseDocument(body)
			if err != nil {
				t.Fatalf("ParseDocument(%s) error = %v", rel, err)
			}
			if !bytes.Equal(doc.Bytes(), body) {
				t.Fatalf("Bytes() != input for %s", rel)
			}
			if !bytes.Equal(doc.Raw(), body) {
				t.Fatalf("Raw() != input for %s", rel)
			}
		})
	}
}

func TestDocumentGetSupportsDottedAndQuotedPaths(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument([]byte("title = 'TOML'\n[owner]\n\"first.name\" = \"Tom\"\n"))
	if err != nil {
		t.Fatalf("ParseDocument error = %v", err)
	}
	if got, ok := doc.Get("title"); !ok || got != "TOML" {
		t.Fatalf("Get(title) = (%#v, %v), want TOML,true", got, ok)
	}
	if got, ok := doc.Get("owner.\"first.name\""); !ok || got != "Tom" {
		t.Fatalf("Get(owner.\"first.name\") = (%#v, %v), want Tom,true", got, ok)
	}
}

func TestDocumentSetSameKindMinimalDiffAndStyle(t *testing.T) {
	t.Parallel()

	input := []byte("# leading\nname = 'old' # keep comment\ncount = 1\n")
	doc, err := ParseDocument(input)
	if err != nil {
		t.Fatalf("ParseDocument error = %v", err)
	}
	if err := doc.Set("name", "new"); err != nil {
		t.Fatalf("Set(name) error = %v", err)
	}
	got := string(doc.Bytes())
	want := "# leading\nname = 'new' # keep comment\ncount = 1\n"
	if got != want {
		t.Fatalf("Bytes after Set(name) = %q, want %q", got, want)
	}
	if strings.Count(got, "# keep comment") != 1 {
		t.Fatalf("inline comment not preserved in %q", got)
	}
}

func TestDocumentSetKindChangeCanonicalizesOnlyValue(t *testing.T) {
	t.Parallel()

	input := []byte("enabled = true # keep\n")
	doc, err := ParseDocument(input)
	if err != nil {
		t.Fatalf("ParseDocument error = %v", err)
	}
	if err := doc.Set("enabled", "yes"); err != nil {
		t.Fatalf("Set(enabled) error = %v", err)
	}
	want := "enabled = \"yes\" # keep\n"
	if got := string(doc.Bytes()); got != want {
		t.Fatalf("Bytes after kind-changing Set = %q, want %q", got, want)
	}
}

func TestDocumentInsertAfterAndDelete(t *testing.T) {
	t.Parallel()

	input := []byte("title = \"demo\"\ncount = 1\n")
	doc, err := ParseDocument(input)
	if err != nil {
		t.Fatalf("ParseDocument error = %v", err)
	}
	if err := doc.InsertAfter("title", "enabled", true); err != nil {
		t.Fatalf("InsertAfter error = %v", err)
	}
	if err := doc.Delete("count"); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	if got, ok := doc.Get("enabled"); !ok || got != true {
		t.Fatalf("Get(enabled) = (%#v, %v), want true,true", got, ok)
	}
	if got, ok := doc.Get("count"); ok {
		t.Fatalf("Get(count) after Delete = (%#v, true), want missing", got)
	}
	want := "title = \"demo\"\nenabled = true\n"
	if got := string(doc.Bytes()); got != want {
		t.Fatalf("Bytes after InsertAfter/Delete = %q, want %q", got, want)
	}
}

func TestParseDocumentCopiesInputIntoDocumentRaw(t *testing.T) {
	t.Parallel()

	body := []byte("name = \"demo\"\n")
	doc, err := ParseDocument(body)
	if err != nil {
		t.Fatalf("ParseDocument error = %v", err)
	}
	body[0] = 'N'
	if got := doc.Bytes(); got[0] != 'n' {
		t.Fatalf("Bytes() should preserve parsed bytes after caller mutates input, got=%q", got)
	}
	if got := doc.Raw(); got[0] != 'n' {
		t.Fatalf("Raw() should preserve parsed bytes after caller mutates input, got=%q", got)
	}
}

func TestDocumentBytesNilSafe(t *testing.T) {
	t.Parallel()

	var doc *Document
	if got := doc.Bytes(); got != nil {
		t.Fatalf("nil Document Bytes() = %v, want nil", got)
	}
	if got := doc.Raw(); got != nil {
		t.Fatalf("nil Document Raw() = %v, want nil", got)
	}
}

func TestParseDocumentRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	_, err := ParseDocument([]byte("broken = [\n"))
	if err == nil {
		t.Fatal("ParseDocument returned nil error for invalid input")
	}
	if !errors.Is(err, io.EOF) {
		t.Logf("ParseDocument invalid input error = %T(%v)", err, err)
	}
}

func TestDocumentBytesAllocationsUntouched(t *testing.T) {
	body := mustReadRepoFile(t, "pkg/toml/testdata/corpus/cargo.lock")
	doc, err := ParseDocument(body)
	if err != nil {
		t.Fatalf("ParseDocument error = %v", err)
	}
	if got := testing.AllocsPerRun(100, func() {
		_ = doc.Bytes()
	}); got != 0 {
		t.Fatalf("Document.Bytes() allocs/run = %.0f, want 0", got)
	}
}

func TestDocumentMutationAPIQuotedDottedPath(t *testing.T) {
	t.Parallel()

	body := []byte("fruit.\"apple.color\" = \"red\"\nfruit.\"apple.taste.sweet\" = true\n")
	doc, err := ParseDocument(body)
	if err != nil {
		t.Fatalf("ParseDocument error = %v", err)
	}

	if got, ok := doc.Get(`fruit."apple.color"`); !ok || got != "red" {
		t.Fatalf(`Get(%q) = %v, %v; want "red", true`, `fruit."apple.color"`, got, ok)
	}
	if got, ok := doc.Get(`fruit."apple.taste.sweet"`); !ok || got != true {
		t.Fatalf(`Get(%q) = %v, %v; want true, true`, `fruit."apple.taste.sweet"`, got, ok)
	}

	if err := doc.Set(`fruit."apple.color"`, "green"); err != nil {
		t.Fatalf("Set color error = %v", err)
	}
	if err := doc.InsertAfter(`fruit."apple.color"`, `fruit."apple.size"`, 3); err != nil {
		t.Fatalf("InsertAfter size error = %v", err)
	}
	if err := doc.Delete(`fruit."apple.taste.sweet"`); err != nil {
		t.Fatalf("Delete taste error = %v", err)
	}
	if _, ok := doc.Get(`fruit."apple.taste.sweet"`); ok {
		t.Fatal(`Get("fruit.\"apple.taste.sweet\"") unexpectedly succeeded after Delete`)
	}

	want := "fruit.\"apple.color\" = \"green\"\n" +
		"fruit.\"apple.size\" = 3\n"
	if got := string(doc.Bytes()); got != want {
		t.Fatalf("Bytes() mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestDocumentParseEqualsDecoderTokenStream(t *testing.T) {
	t.Parallel()

	for _, rel := range []string{
		"pkg/toml/testdata/tokens/basic.toml",
		"pkg/toml/testdata/tokens/comments.toml",
		"pkg/toml/testdata/toml-rs/corpus/valid/ext/table/append-with-dotted-keys-1.toml",
	} {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			body := mustReadRepoFile(t, rel)
			want, err := collectDecoderTokenTrace(body)
			if err != nil {
				t.Fatalf("collectDecoderTokenTrace(%s) error = %v", rel, err)
			}
			got, err := collectDocumentTokenTrace(body)
			if err != nil {
				t.Fatalf("collectDocumentTokenTrace(%s) error = %v", rel, err)
			}
			if !tokenTraceEqual(got, want) {
				t.Fatalf("document token trace diverged from Decoder.ReadToken\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}

type documentTokenTrace struct {
	Kind  TokenKind
	Bytes string
}

func collectDecoderTokenTrace(body []byte) ([]documentTokenTrace, error) {
	dec := NewDecoderBytes(body)
	var out []documentTokenTrace
	for {
		tok, err := dec.ReadToken()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, documentTokenTrace{Kind: tok.Kind, Bytes: string(tok.Bytes)})
	}
}

func collectDocumentTokenTrace(body []byte) ([]documentTokenTrace, error) {
	raw := append([]byte(nil), body...)
	p := &documentParser{data: raw, dec: NewDecoderBytes(raw)}
	var out []documentTokenTrace
	for {
		tok, err := p.readToken()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, documentTokenTrace{Kind: tok.Kind, Bytes: string(tok.Bytes)})
	}
}

func tokenTraceEqual(a, b []documentTokenTrace) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
