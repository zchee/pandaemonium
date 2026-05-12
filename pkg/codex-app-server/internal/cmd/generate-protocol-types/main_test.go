package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestReadSchemaSourceLocalFile(t *testing.T) {
	t.Parallel()

	want := []byte(`{"definitions":{"Sample":{"type":"string"}}}`)
	path := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	got, err := readSchemaSource(path)
	if err != nil {
		t.Fatalf("readSchemaSource(%q) error = %v", path, err)
	}
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Fatalf("readSchemaSource(%q) mismatch (-want +got):\n%s", path, diff)
	}
}

func TestReadSchemaSourceLocalFileWithInvalidURLEscape(t *testing.T) {
	t.Parallel()

	want := []byte(`{"definitions":{"Escaped":{"type":"string"}}}`)
	path := filepath.Join(t.TempDir(), "schema%zz.json")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	got, err := readSchemaSource(path)
	if err != nil {
		t.Fatalf("readSchemaSource(%q) error = %v", path, err)
	}
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Fatalf("readSchemaSource(%q) mismatch (-want +got):\n%s", path, diff)
	}
}

func TestReadSchemaSourceHTTPURL(t *testing.T) {
	t.Parallel()

	want := []byte(`{"definitions":{"Remote":{"type":"string"}}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want %s", r.Method, http.MethodGet)
		}
		w.Header().Set("Content-Type", "application/schema+json")
		_, _ = w.Write(want)
	}))
	t.Cleanup(server.Close)

	got, err := readSchemaSource(server.URL + "/schema.json")
	if err != nil {
		t.Fatalf("readSchemaSource(%q) error = %v", server.URL, err)
	}
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Fatalf("readSchemaSource(%q) mismatch (-want +got):\n%s", server.URL, diff)
	}
}

func TestReadSchemaSourceHTTPStatusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	sensitiveURL := strings.Replace(server.URL, "http://", "http://user:pass@", 1) + "/missing.json?token=secret"
	_, err := readSchemaSource(sensitiveURL)
	if err == nil {
		t.Fatal("readSchemaSource() error = nil, want HTTP status error")
	}
	if !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("readSchemaSource() error = %v, want 404 status", err)
	}
	if strings.Contains(err.Error(), "user:pass") || strings.Contains(err.Error(), "token=secret") {
		t.Fatalf("readSchemaSource() error = %v, want redacted URL credentials and query", err)
	}
}

func TestReadSchemaSourceUnsupportedURLScheme(t *testing.T) {
	t.Parallel()

	_, err := readSchemaSource("ftp://example.test/schema.json")
	if err == nil {
		t.Fatal("readSchemaSource() error = nil, want unsupported scheme error")
	}
	if !strings.Contains(err.Error(), `unsupported schema URL scheme "ftp"`) {
		t.Fatalf("readSchemaSource() error = %v, want unsupported scheme", err)
	}
}

func TestReadSchemaSourceInvalidHTTPURL(t *testing.T) {
	t.Parallel()

	_, err := readSchemaSource("https://")
	if err == nil {
		t.Fatal("readSchemaSource() error = nil, want invalid URL error")
	}
	if !strings.Contains(err.Error(), "missing host") {
		t.Fatalf("readSchemaSource() error = %v, want missing host", err)
	}
}

func TestReadSchemaSourceMalformedHTTPURL(t *testing.T) {
	t.Parallel()

	_, err := readSchemaSource("https://example.test/%zz")
	if err == nil {
		t.Fatal("readSchemaSource() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), "parse source:") {
		t.Fatalf("readSchemaSource() error = %v, want parse error", err)
	}
}

func TestReadHTTPSchemaSourceWithLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("12345"))
	}))
	t.Cleanup(server.Close)

	_, err := readHTTPSchemaSourceWithLimit(server.URL+"/large.json", 4)
	if err == nil {
		t.Fatal("readHTTPSchemaSourceWithLimit() error = nil, want size limit error")
	}
	if !strings.Contains(err.Error(), "schema exceeds 4 bytes") {
		t.Fatalf("readHTTPSchemaSourceWithLimit() error = %v, want size limit", err)
	}
}

func TestSchemaSourceLabel(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  string
	}{
		"success: local path is slash normalized": {
			input: `testdata\schema.json`,
			want:  "testdata/schema.json",
		},
		"success: http url drops query fragment and user info": {
			input: "https://user:pass@example.test/schema.json?token=secret#section",
			want:  "https://example.test/schema.json",
		},
		"success: non-url local path with colon is preserved": {
			input: "schema:backup.json",
			want:  "schema:backup.json",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := schemaSourceLabel(tt.input)
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("schemaSourceLabel() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExportName(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"success: camel id initialism": {
			input: "threadId",
			want:  "ThreadID",
		},
		"success: pascal reference is preserved": {
			input: "TurnError",
			want:  "TurnError",
		},
		"success: slash and underscore delimit words": {
			input: "thread/name_updated",
			want:  "ThreadNameUpdated",
		},
		"success: leading digit is made valid": {
			input: "2fa_status",
			want:  "Value2faStatus",
		},
		"success: exported type keyword is valid identifier": {
			input: "type",
			want:  "Type",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := exportName(tt.input); got != tt.want {
				t.Fatalf("exportName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGoFieldName(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"success: json type field uses idiomatic exported field": {
			input: "type",
			want:  "Type",
		},
		"success: other keyword-derived field names stay compatibility safe": {
			input: "_default",
			want:  "DefaultValue",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := goFieldName(tt.input); got != tt.want {
				t.Fatalf("goFieldName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestUnexportName(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"success: regular identifier lowercases first rune": {
			input: "ThreadID",
			want:  "threadID",
		},
		"success: local go keyword gets suffix": {
			input: "Type",
			want:  "typeValue",
		},
		"success: empty identifier gets fallback": {
			input: "",
			want:  "value",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := unexportName(tt.input); got != tt.want {
				t.Fatalf("unexportName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTypeForSchema(t *testing.T) {
	g := newGenerator(map[string]*jsonschema.Schema{})
	tests := map[string]struct {
		input    *jsonschema.Schema
		optional bool
		want     string
	}{
		"success: required string property": {
			input: &jsonschema.Schema{Type: "string"},
			want:  "string",
		},
		"success: optional nullable scalar becomes pointer": {
			input:    &jsonschema.Schema{Types: []string{"string", "null"}},
			optional: true,
			want:     "*string",
		},
		"success: array of refs": {
			input: &jsonschema.Schema{Type: "array", Items: &jsonschema.Schema{Ref: "#/definitions/AppInfo"}},
			want:  "[]AppInfo",
		},
		"success: array of nullable refs becomes slice of pointers": {
			input: &jsonschema.Schema{
				Type:  "array",
				Items: &jsonschema.Schema{AnyOf: []*jsonschema.Schema{{Ref: "#/definitions/Nested"}, {Type: "null"}}},
			},
			want: "[]*Nested",
		},
		"success: string map": {
			input: &jsonschema.Schema{Type: "object", AdditionalProperties: &jsonschema.Schema{Type: "string"}},
			want:  "map[string]string",
		},
		"success: nullable ref becomes pointer": {
			input: &jsonschema.Schema{AnyOf: []*jsonschema.Schema{{Ref: "#/definitions/ReasoningEffort"}, {Type: "null"}}},
			want:  "*ReasoningEffort",
		},
		"success: complex union stays raw json": {
			input: &jsonschema.Schema{OneOf: []*jsonschema.Schema{{Type: "string"}, {Type: "integer"}}},
			want:  "jsontext.Value",
		},
		"success: multi-ref union stays raw json": {
			input: &jsonschema.Schema{
				OneOf: []*jsonschema.Schema{
					{Ref: "#/definitions/NamedVariant"},
					{Ref: "#/definitions/ExistingVariant"},
				},
			},
			want: "jsontext.Value",
		},
		"success: single-variant oneOf resolves inner type": {
			input: &jsonschema.Schema{
				OneOf: []*jsonschema.Schema{
					{Type: "string"},
				},
			},
			want: "string",
		},
		"success: single-variant oneOf object stays json for anonymous inline object": {
			input: &jsonschema.Schema{
				OneOf: []*jsonschema.Schema{
					{
						Type:     "object",
						Required: []string{"name"},
						Properties: map[string]*jsonschema.Schema{
							"name": {Type: "string"},
						},
					},
				},
			},
			want: "jsontext.Value",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := g.typeForSchema(tt.input, tt.optional); got != tt.want {
				t.Fatalf("typeForSchema() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateCodexAppServerPackageAvoidsSDKNameCollisions(t *testing.T) {
	definitions := map[string]*jsonschema.Schema{
		"Config": {
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"thread": {Ref: "#/definitions/Thread"},
			},
		},
		"Thread": {
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"id": {Type: "string"},
			},
			Required: []string{"id"},
		},
		"ThreadStartResponse": {
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"thread": {Ref: "#/definitions/Thread"},
			},
			Required: []string{"thread"},
		},
	}
	gotBytes, err := newGenerator(definitions).generate("schema.json", "codexappserver")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got := string(gotBytes)
	wantFragments := []string{
		"package codexappserver",
		"type ConfigPayload struct {",
		"Thread *ThreadPayload `json:\"thread,omitzero\"`",
		"type ThreadPayload struct {",
		"Thread ThreadPayload `json:\"thread\"`",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, got)
		}
	}
	for _, fragment := range []string{
		"type Config struct",
		"type Thread struct",
		"type ProtocolConfig struct",
		"type ProtocolThread struct",
	} {
		if strings.Contains(got, fragment) {
			t.Fatalf("generated source unexpectedly contains %q:\n%s", fragment, got)
		}
	}
}

