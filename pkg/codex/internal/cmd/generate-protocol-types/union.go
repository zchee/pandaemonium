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
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
)

type unionVariantKind int

const (
	unionVariantObject unionVariantKind = iota
	unionVariantStringEnum
	unionVariantString
	unionVariantArray
)

type unionVariant struct {
	raw          *jsonschema.Schema
	schema       *jsonschema.Schema
	typeName     string
	kind         unionVariantKind
	goType       string
	stringValues []string
	matchKey     string
	matchValue   string
}

type interfaceUnion struct {
	rawVariants []*jsonschema.Schema
	variants    []unionVariant
}

type unionTaggerMethod struct {
	unionName  string
	targetType string
}

func (g *generator) emitUnionDefinition(out *bytes.Buffer, goName string, def *jsonschema.Schema) error {
	info, ok := g.interfaceUnionForSchema(goName, def)
	if !ok || len(info.variants) == 0 {
		return nil
	}

	// Single-variant object unions are simple aliases of that variant.
	if len(info.rawVariants) == 1 && len(info.variants) == 1 && info.variants[0].kind == unionVariantObject {
		variant := info.variants[0]
		variantType := variant.typeName
		variantSchema := variant.schema
		if variantSchema == nil {
			return nil
		}
		if _, ok := g.definitions[variantType]; !ok && variant.raw.Ref == "" {
			commentName := variant.raw.Title
			if commentName == "" {
				commentName = variantType
			}
			if err := g.emitStruct(out, variantType, commentName, variantSchema); err != nil {
				return err
			}
		}
		writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from the %s schema definition.", goName, goName))
		fmt.Fprintf(out, "type %s = %s\n\n", goName, variantType)
		g.emitUnionTagger(out, goName, variantType)
		return nil
	}
	writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from the %s schema definition.", goName, goName))
	fmt.Fprintf(out, "type %s interface {\n\tis%s()\n}\n\n", goName, goName)
	g.emitRawUnionWrapper(out, goName)
	methodConstants := g.emitMethodConstants(out, goName, info)

	for _, variant := range info.variants {
		switch variant.kind {
		case unionVariantStringEnum:
			g.emitStringEnumUnionVariant(out, goName, variant)
		case unionVariantString, unionVariantArray:
			g.emitTypedUnionVariant(out, goName, variant)
		case unionVariantObject:
			if variant.schema != nil && objectLikeSchema(variant.schema) {
				_, schemaTitleDefined := g.definitions[variant.schema.Title]
				_, typeNameDefined := g.definitions[variant.typeName]
				if !schemaTitleDefined && !typeNameDefined && variant.raw.Ref == "" {
					commentName := variant.raw.Title
					if commentName == "" {
						commentName = variant.typeName
					}
					if err := g.emitStruct(out, variant.typeName, commentName, variant.schema); err != nil {
						return err
					}
					out.WriteByte('\n')
				}
			}
			g.emitObjectUnionTagger(out, goName, variant.typeName)
		}
	}
	g.emitUnionDecodeHelper(out, goName, info, methodConstants)
	return nil
}

type methodConstantConfig struct {
	prefix    string
	unionName string
}

func methodConstantConfigForUnion(unionName string) (methodConstantConfig, bool) {
	switch unionName {
	case "ClientRequest":
		return methodConstantConfig{prefix: "RequestMethod", unionName: unionName}, true
	case "ServerNotification":
		return methodConstantConfig{prefix: "NotificationMethod", unionName: unionName}, true
	default:
		return methodConstantConfig{}, false
	}
}

