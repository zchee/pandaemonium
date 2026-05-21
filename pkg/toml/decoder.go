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
	"strconv"
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

// WithLocalAsUTC allows local TOML datetime forms to decode into time.Time as UTC.
func WithLocalAsUTC() Option {
	return func(d *Decoder) {
		d.localAsUTC = true
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
	decodeStart    int
	line           int
	col            int
	err            error
	decoded        bool
	expectingValue bool
	valueNoNewline bool
	needSeparator  bool
	needLineEnd    bool
	atLineStart    bool

	limits         Limits
	arrayDepth     int
	inlineDepth    int
	containerStack []byte
	localAsUTC     bool
}

// containerArray and containerInline mark entries pushed onto containerStack
// to distinguish array context (where `,` introduces a value) from inline
// table context (where `,` introduces a key).
const (
	containerArray  byte = 'a'
	containerInline byte = 'i'
)

// innermostIsArray reports whether the innermost open container is an array.
// Top-level (no open container) returns false.
func (d *Decoder) innermostIsArray() bool {
	n := len(d.containerStack)
	if n == 0 {
		return false
	}
	return d.containerStack[n-1] == containerArray
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
	d.decodeStart = d.off
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
	d.decodeStart = d.off
	return d
}

// Decode decodes the decoder's TOML document into dst.
//
// Decode is a whole-document convenience over Unmarshal for callers that build a
// decoder from an io.Reader. It is only valid before any ReadToken call consumes
// the token stream; mixed token reads and value binding on the same decoder are
// rejected with *DecoderStateError. Decode is not a multi-document streaming API.
func (d *Decoder) Decode(dst any) error {
	if d == nil {
		return io.ErrUnexpectedEOF
	}
	if d.err != nil {
		return d.err
	}
	if d.decoded || d.off != d.decodeStart || d.expectingValue || d.arrayDepth != 0 || d.inlineDepth != 0 || len(d.containerStack) != 0 {
		return &DecoderStateError{Offset: d.off}
	}
	if err := unmarshalWithOptions(d.buf, dst, UnmarshalOptions{DecoderOptions: d.decodeOptions()}); err != nil {
		return err
	}
	d.decoded = true
	d.off = len(d.buf)
	return nil
}

func (d *Decoder) decodeOptions() []Option {
	opts := []Option{WithLimits(d.limits)}
	if d.localAsUTC {
		opts = append(opts, WithLocalAsUTC())
	}
	return opts
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

		if d.needLineEnd {
			switch d.buf[d.off] {
			case '#':
				return d.scanComment()
			case '\n', '\r':
				continue
			default:
				return Token{}, d.syntaxError("expected newline", d.off)
			}
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
			if d.innermostIsArray() && !d.expectingValue {
				return Token{}, d.syntaxError("expected comma", d.off)
			}
			return d.scanArrayStart()
		case '{':
			if d.innermostIsArray() && !d.expectingValue {
				return Token{}, d.syntaxError("expected comma", d.off)
			}
			return d.scanInlineTableStart()
		case ']':
			return d.scanArrayEnd()
		case '}':
			return d.scanInlineTableEnd()
		case '#':
			return d.scanComment()
		case '=':
			return Token{}, d.syntaxError("unexpected equals", d.off)
		case ',':
			if !d.needSeparator {
				return Token{}, d.syntaxError("unexpected comma", d.off)
			}
			d.advanceOne()
			d.needSeparator = false
			if d.innermostIsArray() {
				d.expectingValue = true
			} else {
				d.expectingValue = false
			}
			continue
		default:
			if d.expectingValue {
				if d.needSeparator && d.innermostIsArray() {
					return Token{}, d.syntaxError("expected array separator", d.off)
				}
				return d.scanValueToken()
			}
			if d.needSeparator && len(d.containerStack) > 0 {
				return Token{}, d.syntaxError("expected inline table separator", d.off)
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
			wasAtLineStart := d.atLineStart
			d.advanceBytes(rem[:n])
			if wasAtLineStart {
				// still at the beginning of line until first non-space.
				d.atLineStart = true
			}
			continue
		}

		if len(rem) > 0 && rem[0] == '\r' {
			if len(rem) >= 2 && rem[1] == '\n' {
				if d.valueNoNewline {
					return
				}
				d.advanceBytes(rem[:2])
				d.atLineStart = true
				d.needLineEnd = false
				continue
			}
		}
		if len(rem) > 0 && rem[0] == '\n' {
			if d.valueNoNewline {
				return
			}
			d.advanceBytes(rem[:1])
			d.atLineStart = true
			d.needLineEnd = false
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
		if i > start && (d.buf[i] == 0x7f || (d.buf[i] < 0x20 && d.buf[i] != '\t' && d.buf[i] != '\n' && d.buf[i] != '\r')) {
			return Token{}, d.syntaxError("control character in comment", i)
		}
		if d.buf[i] == '\n' {
			end = i
			break
		}
		if d.buf[i] == '\r' && i+1 < len(d.buf) && d.buf[i+1] == '\n' {
			end = i
			break
		}
		if d.buf[i] == '\r' {
			return Token{}, d.syntaxError("control character in comment", i)
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
		if ch == '=' {
			break
		}
		if ch == '#' || ch == '\n' || ch == '\r' {
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
		i++
	}
	if i == start {
		return Token{}, d.syntaxError("empty key", i)
	}

	key := bytesTrimRightSpaces(d.buf[start:i])
	if !isSimpleBareKey(key) {
		if _, err := parseDottedKey(key); err != nil {
			d.setErr(err)
			return Token{}, err
		}
	}
	if len(key) > d.limits.MaxKeyLength {
		err := &LimitError{Limit: "MaxKeyLength", Value: d.limits.MaxKeyLength, Span: [2]int{start, i}}
		d.setErr(err)
		return Token{}, err
	}
	eq := i
	for eq < len(d.buf) && (d.buf[eq] == ' ' || d.buf[eq] == '\t') {
		eq++
	}
	if eq >= len(d.buf) || d.buf[eq] != '=' {
		return Token{}, d.syntaxError("expected equals", eq)
	}
	d.advanceBytes(d.buf[start : eq+1])
	d.expectingValue = true
	d.valueNoNewline = true
	d.needLineEnd = false
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
		d.valueNoNewline = false
		if d.innermostIsArray() {
			d.expectingValue = true
		}
		d.needSeparator = len(d.containerStack) > 0
		d.needLineEnd = len(d.containerStack) == 0
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

	end := scanBareValueEnd(d.buf[start:])
	if end == 0 {
		return Token{}, d.syntaxError("invalid value", start)
	}
	end += start
	chunk := d.buf[start:end]
	if len(chunk) == 0 {
		return Token{}, d.syntaxError("expected value", start)
	}
	raw := string(chunk)
	norm := strings.ToLower(raw)
	clean := strings.ReplaceAll(norm, "_", "")
	var kind TokenKind
	switch {
	case looksLikeDatetime(chunk):
		kind = TokenKindValueDatetime
	case strings.ContainsAny(clean, "= "):
		return Token{}, d.syntaxError("unexpected = in value", start)
	case raw == "true" || raw == "false":
		kind = TokenKindValueBool
		if clean == "" || clean == "+" || clean == "-" {
			return Token{}, d.syntaxError("malformed value", start)
		}
	case isSpecialFloat(raw):
		kind = TokenKindValueFloat
	case isIntCandidate(norm):
		if hasCapitalNumericPrefix(raw) {
			return Token{}, d.syntaxError("malformed value", start)
		}
		if _, err := parseIntegerLiteral(chunk); err != nil {
			return Token{}, d.syntaxError("malformed value", start)
		}
		kind = TokenKindValueInteger
	case isFloatCandidate(norm):
		if _, err := strconv.ParseFloat(clean, 64); err != nil {
			return Token{}, d.syntaxError("malformed float", start)
		}
		kind = TokenKindValueFloat
	case strings.IndexFunc(norm, func(r rune) bool {
		return (r < '0' || r > '9') && r != '+' && r != '-' && r != '.' && r != 'e' && r != 'E' && r != '_' && r != ':'
	}) != -1:
		return Token{}, d.syntaxError("malformed value", start)
	default:
		return Token{}, d.syntaxError("malformed value", start)
	}
	d.advanceBytes(chunk)
	d.expectingValue = false
	d.valueNoNewline = false
	if d.innermostIsArray() {
		d.expectingValue = true
	}
	d.needSeparator = len(d.containerStack) > 0
	d.needLineEnd = len(d.containerStack) == 0
	return Token{Kind: kind, Bytes: chunk, Line: line, Col: col}, nil
}

func (d *Decoder) scanArrayStart() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	d.advanceOne()
	d.arrayDepth++
	d.enforceNestedDepth(start, d.arrayDepth)
	d.containerStack = append(d.containerStack, containerArray)
	d.expectingValue = true
	d.valueNoNewline = false
	d.needSeparator = false
	return Token{Kind: TokenKindArrayStart, Bytes: d.buf[start : start+1], Line: line, Col: col}, nil
}

func (d *Decoder) scanArrayEnd() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	d.advanceOne()
	if d.arrayDepth > 0 {
		d.arrayDepth--
	}
	if n := len(d.containerStack); n > 0 && d.containerStack[n-1] == containerArray {
		d.containerStack = d.containerStack[:n-1]
	}
	d.expectingValue = d.innermostIsArray()
	d.valueNoNewline = false
	d.needSeparator = len(d.containerStack) > 0
	d.needLineEnd = len(d.containerStack) == 0
	return Token{Kind: TokenKindArrayEnd, Bytes: d.buf[start : start+1], Line: line, Col: col}, nil
}

func (d *Decoder) scanInlineTableStart() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	d.advanceOne()
	d.inlineDepth++
	d.containerStack = append(d.containerStack, containerInline)
	d.expectingValue = false
	d.valueNoNewline = false
	d.needSeparator = false
	return Token{Kind: TokenKindInlineTableStart, Bytes: d.buf[start : start+1], Line: line, Col: col}, nil
}

func (d *Decoder) scanInlineTableEnd() (Token, error) {
	start := d.off
	line, col := d.line, d.col
	d.advanceOne()
	if d.inlineDepth > 0 {
		d.inlineDepth--
	}
	if n := len(d.containerStack); n > 0 && d.containerStack[n-1] == containerInline {
		d.containerStack = d.containerStack[:n-1]
	}
	d.expectingValue = d.innermostIsArray()
	d.valueNoNewline = false
	d.needSeparator = len(d.containerStack) > 0
	d.needLineEnd = len(d.containerStack) == 0
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
	keyLen := max(tokenEnd-start-2, 0)
	if keyLen > d.limits.MaxKeyLength {
		err := &LimitError{Limit: "MaxKeyLength", Value: d.limits.MaxKeyLength, Span: [2]int{start + 1, i}}
		d.setErr(err)
		return Token{}, err
	}
	d.expectingValue = false
	d.atLineStart = false
	d.needLineEnd = true
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
	keyLen := max(tokenEnd-start-4, 0)
	if keyLen > d.limits.MaxKeyLength {
		err := &LimitError{Limit: "MaxKeyLength", Value: d.limits.MaxKeyLength, Span: [2]int{start + 2, i}}
		d.setErr(err)
		return Token{}, err
	}
	d.expectingValue = false
	d.atLineStart = false
	d.needLineEnd = true
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
			end := i + idx + 1
			if _, err := parseStringValue(d.buf[off:end]); err != nil {
				return 0, err
			}
			return end, nil
		}
	}
	return 0, d.syntaxError("unterminated quoted key", off)
}