func TestGenerateUnionDefinitions(t *testing.T) {
	definitions := map[string]*jsonschema.Schema{
		"InlineUnionVariantA": {
			Type:     "object",
			Required: []string{"type"},
			Properties: map[string]*jsonschema.Schema{
				"type": {Type: "string", Enum: []any{"type_a"}},
				"name": {Type: "string"},
			},
		},
		"InlineUnionVariantB": {
			Type:     "object",
			Required: []string{"type", "value"},
			Properties: map[string]*jsonschema.Schema{
				"type":  {Type: "string", Enum: []any{"type_b"}},
				"value": {Type: "string"},
			},
		},
		"InlineUnion": {
			OneOf: []*jsonschema.Schema{
				{Ref: "#/definitions/InlineUnionVariantA"},
				{Ref: "#/definitions/InlineUnionVariantB"},
			},
		},
		"AnonymousUnion": {
			OneOf: []*jsonschema.Schema{
				{
					Title:    "NamedVariant",
					Type:     "object",
					Required: []string{"kind"},
					Properties: map[string]*jsonschema.Schema{
						"kind": {Type: "string", Enum: []any{"named"}},
					},
				},
				{
					Type:     "object",
					Required: []string{"kind"},
					Properties: map[string]*jsonschema.Schema{
						"kind": {Type: "string", Enum: []any{"fallback"}},
					},
				},
			},
		},
		"ExistingVariant": {
			Type:     "object",
			Required: []string{"kind", "note"},
			Properties: map[string]*jsonschema.Schema{
				"kind": {Type: "string", Enum: []any{"existing"}},
				"note": {Type: "string"},
			},
		},
		"CollisionUnion": {
			OneOf: []*jsonschema.Schema{
				{Ref: "#/definitions/ExistingVariant"},
				{
					Title:    "ExistingVariant",
					Type:     "object",
					Required: []string{"kind", "note"},
					Properties: map[string]*jsonschema.Schema{
						"kind": {Type: "string", Enum: []any{"collision"}},
						"note": {Type: "string"},
					},
				},
			},
		},
		"NoDiscriminatorUnion": {
			OneOf: []*jsonschema.Schema{
				{
					Type:     "object",
					Required: []string{"kind"},
					Properties: map[string]*jsonschema.Schema{
						"kind":   {Type: "string", Enum: []any{"named"}},
						"status": {Type: "string", Enum: []any{"active"}},
						"common": {Type: "boolean"},
					},
				},
				{
					Type:     "object",
					Required: []string{"status"},
					Properties: map[string]*jsonschema.Schema{
						"kind":   {Type: "string"},
						"status": {Type: "string", Enum: []any{"inactive"}},
						"common": {Type: "boolean"},
					},
				},
			},
		},
	}
	g := newGenerator(definitions)
	if got := g.isObjectUnion(definitions["InlineUnion"]); !got {
		t.Fatalf("InlineUnion should be treated as object union")
	}
	if got := g.isObjectUnion(definitions["NoDiscriminatorUnion"]); !got {
		t.Fatalf("NoDiscriminatorUnion should be treated as metadata object union")
	}
	gotBytes, err := g.generate("schema.json", "protocol")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got := string(gotBytes)

	wantFragments := []string{
		"type InlineUnion interface {",
		"isInlineUnion()",
		"func (InlineUnionVariantA) isInlineUnion() {}",
		"func (InlineUnionVariantB) isInlineUnion() {}",
		"func (NamedVariant) isAnonymousUnion() {}",
		"func (AnonymousUnionKind) isAnonymousUnion() {}",
		"type AnonymousUnionKind struct {",
		"Kind string `json:\"kind\"`",
		"func (ExistingVariant) isCollisionUnion() {}",
		"type NoDiscriminatorUnion interface {",
		"func (NoDiscriminatorUnionKind) isNoDiscriminatorUnion() {}",
		"func (NoDiscriminatorUnionStatus) isNoDiscriminatorUnion() {}",
		"type NoDiscriminatorUnionKind struct {",
		"func (NoDiscriminatorUnionStatus) isNoDiscriminatorUnion() {}",
	}

	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, got)
		}
	}
}

