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

import "fmt"

// SyntaxError reports parse errors with line/column and byte span context.
type SyntaxError struct {
	// Line is 1-based.
	Line int
	// Col is 1-based.
	Col int

	// Msg is a parser-specific message.
	Msg string

	// Span is the offending [start, end) byte range in source.
	Span [2]int
}

// Error implements the error interface.
func (e *SyntaxError) Error() string {
	return fmt.Sprintf("toml: %s at line %d col %d", e.Msg, e.Line, e.Col)
}

// LimitError reports DoS-defense cap violations.
type LimitError struct {
	// Limit is one of MaxNestedDepth, MaxKeyLength, MaxStringLength,
	// or MaxDocumentSize.
	Limit string

	// Value is the configured cap value.
	Value int

	// Span is the offending [start, end) byte range in source.
	Span [2]int
}

// Error implements the error interface.
func (e *LimitError) Error() string {
	return fmt.Sprintf("toml: %s cap exceeded (limit=%d, span=[%d,%d))", e.Limit, e.Value, e.Span[0], e.Span[1])
}
