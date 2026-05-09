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
		"success: union inside slice stays raw json": {
			input: &jsonschema.Schema{
				Type: "array",
				Items: &jsonschema.Schema{
					OneOf: []*jsonschema.Schema{
						{Type: "string"},
						{Type: "number"},
						{Type: "boolean"},
					},
				},
			},
			want: "[]jsontext.Value",
		},
		"success: nullable nested union in slice becomes pointer element type": {
			input: &jsonschema.Schema{
				Type: "array",
				Items: &jsonschema.Schema{
					AnyOf: []*jsonschema.Schema{
						{Ref: "#/definitions/SampleStatus"},
						{Type: "null"},
					},
				},
			},
			want: "[]*SampleStatus",
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
