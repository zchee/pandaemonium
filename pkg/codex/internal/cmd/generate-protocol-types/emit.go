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
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

type structField struct {
	name       string
	typ        string
	tag        string
	unionName  string
	unionShape unionShape
}

type unionShape int

const (
	unionShapeNone unionShape = iota
	unionShapeSingle
	unionShapeOptionalSingle
	unionShapeSlice
)

func (g *generator) emitDefinition(out *bytes.Buffer, name string, def *jsonschema.Schema) error {
	if def == nil {
		return fmt.Errorf("nil schema")
	}
	goName := g.goTypeName(name)
	if alias, ok := g.aliases[name]; ok {
		writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from the %s schema definition.", goName, name))
		fmt.Fprintf(out, "type %s = %s\n", goName, alias)
		return nil
	}
	if _, forcedRaw := g.rawUnions[name]; !forcedRaw {
		if _, ok := g.interfaceUnionForSchema(goName, def); ok {
			return g.emitUnionDefinition(out, goName, def)
		}
	}
	if enum := stringEnum(def); len(enum) > 0 && includesType(def, "string") {
		writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from the %s schema definition.", goName, name))
		fmt.Fprintf(out, "type %s string\n\n", goName)
		fmt.Fprintf(out, "const (\n")
		for _, value := range enum {
			constName := goName + exportName(value)
			fmt.Fprintf(out, "\t// %s is the %q %s value.\n", constName, value, goName)
			fmt.Fprintf(out, "\t%s %s = %q\n", constName, goName, value)
		}
		fmt.Fprintf(out, ")\n")
		return nil
	}
	if emptyObjectSchema(def) {
		writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from the %s schema definition.", goName, name))
		fmt.Fprintf(out, "type %s struct{}\n", goName)
		return nil
	}
	if objectSchema(def) {
		return g.emitStruct(out, goName, name, def)
	}
	goType := g.typeForSchema(def, false)
	writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from the %s schema definition.", goName, name))
	if goType == "jsontext.Value" {
		fmt.Fprintf(out, "type %s jsontext.Value\n", goName)
		fmt.Fprintf(out, "\n")
		fmt.Fprintf(out, "var _ json.MarshalerTo = %s{}\n", goName)
		fmt.Fprintf(out, "var _ json.UnmarshalerFrom = (*%s)(nil)\n\n", goName)
		fmt.Fprintf(out, "func (value %s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", goName)
		fmt.Fprintf(out, "\treturn enc.WriteValue(jsontext.Value(value))\n")
		fmt.Fprintf(out, "}\n\n")
		fmt.Fprintf(out, "func (value *%s) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n", goName)
		fmt.Fprintf(out, "\treturn json.UnmarshalDecode(dec, (*jsontext.Value)(value))\n")
		fmt.Fprintf(out, "}\n")
		return nil
	}
	fmt.Fprintf(out, "type %s = %s\n", goName, goType)
	return nil
}

func (g *generator) shouldEmitRawCodecWrapper(name string, def *jsonschema.Schema) bool {
	if def == nil {
		return false
	}
	if _, ok := g.aliases[name]; ok {
		return false
	}
	if _, forcedRaw := g.rawUnions[name]; forcedRaw {
		return true
	}
	if _, ok := g.interfaceUnionForSchema(g.goTypeName(name), def); ok {
		return false
	}
	if len(stringEnum(def)) > 0 && includesType(def, "string") {
		return false
	}
	if emptyObjectSchema(def) {
		return false
	}
	if objectSchema(def) {
		return false
	}
	return g.typeForSchema(def, false) == "jsontext.Value"
}

