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
	"context"
	"errors"
)

// NewClient creates a new [ClaudeSDKClient] for interactive, multi-turn use.
//
// NewClient performs CLI discovery (Options.CLIPath → exec.LookPath("claude")
// → well-known install directories) but does not launch the subprocess until
// the first [ClaudeSDKClient.Query] call. The only side effect at construction
// time is binary lookup (AC-i1).
//
// opts may be nil; a nil Options is equivalent to a zero-value Options.
//
// The body is stubbed to errors.ErrUnsupported until Phase C.
func NewClient(ctx context.Context, opts *Options) (*ClaudeSDKClient, error) {
	_, _ = ctx, opts
	return nil, errors.ErrUnsupported
}