func (g *generator) emitMethodConstants(out *bytes.Buffer, unionName string, info *interfaceUnion) map[string]string {
	config, ok := methodConstantConfigForUnion(unionName)
	if !ok {
		return nil
	}

	type methodConstant struct {
		name  string
		value string
	}

	used := map[string]int{}
	constants := make([]methodConstant, 0, len(info.variants))
	byVariant := make(map[string]string, len(info.variants))
	for _, variant := range info.variants {
		if variant.kind != unionVariantObject || variant.matchKey != "method" || variant.matchValue == "" {
			continue
		}
		constName := uniqueName(config.prefix+exportName(variant.matchValue), used)
		constants = append(constants, methodConstant{
			name:  constName,
			value: variant.matchValue,
		})
		byVariant[variant.typeName] = constName
	}
	if len(constants) == 0 {
		return nil
	}

	fmt.Fprintf(out, "const (\n")
	for _, constant := range constants {
		fmt.Fprintf(out, "\t// %s is the %q %s method.\n", constant.name, constant.value, config.unionName)
		fmt.Fprintf(out, "\t%s = %q\n", constant.name, constant.value)
	}
	fmt.Fprintf(out, ")\n\n")
	return byVariant
}

func (g *generator) emitStringEnumUnionVariant(out *bytes.Buffer, unionName string, variant unionVariant) {
	writeGodoc(out, "", variant.typeName, "", fmt.Sprintf("%s is a string-valued %s variant.", variant.typeName, unionName))
	fmt.Fprintf(out, "type %s string\n\n", variant.typeName)
	g.emitUnionTagger(out, unionName, variant.typeName)
	fmt.Fprintf(out, "const (\n")
	for _, value := range variant.stringValues {
		constName := variant.typeName + exportName(value)
		fmt.Fprintf(out, "\t// %s is the %q %s value.\n", constName, value, variant.typeName)
		fmt.Fprintf(out, "\t%s %s = %q\n", constName, variant.typeName, value)
	}
	fmt.Fprintf(out, ")\n\n")
}

func (g *generator) emitTypedUnionVariant(out *bytes.Buffer, unionName string, variant unionVariant) {
	writeGodoc(out, "", variant.typeName, "", fmt.Sprintf("%s is a %s variant.", variant.typeName, unionName))
	fmt.Fprintf(out, "type %s %s\n\n", variant.typeName, variant.goType)
	g.emitUnionTagger(out, unionName, variant.typeName)
}