func TestGenerateMixedUnionDefinitions(t *testing.T) {
	definitions := map[string]*jsonschema.Schema{
		"MixedSource": {
			OneOf: []*jsonschema.Schema{
				{
					Type: "string",
					Enum: []any{"cli", "vscode"},
				},
				{
					Title:    "CustomSource",
					Type:     "object",
					Required: []string{"custom"},
					Properties: map[string]*jsonschema.Schema{
						"custom": {Type: "string"},
					},
				},
				{
					Title:    "NestedSource",
					Type:     "object",
					Required: []string{"nested"},
					Properties: map[string]*jsonschema.Schema{
						"nested": {Type: "string"},
					},
				},
			},
		},
		"StringOnlyUnion": {
			OneOf: []*jsonschema.Schema{
				{Type: "string", Enum: []any{"auto", "detailed"}},
				{Type: "string", Enum: []any{"none"}},
			},
		},
		"Holder": {
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"source":  {Ref: "#/definitions/MixedSource"},
				"summary": {Ref: "#/definitions/StringOnlyUnion"},
			},
			Required: []string{"source", "summary"},
		},
	}

	gotBytes, err := newGenerator(definitions).generate("schema.json", "protocol")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got := string(gotBytes)
	wantFragments := []string{
		"type MixedSource interface {",
		"type RawMixedSource jsontext.Value",
		"type MixedSourceValue string",
		"MixedSourceValueCli MixedSourceValue = \"cli\"",
		"MixedSourceValueVscode MixedSourceValue = \"vscode\"",
		"type CustomSource struct {",
		"Custom string `json:\"custom\"`",
		"type NestedSource struct {",
		"func decodeGeneratedMixedSource(raw jsontext.Value) (MixedSource, error) {",
		"return MixedSourceValue(text), nil",
		"Custom jsontext.Value `json:\"custom\"`",
		"Nested jsontext.Value `json:\"nested\"`",
		"if object.Custom != nil {",
		"var value CustomSource",
		"if object.Nested != nil {",
		"var value NestedSource",
		"type StringOnlyUnion interface {",
		"type StringOnlyUnionValue string",
		"StringOnlyUnionValueDetailed StringOnlyUnionValue = \"detailed\"",
		"StringOnlyUnionValueNone StringOnlyUnionValue = \"none\"",
		"func decodeGeneratedStringOnlyUnion(raw jsontext.Value) (StringOnlyUnion, error) {",
		"Source jsontext.Value `json:\"source\"`",
		"decodedSource, err := decodeGeneratedMixedSource(raw.Source)",
		"value.Source = decodedSource",
		"Summary jsontext.Value `json:\"summary\"`",
		"decodedSummary, err := decodeGeneratedStringOnlyUnion(raw.Summary)",
		"value.Summary = decodedSummary",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, got)
		}
	}
	if strings.Contains(got, "var object map[string]jsontext.Value") {
		t.Fatalf("mixed union decode helper should use a probe struct, not a map allocation:\n%s", got)
	}
	if strings.Contains(got, "type MixedSource jsontext.Value") {
		t.Fatalf("mixed union collapsed to raw wrapper instead of interface:\n%s", got)
	}
	if strings.Contains(got, "type StringOnlyUnion jsontext.Value") {
		t.Fatalf("string-only union collapsed to raw wrapper instead of interface:\n%s", got)
	}
}

