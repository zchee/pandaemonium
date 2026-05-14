package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
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
	gofumpt "mvdan.cc/gofumpt/format"
)

type generator struct {
	definitions map[string]*jsonschema.Schema
	aliases     map[string]string
	packageName string
	typeNames   map[string]string
	rawUnions   map[string]struct{}
	rawDecodes  map[string]struct{}
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
	formatted, err := gofumpt.Source(generated, gofumpt.Options{
		LangVersion: "go1.27",
		ModulePath:  "github.com/zchee/pandaemonium",
		Extra: gofumpt.Extra{
			GroupParams:   true,
			ClotheReturns: true,
		},
	})
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
	if name == "" || isGoKeyword(name) {
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
	g.packageName = packageName
	g.typeNames = map[string]string{
		"Config": "ConfigPayload",
		"Thread": "ThreadPayload",
	}
	g.rawUnions = map[string]struct{}{}
	g.rawDecodes = map[string]struct{}{}
	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by go generate ./pkg/codex; DO NOT EDIT.\n")
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
	sort.Strings(properties)
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
			g.emitUnionTagger(out, goName, variant.typeName)
			if variant.schema == nil || !objectLikeSchema(variant.schema) {
				continue
			}
			if _, ok := g.definitions[variant.schema.Title]; ok {
				continue
			}
			if _, ok := g.definitions[variant.typeName]; ok {
				continue
			}
			if variant.raw.Ref != "" {
				continue
			}
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