func (d *Decoder) scanString(off int) (int, TokenKind, error) {
	rest := d.buf[off:]
	if hasBytePrefix(rest, '"', '"', '"') {
		for end := off + 3; end < len(d.buf); end++ {
			if d.buf[end] == '\\' {
				if end+1 >= len(d.buf) {
					return 0, TokenKindInvalid, d.syntaxError("unterminated string escape", end)
				}
				end++
				continue
			}
			if d.buf[end] == '"' {
				run := countByteRun(d.buf[end:], '"')
				if run >= 3 {
					tokenEnd := end + min(run, 5)
					if _, err := parseStringValue(d.buf[off:tokenEnd]); err != nil {
						return 0, TokenKindInvalid, err
					}
					return tokenEnd, TokenKindValueString, nil
				}
				end += run - 1
			}
		}
		return 0, TokenKindInvalid, d.syntaxError("unterminated multiline string", off)
	}
	if hasBytePrefix(rest, '\'', '\'', '\'') {
		for end := off + 3; end < len(d.buf); end++ {
			if d.buf[end] == '\'' {
				run := countByteRun(d.buf[end:], '\'')
				if run >= 3 {
					tokenEnd := end + min(run, 5)
					if _, err := parseStringValue(d.buf[off:tokenEnd]); err != nil {
						return 0, TokenKindInvalid, err
					}
					return tokenEnd, TokenKindValueString, nil
				}
				end += run - 1
			}
		}
		return 0, TokenKindInvalid, d.syntaxError("unterminated multiline string", off)
	}
	quote := d.buf[off]
	for i := off + 1; i < len(d.buf); i++ {
		if d.buf[i] == '\n' || d.buf[i] == '\r' {
			return 0, TokenKindInvalid, d.syntaxError("unterminated string", off)
		}
		if quote == '"' && d.buf[i] == '\\' {
			if i+1 >= len(d.buf) {
				return 0, TokenKindInvalid, d.syntaxError("unterminated string", off)
			}
			i++
			continue
		}
		if d.buf[i] == quote {
			end := i + 1
			if _, err := parseStringValue(d.buf[off:end]); err != nil {
				return 0, TokenKindInvalid, err
			}
			return end, TokenKindValueString, nil
		}
	}
	return 0, TokenKindInvalid, d.syntaxError("unterminated string", off)
}

