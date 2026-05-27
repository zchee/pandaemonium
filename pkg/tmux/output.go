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

package tmux

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// DecodeOutputValue decodes tmux `%output` octal escapes to terminal bytes.
func DecodeOutputValue(value string) ([]byte, error) {
	out, err := decodeOutputValuePartial(value)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// decodeOutputValuePartial decodes value and returns the successfully decoded
// bytes alongside the first decode error. The bytes slice always reflects
// everything decoded before the error site so callers can build best-effort
// lossy renderings without re-scanning the input.
func decodeOutputValuePartial(value string) ([]byte, error) {
	out := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b != '\\' {
			out = append(out, b)
			continue
		}
		if i+3 >= len(value) {
			return out, fmt.Errorf("tmux: incomplete octal escape at byte %d", i)
		}
		v := 0
		for j := 1; j <= 3; j++ {
			digit := value[i+j]
			if digit < '0' || digit > '7' {
				return out, fmt.Errorf("tmux: invalid octal digit %q at byte %d", digit, i+j)
			}
			v = v*8 + int(digit-'0')
		}
		if v > 0xff {
			return out, fmt.Errorf("tmux: octal escape at byte %d is out of range", i)
		}
		out = append(out, byte(v))
		i += 3
	}
	return out, nil
}

func decodeOutputText(value string) (string, error) {
	bytes, err := DecodeOutputValue(value)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(bytes) {
		return "", fmt.Errorf("tmux: decoded output is not valid UTF-8")
	}
	return string(bytes), nil
}

func decodeOutputTextLossy(value string) string {
	bytes, _ := decodeOutputValuePartial(value)
	return strings.ToValidUTF8(string(bytes), "\uFFFD")
}
