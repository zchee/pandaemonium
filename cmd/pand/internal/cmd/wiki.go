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

package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	wikiSchemaVersion = 1
	wikiIndexFile     = "index.md"
	wikiLogFile       = "log.md"
	wikiEnvFile       = "environment.md"
)

var (
	wikiReservedFiles = map[string]struct{}{wikiIndexFile: {}, wikiLogFile: {}, wikiEnvFile: {}}
	wikiLinkPattern   = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
)

type wikiPage struct {
	Filename    string         `json:"filename"`
	Frontmatter map[string]any `json:"frontmatter"`
	Content     string         `json:"content"`
}

func wikiDescriptor(env map[string]string, cwd string) mcpDescriptor {
	return mcpDescriptor{
		CommandName: "wiki",
		Title:       "JSON CLI surface for OMX wiki operations.",
		Tools: []mcpTool{
			{Name: "wiki_ingest", Description: "Create or append knowledge into canonical wiki pages."},
			{Name: "wiki_query", Description: "Search wiki pages by keywords, tags, and category."},
			{Name: "wiki_lint", Description: "Run wiki health checks."},
			{Name: "wiki_add", Description: "Quick-add a page; rejects overwrites."},
			{Name: "wiki_list", Description: "List wiki pages and return index content."},
			{Name: "wiki_read", Description: "Read a wiki page."},
			{Name: "wiki_delete", Description: "Delete a non-reserved canonical wiki page."},
			{Name: "wiki_refresh", Description: "Refresh canonical wiki index metadata."},
		},
		Aliases: map[string]string{
			"ingest":  "wiki_ingest",
			"query":   "wiki_query",
			"lint":    "wiki_lint",
			"add":     "wiki_add",
			"list":    "wiki_list",
			"read":    "wiki_read",
			"delete":  "wiki_delete",
			"refresh": "wiki_refresh",
		},
		Handle: func(input map[string]any) (any, bool) {
			store := fileStore{env: env, cwd: cwd}
			return store.handleWiki(input)
		},
	}
}

func (s fileStore) handleWiki(input map[string]any) (any, bool) {
	root, err := s.workingDirectory(input)
	if err != nil {
		return errorPayload(err), true
	}
	store := wikiStore{root: root}
	switch input["tool"] {
	case "wiki_ingest":
		return store.ingest(input, false)
	case "wiki_query":
		return store.query(input)
	case "wiki_lint":
		return store.lint()
	case "wiki_add":
		return store.ingest(input, true)
	case "wiki_list":
		return map[string]any{"pages": store.listPages(), "index": store.readIndex()}, false
	case "wiki_read":
		pageName, err := requiredString(input, "page")
		if err != nil {
			return errorPayload(err), true
		}
		page, ok := store.readPage(normalizeWikiPageName(pageName))
		if !ok {
			return errorPayload(fmt.Errorf("Wiki page not found")), true
		}
		return page, false
	case "wiki_delete":
		pageName, err := requiredString(input, "page")
		if err != nil {
			return errorPayload(err), true
		}
		filename := normalizeWikiPageName(pageName)
		if err := store.deletePage(filename); err != nil {
			return errorPayload(err), true
		}
		store.appendLog("delete", []string{filename}, "Deleted wiki page "+filename)
		return map[string]any{"deleted": true, "page": filename}, false
	case "wiki_refresh":
		if store.legacyFallbackActive() {
			return map[string]any{"refreshed": false, "legacyFallback": true, "pages": store.listPages(), "index": store.readIndex(), "message": "Legacy .omx/wiki fallback is read-only; copy selected pages into omx_wiki/ before refreshing canonical metadata."}, false
		}
		if err := store.updateIndex(); err != nil {
			return errorPayload(err), true
		}
		store.appendLog("add", store.listPages(), "Refreshed wiki index and derived metadata surfaces")
		return map[string]any{"refreshed": true, "pages": store.listPages(), "index": store.readIndex()}, false
	default:
		return errorPayload(fmt.Errorf("unknown tool: %s", input["tool"])), true
	}
}

type wikiStore struct {
	root string
}

func (s wikiStore) canonicalDir() string { return filepath.Join(s.root, "omx_wiki") }
func (s wikiStore) legacyDir() string    { return filepath.Join(s.root, ".omx", "wiki") }

func (s wikiStore) legacyFallbackActive() bool {
	_, canonicalErr := os.Stat(s.canonicalDir())
	_, legacyErr := os.Stat(s.legacyDir())
	return os.IsNotExist(canonicalErr) && legacyErr == nil
}