func (g *generator) emitStruct(out *bytes.Buffer, goName, schemaName string, def *jsonschema.Schema) error {
	if _, ok := g.emittedType[goName]; ok {
		return nil
	}
	g.emittedType[goName] = struct{}{}
	properties := make([]string, 0, len(def.Properties))
	for name := range def.Properties {
		properties = append(properties, name)
	}
	slices.Sort(properties)
	required := make(map[string]bool, len(def.Required))
	for _, name := range def.Required {
		required[name] = true
	}
	namedInlineFields := g.namedInlineObjectFields(goName, def, properties, required)
	for jsonName, typeName := range namedInlineFields {
		if err := g.emitStruct(out, typeName, typeName, def.Properties[jsonName]); err != nil {
			return err
		}
		out.WriteByte('\n')
	}
	writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from the %s schema definition.", goName, schemaName))
	fmt.Fprintf(out, "type %s struct {\n", goName)
	used := map[string]int{}
	fields := make([]structField, 0, len(properties))
	for index, jsonName := range properties {
		fieldName := uniqueName(goFieldName(jsonName), used)
		fieldType := g.typeForSchema(def.Properties[jsonName], !required[jsonName])
		if namedType, ok := namedInlineFields[jsonName]; ok {
			fieldType = pointerIfOptional(namedType, !required[jsonName])
		}
		tag := jsonName
		if !required[jsonName] {
			tag += ",omitzero"
		}
		unionName, shape := g.unionFieldShape(def.Properties[jsonName], !required[jsonName])
		fields = append(fields, structField{
			name:       fieldName,
			typ:        fieldType,
			tag:        tag,
			unionName:  unionName,
			unionShape: shape,
		})
		fieldDesc := ""
		if property := def.Properties[jsonName]; property != nil {
			fieldDesc = property.Description
		}
		if fieldDesc != "" {
			if index > 0 {
				out.WriteByte('\n')
			}
			writeGodoc(out, "\t", fieldName, fieldDesc, "")
		}
		fmt.Fprintf(out, "\t%s %s `json:%q`\n", fieldName, fieldType, tag)
	}
	fmt.Fprintf(out, "}\n")
	if structFieldsNeedCustomUnmarshal(fields) {
		out.WriteByte('\n')
		g.emitStructCustomUnmarshal(out, goName, fields)
	}
	return nil
}

func (g *generator) namedInlineObjectFields(parentGoName string, def *jsonschema.Schema, properties []string, required map[string]bool) map[string]string {
	if !g.shouldNameInlineObjectFields(parentGoName, def, properties, required) {
		return nil
	}
	used := g.reservedDefinitionTypeNames()
	named := make(map[string]string)
	for _, jsonName := range properties {
		property := def.Properties[jsonName]
		if !required[jsonName] || !nameableInlineObjectSchema(property) {
			continue
		}
		named[jsonName] = uniqueName(goFieldName(jsonName), used)
	}
	return named
}

func (g *generator) shouldNameInlineObjectFields(parentGoName string, def *jsonschema.Schema, properties []string, required map[string]bool) bool {
	if def == nil || len(properties) != 1 || len(required) != 1 {
		return false
	}
	if g.isDefinitionType(parentGoName) {
		return false
	}
	if _, ok := g.interfaceUnionForSchema(parentGoName, def); ok {
		return false
	}
	matchKey, matchValue := g.unionVariantMatch(def)
	return matchKey == properties[0] && matchValue == ""
}

func (g *generator) isDefinitionType(goName string) bool {
	for name := range g.definitions {
		if g.goTypeName(name) == goName {
			return true
		}
	}
	return false
}

func nameableInlineObjectSchema(def *jsonschema.Schema) bool {
	if def == nil || def.Ref != "" || def.Title != "" {
		return false
	}
	if !objectSchema(def) || len(def.Properties) == 0 {
		return false
	}
	return def.AdditionalProperties == nil
}

func (g *generator) structNeedsCustomUnmarshal(def *jsonschema.Schema) bool {
	if def == nil || !objectSchema(def) {
		return false
	}
	required := make(map[string]bool, len(def.Required))
	for _, name := range def.Required {
		required[name] = true
	}
	for name, property := range def.Properties {
		if _, shape := g.unionFieldShape(property, !required[name]); shape != unionShapeNone {
			return true
		}
	}
	return false
}

