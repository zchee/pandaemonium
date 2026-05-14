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
	"regexp"
	"slices"
	"strings"
	"unicode"
)

var nonIdentifier = regexp.MustCompile(`[^A-Za-z0-9]+`)

func exportName(name string) string {
	parts := nonIdentifier.Split(name, -1)
	var out strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		out.WriteString(exportPart(part))
	}
	result := out.String()
	if result == "" {
		return "Value"
	}
	if first := []rune(result)[0]; unicode.IsDigit(first) {
		return "Value" + result
	}
	if result == "Type" {
		return result
	}
	if isGoKeyword(lowerFirstRune(result)) {
		return result + "Value"
	}
	return result
}

func goFieldName(jsonName string) string {
	if jsonName == "type" {
		return "Type"
	}
	return exportName(jsonName)
}

func unexportName(name string) string {
	if name == "" {
		return "value"
	}
	result := lowerFirstRune(name)
	if isGoKeyword(result) {
		return result + "Value"
	}
	return result
}

func lowerFirstRune(name string) string {
	runes := []rune(name)
	if len(runes) == 0 {
		return name
	}
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

var goInitialisms = []struct {
	token     string
	goName    string
	normalize bool
}{
	// normalize records whether the old normalizeInitialisms replacement list
	// handled camel/Pascal occurrences. "ip" remains exact-token only for
	// generated-name compatibility.
	{"api", "API", true},
	{"ascii", "ASCII", true},
	{"cpu", "CPU", true},
	{"css", "CSS", true},
	{"dns", "DNS", true},
	{"eof", "EOF", true},
	{"gpu", "GPU", true},
	{"html", "HTML", true},
	{"https", "HTTPS", true},
	{"http", "HTTP", true},
	{"ip", "IP", false},
	{"json", "JSON", true},
	{"lsp", "LSP", true},
	{"mcp", "MCP", true},
	{"oauth", "OAuth", true},
	{"rpc", "RPC", true},
	{"sdp", "SDP", true},
	{"sdk", "SDK", true},
	{"sql", "SQL", true},
	{"ssh", "SSH", true},
	{"tcp", "TCP", true},
	{"tls", "TLS", true},
	{"tty", "TTY", true},
	{"uid", "UID", true},
	{"uri", "URI", true},
	{"url", "URL", true},
	{"utf8", "UTF8", true},
	{"uuid", "UUID", true},
	{"vm", "VM", true},
	{"xml", "XML", true},
	{"id", "ID", true},
	{"ui", "UI", true},
}

func exportPart(part string) string {
	lower := strings.ToLower(part)
	if value, ok := goInitialism(lower); ok {
		return value
	}
	runes := []rune(part)
	if len(runes) == 0 {
		return ""
	}
	if hasUpperAfterFirst(runes) {
		runes[0] = unicode.ToUpper(runes[0])
		return normalizeInitialisms(string(runes))
	}
	for i := range runes {
		runes[i] = unicode.ToLower(runes[i])
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func goInitialism(token string) (string, bool) {
	for _, initialism := range goInitialisms {
		if token == initialism.token {
			return initialism.goName, true
		}
	}
	return "", false
}

func hasUpperAfterFirst(runes []rune) bool {
	return slices.ContainsFunc(runes[1:], unicode.IsUpper)
}

func normalizeInitialisms(name string) string {
	for _, initialism := range goInitialisms {
		if !initialism.normalize {
			continue
		}
		name = strings.ReplaceAll(name, exportInitialismToken(initialism.token), initialism.goName)
	}
	return name
}

func exportInitialismToken(token string) string {
	runes := []rune(token)
	if len(runes) == 0 {
		return token
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func isGoKeyword(name string) bool {
	switch name {
	case "break", "default", "func", "interface", "select", "case", "defer", "go", "map", "struct", "chan", "else", "goto", "package", "switch", "const", "fallthrough", "if", "range", "type", "continue", "for", "import", "return", "var":
		return true
	default:
		return false
	}
}
