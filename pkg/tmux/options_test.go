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
	"strings"
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"
)

func TestOptionsValidation(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		opts    []Option
		wantErr string
	}{
		"success: initial command is explicit": {
			opts: []Option{WithInitialCommand("new-session", "-A", "-s", "safe")},
		},
		"success: session target is explicit": {
			opts: []Option{WithSessionName("safe")},
		},
		"error: implicit default attach rejected": {
			wantErr: "InitialCommand or SessionName is required",
		},
		"error: socket name and path conflict": {
			opts:    []Option{WithSocketName("a"), WithSocketPath("/tmp/a"), WithSessionName("safe")},
			wantErr: "mutually exclusive",
		},
		"error: event buffer must be positive": {
			opts:    []Option{WithSessionName("safe"), WithEventBuffer(0)},
			wantErr: "EventBuffer must be > 0",
		},
		"error: stderr limit must be non-negative": {
			opts:    []Option{WithSessionName("safe"), WithStderrLineLimit(-1)},
			wantErr: "StderrLineLimit must be >= 0",
		},
		"error: shutdown timeout must be positive": {
			opts:    []Option{WithSessionName("safe"), WithShutdownTimeout(0)},
			wantErr: "ShutdownTimeout must be > 0",
		},
		"error: environment entries must be KEY=VALUE": {
			opts:    []Option{WithSessionName("safe"), WithEnv("BROKEN")},
			wantErr: "must be KEY=VALUE",
		},
		"error: initial command rejects newline": {
			opts:    []Option{WithInitialCommand("new-session\n")},
			wantErr: "contains a newline",
		},
		"error: session name rejects newline": {
			opts:    []Option{WithSessionName("bad\n")},
			wantErr: "SessionName contains a newline",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := applyOptions(tt.opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("applyOptions() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("applyOptions() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLaunchArgs(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		opts []Option
		want []string
	}{
		"success: attach explicit session": {
			opts: []Option{WithSessionName("safe")},
			want: []string{"-C", "attach-session", "-t", "safe"},
		},
		"success: create explicit session": {
			opts: []Option{WithSessionName("safe"), WithCreateSession(true)},
			want: []string{"-C", "new-session", "-A", "-s", "safe"},
		},
		"success: socket name and config": {
			opts: []Option{WithSocketName("sock"), WithConfigFile("/dev/null"), WithSessionName("safe")},
			want: []string{"-L", "sock", "-f", "/dev/null", "-C", "attach-session", "-t", "safe"},
		},
		"success: socket path and initial command": {
			opts: []Option{WithSocketPath("/tmp/tmux.sock"), WithInitialCommand("new-session", "-A", "-s", "safe")},
			want: []string{"-S", "/tmp/tmux.sock", "-C", "new-session", "-A", "-s", "safe"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg, err := applyOptions(tt.opts)
			if err != nil {
				t.Fatalf("applyOptions() error = %v", err)
			}
			if diff := gocmp.Diff(tt.want, cfg.launchArgs()); diff != "" {
				t.Fatalf("launchArgs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOptionsCloneEnv(t *testing.T) {
	t.Parallel()
	cfg, err := applyOptions([]Option{WithSessionName("safe"), WithEnv("A=B"), WithShutdownTimeout(time.Second)})
	if err != nil {
		t.Fatalf("applyOptions() error = %v", err)
	}
	got := cfg.cloneEnv()
	got[0] = "CHANGED=1"
	if cfg.Env[0] != "A=B" {
		t.Fatalf("cloneEnv() mutated original env: %v", cfg.Env)
	}
}
