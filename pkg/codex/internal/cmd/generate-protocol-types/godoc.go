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
	"strings"
	"unicode"
)

func writeGodoc(out *bytes.Buffer, indent, goName, description, fallback string) {
	body := strings.TrimSpace(description)
	usedDescription := body != ""
	if !usedDescription {
		body = strings.TrimSpace(fallback)
	}
	if body == "" {
		return
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	first, rest, _ := strings.Cut(body, "\n")
	first = strings.TrimRight(first, " \t")
	if usedDescription && goName != "" {
		first = rewriteGodocFirstLine(goName, first)
	} else if goName != "" && !startsWithIdentifier(first, goName) {
		first = goName + " " + first
	}
	first = ensureSentenceTerminator(first, rest == "")
	fmt.Fprintf(out, "%s// %s\n", indent, first)
	if rest == "" {
		return
	}
	lines := strings.Split(rest, "\n")
	for index, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" {
			fmt.Fprintf(out, "%s//\n", indent)
			continue
		}
		isLast := index == len(lines)-1
		if isLast {
			trimmed = ensureSentenceTerminator(trimmed, true)
		}
		fmt.Fprintf(out, "%s// %s\n", indent, trimmed)
	}
}

// rewriteGodocFirstLine reshapes the first sentence of a JSON-schema description
// into idiomatic Go documentation that begins with goName.
//
// The rules are intentionally conservative because schema descriptions are
// freeform prose and aggressive rewriting tends to introduce grammatical
// errors with mass nouns, pluralisation, or idiomatic phrasing. In priority
// order:
//
//  1. If the line already starts with goName, return it unchanged so that
//     hand-authored upstream godoc passes through.
//  2. Strip an "EXPERIMENTAL -" or "[UNSTABLE]" style header tag because the
//     leading tag is redundant once goName is prepended.
//  3. If the first word is "Whether", emit "<goName> reports whether ...".
//  4. If the first word is the indefinite or definite article "A", "An", or
//     "The", emit "<goName> represents a/an ...". The article is chosen by
//     the first vowel sound of the body and dropped when the leading noun
//     phrase is plural.
//  5. If the first word is a known imperative verb (allow-listed) or a
//     supported irregular verb, conjugate it to the third person singular
//     and emit "<goName> <verb-3sg> ...".
//  6. If the first word already looks like a third person singular verb,
//     emit "<goName> <verb> ..." with the first letter lowercased.
//  7. Otherwise, emit "<goName> <body>" with the first letter lowercased.
//     This is the safe fallback for adjective-led, mass-noun, conjunction,
//     and pronoun phrases where forcing "represents a/an" produces awkward
//     English.
func rewriteGodocFirstLine(goName, line string) string {
	if startsWithIdentifier(line, goName) {
		return line
	}
	if line == "" {
		return goName
	}
	if rewritten, ok := stripLeadingTag(line); ok {
		line = rewritten
	}
	firstWord, remainder := splitFirstWord(line)
	lowerFirst := strings.ToLower(firstWord)
	switch lowerFirst {
	case "a", "an", "the":
		body := remainder
		if body == "" {
			return goName + " represents a value"
		}
		if isPluralNounPhrase(body) {
			return goName + " represents " + lowercaseFirst(body)
		}
		article := chooseArticle(body)
		return goName + " represents " + article + " " + lowercaseFirst(body)
	case "whether":
		if remainder != "" {
			return goName + " reports whether " + lowercaseFirst(remainder)
		}
		return goName + " reports whether the value applies"
	}
	if converted, ok := imperativeToThirdPerson(firstWord); ok {
		if remainder == "" {
			return goName + " " + converted
		}
		return goName + " " + converted + " " + remainder
	}
	if isThirdPersonSingularVerb(firstWord) {
		if remainder == "" {
			return goName + " " + lowerFirst
		}
		return goName + " " + lowerFirst + " " + remainder
	}
	if hasBracketedTagPrefix(line) {
		return goName + " " + line
	}
	return goName + " " + lowercaseFirst(line)
}

// stripLeadingTag removes a leading "TAG -" or "TAG —" header (such as
// "EXPERIMENTAL -" or "[UNSTABLE]") from line and returns the remainder when
// the tag is followed by descriptive prose. The header is dropped because it
// becomes redundant once the godoc has been rewritten with goName.
func stripLeadingTag(line string) (string, bool) {
	for _, separator := range []string{" - ", " — ", " – "} {
		index := strings.Index(line, separator)
		if index <= 0 {
			continue
		}
		head := line[:index]
		if !isHeaderTag(head) {
			continue
		}
		rest := strings.TrimSpace(line[index+len(separator):])
		if rest == "" {
			continue
		}
		return rest, true
	}
	return line, false
}

