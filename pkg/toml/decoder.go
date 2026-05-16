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

package toml

import (
	"io"
	"strings"

	"github.com/zchee/pandaemonium/pkg/toml/internal/scan"
)

const (
	// DefaultMaxNestedDepth is the container nesting cap.
	DefaultMaxNestedDepth = 1024
	// DefaultMaxKeyLength is the default maximum key length in bytes.
	DefaultMaxKeyLength = 64 * 1024
	// DefaultMaxStringLength is the default maximum string length in bytes.
	DefaultMaxStringLength = 16 << 20
	// DefaultMaxDocumentSize is the default maximum reader input size.
	DefaultMaxDocumentSize = 16 << 20
	// MaxKeyLength is the default key-length cap used by Decoder.
	MaxKeyLength = DefaultMaxKeyLength
)

// Option modifies decoder construction behavior.
type Option func(*Decoder)

// Limits are the parser DoS caps for a Decoder.
type Limits struct {
	MaxNestedDepth  int
	MaxKeyLength    int
	MaxStringLength int
	MaxDocumentSize int
}

// WithLimits configures DoS caps for the returned Decoder.
func WithLimits(l Limits) Option {
	return func(d *Decoder) {
		if l.MaxNestedDepth > 0 {
			d.limits.MaxNestedDepth = l.MaxNestedDepth
		}
		if l.MaxKeyLength > 0 {
			d.limits.MaxKeyLength = l.MaxKeyLength
		}
		if l.MaxStringLength > 0 {
			d.limits.MaxStringLength = l.MaxStringLength
		}
		if l.MaxDocumentSize > 0 {
			d.limits.MaxDocumentSize = l.MaxDocumentSize
		}
	}
}

// Decoder reads bytes as a token stream.
//
// `Token.Bytes` aliases source bytes for NewDecoderBytes and aliases an internal
// mutable buffer for NewDecoder. Aliasing is valid until the next ReadToken call.
// Copy if callers need persistence across the next token.
//
// BOM handling: a UTF-8 BOM at the first token position is skipped.
type Decoder struct {
	buf            []byte
	off            int
	line           int
	col            int
	err            error
	expectingValue bool
	atLineStart    bool

	limits      Limits
	arrayDepth  int
	inlineDepth int
}

// NewDecoder creates a Decoder over an io.Reader input.
func NewDecoder(r io.Reader, opts ...Option) *Decoder {
	b, err := io.ReadAll(r)
	d := &Decoder{
		off:         0,
		line:        1,
		col:         1,
		atLineStart: true,
		limits: Limits{
			MaxNestedDepth:  DefaultMaxNestedDepth,
			MaxKeyLength:    DefaultMaxKeyLength,
			MaxStringLength: DefaultMaxStringLength,
			MaxDocumentSize: DefaultMaxDocumentSize,
		},
	}
	for _, opt := range opts {
		opt(d)
	}
	if d.err != nil {
		return d
	}

	d.buf = b
	if err != nil {
		d.err = err
		return d
	}
	if d.limits.MaxDocumentSize > 0 && len(d.buf) > d.limits.MaxDocumentSize {
		d.err = &LimitError{Limit: "MaxDocumentSize", Value: d.limits.MaxDocumentSize, Span: [2]int{0, d.limits.MaxDocumentSize}}
		return d
	}

	if bad := scan.ValidateUTF8(d.buf); bad != len(d.buf) {
		d.err = &SyntaxError{Line: 1, Col: bad + 1, Msg: "invalid utf-8", Span: [2]int{bad, bad + 1}}
		return d
	}
	d.skipBOM()
	return d
}