func (g *generator) emitUnionDecodeHelper(out *bytes.Buffer, unionName string, info *interfaceUnion, methodConstants map[string]string) {
	fmt.Fprintf(out, "func decodeGenerated%s(raw jsontext.Value) (%s, error) {\n", unionName, unionName)
	fmt.Fprintf(out, "\tif raw == nil {\n")
	fmt.Fprintf(out, "\t\treturn nil, nil\n")
	fmt.Fprintf(out, "\t}\n")
	g.emitScalarUnionDecodeBranches(out, info)
	for _, variant := range info.variants {
		if variant.kind != unionVariantStringEnum {
			continue
		}
		fmt.Fprintf(out, "\tvar text string\n")
		fmt.Fprintf(out, "\tif err := json.Unmarshal(raw, &text); err == nil {\n")
		fmt.Fprintf(out, "\t\tswitch text {\n")
		for _, value := range variant.stringValues {
			fmt.Fprintf(out, "\t\tcase %q:\n", value)
			fmt.Fprintf(out, "\t\t\treturn %s(text), nil\n", variant.typeName)
		}
		fmt.Fprintf(out, "\t\t}\n")
		fmt.Fprintf(out, "\t\treturn Raw%s(raw), nil\n", unionName)
		fmt.Fprintf(out, "\t}\n")
		break
	}
	probeFields := unionObjectProbeFields(info.variants)
	if len(probeFields) == 0 {
		fmt.Fprintf(out, "\treturn Raw%s(raw), nil\n", unionName)
		fmt.Fprintf(out, "}\n\n")
		return
	}
	probeFieldNames := map[string]string{}
	probeFieldTypes := map[string]string{}
	fmt.Fprintf(out, "\tvar object struct {\n")
	for _, field := range probeFields {
		probeFieldNames[field.key] = field.name
		probeFieldTypes[field.key] = field.typ
		fmt.Fprintf(out, "\t\t%s %s `json:%q`\n", field.name, field.typ, field.key)
	}
	fmt.Fprintf(out, "\t}\n")
	fmt.Fprintf(out, "\tif err := json.Unmarshal(raw, &object); err == nil {\n")
	discriminatorKeys, discriminatorVariants := unionDiscriminatorVariantGroups(info.variants)
	for _, matchKey := range discriminatorKeys {
		fieldName := probeFieldNames[matchKey]
		if probeFieldTypes[matchKey] == "string" {
			fmt.Fprintf(out, "\t\tswitch object.%s {\n", fieldName)
			for _, variant := range discriminatorVariants[matchKey] {
				fmt.Fprintf(out, "\t\tcase %s:\n", unionObjectVariantMatchLabel(variant, methodConstants))
				emitUnionObjectVariantReturn(out, "\t\t\t", variant.typeName)
			}
			fmt.Fprintf(out, "\t\t}\n")
		} else {
			fmt.Fprintf(out, "\t\tif object.%s != nil {\n", fieldName)
			fmt.Fprintf(out, "\t\t\tvar discriminator string\n")
			fmt.Fprintf(out, "\t\t\tif err := json.Unmarshal(object.%s, &discriminator); err == nil {\n", fieldName)
			fmt.Fprintf(out, "\t\t\t\tswitch discriminator {\n")
			for _, variant := range discriminatorVariants[matchKey] {
				fmt.Fprintf(out, "\t\t\t\tcase %s:\n", unionObjectVariantMatchLabel(variant, methodConstants))
				emitUnionObjectVariantReturn(out, "\t\t\t\t\t", variant.typeName)
			}
			fmt.Fprintf(out, "\t\t\t\t}\n")
			fmt.Fprintf(out, "\t\t\t}\n")
			fmt.Fprintf(out, "\t\t}\n")
		}
	}
	for _, variant := range info.variants {
		if variant.kind != unionVariantObject || variant.matchKey == "" || variant.matchValue != "" {
			continue
		}
		fieldName := probeFieldNames[variant.matchKey]
		fmt.Fprintf(out, "\t\tif object.%s != nil {\n", fieldName)
		emitUnionObjectVariantReturn(out, "\t\t\t", variant.typeName)
		fmt.Fprintf(out, "\t\t}\n")
	}
	fmt.Fprintf(out, "\t\treturn Raw%s(raw), nil\n", unionName)
	fmt.Fprintf(out, "\t}\n")
	fmt.Fprintf(out, "\treturn Raw%s(raw), nil\n", unionName)
	fmt.Fprintf(out, "}\n\n")
}

func (g *generator) emitScalarUnionDecodeBranches(out *bytes.Buffer, info *interfaceUnion) {
	for _, variant := range info.variants {
		switch variant.kind {
		case unionVariantString:
			fmt.Fprintf(out, "\tvar text string\n")
			fmt.Fprintf(out, "\tif err := json.Unmarshal(raw, &text); err == nil {\n")
			fmt.Fprintf(out, "\t\treturn %s(text), nil\n", variant.typeName)
			fmt.Fprintf(out, "\t}\n")
		case unionVariantArray:
			g.emitArrayUnionDecodeBranch(out, variant)
		}
	}
}

func (g *generator) emitArrayUnionDecodeBranch(out *bytes.Buffer, variant unionVariant) {
	itemType, ok := g.arrayVariantItemType(variant.schema)
	if !ok {
		fmt.Fprintf(out, "\tvar value %s\n", variant.typeName)
		fmt.Fprintf(out, "\tif err := json.Unmarshal(raw, &value); err == nil {\n")
		fmt.Fprintf(out, "\t\treturn value, nil\n")
		fmt.Fprintf(out, "\t}\n")
		return
	}
	fmt.Fprintf(out, "\tvar rawItems []jsontext.Value\n")
	fmt.Fprintf(out, "\tif err := json.Unmarshal(raw, &rawItems); err == nil {\n")
	fmt.Fprintf(out, "\t\tvalue := make(%s, len(rawItems))\n", variant.typeName)
	fmt.Fprintf(out, "\t\tfor i, item := range rawItems {\n")
	fmt.Fprintf(out, "\t\t\tif item == nil {\n")
	fmt.Fprintf(out, "\t\t\t\tcontinue\n")
	fmt.Fprintf(out, "\t\t\t}\n")
	fmt.Fprintf(out, "\t\t\tdecodedItem, err := decodeGenerated%s(item)\n", itemType)
	fmt.Fprintf(out, "\t\t\tif err != nil {\n")
	fmt.Fprintf(out, "\t\t\t\treturn nil, err\n")
	fmt.Fprintf(out, "\t\t\t}\n")
	fmt.Fprintf(out, "\t\t\tvalue[i] = decodedItem\n")
	fmt.Fprintf(out, "\t\t}\n")
	fmt.Fprintf(out, "\t\treturn value, nil\n")
	fmt.Fprintf(out, "\t}\n")
}