func TestGenerateUnionVariantNamesAvoidTopLevelDefinitionCollisions(t *testing.T) {
	definitions := map[string]*jsonschema.Schema{
		"Event": {
			OneOf: []*jsonschema.Schema{
				{
					Title: "Payload/wrapper",
					Type:  "object",
					Properties: map[string]*jsonschema.Schema{
						"params": {Ref: "#/definitions/PayloadWrapper"},
						"type":   {Type: "string", Enum: []any{"payload"}},
					},
					Required: []string{"params", "type"},
				},
			},
		},
		"PayloadWrapper": {
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"value": {Type: "string"},
			},
			Required: []string{"value"},
		},
	}
	gotBytes, err := newGenerator(definitions).generate("schema.json", "protocol")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got := string(gotBytes)
	for _, fragment := range []string{
		"type PayloadWrapper struct {",
		"type PayloadWrapper2 struct {",
		"Params PayloadWrapper `json:\"params\"`",
		"func (PayloadWrapper2) isEvent() {}",
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated output missing %q:\n%s", fragment, got)
		}
	}
}

func TestGenerateUnionDefinitionVariants(t *testing.T) {
	definitions := map[string]*jsonschema.Schema{
		"SingleVariant": {
			Type:     "object",
			Title:    "SingleVariant",
			Required: []string{"kind"},
			Properties: map[string]*jsonschema.Schema{
				"kind": {Type: "string", Enum: []any{"single"}},
				"id":   {Type: "string"},
			},
		},
		"SingleVariantUnion": {
			OneOf: []*jsonschema.Schema{
				{Ref: "#/definitions/SingleVariant"},
			},
		},
		"NoDiscriminatorUnion": {
			OneOf: []*jsonschema.Schema{
				{
					Type:     "object",
					Required: []string{"kind"},
					Properties: map[string]*jsonschema.Schema{
						"kind": {Type: "string"},
						"id":   {Type: "string"},
					},
				},
				{
					Type:     "object",
					Required: []string{"kind"},
					Properties: map[string]*jsonschema.Schema{
						"kind": {Type: "string"},
						"size": {Type: "integer"},
					},
				},
			},
		},
	}

	g := newGenerator(definitions)
	gotBytes, err := g.generate("schema.json", "protocol")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got := string(gotBytes)

	wantFragments := []string{
		"type SingleVariantUnion = SingleVariant",
		"func (SingleVariant) isSingleVariantUnion() {}",
		"type NoDiscriminatorUnion interface {",
		"func (NoDiscriminatorUnionKind) isNoDiscriminatorUnion() {}",
		"func (NoDiscriminatorUnionKind2) isNoDiscriminatorUnion() {}",
	}

	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, got)
		}
	}
}

