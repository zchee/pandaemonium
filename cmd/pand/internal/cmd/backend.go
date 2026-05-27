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
	"context"

	"github.com/zchee/pandaemonium/pkg/codex"
)

type CodexBackend struct {
	c *codex.Codex
}

// NewCodexBackend constructs a new [CodexBackend] instance.
func NewCodexBackend(ctx context.Context) (*CodexBackend, error) {
	c, err := codex.NewCodex(ctx, &codex.Config{
		Listen: codex.ListenConfig{
			WebSocket: &codex.WebSocketConfig{
				AuthMode: codex.WebSocketAuthNone,
			},
			AllowInsecureRemoteWebSocket: true,
		},
	})
	if err != nil {
		return nil, err
	}

	return &CodexBackend{
		c: c,
	}, nil
}

// Run discovers and executes the upstream OMX backend.
func (b *CodexBackend) Run(ctx context.Context) error {}
