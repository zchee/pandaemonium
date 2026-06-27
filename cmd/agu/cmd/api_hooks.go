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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/spf13/cobra"

	"github.com/zchee/pandaemonium/pkg/llm/codex"
)

type hook struct {
	log       *os.File
	mkdirOnce sync.Once
}

func newAPIHooksCommand(loadConfig configLoader) *cobra.Command {
	return &cobra.Command{
		Use:   "hooks",
		Short: "Manage agent hooks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := loadConfig(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("load config: nil config")
			}

			hookLogDir := filepath.Join(cfg.StateHome, "agu")

			h := &hook{}
			var err error
			h.mkdirOnce.Do(func() {
				err = os.MkdirAll(hookLogDir, 0o755)
			})
			if err != nil {
				return err
			}

			h.log, err = os.OpenFile(filepath.Join(hookLogDir, "hooks.log.jsonl"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666)
			if err != nil {
				return fmt.Errorf("open hooks log: %w", err)
			}
			defer h.log.Close()

			return h.runAPIHooksCommand(cmd.Context(), cmd.InOrStdin())
		},
	}
}

func (h *hook) runAPIHooksCommand(_ context.Context, stdin io.Reader) error {
	in, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	payload, err := codex.DecodeHookInput(in)
	if err != nil {
		return fmt.Errorf("decode input: %w", err)
	}

	data, err := json.Marshal(payload, jsontext.Multiline(true), jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	_, err = io.WriteString(h.log, string(data)+"\n")
	return err
}
