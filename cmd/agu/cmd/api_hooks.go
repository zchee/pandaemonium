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
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/spf13/cobra"

	"github.com/zchee/pandaemonium/cmd/agu/env"
	"github.com/zchee/pandaemonium/pkg/llm/codex"
)

func newAPIHooksCommand(loadConfig env.ConfigLoader) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Manage agent hooks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("load config: nil config")
			}

			hookLogDir := filepath.Join(cfg.StateHome, "agu")

			h := &hook{
				logs: make(map[codex.HookInputEventName]*os.File),
			}
			var err error
			h.mkdirOnce.Do(func() {
				err = os.MkdirAll(hookLogDir, 0o755)
			})
			if err != nil {
				return fmt.Errorf("mkdir hooks log dir: %w", err)
			}

			deferFn := make([]func() error, 0, len(logMap))
			for ev, f := range logMap {
				logf, err := os.OpenFile(filepath.Join(hookLogDir, f), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666)
				if err != nil {
					return fmt.Errorf("open hooks log for %s: %w", ev, err)
				}
				h.logs[ev] = logf
				deferFn = append(deferFn, logf.Close)
			}
			defer func() {
				for _, fn := range deferFn {
					fn()
				}
			}()

			return h.run(cmd.Context(), cmd.InOrStdin())
		},
	}

	cmd.AddCommand(newAPIHooksParseCommand())

	return cmd
}

func newAPIHooksParseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "parse [filepath]",
		Short: "Parse agent hooks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return (&hook{}).runParse(cmd.Context(), args[0], cmd.OutOrStdout())
		},
	}

	return cmd
}

type hook struct {
	logs      map[codex.HookInputEventName]*os.File
	mkdirOnce sync.Once
}

var logMap = map[codex.HookInputEventName]string{
	codex.HookInputEventNamePermissionRequest: "hooks.PermissionRequest.jsonl",
	codex.HookInputEventNamePreCompact:        "hooks.PreCompact.jsonl",
	codex.HookInputEventNamePostCompact:       "hooks.PostCompact.jsonl",
	codex.HookInputEventNamePreToolUse:        "hooks.PreToolUse.jsonl",
	codex.HookInputEventNamePostToolUse:       "hooks.PostToolUse.jsonl",
	codex.HookInputEventNameSessionStart:      "hooks.SessionStart.jsonl",
	codex.HookInputEventNameStop:              "hooks.Stop.jsonl",
	codex.HookInputEventNameSubagentStart:     "hooks.SubagentStart.jsonl",
	codex.HookInputEventNameSubagentStop:      "hooks.SubagentStop.jsonl",
	codex.HookInputEventNameUserPromptSubmit:  "hooks.UserPromptSubmit.jsonl",
}

func (h *hook) run(_ context.Context, stdin io.Reader) error {
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

	out, ok := h.logs[payload.EventName()]
	if !ok {
		return fmt.Errorf("unknown event name: %s", payload.EventName())
	}

	_, err = io.WriteString(out, string(data)+"\n")
	return err
}

func (h *hook) runParse(_ context.Context, path string, out io.Writer) error {
	in, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	payloads, err := codex.DecodeHookInputs(in)
	if err != nil {
		return fmt.Errorf("decode input: %w", err)
	}

	// TODO(zchee): for debug
	payloads = slices.DeleteFunc(payloads, func(in codex.HookInput) bool {
		switch in := in.(type) {
		case codex.PreToolUseHookInput:
			return strings.EqualFold(in.ToolName, "bash")
		case codex.PostToolUseHookInput:
			return strings.EqualFold(in.ToolName, "bash")
		}
		return false
	})
	slices.SortFunc(payloads, func(x, y codex.HookInput) int {
		return cmp.Compare(string(x.EventName()), string(y.EventName()))
	})
	payloads = slices.CompactFunc(payloads, func(x, y codex.HookInput) bool {
		return strings.EqualFold(string(x.EventName()), string(y.EventName()))
	})

	var buf bytes.Buffer
	for _, payload := range payloads {
		if err := json.MarshalWrite(&buf, payload, jsontext.Multiline(true), jsontext.WithIndent("  ")); err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
	}

	_, err = out.Write(buf.Bytes())
	return err
}