func (s wikiStore) readableDir() string {
	if s.legacyFallbackActive() {
		return s.legacyDir()
	}
	return s.canonicalDir()
}

func (s wikiStore) ingest(input map[string]any, rejectOverwrite bool) (any, bool) {
	title, err := requiredString(input, "title")
	if err != nil {
		return errorPayload(err), true
	}
	content, err := requiredString(input, "content")
	if err != nil {
		return errorPayload(err), true
	}
	slug := titleToWikiSlug(title)
	if rejectOverwrite {
		if _, ok := s.readCanonicalPage(slug); ok {
			return errorPayload(fmt.Errorf("Page %q already exists. Use wiki_ingest to merge into it.", slug)), true
		}
	}
	inputData := map[string]any{
		"title":      title,
		"content":    content,
		"tags":       optionalStringSlice(input, "tags"),
		"category":   defaultString(stringField(input, "category"), "reference"),
		"sources":    optionalStringSlice(input, "sources"),
		"confidence": defaultString(stringField(input, "confidence"), "medium"),
	}
	now := nowISO()
	result := map[string]any{"created": []string{}, "updated": []string{}, "totalAffected": 0}
	if existing, ok := s.readCanonicalPage(slug); ok {
		merged := mergeWikiPage(existing, inputData, now)
		if err := s.writePage(merged); err != nil {
			return errorPayload(err), true
		}
		result["updated"] = []string{slug}
		result["totalAffected"] = 1
		s.appendLog("ingest", []string{slug}, fmt.Sprintf("Updated %q with new content", title))
	} else {
		page := createWikiPage(slug, inputData, now)
		if err := s.writePage(page); err != nil {
			return errorPayload(err), true
		}
		result["created"] = []string{slug}
		result["totalAffected"] = 1
		op := "ingest"
		if rejectOverwrite {
			op = "add"
		}
		s.appendLog(op, []string{slug}, fmt.Sprintf("Created new page %q", title))
	}
	if err := s.updateIndex(); err != nil {
		return errorPayload(err), true
	}
	return result, false
}

func (s wikiStore) query(input map[string]any) (any, bool) {
	query, err := requiredString(input, "query")
	if err != nil {
		return errorPayload(err), true
	}
	filterTags := optionalStringSlice(input, "tags")
	category := stringField(input, "category")
	limit := maxInt(input["limit"], 20, 50)
	queryLower := strings.ToLower(query)
	terms := tokenizeWiki(query)
	matches := make([]map[string]any, 0)
	for _, page := range s.readAllPages() {
		if category != "" && stringField(page.Frontmatter, "category") != category {
			continue
		}
		score := 0
		snippet := ""
		pageTags := stringSliceFromAny(page.Frontmatter["tags"])
		for _, tag := range filterTags {
			for _, pageTag := range pageTags {
				if strings.EqualFold(tag, pageTag) {
					score += 3
				}
			}
		}
		for _, term := range terms {
			for _, tag := range pageTags {
				if strings.Contains(strings.ToLower(tag), term) {
					score += 2
				}
			}
		}
		titleLower := strings.ToLower(stringField(page.Frontmatter, "title"))
		if strings.Contains(titleLower, queryLower) {
			score += 5
		} else {
			for _, term := range terms {
				if strings.Contains(titleLower, term) {
					score += 2
				}
			}
		}
		contentLower := strings.ToLower(page.Content)
		for _, term := range terms {
			idx := strings.Index(contentLower, term)
			if idx == -1 {
				continue
			}
			score++
			if snippet == "" {
				start := max(0, idx-40)
				end := min(len(page.Content), idx+len(term)+80)
				snippet = strings.TrimSpace(strings.ReplaceAll(page.Content[start:end], "\n", " "))
				if start > 0 {
					snippet = "..." + snippet
				}
				if end < len(page.Content) {
					snippet += "..."
				}
			}
		}
		if score > 0 {
			if snippet == "" {
				snippet = firstNonEmptyLine(page.Content)
				if len(snippet) > 120 {
					snippet = snippet[:117] + "..."
				}
			}
			matches = append(matches, map[string]any{"page": page, "snippet": snippet, "score": score})
		}
	}
	sort.Slice(matches, func(i, j int) bool { return intField(matches[i], "score") > intField(matches[j], "score") })
	if len(matches) > limit {
		matches = matches[:limit]
	}
	if !s.legacyFallbackActive() {
		pages := make([]string, 0, len(matches))
		for _, match := range matches {
			if page, ok := match["page"].(wikiPage); ok {
				pages = append(pages, page.Filename)
			}
		}
		s.appendLog("query", pages, fmt.Sprintf("Query %q returned %d results", query, len(matches)))
	}
	return matches, false
}

