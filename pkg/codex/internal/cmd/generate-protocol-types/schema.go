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

package main

import (
	"slices"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

func (g *generator) resolvedSchema(def *jsonschema.Schema) *jsonschema.Schema {
	if def == nil {
		return nil
	}
	if def.Ref == "" {
		return def
	}
	name := strings.TrimPrefix(def.Ref, "#/definitions/")
	return g.definitions[name]
}

func uniqueName(name string, used map[string]int) string {
	if name == "" || name == "_" {
		name = "Value"
	}
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	used[name]++
	return name + strconv.Itoa(used[name])
}

func (g *generator) typeForSchema(def *jsonschema.Schema, optional bool) string {
	if def == nil {
		return "jsontext.Value"
	}
	if def.Ref != "" {
		return pointerIfOptional(g.refType(def.Ref), optional)
	}
	if len(def.AllOf) == 1 {
		return g.typeForSchema(def.AllOf[0], optional)
	}
	if typ, nullable := nullableType(def); typ != "" {
		copy := *def
		copy.Type = typ
		copy.Types = nil
		return g.typeForSchema(&copy, optional || nullable)
	}
	if variant, nullable := nullableVariant(def.AnyOf); variant != nil {
		return g.typeForSchema(variant, optional || nullable)
	}
	if variant, nullable := nullableVariant(def.OneOf); variant != nil {
		return g.typeForSchema(variant, optional || nullable)
	}
	if len(def.AnyOf) == 1 {
		return g.typeForSchema(def.AnyOf[0], optional)
	}
	if len(def.OneOf) == 1 {
		return g.typeForSchema(def.OneOf[0], optional)
	}
	if len(def.AnyOf) > 0 || len(def.OneOf) > 0 || len(def.AllOf) > 0 {
		return "jsontext.Value"
	}
	if def.Items != nil && includesType(def, "array") {
		return "[]" + g.typeForSchema(def.Items, false)
	}
	if includesType(def, "object") {
		if def.AdditionalProperties != nil && len(def.Properties) == 0 {
			return "map[string]" + g.typeForSchema(def.AdditionalProperties, false)
		}
		return "jsontext.Value"
	}
	switch {
	case includesType(def, "string"):
		return pointerIfOptional("string", optional)
	case includesType(def, "boolean"):
		return pointerIfOptional("bool", optional)
	case includesType(def, "integer"):
		return pointerIfOptional(integerType(def.Format), optional)
	case includesType(def, "number"):
		return pointerIfOptional("float64", optional)
	default:
		return "jsontext.Value"
	}
}

func unionDiscriminatorProperty(def *jsonschema.Schema) (string, bool) {
	var discovered string
	for _, name := range def.Required {
		property, ok := def.Properties[name]
		if !ok {
			continue
		}
		enum := stringEnum(property)
		if len(enum) != 1 {
			continue
		}
		if discovered == "" {
			discovered = name
			continue
		}
		if discovered != name {
			return "", false
		}
	}
	if discovered == "" {
		return "", false
	}
	return discovered, true
}

func (g *generator) refType(ref string) string {
	name := strings.TrimPrefix(ref, "#/definitions/")
	if alias, ok := g.aliases[name]; ok {
		return alias
	}
	return g.goTypeName(name)
}

func (g *generator) goTypeName(name string) string {
	if renamed, ok := g.typeNames[name]; ok {
		return renamed
	}
	return exportName(name)
}

func pointerIfOptional(name string, optional bool) string {
	if !optional || strings.HasPrefix(name, "[]") || strings.HasPrefix(name, "map[") || name == "any" || name == "jsontext.Value" {
		return name
	}
	if strings.HasPrefix(name, "*") {
		return name
	}
	return "*" + name
}

func integerType(format string) string {
	switch format {
	case "int32", "uint32":
		return strings.TrimPrefix(format, "u")
	case "uint", "uint64":
		return format
	default:
		return "int64"
	}
}

func objectSchema(def *jsonschema.Schema) bool {
	return def != nil && includesType(def, "object") && len(def.Properties) > 0
}

func emptyObjectSchema(def *jsonschema.Schema) bool {
	return def != nil && includesType(def, "object") && len(def.Properties) == 0 && def.AdditionalProperties == nil
}

func objectLikeSchema(def *jsonschema.Schema) bool {
	return def != nil && includesType(def, "object") && (len(def.Properties) > 0 || def.AdditionalProperties != nil)
}

func nullableVariant(variants []*jsonschema.Schema) (*jsonschema.Schema, bool) {
	if len(variants) != 2 {
		return nil, false
	}
	var nonNull *jsonschema.Schema
	nullCount := 0
	for _, variant := range variants {
		if variant != nil && includesType(variant, "null") && variant.Ref == "" && len(variant.Properties) == 0 {
			nullCount++
			continue
		}
		nonNull = variant
	}
	if nullCount == 1 && nonNull != nil {
		return nonNull, true
	}
	return nil, false
}

func nullableType(def *jsonschema.Schema) (string, bool) {
	values := typeStrings(def)
	if !slices.Contains(values, "null") || len(values) != 2 {
		return "", false
	}
	for _, value := range values {
		if value != "null" {
			return value, true
		}
	}
	return "", false
}

func includesType(def *jsonschema.Schema, target string) bool {
	return slices.Contains(typeStrings(def), target)
}

func typeStrings(def *jsonschema.Schema) []string {
	if def == nil {
		return nil
	}
	if def.Type != "" {
		return []string{def.Type}
	}
	return def.Types
}

func stringEnum(def *jsonschema.Schema) []string {
	if def == nil || len(def.Enum) == 0 {
		return nil
	}
	out := make([]string, 0, len(def.Enum))
	for _, value := range def.Enum {
		text, ok := value.(string)
		if !ok {
			return nil
		}
		out = append(out, text)
	}
	return out
}