// NewDecoderBytes creates a Decoder over an in-memory TOML payload.
func NewDecoderBytes(data []byte, opts ...Option) *Decoder {
	d := &Decoder{
		off:         0,
		line:        1,
		col:         1,
		atLineStart: true,
		buf:         data,
		limits: Limits{
			MaxNestedDepth:  DefaultMaxNestedDepth,
			MaxKeyLength:    DefaultMaxKeyLength,
			MaxStringLength: DefaultMaxStringLength,
			MaxDocumentSize: DefaultMaxDocumentSize,
		},
	}
	for _, opt := range opts {
		opt(d)
	}
	if d.limits.MaxDocumentSize > 0 && len(d.buf) > d.limits.MaxDocumentSize {
		d.err = &LimitError{Limit: "MaxDocumentSize", Value: d.limits.MaxDocumentSize, Span: [2]int{0, d.limits.MaxDocumentSize}}
		return d
	}
	if bad := scan.ValidateUTF8(d.buf); bad != len(d.buf) {
		d.err = &SyntaxError{Line: 1, Col: bad + 1, Msg: "invalid utf-8", Span: [2]int{bad, bad + 1}}
		return d
	}
	d.skipBOM()
	return d
}

// ReadToken returns the next token in the stream.
//
// The return contract is fail-fast: the first SyntaxError or LimitError enters
// sticky state and is returned until the decoder is replaced.
func (d *Decoder) ReadToken() (Token, error) {
	if d == nil {
		return Token{}, io.ErrUnexpectedEOF
	}
	if d.err != nil {
		return Token{}, d.err
	}

	for {
		if d.off >= len(d.buf) {
			if d.expectingValue {
				return Token{}, d.syntaxError("expected value", d.off)
			}
			return Token{}, io.EOF
		}

		d.skipSpaces()
		if d.off >= len(d.buf) {
			if d.expectingValue {
				return Token{}, d.syntaxError("expected value", d.off)
			}
			return Token{}, io.EOF
		}

		if d.atLineStart && !d.expectingValue {
			if d.matchPrefix("[[") {
				return d.scanArrayTableHeader()
			}
			if d.buf[d.off] == '[' {
				return d.scanTableHeader()
			}
		}

		ch := d.buf[d.off]
		switch ch {
		case '[':
			return d.scanArrayStart()
		case '{':
			return d.scanInlineTableStart()
		case ']':
			return d.scanArrayEnd()
		case '}':
			return d.scanInlineTableEnd()
		case '#':
			return d.scanComment()
		case '=':
			d.advanceOne()
			d.expectingValue = true
			continue
		case ',':
			d.advanceOne()
			if d.arrayDepth > 0 || d.inlineDepth > 0 {
				d.expectingValue = true
			}
			continue
		default:
			if d.expectingValue {
				return d.scanValueToken()
			}
			return d.scanKeyToken()
		}
	}
}

func (d *Decoder) skipSpaces() {
	for d.off < len(d.buf) {
		rem := d.buf[d.off:]
		n := scan.SkipWhitespace(rem)
		if n > 0 {
			d.advanceBytes(rem[:n])
			if n > 0 && d.atLineStart {
				// still at the beginning of line until first non-space.
				d.atLineStart = true
			}
			continue
		}

		if len(rem) > 0 && rem[0] == '\r' {
			if len(rem) >= 2 && rem[1] == '\n' {
				d.advanceBytes(rem[:2])
				d.atLineStart = true
				continue
			}
		}
		if len(rem) > 0 && rem[0] == '\n' {
			d.advanceBytes(rem[:1])
			d.atLineStart = true
			continue
		}
		if len(rem) > 0 && rem[0] == '#' {
			return
		}
		return
	}
}

func (d *Decoder) scanComment() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	end := len(d.buf)
	for i := start; i < len(d.buf); i++ {
		if d.buf[i] == '\n' {
			end = i
			break
		}
		if d.buf[i] == '\r' && i+1 < len(d.buf) && d.buf[i+1] == '\n' {
			end = i
			break
		}
	}
	d.advanceBytes(d.buf[start:end])
	return Token{Kind: TokenKindComment, Bytes: d.buf[start:end], Line: line, Col: col}, nil
}