func (s wikiStore) lint() (any, bool) {
	pages := s.readAllPages()
	issues := make([]map[string]any, 0)
	pageNames := map[string]struct{}{}
	incoming := map[string]int{}
	for _, page := range pages {
		pageNames[page.Filename] = struct{}{}
	}
	for _, page := range pages {
		for _, link := range stringSliceFromAny(page.Frontmatter["links"]) {
			incoming[link]++
		}
	}
	for _, page := range pages {
		if incoming[page.Filename] == 0 {
			issues = append(issues, wikiIssue(page.Filename, "info", "orphan", fmt.Sprintf("No other pages link to %q", stringField(page.Frontmatter, "title"))))
		}
		if updated := stringField(page.Frontmatter, "updated"); updated != "" {
			if stamp, err := time.Parse(time.RFC3339Nano, updated); err == nil && time.Since(stamp) > 90*24*time.Hour {
				issues = append(issues, wikiIssue(page.Filename, "warning", "stale", fmt.Sprintf("%q not updated in %d days", stringField(page.Frontmatter, "title"), int(time.Since(stamp).Hours()/24))))
			}
		}
		for _, link := range stringSliceFromAny(page.Frontmatter["links"]) {
			if _, ok := pageNames[link]; !ok {
				issues = append(issues, wikiIssue(page.Filename, "error", "broken-ref", fmt.Sprintf("Broken link to %q", link)))
			}
		}
		if stringField(page.Frontmatter, "confidence") == "low" {
			issues = append(issues, wikiIssue(page.Filename, "info", "low-confidence", fmt.Sprintf("%q has low confidence", stringField(page.Frontmatter, "title"))))
		}
		if len(page.Content) > 50_000 {
			issues = append(issues, wikiIssue(page.Filename, "warning", "oversized", fmt.Sprintf("%q is oversized", stringField(page.Frontmatter, "title"))))
		}
	}
	stats := map[string]any{
		"totalPages":         len(pages),
		"orphanCount":        countIssues(issues, "orphan"),
		"staleCount":         countIssues(issues, "stale"),
		"brokenRefCount":     countIssues(issues, "broken-ref"),
		"lowConfidenceCount": countIssues(issues, "low-confidence"),
		"oversizedCount":     countIssues(issues, "oversized"),
		"contradictionCount": 0,
	}
	if !s.legacyFallbackActive() {
		s.appendLog("lint", nil, fmt.Sprintf("Lint: %d issues", len(issues)))
	}
	return map[string]any{"issues": issues, "stats": stats}, false
}

func (s wikiStore) listPages() []string {
	entries, err := os.ReadDir(s.readableDir())
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, reserved := wikiReservedFiles[entry.Name()]; reserved {
			continue
		}
		out = append(out, entry.Name())
	}
	sort.Strings(out)
	return out
}

func (s wikiStore) readAllPages() []wikiPage {
	pages := make([]wikiPage, 0)
	for _, filename := range s.listPages() {
		if page, ok := s.readPage(filename); ok {
			pages = append(pages, page)
		}
	}
	return pages
}

func (s wikiStore) readIndex() any {
	data, err := os.ReadFile(filepath.Join(s.readableDir(), wikiIndexFile))
	if err != nil {
		return nil
	}
	return string(data)
}

func (s wikiStore) readPage(filename string) (wikiPage, bool) {
	return s.readPageFromDir(s.readableDir(), filename)
}

func (s wikiStore) readCanonicalPage(filename string) (wikiPage, bool) {
	return s.readPageFromDir(s.canonicalDir(), filename)
}

func (s wikiStore) readPageFromDir(dir, filename string) (wikiPage, bool) {
	path, ok := safeWikiPath(dir, filename)
	if !ok {
		return wikiPage{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return wikiPage{}, false
	}
	frontmatter, content, ok := parseWikiFrontmatter(string(data))
	if !ok {
		return wikiPage{}, false
	}
	return wikiPage{Filename: filename, Frontmatter: frontmatter, Content: content}, true
}

func (s wikiStore) writePage(page wikiPage) error {
	path, ok := safeWikiPath(s.canonicalDir(), page.Filename)
	if !ok {
		return fmt.Errorf("invalid wiki page filename: %s", page.Filename)
	}
	if _, reserved := wikiReservedFiles[page.Filename]; reserved {
		return fmt.Errorf("cannot write to reserved wiki file: %s", page.Filename)
	}
	return writeFileAtomic(path, []byte(serializeWikiPage(page)))
}

func (s wikiStore) deletePage(filename string) error {
	if _, reserved := wikiReservedFiles[filename]; reserved {
		return fmt.Errorf("Wiki page not found or reserved: %s", filename)
	}
	path, ok := safeWikiPath(s.canonicalDir(), filename)
	if !ok {
		return fmt.Errorf("invalid wiki page filename: %s", filename)
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Wiki page not found or reserved: %s", filename)
		}
		return err
	}
	return s.updateIndex()
}

