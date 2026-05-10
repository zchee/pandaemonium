package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-json-experiment/json"
	"github.com/google/jsonschema-go/jsonschema"
)

type generator struct {
	definitions map[string]*jsonschema.Schema
	aliases     map[string]string
	unionTagger map[string]struct{}
	emittedType map[string]struct{}
}

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

const maxHTTPSchemaBytes = 32 << 20

func main() {
	schemaPath := flag.String("schema", "", "path or URL to protocol JSON schema")
	outPath := flag.String("out", "", "path for generated Go file")
	packageName := flag.String("package", "codexappserver", "package name for generated Go file")
	flag.Parse()
	if strings.TrimSpace(*schemaPath) == "" || strings.TrimSpace(*outPath) == "" {
		fatalf("both -schema and -out are required")
	}
	if !validPackageName(*packageName) {
		fatalf("invalid package name %q", *packageName)
	}
	input, err := readSchemaSource(*schemaPath)
	if err != nil {
		fatalf("read schema: %v", err)
	}
	var parsed jsonschema.Schema
	if err := json.Unmarshal(input, &parsed); err != nil {
		fatalf("decode schema: %v", err)
	}
	generated, err := newGenerator(parsed.Definitions).generate(*schemaPath, *packageName)
	if err != nil {
		fatalf("generate: %v", err)
	}
	formatted, err := format.Source(generated)
	if err != nil {
		fatalf("format generated source: %v\n%s", err, generated)
	}
	if err := os.WriteFile(*outPath, formatted, 0o644); err != nil {
		fatalf("write output: %v", err)
	}
}

func readSchemaSource(source string) ([]byte, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		if !hasHTTPScheme(source) {
			return os.ReadFile(source)
		}
		return nil, fmt.Errorf("parse source: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
		if parsed.Host == "" {
			return nil, fmt.Errorf("invalid %s URL: missing host", parsed.Scheme)
		}
		return readHTTPSchemaSource(source)
	case "":
		return os.ReadFile(source)
	default:
		if parsed.Host != "" {
			return nil, fmt.Errorf("unsupported schema URL scheme %q", parsed.Scheme)
		}
		return os.ReadFile(source)
	}
}

func hasHTTPScheme(source string) bool {
	scheme, _, ok := strings.Cut(source, ":")
	if !ok {
		return false
	}
	return strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https")
}

func readHTTPSchemaSource(source string) ([]byte, error) {
	return readHTTPSchemaSourceWithLimit(source, maxHTTPSchemaBytes)
}

