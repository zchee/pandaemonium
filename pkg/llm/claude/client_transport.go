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

package claude

import (
	"bufio"
	"io"

	"github.com/zchee/pandaemonium/pkg/llm"
)

// newStdioTransport returns the stdio-backed transport for a launched claude
// CLI subprocess, wiring [CLIConnectionError] into the shared
// [llm.StdioTransport]. Concurrent safety for WriteJSON is provided by
// ClaudeSDKClient.writeMu, not by an internal mutex — the same discipline
// used by the codex client in pkg/llm/codex.
func newStdioTransport(stdin io.WriteCloser, stdout *bufio.Reader) *llm.StdioTransport {
	return &llm.StdioTransport{
		Stdin:        stdin,
		Stdout:       stdout,
		ClosedErr:    func() error { return &CLIConnectionError{Message: "\"claude\" is not running"} },
		WrapWriteErr: func(err error) error { return &CLIConnectionError{Message: err.Error()} },
	}
}