func TestValidPackageName(t *testing.T) {
	tests := map[string]struct {
		input string
		want  bool
	}{
		"success: normal package":     {input: "protocol", want: true},
		"success: underscore package": {input: "protocol_v2", want: true},
		"error: leading digit":        {input: "2protocol"},
		"error: keyword":              {input: "type"},
		"error: dash":                 {input: "protocol-v2"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := validPackageName(tt.input); got != tt.want {
				t.Fatalf("validPackageName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateRepresentativeTypes(t *testing.T) {
	definitions := map[string]*jsonschema.Schema{
		"SampleStatus": {Type: "string", Enum: []any{"ready", "needs_input"}},
		"Nested":       {Type: "object", Properties: map[string]*jsonschema.Schema{"id": {Type: "string"}}, Required: []string{"id"}},
		"Sample": {
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"enabled": {Types: []string{"boolean", "null"}},
				"labels":  {Type: "object", AdditionalProperties: &jsonschema.Schema{Type: "string"}},
				"items":   {Type: "array", Items: &jsonschema.Schema{AnyOf: []*jsonschema.Schema{{Ref: "#/definitions/Nested"}, {Type: "null"}}}},
				"nested":  {AnyOf: []*jsonschema.Schema{{Ref: "#/definitions/Nested"}, {Type: "null"}}},
				"status":  {Ref: "#/definitions/SampleStatus"},
			},
			Required: []string{"status"},
		},
		"UnionSliceHolder": {
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"unionSlice": {
					Type: "array",
					Items: &jsonschema.Schema{
						OneOf: []*jsonschema.Schema{
							{Type: "string"},
							{Type: "integer"},
						},
					},
				},
				"nestedUnion": {
					Type: "array",
					Items: &jsonschema.Schema{
						AnyOf: []*jsonschema.Schema{
							{Ref: "#/definitions/SampleStatus"},
							{Type: "null"},
						},
					},
				},
			},
			Required: []string{"unionSlice", "nestedUnion"},
		},
	}
	g := newGenerator(definitions)
	gotBytes, err := g.generate("schema.json", "protocol")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got := string(gotBytes)
	wantFragments := []string{
		"package protocol",
		"type SampleStatus string",
		"SampleStatusNeedsInput SampleStatus = \"needs_input\"",
		"Enabled *bool `json:\"enabled,omitzero\"`",
		"Labels map[string]string `json:\"labels,omitzero\"`",
		"Items []*Nested `json:\"items,omitzero\"`",
		"Nested *Nested `json:\"nested,omitzero\"`",
		"Status SampleStatus `json:\"status\"`",
		"UnionSlice []jsontext.Value `json:\"unionSlice\"`",
		"NestedUnion []*SampleStatus `json:\"nestedUnion\"`",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, got)
		}
	}
}