func (s wikiStore) updateIndex() error {
	pages := s.readAllCanonicalPages()
	byCategory := map[string][]wikiPage{}
	for _, page := range pages {
		category := defaultString(stringField(page.Frontmatter, "category"), "reference")
		byCategory[category] = append(byCategory[category], page)
	}
	categories := make([]string, 0, len(byCategory))
	for category := range byCategory {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	lines := []string{"# Wiki Index", "", fmt.Sprintf("> %d pages | Last updated: %s", len(pages), nowISO()), ""}
	for _, category := range categories {
		lines = append(lines, "## "+category, "")
		for _, page := range byCategory[category] {
			summary := firstNonEmptyLine(page.Content)
			if len(summary) > 80 {
				summary = summary[:77] + "..."
			}
			lines = append(lines, fmt.Sprintf("- [%s](%s) — %s", stringField(page.Frontmatter, "title"), page.Filename, summary))
		}
		lines = append(lines, "")
	}
	return writeFileAtomic(filepath.Join(s.canonicalDir(), wikiIndexFile), []byte(strings.Join(lines, "\n")))
}

func (s wikiStore) readAllCanonicalPages() []wikiPage {
	dir := s.canonicalDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	pages := make([]wikiPage, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, reserved := wikiReservedFiles[entry.Name()]; reserved {
			continue
		}
		if page, ok := s.readCanonicalPage(entry.Name()); ok {
			pages = append(pages, page)
		}
	}
	return pages
}

func (s wikiStore) appendLog(operation string, pages []string, summary string) {
	path := filepath.Join(s.canonicalDir(), wikiLogFile)
	existing, _ := readTextFileOrEmpty(path)
	if existing == "" {
		existing = "# Wiki Log\n\n"
	}
	line := fmt.Sprintf("## [%s] %s\n- **Pages:** %s\n- **Summary:** %s\n\n", nowISO(), operation, defaultString(strings.Join(pages, ", "), "none"), summary)
	_ = writeFileAtomic(path, []byte(existing+line))
}

func createWikiPage(filename string, input map[string]any, now string) wikiPage {
	content := stringField(input, "content")
	frontmatter := map[string]any{
		"title":         stringField(input, "title"),
		"tags":          input["tags"],
		"created":       now,
		"updated":       now,
		"sources":       input["sources"],
		"links":         extractWikiLinks(content),
		"category":      input["category"],
		"confidence":    input["confidence"],
		"schemaVersion": wikiSchemaVersion,
	}
	return wikiPage{Filename: filename, Frontmatter: frontmatter, Content: "\n# " + stringField(input, "title") + "\n\n" + content + "\n"}
}

func mergeWikiPage(existing wikiPage, input map[string]any, now string) wikiPage {
	frontmatter := cloneMap(existing.Frontmatter)
	frontmatter["tags"] = uniqueStrings(append(stringSliceFromAny(frontmatter["tags"]), stringSliceFromAny(input["tags"])...))
	frontmatter["sources"] = uniqueStrings(append(stringSliceFromAny(frontmatter["sources"]), stringSliceFromAny(input["sources"])...))
	frontmatter["links"] = uniqueStrings(append(stringSliceFromAny(frontmatter["links"]), extractWikiLinks(stringField(input, "content"))...))
	frontmatter["updated"] = now
	if confidenceRank(stringField(input, "confidence")) >= confidenceRank(stringField(frontmatter, "confidence")) {
		frontmatter["confidence"] = input["confidence"]
	}
	content := strings.TrimRight(existing.Content, "\n") + "\n\n---\n\n## Update (" + now + ")\n\n" + stringField(input, "content") + "\n"
	return wikiPage{Filename: existing.Filename, Frontmatter: frontmatter, Content: content}
}

func parseWikiFrontmatter(raw string) (map[string]any, string, bool) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, "", false
	}
	end := strings.Index(normalized[4:], "\n---\n")
	if end < 0 {
		return nil, "", false
	}
	yamlBlock := normalized[4 : 4+end]
	content := normalized[4+end+5:]
	frontmatter := map[string]any{
		"tags":          []string{},
		"sources":       []string{},
		"links":         []string{},
		"category":      "reference",
		"confidence":    "medium",
		"schemaVersion": wikiSchemaVersion,
	}
	for line := range strings.SplitSeq(yamlBlock, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		switch key {
		case "tags", "sources", "links":
			frontmatter[key] = parseWikiArray(value)
		case "schemaVersion":
			frontmatter[key] = wikiSchemaVersion
		default:
			frontmatter[key] = value
		}
	}
	return frontmatter, content, true
}

