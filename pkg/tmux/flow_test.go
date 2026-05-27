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
	"context"
	"strings"
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"
)

func TestRefreshClientHelpersRenderCommands(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		call func(context.Context, *Client) (Response, error)
		want string
	}{
		"success: client size": {
			call: func(ctx context.Context, c *Client) (Response, error) { return c.RefreshClientSize(ctx, 120, 40) },
			want: "refresh-client -C 120x40",
		},
		"success: client flags": {
			call: func(ctx context.Context, c *Client) (Response, error) {
				return c.SetClientFlags(ctx, ClientFlagNoOutput, ClientFlagWaitExit)
			},
			want: "refresh-client -f no-output,wait-exit",
		},
		"success: pause after rounds up seconds": {
			call: func(ctx context.Context, c *Client) (Response, error) {
				return c.SetPauseAfter(ctx, 1500*time.Millisecond)
			},
			want: "refresh-client -f pause-after=2",
		},
		"success: pause pane": {
			call: func(ctx context.Context, c *Client) (Response, error) { return c.PausePane(ctx, "%1") },
			want: "refresh-client -A %1:pause",
		},
		"success: continue pane": {
			call: func(ctx context.Context, c *Client) (Response, error) { return c.ContinuePane(ctx, "%1") },
			want: "refresh-client -A %1:continue",
		},
		"success: disable pane output": {
			call: func(ctx context.Context, c *Client) (Response, error) { return c.DisablePaneOutput(ctx, "%1") },
			want: "refresh-client -A %1:off",
		},
		"success: enable pane output": {
			call: func(ctx context.Context, c *Client) (Response, error) { return c.EnablePaneOutput(ctx, "%1") },
			want: "refresh-client -A %1:on",
		},
		"success: subscribe format": {
			call: func(ctx context.Context, c *Client) (Response, error) {
				return c.SubscribeFormat(ctx, "sub", SubscriptionAllPanes, "#{pane_id}:#{pane_current_command}")
			},
			want: "refresh-client -B 'sub:%*:#{pane_id}:#{pane_current_command}'",
		},
		"success: unsubscribe format": {
			call: func(ctx context.Context, c *Client) (Response, error) { return c.UnsubscribeFormat(ctx, "sub") },
			want: "refresh-client -B sub",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, tr := newScriptedClient(t, 8)
			tr.onWrite = func(line string) { tr.writeLines("%begin 1 2 1", "%end 1 2 1") }
			if _, err := tt.call(t.Context(), client); err != nil {
				t.Fatalf("helper error = %v", err)
			}
			if diff := gocmp.Diff([]string{tt.want}, tr.written()); diff != "" {
				t.Fatalf("written command mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRefreshClientHelperValidation(t *testing.T) {
	t.Parallel()
	client, _ := newScriptedClient(t, 8)
	tests := map[string]struct {
		call    func() error
		wantErr string
	}{
		"error: client size positive": {
			call:    func() error { _, err := client.RefreshClientSize(t.Context(), 0, 40); return err },
			wantErr: "positive",
		},
		"error: flags required": {
			call:    func() error { _, err := client.SetClientFlags(t.Context()); return err },
			wantErr: "at least one",
		},
		"error: pause after positive": {
			call:    func() error { _, err := client.SetPauseAfter(t.Context(), 0); return err },
			wantErr: "positive",
		},
		"error: pane id prefix": {
			call:    func() error { _, err := client.PausePane(t.Context(), "1"); return err },
			wantErr: "pane ID",
		},
		"error: subscription name required": {
			call: func() error {
				_, err := client.SubscribeFormat(t.Context(), "", SubscriptionAllPanes, "#{pane_id}")
				return err
			},
			wantErr: "subscription name",
		},
		"error: subscription target no colon": {
			call: func() error {
				_, err := client.SubscribeFormat(t.Context(), "sub", SubscriptionTarget("bad:target"), "#{pane_id}")
				return err
			},
			wantErr: "target",
		},
		"error: subscription format no newline": {
			call: func() error {
				_, err := client.SubscribeFormat(t.Context(), "sub", SubscriptionAllPanes, "bad\n")
				return err
			},
			wantErr: "format",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := tt.call(); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("helper validation error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