func readHTTPSchemaSourceWithLimit(source string, maxBytes int64) ([]byte, error) {
	sourceLabel := schemaSourceLabel(source)
	request, err := http.NewRequest(http.MethodGet, source, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		if urlError, ok := errors.AsType[*url.Error](err); ok {
			err = urlError.Err
		}
		return nil, fmt.Errorf("fetch %s: %w", sourceLabel, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("fetch %s: unexpected HTTP status %s", sourceLabel, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", sourceLabel, err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("read %s response: schema exceeds %d bytes", sourceLabel, maxBytes)
	}
	return body, nil
}

func schemaSourceLabel(source string) string {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return filepathSlash(source)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return filepathSlash(source)
	}
	redacted := *parsed
	redacted.User = nil
	redacted.RawQuery = ""
	redacted.Fragment = ""
	return redacted.String()
}

func validPackageName(name string) bool {
	if name == "" || isGoPackageKeyword(name) {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isGoPackageKeyword(name string) bool {
	switch name {
	case "break", "default", "func", "interface", "select", "case", "defer", "go", "map", "struct", "chan", "else", "goto", "package", "switch", "const", "fallthrough", "if", "range", "type", "continue", "for", "import", "return", "var":
		return true
	default:
		return false
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func newGenerator(definitions map[string]*jsonschema.Schema) *generator {
	return &generator{
		definitions: definitions,
		aliases: map[string]string{
			"AbsolutePathBuf": "string",
			"RequestId":       "string",
		},
		unionTagger: make(map[string]struct{}),
		emittedType: make(map[string]struct{}),
	}
}

func (g *generator) generate(schemaPath, packageName string) ([]byte, error) {
	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by go generate ./pkg/codex-app-server; DO NOT EDIT.\n")
	fmt.Fprintf(&out, "// Source: %s\n\n", schemaSourceLabel(schemaPath))
	fmt.Fprintf(&out, "package %s\n\n", packageName)
	keys := make([]string, 0, len(g.definitions))
	for name := range g.definitions {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	needsJSONImport := false
	for _, name := range keys {
		needsJSONImport = needsJSONImport || g.shouldEmitRawCodecWrapper(name, g.definitions[name])
		needsJSONImport = needsJSONImport || g.structNeedsCustomUnmarshal(g.definitions[name])
	}
	if len(keys) > 0 {
		imports := []string{"github.com/go-json-experiment/json/jsontext"}
		if needsJSONImport {
			imports = append(imports, "github.com/go-json-experiment/json")
		}
		sort.Strings(imports)
		if len(imports) == 1 {
			fmt.Fprintf(&out, "import %q\n\n", imports[0])
		} else {
			fmt.Fprintln(&out, "import (")
			for _, importPath := range imports {
				fmt.Fprintf(&out, "\t%q\n", importPath)
			}
			fmt.Fprintln(&out, ")")
			fmt.Fprintln(&out)
		}
	}

	for _, name := range keys {
		if err := g.emitDefinition(&out, name, g.definitions[name]); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func filepathSlash(path string) string { return strings.ReplaceAll(path, "\\", "/") }

// writeGodoc emits a godoc comment block for the identifier goName. When
// description is non-empty it is rewritten into idiomatic Go documentation
// prefixed with goName; otherwise fallback is used verbatim. When both inputs
// are empty no comment is emitted.
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

func (g *generator) emitDefinition(out *bytes.Buffer, name string, def *jsonschema.Schema) error {
	if def == nil {
		return fmt.Errorf("nil schema")
	}
	goName := exportName(name)
	if alias, ok := g.aliases[name]; ok {
		writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from the %s schema definition.", goName, name))
		fmt.Fprintf(out, "type %s = %s\n", goName, alias)
		return nil
	}
	if g.isObjectUnion(def) {
		return g.emitUnionDefinition(out, goName, def)
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
	if len(stringEnum(def)) > 0 && includesType(def, "string") {
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
	writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from the %s schema definition.", goName, schemaName))
	fmt.Fprintf(out, "type %s struct {\n", goName)
	properties := make([]string, 0, len(def.Properties))
	for name := range def.Properties {
		properties = append(properties, name)
	}
	sort.Strings(properties)
	required := make(map[string]bool, len(def.Required))
	for _, name := range def.Required {
		required[name] = true
	}
	used := map[string]int{}
	fields := make([]structField, 0, len(properties))
	for index, jsonName := range properties {
		fieldName := uniqueName(exportName(jsonName), used)
		fieldType := g.typeForSchema(def.Properties[jsonName], !required[jsonName])
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
			fmt.Fprintf(out, "\t\tvalue.%s = Raw%s(raw.%s)\n", field.name, field.unionName, field.name)
			fmt.Fprintf(out, "\t}\n")
		case unionShapeOptionalSingle:
			fmt.Fprintf(out, "\tif raw.%s == nil {\n", field.name)
			fmt.Fprintf(out, "\t\tvalue.%s = nil\n", field.name)
			fmt.Fprintf(out, "\t} else {\n")
			fmt.Fprintf(out, "\t\t%s := %s(Raw%s(raw.%s))\n", unexportName(field.name), field.unionName, field.unionName, field.name)
			fmt.Fprintf(out, "\t\tvalue.%s = &%s\n", field.name, unexportName(field.name))
			fmt.Fprintf(out, "\t}\n")
		case unionShapeSlice:
			fmt.Fprintf(out, "\tif raw.%s == nil {\n", field.name)
			fmt.Fprintf(out, "\t\tvalue.%s = nil\n", field.name)
			fmt.Fprintf(out, "\t} else {\n")
			fmt.Fprintf(out, "\t\tvalue.%s = make([]%s, len(raw.%s))\n", field.name, field.unionName, field.name)
			fmt.Fprintf(out, "\t\tfor i, item := range raw.%s {\n", field.name)
			fmt.Fprintf(out, "\t\t\tif item != nil {\n")
			fmt.Fprintf(out, "\t\t\t\tvalue.%s[i] = Raw%s(item)\n", field.name, field.unionName)
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
		if g.emitsRawUnionWrapper(g.definitions[name]) {
			return exportName(name), true
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

func (g *generator) emitsRawUnionWrapper(def *jsonschema.Schema) bool {
	rawVariants, variants := g.unionObjectVariants(def)
	if len(variants) <= 1 {
		return false
	}
	_, hasDiscriminator := g.unionDiscriminatorProperty(rawVariants)
	if hasDiscriminator {
		return true
	}
	_, hasMetadata := g.unionMetadata(variants)
	return hasMetadata
}

func (g *generator) emitUnionDefinition(out *bytes.Buffer, goName string, def *jsonschema.Schema) error {
	rawVariants, variants := g.unionObjectVariants(def)

	if len(variants) == 0 {
		return nil
	}
	_, hasDiscriminator := g.unionDiscriminatorProperty(rawVariants)
	if !hasDiscriminator {
		if _, ok := g.unionMetadata(variants); !ok {
			return nil
		}
	}

	// Single-variant object unions are simple aliases of that variant.
	if len(variants) == 1 {
		variantType := g.unionVariantTypeName(goName, rawVariants[0], g.reservedDefinitionTypeNames())
		variantSchema := g.resolvedSchema(rawVariants[0])
		if variantSchema == nil {
			return nil
		}
		if _, ok := g.definitions[variantType]; !ok && rawVariants[0].Ref == "" {
			commentName := rawVariants[0].Title
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
	if hasDiscriminator {
		writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from the %s schema definition.", goName, goName))
	} else {
		writeGodoc(out, "", goName, def.Description, fmt.Sprintf("%s is generated from metadata union variants.", goName))
	}
	fmt.Fprintf(out, "type %s interface {\n\tis%s()\n}\n\n", goName, goName)
	g.emitRawUnionWrapper(out, goName)

	used := g.reservedDefinitionTypeNames()
	for _, variant := range rawVariants {
		variantType := g.unionVariantTypeName(goName, variant, used)
		g.emitUnionTagger(out, goName, variantType)
		variantSchema := g.resolvedSchema(variant)
		if variantSchema == nil {
			continue
		}
		if !objectLikeSchema(variantSchema) {
			continue
		}
		if _, ok := g.definitions[variantSchema.Title]; ok {
			g.emitUnionTagger(out, goName, variantType)
			continue
		}
		if _, ok := g.definitions[variantType]; ok {
			g.emitUnionTagger(out, goName, variantType)
			continue
		}
		if variant.Ref != "" {
			continue
		}
		commentName := variant.Title
		if commentName == "" {
			commentName = variantType
		}
		if err := g.emitStruct(out, variantType, commentName, variantSchema); err != nil {
			return err
		}
		out.WriteByte('\n')
	}
	return nil
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

func (g *generator) emitUnionTagger(out *bytes.Buffer, unionName, targetType string) {
	if targetType == "" {
		return
	}
	key := unionName + "\x00" + targetType
	if _, ok := g.unionTagger[key]; ok {
		return
	}
	g.unionTagger[key] = struct{}{}
	fmt.Fprintf(out, "func (%s) is%s() {}\n\n", targetType, unionName)
}

func (g *generator) reservedDefinitionTypeNames() map[string]int {
	used := make(map[string]int, len(g.definitions))
	for name := range g.definitions {
		used[exportName(name)] = 1
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
			return exportName(variant.Title)
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

func (g *generator) unionObjectVariants(def *jsonschema.Schema) ([]*jsonschema.Schema, []*jsonschema.Schema) {
	if def == nil {
		return nil, nil
	}
	var rawVariants []*jsonschema.Schema
	if len(def.AnyOf) > 0 {
		rawVariants = def.AnyOf
	} else if len(def.OneOf) > 0 {
		rawVariants = def.OneOf
	} else {
		return nil, nil
	}
	if len(rawVariants) == 0 {
		return nil, nil
	}

	resolvedVariants := make([]*jsonschema.Schema, 0, len(rawVariants))
	for _, variant := range rawVariants {
		if variant == nil {
			return nil, nil
		}
		resolved := g.resolvedSchema(variant)
		if !objectLikeSchema(resolved) {
			return nil, nil
		}
		resolvedVariants = append(resolvedVariants, resolved)
	}
	return rawVariants, resolvedVariants
}

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

func (g *generator) isObjectUnion(def *jsonschema.Schema) bool {
	_, variants := g.unionObjectVariants(def)
	if len(variants) == 0 {
		return false
	}
	if len(variants) == 2 {
		if _, nullable := nullableVariant(variants); nullable {
			return false
		}
	}
	if _, ok := g.unionDiscriminatorProperty(variants); ok {
		return true
	}
	_, ok := g.unionMetadata(variants)
	return ok
}

func (g *generator) unionMetadata(variants []*jsonschema.Schema) (string, bool) {
	for _, variant := range variants {
		if variant == nil {
			return "", false
		}
		if variant.Title != "" {
			continue
		}
		if variant.Ref != "" {
			continue
		}
		if len(variant.Required) == 0 {
			return "", false
		}
	}
	return "metadata", true
}

func (g *generator) unionDiscriminatorProperty(variants []*jsonschema.Schema) (string, bool) {
	if len(variants) == 0 {
		return "", false
	}
	var expected string
	for _, variant := range variants {
		variantSchema := g.resolvedSchema(variant)
		if !objectLikeSchema(variantSchema) {
			return "", false
		}
		if variantSchema == nil {
			return "", false
		}
		discriminator, ok := unionDiscriminatorProperty(variantSchema)
		if !ok {
			return "", false
		}
		if expected == "" {
			expected = discriminator
			continue
		}
		if expected != discriminator {
			return "", false
		}
	}
	return expected, true
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
	if isGoKeyword(result) {
		return result + "Value"
	}
	return result
}

func unexportName(name string) string {
	if name == "" {
		return "value"
	}
	runes := []rune(name)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func exportPart(part string) string {
	lower := strings.ToLower(part)
	initialisms := map[string]string{
		"api": "API", "ascii": "ASCII", "cpu": "CPU", "css": "CSS", "dns": "DNS", "eof": "EOF",
		"gpu": "GPU", "html": "HTML", "http": "HTTP", "https": "HTTPS", "id": "ID", "ip": "IP",
		"json": "JSON", "lsp": "LSP", "mcp": "MCP", "oauth": "OAuth", "rpc": "RPC", "sdp": "SDP",
		"sdk": "SDK", "sql": "SQL", "ssh": "SSH", "tcp": "TCP", "tls": "TLS", "tty": "TTY",
		"ui": "UI", "uid": "UID", "uri": "URI", "url": "URL", "utf8": "UTF8", "uuid": "UUID",
		"vm": "VM", "xml": "XML",
	}
	if value, ok := initialisms[lower]; ok {
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

func hasUpperAfterFirst(runes []rune) bool {
	return slices.ContainsFunc(runes[1:], unicode.IsUpper)
}

func normalizeInitialisms(name string) string {
	replacements := []struct{ old, new string }{
		{"Api", "API"},
		{"Ascii", "ASCII"},
		{"Cpu", "CPU"},
		{"Css", "CSS"},
		{"Dns", "DNS"},
		{"Eof", "EOF"},
		{"Gpu", "GPU"},
		{"Html", "HTML"},
		{"Https", "HTTPS"},
		{"Http", "HTTP"},
		{"Json", "JSON"},
		{"Lsp", "LSP"},
		{"Mcp", "MCP"},
		{"Oauth", "OAuth"},
		{"Rpc", "RPC"},
		{"Sdp", "SDP"},
		{"Sdk", "SDK"},
		{"Sql", "SQL"},
		{"Ssh", "SSH"},
		{"Tcp", "TCP"},
		{"Tls", "TLS"},
		{"Tty", "TTY"},
		{"Uid", "UID"},
		{"Uri", "URI"},
		{"Url", "URL"},
		{"Utf8", "UTF8"},
		{"Uuid", "UUID"},
		{"Xml", "XML"},
		{"Id", "ID"},
		{"Ui", "UI"},
	}
	for _, replacement := range replacements {
		name = strings.ReplaceAll(name, replacement.old, replacement.new)
	}
	return name
}

func isGoKeyword(name string) bool {
	switch name {
	case "Break", "Default", "Func", "Interface", "Select", "Case", "Defer", "Go", "Map", "Struct", "Chan", "Else", "Goto", "Package", "Switch", "Const", "Fallthrough", "If", "Range", "Type", "Continue", "For", "Import", "Return", "Var":
		return true
	default:
		return false
	}
}