func isHeaderTag(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r == ' ' || r == '-' || r == '_' {
			continue
		}
		if unicode.IsLetter(r) {
			if !unicode.IsUpper(r) {
				return false
			}
			continue
		}
		if unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}

func hasBracketedTagPrefix(line string) bool {
	if line == "" {
		return false
	}
	switch line[0] {
	case '[', '(':
		return true
	}
	return false
}

func isPluralNounPhrase(body string) bool {
	first, _ := splitFirstWord(body)
	stripped, _ := splitTrailingPunctuation(first)
	if !looksLikeASCIIWord(stripped) {
		return false
	}
	lower := strings.ToLower(stripped)
	if !strings.HasSuffix(lower, "s") {
		return false
	}
	if strings.HasSuffix(lower, "ss") || strings.HasSuffix(lower, "us") || strings.HasSuffix(lower, "is") {
		return false
	}
	if isAcronymWord(stripped) {
		return false
	}
	if _, ok := imperativeStopwords[lower]; ok {
		return false
	}
	for _, candidate := range singularCandidates(lower) {
		if _, ok := imperativeVerbAllowlist[candidate]; ok {
			return false
		}
		if _, ok := imperativeOverrides[candidate]; ok {
			return false
		}
	}
	return true
}

func splitFirstWord(line string) (string, string) {
	index := strings.IndexAny(line, " \t")
	if index < 0 {
		return line, ""
	}
	return line[:index], strings.TrimLeft(line[index:], " \t")
}