func (d *Decoder) scanKeyToken() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	i := start
	for i < len(d.buf) {
		ch := d.buf[i]
		if ch == '#' {
			break
		}
		if ch == '=' {
			break
		}
		if ch == '\n' || ch == '\r' {
			break
		}
		if ch == ' ' || ch == '\t' {
			break
		}
		if ch == '"' || ch == '\'' {
			j, err := d.scanQuoted(ch, i)
			if err != nil {
				return Token{}, err
			}
			i = j
			continue
		}
		if ch == '.' {
			i++
			continue
		}
		n := scan.ScanBareKey(d.buf[i:])
		if n == 0 {
			return Token{}, d.syntaxError("unexpected token in key", i)
		}
		i += n
		if i < len(d.buf) && d.buf[i] == '.' {
			continue
		}
		if i < len(d.buf) && (d.buf[i] == ' ' || d.buf[i] == '\t') {
			break
		}
	}
	if i == start {
		return Token{}, d.syntaxError("empty key", i)
	}

	key := d.buf[start:i]
	if len(key) > d.limits.MaxKeyLength {
		err := &LimitError{Limit: "MaxKeyLength", Value: d.limits.MaxKeyLength, Span: [2]int{start, i}}
		d.setErr(err)
		return Token{}, err
	}
	d.advanceBytes(key)
	d.expectingValue = true
	return Token{Kind: TokenKindKey, Bytes: key, Line: line, Col: col}, nil
}

func (d *Decoder) scanValueToken() (Token, error) {
	start := d.off
	line, col := d.line, d.col

	ch := d.buf[start]
	switch ch {
	case '"', '\'':
		end, kind, err := d.scanString(start)
		if err != nil {
			return Token{}, err
		}
		chunk := d.buf[start:end]
		if len(chunk) > d.limits.MaxStringLength {
			err := &LimitError{Limit: "MaxStringLength", Value: d.limits.MaxStringLength, Span: [2]int{start, end}}
			d.setErr(err)
			return Token{}, err
		}
		d.advanceBytes(chunk)
		d.expectingValue = false
		if d.arrayDepth > 0 || d.inlineDepth > 0 {
			d.expectingValue = true
		}
		return Token{Kind: kind, Bytes: chunk, Line: line, Col: col}, nil
	case '[':
		return d.scanArrayStart()
	case '{':
		return d.scanInlineTableStart()
	case ']':
		return d.scanArrayEnd()
	case '}':
		return d.scanInlineTableEnd()
	case '#':
		return d.scanComment()
	}

	end := scanUntilDelimiter(d.buf[start:])
	if end == 0 {
		return Token{}, d.syntaxError("invalid value", start)
	}
	end += start
	chunk := d.buf[start:end]
	if len(chunk) == 0 {
		return Token{}, d.syntaxError("expected value", start)
	}
	norm := strings.ToLower(string(chunk))
	kind := TokenKindValueInteger
	clean := strings.ReplaceAll(norm, "_", "")
	switch {
	case strings.ContainsAny(clean, "= "):
		return Token{}, d.syntaxError("unexpected = in value", start)
	case norm == "true" || norm == "false":
		kind = TokenKindValueBool
	case looksLikeDatetime(chunk):
		kind = TokenKindValueDatetime
	case looksLikeFloat([]byte(clean)):
		if !isFloatLiteral(clean) {
			return Token{}, d.syntaxError("malformed float", start)
		}
		kind = TokenKindValueFloat
	case isIntLiteral(clean):
		kind = TokenKindValueInteger
	case strings.IndexFunc(clean, func(r rune) bool { return (r < '0' || r > '9') && r != '+' && r != '-' && r != '.' }) != -1:
		return Token{}, d.syntaxError("malformed value", start)
	default:
		kind = TokenKindValueInteger
	}
	d.advanceBytes(chunk)
	d.expectingValue = false
	if d.arrayDepth > 0 || d.inlineDepth > 0 {
		d.expectingValue = true
	}
	return Token{Kind: kind, Bytes: chunk, Line: line, Col: col}, nil
}

func (d *Decoder) scanArrayStart() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	d.advanceOne()
	d.arrayDepth++
	d.enforceNestedDepth(start, d.arrayDepth)
	d.expectingValue = true
	return Token{Kind: TokenKindArrayStart, Bytes: d.buf[start : start+1], Line: line, Col: col}, nil
}