func (g *generator) arrayVariantItemType(def *jsonschema.Schema) (string, bool) {
	if def == nil || def.Items == nil {
		return "", false
	}
	itemType, ok := g.unionRefName(def.Items)
	if !ok {
		return "", false
	}
	return itemType, true
}

func unionObjectVariantMatchLabel(variant unionVariant, methodConstants map[string]string) string {
	if variant.matchKey == "method" && methodConstants != nil {
		if constantName, ok := methodConstants[variant.typeName]; ok {
			return constantName
		}
	}
	return strconv.Quote(variant.matchValue)
}

type unionObjectProbeField struct {
	key  string
	name string
	typ  string
}

func unionObjectProbeFields(variants []unionVariant) []unionObjectProbeField {
	type fieldShape struct {
		hasDiscriminator bool
		hasPresence      bool
	}
	used := map[string]int{}
	seen := map[string]fieldShape{}
	keys := make([]string, 0)
	for _, variant := range variants {
		if variant.kind != unionVariantObject || variant.matchKey == "" {
			continue
		}
		shape, ok := seen[variant.matchKey]
		if !ok {
			keys = append(keys, variant.matchKey)
		}
		if variant.matchValue == "" {
			shape.hasPresence = true
		} else {
			shape.hasDiscriminator = true
		}
		seen[variant.matchKey] = shape
	}
	fields := make([]unionObjectProbeField, 0)
	for _, key := range keys {
		shape := seen[key]
		field := unionObjectProbeField{
			key:  key,
			name: uniqueName(goFieldName(key), used),
			typ:  "jsontext.Value",
		}
		if shape.hasDiscriminator && !shape.hasPresence {
			field.typ = "string"
		}
		fields = append(fields, field)
	}
	return fields
}

func unionDiscriminatorVariantGroups(variants []unionVariant) ([]string, map[string][]unionVariant) {
	keys := make([]string, 0)
	groups := make(map[string][]unionVariant)
	for _, variant := range variants {
		if variant.kind != unionVariantObject || variant.matchKey == "" || variant.matchValue == "" {
			continue
		}
		if len(groups[variant.matchKey]) == 0 {
			keys = append(keys, variant.matchKey)
		}
		groups[variant.matchKey] = append(groups[variant.matchKey], variant)
	}
	return keys, groups
}

func emitUnionObjectVariantReturn(out *bytes.Buffer, indent, typeName string) {
	fmt.Fprintf(out, "%svar value %s\n", indent, typeName)
	fmt.Fprintf(out, "%sif err := json.Unmarshal(raw, &value); err != nil {\n", indent)
	fmt.Fprintf(out, "%s\treturn nil, err\n", indent)
	fmt.Fprintf(out, "%s}\n", indent)
	fmt.Fprintf(out, "%sreturn value, nil\n", indent)
}