func countByteRun(raw []byte, b byte) int {
	n := 0
	for n < len(raw) && raw[n] == b {
		n++
	}
	return n
}

func hasBytePrefix(raw []byte, want ...byte) bool {
	if len(raw) < len(want) {
		return false
	}
	for i, b := range want {
		if raw[i] != b {
			return false
		}
	}
	return true
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

func (d *Decoder) enforceNestedDepth(off, depth int) {
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

func looksLikeDatetime(raw []byte) bool {
	if !hasDateTimeShape(raw) {
		return false
	}
	_, _, err := parseDateTimeValue(raw)
	return err == nil
}

func hasDateTimeShape(raw []byte) bool {
	switch {
	case hasDateShape(raw):
		return true
	case hasTimeShape(raw):
		return true
	default:
		return false
	}
}

func hasDateShape(raw []byte) bool {
	if len(raw) < len("0000-00-00") {
		return false
	}
	return isDigit(raw[0]) &&
		isDigit(raw[1]) &&
		isDigit(raw[2]) &&
		isDigit(raw[3]) &&
		raw[4] == '-' &&
		isDigit(raw[5]) &&
		isDigit(raw[6]) &&
		raw[7] == '-' &&
		isDigit(raw[8]) &&
		isDigit(raw[9])
}

func hasTimeShape(raw []byte) bool {
	if len(raw) < len("00:00") {
		return false
	}
	return isDigit(raw[0]) &&
		isDigit(raw[1]) &&
		raw[2] == ':' &&
		isDigit(raw[3]) &&
		isDigit(raw[4])
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func scanBareValueEnd(raw []byte) int {
	end := scanUntilDelimiter(raw)
	if end != len("0000-00-00") || end >= len(raw) || raw[end] != ' ' {
		return end
	}
	tailEnd := end + 1 + scanUntilDelimiter(raw[end+1:])
	if _, _, err := parseDateTimeValue(raw[:tailEnd]); err == nil {
		return tailEnd
	}
	return end
}

func bytesTrimRightSpaces(raw []byte) []byte {
	for len(raw) > 0 {
		switch raw[len(raw)-1] {
		case ' ', '\t':
			raw = raw[:len(raw)-1]
		default:
			return raw
		}
	}
	return raw
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

func isIntCandidate(raw string) bool {
	if raw == "" {
		return false
	}
	n := strings.TrimPrefix(strings.TrimPrefix(raw, "+"), "-")
	if n == "" {
		return false
	}
	if strings.HasPrefix(n, "0_") {
		return false
	}
	if len(n) > 2 && n[0] == '0' {
		switch n[1] {
		case 'b', 'o', 'x':
			if raw[0] == '+' || raw[0] == '-' {
				return false
			}
			return prefixedDigitsValid(n[2:], n[1])
		}
	}
	if len(n) > 1 && n[0] == '0' && n[1] >= '0' && n[1] <= '9' {
		return false
	}
	for _, r := range n {
		if (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return hasValidNumberUnderscores(n)
}

func prefixedDigitsValid(raw string, prefix byte) bool {
	if raw == "" || strings.HasPrefix(raw, "_") || strings.HasSuffix(raw, "_") || strings.Contains(raw, "__") {
		return false
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '_' {
			if !validPrefixedDigit(raw[i-1], prefix) || !validPrefixedDigit(raw[i+1], prefix) {
				return false
			}
			continue
		}
		if !validPrefixedDigit(c, prefix) {
			return false
		}
	}
	return true
}

func validPrefixedDigit(c, prefix byte) bool {
	switch prefix {
	case 'b':
		return c == '0' || c == '1'
	case 'o':
		return c >= '0' && c <= '7'
	case 'x':
		return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	default:
		return false
	}
}

func hasCapitalNumericPrefix(raw string) bool {
	n := strings.TrimPrefix(strings.TrimPrefix(raw, "+"), "-")
	return len(n) > 1 && n[0] == '0' && (n[1] == 'B' || n[1] == 'O' || n[1] == 'X')
}

func isSpecialFloat(raw string) bool {
	switch raw {
	case "inf", "+inf", "-inf", "nan", "+nan", "-nan":
		return true
	default:
		return false
	}
}

func isFloatCandidate(raw string) bool {
	if raw == "" {
		return false
	}
	n := strings.TrimPrefix(raw, "+")
	n = strings.TrimPrefix(n, "-")
	if n == "" {
		return false
	}
	if len(n) > 1 && n[0] == '0' {
		next := n[1]
		if (next >= '0' && next <= '9') || next == '_' {
			return false
		}
	}
	hasExp := false
	hasDecimalPoint := false
	for i := 0; i < len(n); i++ {
		switch n[i] {
		case '.':
			if strings.HasPrefix(n, ".") || hasDecimalPoint || i+1 >= len(n) || n[i+1] < '0' || n[i+1] > '9' {
				return false
			}
			hasDecimalPoint = true
		case 'e', 'E':
			if hasExp {
				return false
			}
			hasExp = true
		case '+', '-':
			// Sign is valid only as the first character or immediately after 'e'/'E'.
			if i != 0 && n[i-1] != 'e' && n[i-1] != 'E' {
				return false
			}
		case '_':
			// underscore validity checked below.
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		default:
			return false
		}
	}
	if !hasDecimalPoint && !hasExp {
		return false
	}
	return hasValidNumberUnderscores(n)
}

func hasValidNumberUnderscores(raw string) bool {
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "_") || strings.HasSuffix(raw, "_") || strings.Contains(raw, "__") {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '_' {
			continue
		}
		prev, next := raw[i-1], raw[i+1]
		if prev < '0' || prev > '9' {
			return false
		}
		if next < '0' || next > '9' {
			return false
		}
	}
	return true
}