func structFieldsNeedCustomUnmarshal(fields []structField) bool {
	for _, field := range fields {
		if field.unionShape != unionShapeNone {
			return true
		}
	}
	return false
}

func (g *generator) emitStructCustomUnmarshal(out *bytes.Buffer, goName string, fields []structField) {
	fmt.Fprintf(out, "func (value *%s) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n", goName)
	fmt.Fprintf(out, "\tvar raw struct {\n")
	for _, field := range fields {
		rawType := field.typ
		switch field.unionShape {
		case unionShapeSingle, unionShapeOptionalSingle:
			rawType = "jsontext.Value"
		case unionShapeSlice:
			rawType = "[]jsontext.Value"
		}
		fmt.Fprintf(out, "\t\t%s %s `json:%q`\n", field.name, rawType, field.tag)
	}
	fmt.Fprintf(out, "\t}\n")
	fmt.Fprintf(out, "\tif err := json.UnmarshalDecode(dec, &raw); err != nil {\n")
	fmt.Fprintf(out, "\t\treturn err\n")
	fmt.Fprintf(out, "\t}\n")
	for _, field := range fields {
		switch field.unionShape {
		case unionShapeSingle:
			fmt.Fprintf(out, "\tif raw.%s == nil {\n", field.name)
			fmt.Fprintf(out, "\t\tvalue.%s = nil\n", field.name)
			fmt.Fprintf(out, "\t} else {\n")
			if _, rawDecode := g.rawDecodes[field.unionName]; rawDecode {
				fmt.Fprintf(out, "\t\tvalue.%s = Raw%s(raw.%s)\n", field.name, field.unionName, field.name)
				fmt.Fprintf(out, "\t}\n")
				continue
			}
			decodedName := "decoded" + field.name
			fmt.Fprintf(out, "\t\t%s, err := decodeGenerated%s(raw.%s)\n", decodedName, field.unionName, field.name)
			fmt.Fprintf(out, "\t\tif err != nil {\n")
			fmt.Fprintf(out, "\t\t\treturn err\n")
			fmt.Fprintf(out, "\t\t}\n")
			fmt.Fprintf(out, "\t\tvalue.%s = %s\n", field.name, decodedName)
			fmt.Fprintf(out, "\t}\n")
		case unionShapeOptionalSingle:
			fmt.Fprintf(out, "\tif raw.%s == nil {\n", field.name)
			fmt.Fprintf(out, "\t\tvalue.%s = nil\n", field.name)
			fmt.Fprintf(out, "\t} else {\n")
			if _, rawDecode := g.rawDecodes[field.unionName]; rawDecode {
				fmt.Fprintf(out, "\t\t%s := %s(Raw%s(raw.%s))\n", unexportName(field.name), field.unionName, field.unionName, field.name)
				fmt.Fprintf(out, "\t\tvalue.%s = &%s\n", field.name, unexportName(field.name))
				fmt.Fprintf(out, "\t}\n")
				continue
			}
			decodedName := "decoded" + field.name
			fmt.Fprintf(out, "\t\t%s, err := decodeGenerated%s(raw.%s)\n", decodedName, field.unionName, field.name)
			fmt.Fprintf(out, "\t\tif err != nil {\n")
			fmt.Fprintf(out, "\t\t\treturn err\n")
			fmt.Fprintf(out, "\t\t}\n")
			fmt.Fprintf(out, "\t\tvalue.%s = &%s\n", field.name, decodedName)
			fmt.Fprintf(out, "\t}\n")
		case unionShapeSlice:
			fmt.Fprintf(out, "\tif raw.%s == nil {\n", field.name)
			fmt.Fprintf(out, "\t\tvalue.%s = nil\n", field.name)
			fmt.Fprintf(out, "\t} else {\n")
			fmt.Fprintf(out, "\t\tvalue.%s = make([]%s, len(raw.%s))\n", field.name, field.unionName, field.name)
			fmt.Fprintf(out, "\t\tfor i, item := range raw.%s {\n", field.name)
			fmt.Fprintf(out, "\t\t\tif item != nil {\n")
			if _, rawDecode := g.rawDecodes[field.unionName]; rawDecode {
				fmt.Fprintf(out, "\t\t\t\tvalue.%s[i] = Raw%s(item)\n", field.name, field.unionName)
				fmt.Fprintf(out, "\t\t\t}\n")
				fmt.Fprintf(out, "\t\t}\n")
				fmt.Fprintf(out, "\t}\n")
				continue
			}
			decodedName := "decoded" + field.name
			fmt.Fprintf(out, "\t\t\t\t%s, err := decodeGenerated%s(item)\n", decodedName, field.unionName)
			fmt.Fprintf(out, "\t\t\t\tif err != nil {\n")
			fmt.Fprintf(out, "\t\t\t\t\treturn err\n")
			fmt.Fprintf(out, "\t\t\t\t}\n")
			fmt.Fprintf(out, "\t\t\t\tvalue.%s[i] = %s\n", field.name, decodedName)
			fmt.Fprintf(out, "\t\t\t}\n")
			fmt.Fprintf(out, "\t\t}\n")
			fmt.Fprintf(out, "\t}\n")
		default:
			fmt.Fprintf(out, "\tvalue.%s = raw.%s\n", field.name, field.name)
		}
	}
	fmt.Fprintf(out, "\treturn nil\n")
	fmt.Fprintf(out, "}\n")
}

