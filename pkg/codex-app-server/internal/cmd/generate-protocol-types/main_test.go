package main

import (
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

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
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := exportName(tt.input); got != tt.want {
				t.Fatalf("exportName(%q) = %q, want %q", tt.input, got, tt.want)
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
		"value.Item = RawObjectUnion(raw.Item)",
		"maybeItem := ObjectUnion(RawObjectUnion(raw.MaybeItem))",
		"value.MaybeItem = &maybeItem",
		"value.Items = make([]ObjectUnion, len(raw.Items))",
		"value.Items[i] = RawObjectUnion(item)",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, got)
		}
	}
	if strings.Contains(got, "RawSingleVariantUnion") {
		t.Fatalf("single-variant aliases must not use a missing raw union wrapper:\n%s", got)
	}
}