func TestGenerateRawWrappersAndCodecImports(t *testing.T) {
	definitions := map[string]*jsonschema.Schema{
		"SimpleEnum": {
			Type: "string",
			Enum: []any{"ready"},
		},
		"RawUnion": {
			OneOf: []*jsonschema.Schema{
				{Type: "string"},
				{Type: "integer"},
			},
		},
		"SimpleObject": {
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"status": {Ref: "#/definitions/SimpleEnum"},
			},
			Required: []string{"status"},
		},
	}
	g := newGenerator(definitions)
	gotBytes, err := g.generate("schema.json", "protocol")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got := string(gotBytes)
	wantFragments := []string{
		"import (\n\t\"github.com/go-json-experiment/json\"\n\t\"github.com/go-json-experiment/json/jsontext\"\n)\n",
		"type RawUnion jsontext.Value",
		"var _ json.MarshalerTo = RawUnion{}",
		"var _ json.UnmarshalerFrom = (*RawUnion)(nil)",
		"func (value RawUnion) MarshalJSONTo(enc *jsontext.Encoder) error {",
		"func (value *RawUnion) UnmarshalJSONFrom(dec *jsontext.Decoder) error {",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, got)
		}
	}

	plain := newGenerator(map[string]*jsonschema.Schema{
		"SimpleType": {Type: "string"},
	})
	gotBytes, err = plain.generate("schema.json", "protocol")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got = string(gotBytes)
	if want := "import \"github.com/go-json-experiment/json/jsontext\"\n\n"; !strings.Contains(got, want) {
		t.Fatalf("expected single import for non-raw schema; got:\n%s", got)
	}
}