func lowercaseFirst(value string) string {
	if value == "" {
		return value
	}
	if isAcronymWord(firstWordOf(value)) {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func firstWordOf(value string) string {
	first, _ := splitFirstWord(value)
	return first
}

func isAcronymWord(word string) bool {
	stripped := strings.TrimRight(word, ".,;:!?)")
	if len(stripped) < 2 {
		return false
	}
	upper := 0
	for _, r := range stripped {
		if unicode.IsUpper(r) {
			upper++
			continue
		}
		if r == '-' || unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return upper >= 2
}

func chooseArticle(body string) string {
	for _, r := range body {
		switch {
		case unicode.IsLetter(r):
			lower := unicode.ToLower(r)
			switch lower {
			case 'a', 'e', 'i', 'o', 'u':
				return "an"
			}
			return "a"
		case unicode.IsDigit(r):
			return "a"
		}
	}
	return "a"
}

var imperativeOverrides = map[string]string{
	"be":     "is",
	"have":   "has",
	"do":     "does",
	"go":     "goes",
	"try":    "tries",
	"set":    "sets",
	"get":    "gets",
	"add":    "adds",
	"run":    "runs",
	"use":    "uses",
	"map":    "maps",
	"emit":   "emits",
	"omit":   "omits",
	"send":   "sends",
	"read":   "reads",
	"hold":   "holds",
	"keep":   "keeps",
	"force":  "forces",
	"close":  "closes",
	"reset":  "resets",
	"track":  "tracks",
	"write":  "writes",
	"append": "appends",
	"resize": "resizes",
	"return": "returns",
	"select": "selects",
	"create": "creates",
	"delete": "deletes",
	"cancel": "cancels",
	"remove": "removes",
	"update": "updates",
	"invoke": "invokes",
	"notify": "notifies",
	"verify": "verifies",
	"modify": "modifies",
	"apply":  "applies",
}

var imperativeStopwords = map[string]struct{}{
	"if":           {},
	"when":         {},
	"while":        {},
	"this":         {},
	"that":         {},
	"these":        {},
	"those":        {},
	"it":           {},
	"please":       {},
	"note":         {},
	"see":          {},
	"experimental": {},
	"unstable":     {},
	"deprecated":   {},
	"todo":         {},
	"important":    {},
	"warning":      {},
	"new":          {},
	"old":          {},
	"legacy":       {},
}

func imperativeToThirdPerson(word string) (string, bool) {
	stripped, suffix := splitTrailingPunctuation(word)
	if stripped == "" {
		return "", false
	}
	lower := strings.ToLower(stripped)
	if _, ok := imperativeStopwords[lower]; ok {
		return "", false
	}
	if isAcronymWord(stripped) {
		return "", false
	}
	if !looksLikeASCIIWord(stripped) {
		return "", false
	}
	first, _ := utf8DecodeFirst(stripped)
	if !unicode.IsUpper(first) {
		return "", false
	}
	if override, ok := imperativeOverrides[lower]; ok {
		return override + suffix, true
	}
	if !looksLikeImperativeVerb(lower) {
		return "", false
	}
	return conjugateThirdPerson(lower) + suffix, true
}

func splitTrailingPunctuation(word string) (string, string) {
	end := len(word)
	for end > 0 {
		switch word[end-1] {
		case '.', ',', ';', ':', '!', '?', ')':
			end--
			continue
		}
		break
	}
	return word[:end], word[end:]
}

func looksLikeASCIIWord(word string) bool {
	for _, r := range word {
		if r > unicode.MaxASCII {
			return false
		}
		if !unicode.IsLetter(r) && r != '-' {
			return false
		}
	}
	return word != ""
}

var imperativeVerbAllowlist = map[string]struct{}{
	"abort":       {},
	"accept":      {},
	"activate":    {},
	"allow":       {},
	"announce":    {},
	"approve":     {},
	"attach":      {},
	"attempt":     {},
	"authorize":   {},
	"begin":       {},
	"bind":        {},
	"build":       {},
	"call":        {},
	"capture":     {},
	"check":       {},
	"clear":       {},
	"clone":       {},
	"collect":     {},
	"combine":     {},
	"complete":    {},
	"compose":     {},
	"compute":     {},
	"configure":   {},
	"confirm":     {},
	"connect":     {},
	"construct":   {},
	"continue":    {},
	"control":     {},
	"convert":     {},
	"copy":        {},
	"count":       {},
	"declare":     {},
	"decode":      {},
	"decrypt":     {},
	"defer":       {},
	"define":      {},
	"deny":        {},
	"deploy":      {},
	"describe":    {},
	"detach":      {},
	"detect":      {},
	"determine":   {},
	"disable":     {},
	"disallow":    {},
	"discard":     {},
	"discover":    {},
	"dismiss":     {},
	"dispatch":    {},
	"display":     {},
	"distribute":  {},
	"download":    {},
	"draft":       {},
	"drop":        {},
	"emit":        {},
	"enable":      {},
	"encode":      {},
	"encrypt":     {},
	"end":         {},
	"ensure":      {},
	"enter":       {},
	"escalate":    {},
	"escape":      {},
	"establish":   {},
	"evaluate":    {},
	"execute":     {},
	"exit":        {},
	"expand":      {},
	"export":      {},
	"extend":      {},
	"extract":     {},
	"fail":        {},
	"fetch":       {},
	"filter":      {},
	"finalize":    {},
	"finish":      {},
	"flag":        {},
	"flush":       {},
	"focus":       {},
	"follow":      {},
	"fork":        {},
	"format":      {},
	"forward":     {},
	"generate":    {},
	"grant":       {},
	"group":       {},
	"guard":       {},
	"handle":      {},
	"hide":        {},
	"identify":    {},
	"ignore":      {},
	"import":      {},
	"include":     {},
	"increment":   {},
	"index":       {},
	"indicate":    {},
	"infer":       {},
	"inject":      {},
	"initialize":  {},
	"inspect":     {},
	"install":     {},
	"interpret":   {},
	"interrupt":   {},
	"introduce":   {},
	"join":        {},
	"launch":      {},
	"limit":       {},
	"link":        {},
	"list":        {},
	"load":        {},
	"lock":        {},
	"log":         {},
	"manage":      {},
	"mark":        {},
	"match":       {},
	"merge":       {},
	"mirror":      {},
	"mount":       {},
	"move":        {},
	"name":        {},
	"navigate":    {},
	"negotiate":   {},
	"observe":     {},
	"open":        {},
	"override":    {},
	"pack":        {},
	"parse":       {},
	"pass":        {},
	"pause":       {},
	"persist":     {},
	"pin":         {},
	"pipe":        {},
	"plan":        {},
	"poll":        {},
	"post":        {},
	"prefer":      {},
	"prepare":     {},
	"present":     {},
	"preserve":    {},
	"prevent":     {},
	"print":       {},
	"process":     {},
	"produce":     {},
	"prompt":      {},
	"propagate":   {},
	"propose":     {},
	"protect":     {},
	"provide":     {},
	"publish":     {},
	"purge":       {},
	"push":        {},
	"queue":       {},
	"raise":       {},
	"reach":       {},
	"receive":     {},
	"recognize":   {},
	"reconcile":   {},
	"record":      {},
	"recover":     {},
	"redirect":    {},
	"reduce":      {},
	"refresh":     {},
	"register":    {},
	"reject":      {},
	"release":     {},
	"reload":      {},
	"render":      {},
	"renew":       {},
	"reorder":     {},
	"repeat":      {},
	"replace":     {},
	"reply":       {},
	"report":      {},
	"request":     {},
	"require":     {},
	"reserve":     {},
	"resolve":     {},
	"respond":     {},
	"restart":     {},
	"restore":     {},
	"restrict":    {},
	"resume":      {},
	"retain":      {},
	"retrieve":    {},
	"retry":       {},
	"reveal":      {},
	"reverse":     {},
	"review":      {},
	"revoke":      {},
	"roll":        {},
	"rotate":      {},
	"route":       {},
	"sample":      {},
	"save":        {},
	"scan":        {},
	"schedule":    {},
	"search":      {},
	"serialize":   {},
	"serve":       {},
	"share":       {},
	"show":        {},
	"sign":        {},
	"signal":      {},
	"skip":        {},
	"sleep":       {},
	"sort":        {},
	"specify":     {},
	"split":       {},
	"start":       {},
	"stop":        {},
	"store":       {},
	"stream":      {},
	"submit":      {},
	"subscribe":   {},
	"summarize":   {},
	"suspend":     {},
	"swap":        {},
	"sync":        {},
	"tag":         {},
	"target":      {},
	"terminate":   {},
	"test":        {},
	"throw":       {},
	"toggle":      {},
	"trace":       {},
	"transfer":    {},
	"transform":   {},
	"translate":   {},
	"transmit":    {},
	"trigger":     {},
	"trim":        {},
	"trust":       {},
	"truncate":    {},
	"unblock":     {},
	"unfollow":    {},
	"unlink":      {},
	"unlock":      {},
	"unmount":     {},
	"unmute":      {},
	"unpack":      {},
	"unset":       {},
	"unsubscribe": {},
	"upload":      {},
	"validate":    {},
	"watch":       {},
	"wrap":        {},
	"write":       {},
	"yield":       {},
}

func looksLikeImperativeVerb(lower string) bool {
	_, ok := imperativeVerbAllowlist[lower]
	return ok
}

func conjugateThirdPerson(verb string) string {
	if verb == "" {
		return verb
	}
	if strings.HasSuffix(verb, "y") && len(verb) >= 2 {
		prev := verb[len(verb)-2]
		if !isVowel(rune(prev)) {
			return verb[:len(verb)-1] + "ies"
		}
	}
	if strings.HasSuffix(verb, "s") || strings.HasSuffix(verb, "x") || strings.HasSuffix(verb, "z") || strings.HasSuffix(verb, "ch") || strings.HasSuffix(verb, "sh") {
		return verb + "es"
	}
	return verb + "s"
}

func isVowel(r rune) bool {
	switch unicode.ToLower(r) {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

func isThirdPersonSingularVerb(word string) bool {
	stripped, _ := splitTrailingPunctuation(word)
	if !looksLikeASCIIWord(stripped) {
		return false
	}
	if isAcronymWord(stripped) {
		return false
	}
	first, _ := utf8DecodeFirst(stripped)
	if !unicode.IsUpper(first) {
		return false
	}
	lower := strings.ToLower(stripped)
	if !strings.HasSuffix(lower, "s") {
		return false
	}
	if _, ok := imperativeStopwords[lower]; ok {
		return false
	}
	for _, candidate := range singularCandidates(lower) {
		if _, ok := imperativeVerbAllowlist[candidate]; ok {
			return true
		}
		if _, ok := imperativeOverrides[candidate]; ok {
			return true
		}
	}
	return false
}

func singularCandidates(verb string) []string {
	if strings.HasSuffix(verb, "ies") && len(verb) >= 4 {
		return []string{verb[:len(verb)-3] + "y"}
	}
	if strings.HasSuffix(verb, "sses") || strings.HasSuffix(verb, "xes") || strings.HasSuffix(verb, "zes") || strings.HasSuffix(verb, "ches") || strings.HasSuffix(verb, "shes") {
		return []string{verb[:len(verb)-2]}
	}
	if strings.HasSuffix(verb, "es") {
		return []string{verb[:len(verb)-1], verb[:len(verb)-2]}
	}
	if strings.HasSuffix(verb, "s") {
		return []string{verb[:len(verb)-1]}
	}
	return []string{verb}
}

func startsWithIdentifier(line, name string) bool {
	if !strings.HasPrefix(line, name) {
		return false
	}
	rest := line[len(name):]
	if rest == "" {
		return true
	}
	next, _ := utf8DecodeFirst(rest)
	return next == ' ' || next == '\t' || next == '.' || next == ',' || next == ':' || next == ';' || next == '!' || next == '?'
}

func utf8DecodeFirst(value string) (rune, int) {
	for _, r := range value {
		size := len(string(r))
		return r, size
	}
	return 0, 0
}

func ensureSentenceTerminator(line string, isLast bool) string {
	if !isLast {
		return line
	}
	if line == "" {
		return line
	}
	last := line[len(line)-1]
	switch last {
	case '.', '!', '?', ':', ')', '`', '"', '\'':
		return line
	}
	return line + "."
}