func (g *generator) emitRawUnionWrapper(out *bytes.Buffer, unionName string) {
	rawName := "Raw" + unionName
	fmt.Fprintf(out, "// %s preserves an uninterpreted %s JSON value.\n", rawName, unionName)
	fmt.Fprintf(out, "type %s jsontext.Value\n\n", rawName)
	g.emitUnionTagger(out, unionName, rawName)
	fmt.Fprintf(out, "var _ json.MarshalerTo = %s{}\n", rawName)
	fmt.Fprintf(out, "var _ json.UnmarshalerFrom = (*%s)(nil)\n\n", rawName)
	fmt.Fprintf(out, "func (value %s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", rawName)
	fmt.Fprintf(out, "\treturn enc.WriteValue(jsontext.Value(value))\n")
	fmt.Fprintf(out, "}\n\n")
	fmt.Fprintf(out, "func (value *%s) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n", rawName)
	fmt.Fprintf(out, "\treturn json.UnmarshalDecode(dec, (*jsontext.Value)(value))\n")
	fmt.Fprintf(out, "}\n\n")
}

func (g *generator) emitObjectUnionTagger(out *bytes.Buffer, unionName, targetType string) {
	if _, emitted := g.emittedType[targetType]; emitted {
		g.emitUnionTagger(out, unionName, targetType)
		return
	}
	g.deferUnionTagger(unionName, targetType)
}

func (g *generator) deferUnionTagger(unionName, targetType string) {
	if targetType == "" {
		return
	}
	key := unionTaggerKey(unionName, targetType)
	if _, ok := g.unionTagger[key]; ok {
		return
	}
	g.unionTagger[key] = struct{}{}
	g.pendingUnionTaggers = append(g.pendingUnionTaggers, unionTaggerMethod{
		unionName:  unionName,
		targetType: targetType,
	})
}

func (g *generator) emitPendingUnionTaggers(out *bytes.Buffer) {
	for _, tagger := range g.pendingUnionTaggers {
		writeUnionTagger(out, tagger.unionName, tagger.targetType)
	}
}

func (g *generator) emitUnionTagger(out *bytes.Buffer, unionName, targetType string) {
	if targetType == "" {
		return
	}
	key := unionTaggerKey(unionName, targetType)
	if _, ok := g.unionTagger[key]; ok {
		return
	}
	g.unionTagger[key] = struct{}{}
	writeUnionTagger(out, unionName, targetType)
}

func unionTaggerKey(unionName, targetType string) string {
	return unionName + "\x00" + targetType
}

func writeUnionTagger(out *bytes.Buffer, unionName, targetType string) {
	fmt.Fprintf(out, "func (%s) is%s() {}\n\n", targetType, unionName)
}

func (g *generator) reservedDefinitionTypeNames() map[string]int {
	used := make(map[string]int, len(g.definitions))
	for name := range g.definitions {
		used[g.goTypeName(name)] = 1
	}
	return used
}

func (g *generator) unionVariantTypeName(unionName string, variant *jsonschema.Schema, used map[string]int) string {
	if variant == nil {
		return ""
	}
	if variant.Ref != "" {
		return g.refType(variant.Ref)
	}
	if variant.Title != "" {
		if _, ok := g.definitions[variant.Title]; ok {
			return g.goTypeName(variant.Title)
		}
	}
	name := variant.Title
	if name == "" {
		if len(variant.Required) > 0 {
			name = unionName + exportName(variant.Required[0])
		} else {
			name = unionName + "Variant"
		}
	}
	name = exportName(name)
	return uniqueName(name, used)
}

