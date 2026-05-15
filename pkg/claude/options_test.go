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
	"testing"
)

func TestOptions_Validate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		opts    *Options
		wantErr bool
	}{
		"success: nil options is valid (zero value, AC-i1)": {
			opts:    nil,
			wantErr: false,
		},
		"success: zero value Options is valid (AC-i1)": {
			opts:    &Options{},
			wantErr: false,
		},
		"success: positive MaxTurns is valid": {
			opts:    &Options{MaxTurns: 10},
			wantErr: false,
		},
		"success: positive MaxBudgetUSD is valid": {
			opts:    &Options{MaxBudgetUSD: 5.0},
			wantErr: false,
		},
		"success: all non-numeric fields set": {
			opts: &Options{
				SystemPrompt:           "Be concise.",
				AllowedTools:           []string{"Bash", "Write"},
				CLIPath:                "/usr/local/bin/claude",
				PermissionMode:         "bypassPermissions",
				Model:                  "claude-opus-4-5",
				OutputFormat:           "stream-json",
				InputFormat:            "text",
				APIKeyHelper:           "/usr/bin/get-key",
				Verbose:                true,
				IncludePartialMessages: true,
			},
			wantErr: false,
		},
		"error: negative MaxTurns is invalid": {
			opts:    &Options{MaxTurns: -1},
			wantErr: true,
		},
		"error: negative MaxBudgetUSD is invalid": {
			opts:    &Options{MaxBudgetUSD: -0.01},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tt.opts.validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