func TestGenerateStructUnmarshalForRawUnionFields(t *testing.T) {
	definitions := map[string]*jsonschema.Schema{
		"Container": {
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"item": {
					Ref: "#/definitions/ObjectUnion",
				},
				"maybeItem": {
					Ref: "#/definitions/ObjectUnion",
				},
				"items": {
					Type: "array",
					Items: &jsonschema.Schema{
						Ref: "#/definitions/ObjectUnion",
					},
				},
				"single": {
					Ref: "#/definitions/SingleVariantUnion",
				},
			},
			Required: []string{"item", "items", "single"},
		},
		"ObjectUnion": {
			OneOf: []*jsonschema.Schema{
				{
					Title: "Alpha",
					Type:  "object",
					Properties: map[string]*jsonschema.Schema{
						"type": {
							Type: "string",
							Enum: []any{"alpha"},
						},
					},
					Required: []string{"type"},
				},
				{
					Title: "Beta",
					Type:  "object",
					Properties: map[string]*jsonschema.Schema{
						"type": {
							Type: "string",
							Enum: []any{"beta"},
						},
					},
					Required: []string{"type"},
				},
			},
		},
		"SingleVariantUnion": {
			OneOf: []*jsonschema.Schema{
				{
					Title: "SinglePayload",
					Type:  "object",
					Properties: map[string]*jsonschema.Schema{
						"type": {
							Type: "string",
							Enum: []any{"single"},
						},
					},
					Required: []string{"type"},
				},
			},
		},
	}
	gotBytes, err := newGenerator(definitions).generate("schema.json", "protocol")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got := string(gotBytes)
	wantFragments := []string{
		"func (value *Container) UnmarshalJSONFrom(dec *jsontext.Decoder) error {",
		"Item jsontext.Value `json:\"item\"`",
		"Items []jsontext.Value `json:\"items\"`",
		"MaybeItem jsontext.Value `json:\"maybeItem,omitzero\"`",
		"Single SingleVariantUnion `json:\"single\"`",
		"decodedItem, err := decodeGeneratedObjectUnion(raw.Item)",
		"value.Item = decodedItem",
		"decodedMaybeItem, err := decodeGeneratedObjectUnion(raw.MaybeItem)",
		"value.MaybeItem = &decodedMaybeItem",
		"value.Items = make([]ObjectUnion, len(raw.Items))",
		"decodedItems, err := decodeGeneratedObjectUnion(item)",
		"value.Items[i] = decodedItems",
		"func decodeGeneratedObjectUnion(raw jsontext.Value) (ObjectUnion, error) {",
		"Type string `json:\"type\"`",
		"switch object.Type {",
		"case \"alpha\":",
		"var value Alpha",
		"case \"beta\":",
		"var value Beta",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, got)
		}
	}
	if strings.Contains(got, "rawDiscriminator") || strings.Contains(got, "object[\"type\"]") {
		t.Fatalf("object union decode helper should use probe struct fields instead of map lookups:\n%s", got)
	}
	if strings.Contains(got, "RawSingleVariantUnion") {
		t.Fatalf("single-variant aliases must not use a missing raw union wrapper:\n%s", got)
	}
}

func TestGenerateUnionProbeFieldHandlesSharedDiscriminatorAndPresenceKey(t *testing.T) {
	definitions := map[string]*jsonschema.Schema{
		"SharedKeyUnion": {
			OneOf: []*jsonschema.Schema{
				{
					Title: "DiscriminatedSharedKey",
					Type:  "object",
					Properties: map[string]*jsonschema.Schema{
						"kind": {
							Type: "string",
							Enum: []any{"alpha"},
						},
						"value": {Type: "string"},
					},
					Required: []string{"kind", "value"},
				},
				{
					Title: "PresenceSharedKey",
					Type:  "object",
					Properties: map[string]*jsonschema.Schema{
						"kind": {Type: "string"},
					},
					Required: []string{"kind"},
				},
			},
		},
	}
	gotBytes, err := newGenerator(definitions).generate("schema.json", "protocol")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got := string(gotBytes)
	wantFragments := []string{
		"Kind jsontext.Value `json:\"kind\"`",
		"if object.Kind != nil {",
		"var discriminator string",
		"if err := json.Unmarshal(object.Kind, &discriminator); err == nil {",
		"switch discriminator {",
		"case \"alpha\":",
		"if object.Kind != nil {",
		"var value PresenceSharedKey",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, got)
		}
	}
	if strings.Contains(got, "var object struct {\n\t\tKind string `json:\"kind\"`") {
		t.Fatalf("shared discriminator/presence key must keep a raw probe field:\n%s", got)
	}
}

