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

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/zchee/pandaemonium/pkg/tmux"
)

const (
	sessionEnv    = "PANDAEMONIUM_TMUX_SESSION"
	socketPathEnv = "PANDAEMONIUM_TMUX_SOCKET_PATH"
	socketNameEnv = "PANDAEMONIUM_TMUX_SOCKET_NAME"
	configFileEnv = "PANDAEMONIUM_TMUX_CONFIG_FILE"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := run(ctx, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, out io.Writer) error {
	session := os.Getenv(sessionEnv)
	if session == "" {
		_, err := fmt.Fprintf(out, "set %s to an existing tmux session name to run this example\n", sessionEnv)
		return err
	}
	return runExistingSession(ctx, out, existingSessionConfig{
		session:    session,
		socketPath: os.Getenv(socketPathEnv),
		socketName: os.Getenv(socketNameEnv),
		configFile: os.Getenv(configFileEnv),
	})
}

type existingSessionConfig struct {
	session    string
	socketPath string
	socketName string
	configFile string
}

func runExistingSession(ctx context.Context, out io.Writer, cfg existingSessionConfig) error {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("find tmux executable: %w", err)
	}
	if cfg.session == "" {
		return fmt.Errorf("%s must name an existing tmux session", sessionEnv)
	}
	if cfg.socketName != "" && cfg.socketPath != "" {
		return fmt.Errorf("%s and %s are mutually exclusive", socketNameEnv, socketPathEnv)
	}

	opts := []tmux.Option{
		tmux.WithPath(path),
		tmux.WithSessionName(cfg.session),
		tmux.WithCreateSession(false),
		tmux.WithShutdownTimeout(2 * time.Second),
	}
	if cfg.socketPath != "" {
		opts = append(opts, tmux.WithSocketPath(cfg.socketPath))
	}
	if cfg.socketName != "" {
		opts = append(opts, tmux.WithSocketName(cfg.socketName))
	}
	if cfg.configFile != "" {
		opts = append(opts, tmux.WithConfigFile(cfg.configFile))
	}

	client, err := tmux.New(ctx, opts...)
	if err != nil {
		return fmt.Errorf("attach to existing tmux session %q: %w", cfg.session, err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = client.Close(cleanupCtx)
	}()

	sessionResp, err := client.Exec(ctx, tmux.DisplayMessage, tmux.RawArg("-p"), tmux.StringArg("#{session_name}"))
	if err != nil {
		return fmt.Errorf("read attached session name: %w", err)
	}
	attachedSession := strings.Join(sessionResp.Lines, "\n")
	if attachedSession != cfg.session {
		return fmt.Errorf("attached to session %q, want %q", attachedSession, cfg.session)
	}

	paneResp, err := client.Exec(ctx, tmux.DisplayMessage, tmux.RawArg("-p"), tmux.StringArg("#{pane_id}"))
	if err != nil {
		return fmt.Errorf("read active pane id: %w", err)
	}
	paneID := strings.Join(paneResp.Lines, "\n")
	if paneID == "" {
		return fmt.Errorf("active pane id is empty")
	}

	panesResp, err := client.Exec(ctx, tmux.ListPanes, tmux.RawArg("-t"), tmux.StringArg(cfg.session), tmux.RawArg("-F"), tmux.StringArg("#{pane_id}:#{window_index}.#{pane_index}:#{pane_current_command}"))
	if err != nil {
		return fmt.Errorf("list panes in %q: %w", cfg.session, err)
	}
	if len(panesResp.Lines) == 0 {
		return fmt.Errorf("session %q has no panes", cfg.session)
	}

	fmt.Fprintf(out, "attached session: %s\n", attachedSession)
	fmt.Fprintf(out, "active pane: %s\n", paneID)
	fmt.Fprintf(out, "panes: %d\n", len(panesResp.Lines))
	fmt.Fprintf(out, "first pane: %s\n", panesResp.Lines[0])
	fmt.Fprintf(out, "dropped notifications: %d\n", client.DroppedNotifications())
	return nil
}