func (d *Decoder) scanArrayEnd() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	d.advanceOne()
	if d.arrayDepth > 0 {
		d.arrayDepth--
	}
	d.expectingValue = false
	if d.arrayDepth > 0 || d.inlineDepth > 0 {
		d.expectingValue = true
	}
	return Token{Kind: TokenKindArrayEnd, Bytes: d.buf[start : start+1], Line: line, Col: col}, nil
}

func (d *Decoder) scanInlineTableStart() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	d.advanceOne()
	d.inlineDepth++
	d.expectingValue = false
	return Token{Kind: TokenKindInlineTableStart, Bytes: d.buf[start : start+1], Line: line, Col: col}, nil
}

func (d *Decoder) scanInlineTableEnd() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	d.advanceOne()
	if d.inlineDepth > 0 {
		d.inlineDepth--
	}
	d.expectingValue = d.inlineDepth > 0
	if !d.expectingValue {
		d.expectingValue = false
	}
	return Token{Kind: TokenKindInlineTableEnd, Bytes: d.buf[start : start+1], Line: line, Col: col}, nil
}

func (d *Decoder) scanTableHeader() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	i, err := d.scanHeaderEnd(start+1, false)
	if err != nil {
		return Token{}, err
	}
	if err := d.validateHeaderKey(start+1, i); err != nil {
		return Token{}, err
	}
	tokenEnd := i + 1
	head := d.buf[start:tokenEnd]
	d.advanceBytes(head)
	keyLen := tokenEnd - start - 2
	if keyLen < 0 {
		keyLen = 0
	}
	if keyLen > d.limits.MaxKeyLength {
		err := &LimitError{Limit: "MaxKeyLength", Value: d.limits.MaxKeyLength, Span: [2]int{start + 1, i}}
		d.setErr(err)
		return Token{}, err
	}
	d.expectingValue = false
	d.atLineStart = false
	return Token{Kind: TokenKindTableHeader, Bytes: head, Line: line, Col: col}, nil
}

func (d *Decoder) scanArrayTableHeader() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	i, err := d.scanHeaderEnd(start+2, true)
	if err != nil {
		return Token{}, err
	}
	if err := d.validateHeaderKey(start+2, i); err != nil {
		return Token{}, err
	}
	tokenEnd := i + 2
	head := d.buf[start:tokenEnd]
	d.advanceBytes(head)
	keyLen := tokenEnd - start - 4
	if keyLen < 0 {
		keyLen = 0
	}
	if keyLen > d.limits.MaxKeyLength {
		err := &LimitError{Limit: "MaxKeyLength", Value: d.limits.MaxKeyLength, Span: [2]int{start + 2, i}}
		d.setErr(err)
		return Token{}, err
	}
	d.expectingValue = false
	d.atLineStart = false
	return Token{Kind: TokenKindArrayTableHeader, Bytes: head, Line: line, Col: col}, nil
}

func (d *Decoder) scanHeaderEnd(i int, array bool) (int, error) {
	for i < len(d.buf) {
		ch := d.buf[i]
		if ch == '\n' || ch == '\r' {
			if array {
				return 0, d.syntaxError("unterminated array-of-tables header", i)
			}
			return 0, d.syntaxError("unterminated table header", i)
		}
		if ch == '"' || ch == '\'' {
			j, err := d.scanQuoted(ch, i)
			if err != nil {
				return 0, err
			}
			i = j
			continue
		}
		if ch == ']' {
			if !array {
				return i, nil
			}
			if i+1 < len(d.buf) && d.buf[i+1] == ']' {
				return i, nil
			}
		}
		i++
	}
	if array {
		return 0, d.syntaxError("unterminated array-of-tables header", len(d.buf))
	}
	return 0, d.syntaxError("unterminated table header", len(d.buf))
}