func serializeWikiPage(page wikiPage) string {
	fm := page.Frontmatter
	lines := []string{
		"---",
		fmt.Sprintf("title: %q", stringField(fm, "title")),
		"tags: [" + quoteJoin(stringSliceFromAny(fm["tags"])) + "]",
		"created: " + stringField(fm, "created"),
		"updated: " + stringField(fm, "updated"),
		"sources: [" + quoteJoin(stringSliceFromAny(fm["sources"])) + "]",
		"links: [" + quoteJoin(stringSliceFromAny(fm["links"])) + "]",
		"category: " + defaultString(stringField(fm, "category"), "reference"),
		"confidence: " + defaultString(stringField(fm, "confidence"), "medium"),
		fmt.Sprintf("schemaVersion: %d", wikiSchemaVersion),
		"---",
		page.Content,
	}
	return strings.Join(lines, "\n")
}

func titleToWikiSlug(title string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen && b.Len() > 0 {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	base := strings.Trim(b.String(), "-")
	if len([]rune(base)) > 64 {
		base = string([]rune(base)[:64])
		base = strings.Trim(base, "-")
	}
	if base == "" {
		digest := sha256.Sum256([]byte(title))
		base = "page-" + hex.EncodeToString(digest[:4])
	}
	return base + ".md"
}

func normalizeWikiPageName(page string) string {
	page = strings.TrimSpace(page)
	if strings.HasSuffix(page, ".md") {
		return page
	}
	return page + ".md"
}

func safeWikiPath(dir, filename string) (string, bool) {
	if filename == "" || strings.Contains(filename, "..") || strings.ContainsAny(filename, `/\\`) {
		return "", false
	}
	path := filepath.Join(dir, filename)
	cleanDir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(cleanDir, cleanPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return cleanPath, true
}

func extractWikiLinks(content string) []string {
	matches := wikiLinkPattern.FindAllStringSubmatch(content, -1)
	links := make([]string, 0, len(matches))
	for _, match := range matches {
		links = append(links, titleToWikiSlug(strings.TrimSpace(match[1])))
	}
	return uniqueStrings(links)
}

func tokenizeWiki(text string) []string {
	lower := strings.ToLower(text)
	tokens := make([]string, 0)
	var word strings.Builder
	flushWord := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}
	var cjk []rune
	flushCJK := func() {
		for i, r := range cjk {
			tokens = append(tokens, string(r))
			if i+1 < len(cjk) {
				tokens = append(tokens, string(cjk[i:i+2]))
			}
		}
		cjk = nil
	}
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if isCJK(r) {
				flushWord()
				cjk = append(cjk, r)
			} else {
				flushCJK()
				word.WriteRune(r)
			}
			continue
		}
		flushWord()
		flushCJK()
	}
	flushWord()
	flushCJK()
	return uniqueStrings(tokens)
}

func isCJK(r rune) bool {
	return (r >= 0x3040 && r <= 0x30FF) || (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0xAC00 && r <= 0xD7AF)
}

func parseWikiArray(value string) []string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func quoteJoin(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return strings.Join(quoted, ", ")
}

func stringSliceFromAny(value any) []string {
	switch got := value.(type) {
	case []string:
		return append([]string(nil), got...)
	case []any:
		out := make([]string, 0, len(got))
		for _, item := range got {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func confidenceRank(value string) int {
	switch value {
	case "high":
		return 3
	case "low":
		return 1
	default:
		return 2
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmptyLine(content string) string {
	for line := range strings.SplitSeq(content, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func wikiIssue(page, severity, issueType, message string) map[string]any {
	return map[string]any{"page": page, "severity": severity, "type": issueType, "message": message}
}

func countIssues(issues []map[string]any, issueType string) int {
	count := 0
	for _, issue := range issues {
		if stringField(issue, "type") == issueType {
			count++
		}
	}
	return count
}

func intField(input map[string]any, key string) int {
	switch got := input[key].(type) {
	case int:
		return got
	case float64:
		return int(got)
	default:
		return 0
	}
}