func (g *generator) interfaceUnionForSchema(unionName string, def *jsonschema.Schema) (*interfaceUnion, bool) {
	if def == nil {
		return nil, false
	}
	var rawVariants []*jsonschema.Schema
	if len(def.AnyOf) > 0 {
		rawVariants = def.AnyOf
	} else if len(def.OneOf) > 0 {
		rawVariants = def.OneOf
	} else {
		return nil, false
	}
	if len(rawVariants) == 0 {
		return nil, false
	}
	if len(rawVariants) == 2 {
		if _, nullable := nullableVariant(rawVariants); nullable {
			return nil, false
		}
	}
	if unionName == "" {
		unionName = "Union"
	}
	info := &interfaceUnion{rawVariants: rawVariants}
	used := g.reservedDefinitionTypeNames()
	stringValues := make([]string, 0)
	hasStringEnum := false
	for _, rawVariant := range rawVariants {
		if rawVariant == nil {
			return nil, false
		}
		resolved := g.resolvedSchema(rawVariant)
		if enum := stringEnum(resolved); len(enum) > 0 && includesType(resolved, "string") {
			hasStringEnum = true
			stringValues = append(stringValues, enum...)
			continue
		}
		if includesType(resolved, "string") {
			info.variants = append(info.variants, unionVariant{
				raw:      rawVariant,
				schema:   resolved,
				typeName: uniqueName(unionName+"String", used),
				kind:     unionVariantString,
				goType:   "string",
			})
			continue
		}
		if includesType(resolved, "array") && resolved.Items != nil {
			info.variants = append(info.variants, unionVariant{
				raw:      rawVariant,
				schema:   resolved,
				typeName: uniqueName(unionName+"Items", used),
				kind:     unionVariantArray,
				goType:   "[]" + g.typeForSchema(resolved.Items, false),
			})
			continue
		}
		if !objectLikeSchema(resolved) {
			return nil, false
		}
		variantType := g.unionVariantTypeName(unionName, rawVariant, used)
		matchKey, matchValue := g.unionVariantMatch(resolved)
		info.variants = append(info.variants, unionVariant{
			raw:        rawVariant,
			schema:     resolved,
			typeName:   variantType,
			kind:       unionVariantObject,
			matchKey:   matchKey,
			matchValue: matchValue,
		})
	}
	if hasStringEnum {
		stringValues = compactSortedStrings(stringValues)
		info.variants = append([]unionVariant{{
			typeName:     unionName + "Value",
			kind:         unionVariantStringEnum,
			stringValues: stringValues,
		}}, info.variants...)
	}
	if len(info.variants) == 0 {
		return nil, false
	}
	if len(info.variants) == 1 && info.variants[0].kind == unionVariantStringEnum && !g.allowsStringOnlyInterfaceUnion(unionName) {
		return nil, false
	}
	if len(info.rawVariants) == 1 && len(info.variants) == 1 {
		return info, true
	}
	if len(info.variants) == 1 && !hasStringEnum {
		return nil, false
	}
	assignUniquePresenceMatchKeys(info.variants)
	matchCounts := map[string]int{}
	for _, variant := range info.variants {
		if variant.kind == unionVariantObject && variant.matchKey != "" && variant.matchValue == "" {
			matchCounts[variant.matchKey]++
		}
	}
	for index := range info.variants {
		variant := &info.variants[index]
		if variant.kind == unionVariantObject && variant.matchValue == "" && matchCounts[variant.matchKey] > 1 {
			variant.matchKey = ""
		}
	}
	return info, true
}

func assignUniquePresenceMatchKeys(variants []unionVariant) {
	requiredCounts := map[string]int{}
	for _, variant := range variants {
		if variant.kind != unionVariantObject || variant.schema == nil {
			continue
		}
		for _, key := range variant.schema.Required {
			requiredCounts[key]++
		}
	}
	for index := range variants {
		variant := &variants[index]
		if variant.kind != unionVariantObject || variant.matchKey != "" || variant.schema == nil {
			continue
		}
		for _, key := range variant.schema.Required {
			if requiredCounts[key] == 1 {
				variant.matchKey = key
				break
			}
		}
	}
}

func (g *generator) allowsStringOnlyInterfaceUnion(unionName string) bool {
	if g.packageName != "codexappserver" {
		return true
	}
	return unionName == "ReasoningSummary"
}

func (g *generator) unionVariantMatch(def *jsonschema.Schema) (string, string) {
	if def == nil {
		return "", ""
	}
	if discriminator, ok := unionDiscriminatorProperty(def); ok {
		property := def.Properties[discriminator]
		enum := stringEnum(property)
		if len(enum) == 1 {
			return discriminator, enum[0]
		}
	}
	if len(def.Required) == 1 {
		return def.Required[0], ""
	}
	return "", ""
}

func compactSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	slices.Sort(values)
	return slices.Compact(values)
}