func (d *Decoder) validateHeaderKey(start, end int) error {
	i := start
	needSegment := true
	for {
		for i < end && (d.buf[i] == ' ' || d.buf[i] == '\t') {
			i++
		}
		if i >= end {
			if needSegment {
				return d.syntaxError("empty table key", end)
			}
			return nil
		}
		if !needSegment {
			if d.buf[i] != '.' {
				return d.syntaxError("unexpected token in table key", i)
			}
			i++
			needSegment = true
			continue
		}
		if d.buf[i] == '"' || d.buf[i] == '\'' {
			j, err := d.scanQuoted(d.buf[i], i)
			if err != nil {
				return err
			}
			if j > end {
				return d.syntaxError("unterminated quoted table key", i)
			}
			i = j
		} else {
			n := scan.ScanBareKey(d.buf[i:end])
			if n == 0 {
				return d.syntaxError("unexpected token in table key", i)
			}
			i += n
		}
		needSegment = false
	}
}

func (d *Decoder) scanQuoted(quote byte, off int) (int, error) {
	if off+2 < len(d.buf) && d.buf[off+1] == quote && d.buf[off+2] == quote {
		return 0, d.syntaxError("multiline quoted keys are not supported", off)
	}
	i := off + 1
	for i < len(d.buf) {
		var idx int
		if quote == '\'' {
			idx = scan.ScanLiteralString(d.buf[i:])
		} else {
			idx = scan.ScanBasicString(d.buf[i:])
		}
		if hasNewlineBefore(d.buf[i:], idx) {
			return 0, d.syntaxError("unterminated quoted key", off)
		}
		if idx >= len(d.buf[i:]) {
			return 0, d.syntaxError("unterminated quoted key", off)
		}
		if quote == '"' && d.buf[i+idx] == '\\' {
			if i+idx+1 >= len(d.buf) {
				return 0, d.syntaxError("unterminated quoted key", off)
			}
			i += idx + 2
			continue
		}
		if d.buf[i+idx] == quote {
			return i + idx + 1, nil
		}
	}
	return 0, d.syntaxError("unterminated quoted key", off)
}

func (d *Decoder) scanString(off int) (int, TokenKind, error) {
	rest := d.buf[off:]
	if strings.HasPrefix(string(rest), "\"\"\"") {
		end := off + 3
		for end+2 < len(d.buf) {
			if d.buf[end] == '"' && d.buf[end+1] == '"' && d.buf[end+2] == '"' {
				return end + 3, TokenKindValueString, nil
			}
			end++
		}
		return 0, TokenKindInvalid, d.syntaxError("unterminated multiline string", off)
	}
	if strings.HasPrefix(string(rest), "'''") {
		end := off + 3
		for end+2 < len(d.buf) {
			if d.buf[end] == '\'' && d.buf[end+1] == '\'' && d.buf[end+2] == '\'' {
				return end + 3, TokenKindValueString, nil
			}
			end++
		}
		return 0, TokenKindInvalid, d.syntaxError("unterminated multiline string", off)
	}
	quote := d.buf[off]
	i := off + 1
	for i < len(d.buf) {
		var idx int
		if quote == '\'' {
			idx = scan.ScanLiteralString(d.buf[i:])
		} else {
			idx = scan.ScanBasicString(d.buf[i:])
		}
		if hasNewlineBefore(d.buf[i:], idx) {
			return 0, TokenKindInvalid, d.syntaxError("unterminated string", off)
		}
		if idx >= len(d.buf[i:]) {
			return 0, TokenKindInvalid, d.syntaxError("unterminated string", off)
		}
		if quote == '"' && d.buf[i+idx] == '\\' {
			if i+idx+1 >= len(d.buf) {
				return 0, TokenKindInvalid, d.syntaxError("unterminated string", off)
			}
			i += idx + 2
			continue
		}
		return i + idx + 1, TokenKindValueString, nil
	}
	return 0, TokenKindInvalid, d.syntaxError("unterminated string", off)
}

