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

package tmux_test

import (
	"fmt"

	"github.com/zchee/pandaemonium/pkg/tmux"
)

func ExampleDecodeOutputValue() {
	bytes, err := tmux.DecodeOutputValue(`hello\015\012\033[31mred\033[0m`)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%q\n", bytes)
	// Output: "hello\r\n\x1b[31mred\x1b[0m"
}

func ExampleCommandLine() {
	line := tmux.NewCommandLine(tmux.DisplayMessage, tmux.RawArg("-p"), tmux.StringArg("#{session_name}"))
	rendered, err := line.String()
	if err != nil {
		panic(err)
	}
	fmt.Println(rendered)
	// Output: display-message -p '#{session_name}'
}