func (g *generator) unionFieldShape(def *jsonschema.Schema, optional bool) (string, unionShape) {
	unionName, ok := g.unionRefName(def)
	if ok {
		if optional {
			return unionName, unionShapeOptionalSingle
		}
		return unionName, unionShapeSingle
	}
	if def == nil {
		return "", unionShapeNone
	}
	if def.Items != nil && includesType(def, "array") {
		unionName, ok := g.unionRefName(def.Items)
		if ok {
			return unionName, unionShapeSlice
		}
	}
	return "", unionShapeNone
}

func (g *generator) unionRefName(def *jsonschema.Schema) (string, bool) {
	if def == nil {
		return "", false
	}
	if def.Ref != "" {
		name := strings.TrimPrefix(def.Ref, "#/definitions/")
		if _, forcedRaw := g.rawUnions[name]; forcedRaw {
			return "", false
		}
		if _, ok := g.interfaceUnionForSchema(g.goTypeName(name), g.definitions[name]); ok && g.emitsInterfaceUnionName(g.goTypeName(name), g.definitions[name]) {
			return g.goTypeName(name), true
		}
		return "", false
	}
	if len(def.AllOf) == 1 {
		return g.unionRefName(def.AllOf[0])
	}
	if typ, nullable := nullableType(def); typ != "" {
		copy := *def
		copy.Type = typ
		copy.Types = nil
		if nullable {
			return g.unionRefName(&copy)
		}
	}
	if variant, _ := nullableVariant(def.AnyOf); variant != nil {
		return g.unionRefName(variant)
	}
	if variant, _ := nullableVariant(def.OneOf); variant != nil {
		return g.unionRefName(variant)
	}
	if len(def.AnyOf) == 1 {
		return g.unionRefName(def.AnyOf[0])
	}
	if len(def.OneOf) == 1 {
		return g.unionRefName(def.OneOf[0])
	}
	return "", false
}

func (g *generator) emitsInterfaceUnionName(unionName string, def *jsonschema.Schema) bool {
	info, ok := g.interfaceUnionForSchema(unionName, def)
	if !ok {
		return false
	}
	return len(info.rawVariants) > 1 || len(info.variants) > 1
}
