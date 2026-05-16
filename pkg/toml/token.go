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

// TokenKind is the decoded token type emitted by Decoder.
//
// The value names intentionally mirror the minimal streaming contract for
// parser-first architecture where all downstream layers consume this token stream.
type TokenKind uint8

const (
	// TokenKindInvalid is emitted only for internal fallback and should not
	// appear on successful tokenization.
	TokenKindInvalid TokenKind = iota

	// TokenKindTableHeader is `[table]`.
	TokenKindTableHeader

	// TokenKindArrayTableHeader is `[[table]]`.
	TokenKindArrayTableHeader

	// TokenKindKey is a bare, quoted, or dotted key.
	TokenKindKey

	// TokenKindValueString is a quoted value token.
	TokenKindValueString

	// TokenKindValueInteger is an integer value token.
	TokenKindValueInteger

	// TokenKindValueFloat is a floating-point value token.
	TokenKindValueFloat

	// TokenKindValueBool is `true` or `false`.
	TokenKindValueBool

	// TokenKindValueDatetime is one of the TOML datetime forms.
	TokenKindValueDatetime

	// TokenKindArrayStart is `[`. For value position, it starts an array value.
	TokenKindArrayStart

	// TokenKindArrayEnd is `]`.
	TokenKindArrayEnd

	// TokenKindInlineTableStart is `{`.
	TokenKindInlineTableStart

	// TokenKindInlineTableEnd is `}`.
	TokenKindInlineTableEnd

	// TokenKindComment is `# ...` up to newline or EOF.
	TokenKindComment
)

// String returns a stable token kind name.
func (k TokenKind) String() string {
	switch k {
	case TokenKindInvalid:
		return "Invalid"
	case TokenKindTableHeader:
		return "TableHeader"
	case TokenKindArrayTableHeader:
		return "ArrayTableHeader"
	case TokenKindKey:
		return "Key"
	case TokenKindValueString:
		return "ValueString"
	case TokenKindValueInteger:
		return "ValueInteger"
	case TokenKindValueFloat:
		return "ValueFloat"
	case TokenKindValueBool:
		return "ValueBool"
	case TokenKindValueDatetime:
		return "ValueDatetime"
	case TokenKindArrayStart:
		return "ArrayStart"
	case TokenKindArrayEnd:
		return "ArrayEnd"
	case TokenKindInlineTableStart:
		return "InlineTableStart"
	case TokenKindInlineTableEnd:
		return "InlineTableEnd"
	case TokenKindComment:
		return "Comment"
	default:
		return "TokenKind(unknown)"
	}
}

// Token is a single logical slice emitted by Decoder.ReadToken.
//
// Token.Bytes aliases caller-owned source bytes for NewDecoderBytes and aliases
// internal parser storage for NewDecoder. See Decoder.ReadToken for the
// aliasing rules.
type Token struct {
	// Kind is the token kind.
	Kind TokenKind

	// Bytes is the raw token content.
	Bytes []byte

	// Line is the current 1-based line at token start.
	Line int

	// Col is the current 1-based column at token start.
	Col int
}