func TestGenerateDescriptionGodoc(t *testing.T) {
	definitions := map[string]*jsonschema.Schema{
		"Aliased": {
			Type:        "string",
			Description: "Opaque identifier referencing an upstream record.",
		},
		"Status": {
			Type:        "string",
			Enum:        []any{"ready"},
			Description: "Lifecycle state of the workflow.",
		},
		"NoDescription": {
			Type: "string",
		},
		"Sample": {
			Type:        "object",
			Description: "Sample carries demo data.\n\nIt has multiple paragraphs.",
			Required:    []string{"id"},
			Properties: map[string]*jsonschema.Schema{
				"id": {
					Type:        "string",
					Description: "Stable identifier for this record",
				},
				"plain": {
					Type: "string",
				},
			},
		},
	}
	gotBytes, err := newGenerator(definitions).generate("schema.json", "protocol")
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	got := string(gotBytes)
	wantFragments := []string{
		"// Aliased opaque identifier referencing an upstream record.\ntype Aliased = string",
		"// Status lifecycle state of the workflow.\ntype Status string",
		"// NoDescription is generated from the NoDescription schema definition.",
		"// Sample carries demo data.",
		"//\n// It has multiple paragraphs.",
		"// ID stable identifier for this record.",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, got)
		}
	}
}

func TestRewriteGodocFirstLine(t *testing.T) {
	tests := map[string]struct {
		name string
		line string
		want string
	}{
		"already godoc-prefixed line is preserved": {
			name: "Sample",
			line: "Sample carries demo data.",
			want: "Sample carries demo data.",
		},
		"leading article 'A' becomes 'represents a'": {
			name: "AbsolutePathBuf",
			line: "A path that is guaranteed to be absolute and normalized.",
			want: "AbsolutePathBuf represents a path that is guaranteed to be absolute and normalized.",
		},
		"leading article 'An' becomes 'represents an'": {
			name: "AppInfo",
			line: "An optional metadata blob.",
			want: "AppInfo represents an optional metadata blob.",
		},
		"leading article 'The' becomes 'represents a/an'": {
			name: "ServerNotification",
			line: "The notification emitted by the server.",
			want: "ServerNotification represents a notification emitted by the server.",
		},
		"adjective starter is lowercased without article": {
			name: "AppBranding",
			line: "Optional branding shown in the app picker.",
			want: "AppBranding optional branding shown in the app picker.",
		},
		"noun starter is lowercased without article": {
			name: "Status",
			line: "Lifecycle state of the workflow.",
			want: "Status lifecycle state of the workflow.",
		},
		"leading uppercase tag is stripped and body is lowercased": {
			name: "AppInfo",
			line: "EXPERIMENTAL - app metadata returned by app-list APIs.",
			want: "AppInfo app metadata returned by app-list APIs.",
		},
		"imperative verb is conjugated to third person": {
			name: "CommandExecRequest",
			line: "Execute a standalone command in the sandbox.",
			want: "CommandExecRequest executes a standalone command in the sandbox.",
		},
		"imperative verb ending in y is conjugated to ies": {
			name: "Notifier",
			line: "Notify subscribers when state changes.",
			want: "Notifier notifies subscribers when state changes.",
		},
		"third person singular verb is lowercased": {
			name: "ApprovalsReviewer",
			line: "Configures who approval requests are routed to.",
			want: "ApprovalsReviewer configures who approval requests are routed to.",
		},
		"Whether prefix becomes 'reports whether'": {
			name: "IsEnabled",
			line: "Whether this app is enabled in config.toml.",
			want: "IsEnabled reports whether this app is enabled in config.toml.",
		},
		"empty line returns just the name": {
			name: "Empty",
			line: "",
			want: "Empty",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := rewriteGodocFirstLine(tt.name, tt.line); got != tt.want {
				t.Fatalf("rewriteGodocFirstLine(%q, %q) = %q, want %q", tt.name, tt.line, got, tt.want)
			}
		})
	}
}
