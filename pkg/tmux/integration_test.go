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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealTmuxControlMode(t *testing.T) {
	if os.Getenv("RUN_REAL_TMUX_TESTS") != "1" {
		t.Skip("RUN_REAL_TMUX_TESTS=1 is required for isolated real tmux integration tests")
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		t.Skipf("tmux executable not found on PATH: %v", err)
	}
	tmp := t.TempDir()
	socket := filepath.Join(tmp, "tmux.sock")
	config := filepath.Join(tmp, "tmux.conf")
	if err := os.WriteFile(config, nil, 0o600); err != nil {
		t.Fatalf("write empty config: %v", err)
	}
	session := "pandaemonium-" + strings.ReplaceAll(t.Name(), "/", "-")
	t.Cleanup(func() { _ = exec.Command(path, "-S", socket, "kill-server").Run() })

	startupCtx, cancelStartup := context.WithTimeout(t.Context(), 15*time.Second)
	client, err := New(
		startupCtx,
		WithPath(path),
		WithSocketPath(socket),
		WithConfigFile(config),
		WithSessionName(session),
		WithCreateSession(true),
		WithShutdownTimeout(2*time.Second),
	)
	cancelStartup()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = client.Close(context.Background()) }()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	resp, err := client.Exec(ctx, DisplayMessage, RawArg("-p"), StringArg("hello-from-tmux"))
	if err != nil {
		t.Fatalf("display-message Exec() error = %v stderr=%v", err, client.StderrTail())
	}
	if got := strings.Join(resp.Lines, "\n"); got != "hello-from-tmux" {
		t.Fatalf("display-message output = %q, want hello-from-tmux", got)
	}

	paneResp, err := client.Exec(ctx, DisplayMessage, RawArg("-p"), StringArg("#{pane_id}"))
	if err != nil {
		t.Fatalf("pane id Exec() error = %v", err)
	}
	if len(paneResp.Lines) != 1 || !strings.HasPrefix(paneResp.Lines[0], "%") {
		t.Fatalf("pane id response = %#v, want one %% pane id", paneResp.Lines)
	}
	pane := PaneID(paneResp.Lines[0])
	if _, err := client.SubscribeFormat(ctx, "pane-cmd", SubscriptionTarget(pane), "#{pane_current_command}"); err != nil {
		t.Fatalf("SubscribeFormat() error = %v", err)
	}
	if _, err := client.Exec(ctx, Command("send-keys"), RawArg("-t"), StringArg(string(pane)), StringArg("printf pandaemonium-output"), RawArg("Enter")); err != nil {
		t.Fatalf("send-keys Exec() error = %v", err)
	}
	waitForRealTmuxEvidence(t, ctx, client)
	_, _ = client.ExecRaw(ctx, "kill-server")
}

func waitForRealTmuxEvidence(t *testing.T, ctx context.Context, client *Client) {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	seenOutput := false
	seenSubscription := false
	for !seenOutput || !seenSubscription {
		select {
		case notification, ok := <-client.Events():
			if !ok {
				t.Fatalf("events closed before evidence: output=%v subscription=%v", seenOutput, seenSubscription)
			}
			if out, ok, err := notification.Output(); ok {
				if err != nil {
					t.Fatalf("Output() error = %v", err)
				}
				bytes, err := out.Bytes()
				if err != nil {
					t.Fatalf("Output Bytes() error = %v", err)
				}
				if strings.Contains(string(bytes), "pandaemonium-output") {
					seenOutput = true
				}
			}
			if _, ok, err := notification.SubscriptionChanged(); ok {
				if err != nil {
					t.Fatalf("SubscriptionChanged() error = %v", err)
				}
				seenSubscription = true
			}
		case <-deadline.Done():
			if errors.Is(deadline.Err(), context.DeadlineExceeded) {
				t.Fatalf("timed out waiting for real tmux output/subscription evidence: output=%v subscription=%v drops=%d", seenOutput, seenSubscription, client.DroppedNotifications())
			}
			t.Fatalf("context done waiting for real tmux evidence: %v", deadline.Err())
		}
	}
}