func hasNewlineBefore(raw []byte, end int) bool {
	if end > len(raw) {
		end = len(raw)
	}
	for _, ch := range raw[:end] {
		if ch == '\n' || ch == '\r' {
			return true
		}
	}
	return false
}

func (d *Decoder) enforceNestedDepth(off int, depth int) {
	if d.limits.MaxNestedDepth > 0 && depth > d.limits.MaxNestedDepth {
		err := &LimitError{Limit: "MaxNestedDepth", Value: d.limits.MaxNestedDepth, Span: [2]int{off, off + 1}}
		d.setErr(err)
	}
}

func (d *Decoder) setErr(err error) {
	d.err = err
}

func (d *Decoder) syntaxError(msg string, off int) *SyntaxError {
	line, col := d.computeLineCol(off)
	err := &SyntaxError{Line: line, Col: col, Msg: msg, Span: [2]int{off, off + 1}}
	d.err = err
	return err
}

func (d *Decoder) computeLineCol(off int) (int, int) {
	if off <= d.off {
		return d.line, d.col
	}
	line := 1
	col := 1
	for i := 0; i < off && i < len(d.buf); i++ {
		if d.buf[i] == '\n' {
			line++
			col = 1
			continue
		}
		if d.buf[i] == '\r' {
			continue
		}
		col++
	}
	return line, col
}

func (d *Decoder) advanceOne() {
	d.advanceBytes(d.buf[d.off : d.off+1])
}

func (d *Decoder) advanceBytes(raw []byte) {
	if len(raw) == 0 {
		return
	}
	consumed := len(raw)
	for i := 0; i < consumed; {
		n := scan.LocateNewline(raw[i:])
		if n < 0 {
			d.col += consumed - i
			break
		}
		if n > 0 {
			d.col += n
		}
		i += n + 1
		d.line++
		d.col = 1
		if i < consumed && raw[i-1] == '\r' {
			// CRLF counts as a single line break and keeps column at 1.
			d.col = 1
		}
	}
	d.off += consumed
	d.atLineStart = false
	if d.off >= len(d.buf) {
		d.atLineStart = false
	}
}

func (d *Decoder) matchPrefix(prefix string) bool {
	if len(prefix) == 0 {
		return false
	}
	p := []byte(prefix)
	if d.off+len(p) > len(d.buf) {
		return false
	}
	for i := range p {
		if d.buf[d.off+i] != p[i] {
			return false
		}
	}
	return true
}

func (d *Decoder) skipBOM() {
	if len(d.buf)-d.off >= 3 && d.buf[d.off] == 0xEF && d.buf[d.off+1] == 0xBB && d.buf[d.off+2] == 0xBF {
		d.advanceBytes(d.buf[d.off : d.off+3])
	}
}

func looksLikeFloat(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	s := string(raw)
	if strings.Count(s, ".") > 1 {
		return false
	}
	if strings.Count(s, "e") > 1 || strings.Count(s, "E") > 1 {
		return false
	}
	return strings.ContainsAny(s, ".eE")
}

func looksLikeDatetime(raw []byte) bool {
	s := string(raw)
	return len(s) >= 10 && s[4] == '-' && s[7] == '-' && strings.Contains(s, "T")
}

func scanUntilDelimiter(raw []byte) int {
	for i, ch := range raw {
		switch ch {
		case ' ', '\t', '\r', '\n', ',', ']', '}', '#', '=':
			return i
		}
	}
	return len(raw)
}

func isIntLiteral(raw string) bool {
	if raw == "" {
		return false
	}
	n := strings.TrimPrefix(raw, "+")
	n = strings.TrimPrefix(n, "-")
	if n == "" {
		return false
	}
	for _, r := range n {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isFloatLiteral(raw string) bool {
	if raw == "" {
		return false
	}
	n := strings.TrimPrefix(raw, "+")
	n = strings.TrimPrefix(n, "-")
	if n == "" {
		return false
	}
	for _, r := range n {
		if (r >= '0' && r <= '9') || r == '.' || r == 'e' || r == 'E' || r == '+' || r == '-' {
			continue
		}
		return false
	}
	return true
}
