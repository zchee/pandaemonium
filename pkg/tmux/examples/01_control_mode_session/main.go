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
	"path/filepath"
	"strings"
	"time"

	"github.com/zchee/pandaemonium/pkg/tmux"
)

const runRealTmuxTestsEnv = "RUN_REAL_TMUX_TESTS"

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := run(ctx, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, out io.Writer) error {
	if os.Getenv(runRealTmuxTestsEnv) != "1" {
		_, err := fmt.Fprintf(out, "set %s=1 to run this example against an isolated real tmux server\n", runRealTmuxTestsEnv)
		return err
	}
	return runRealTmux(ctx, out)
}

func runRealTmux(ctx context.Context, out io.Writer) error {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("find tmux executable: %w", err)
	}
	tmp, err := os.MkdirTemp("", "pandaemonium-tmux-example-*")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmp)

	socket := filepath.Join(tmp, "tmux.sock")
	config := filepath.Join(tmp, "tmux.conf")
	if err := os.WriteFile(config, nil, 0o600); err != nil {
		return fmt.Errorf("write empty tmux config: %w", err)
	}
	session := fmt.Sprintf("pandaemonium-example-%d", os.Getpid())
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanupCtx, path, "-S", socket, "kill-server").Run()
	}()

	client, err := tmux.New(
		ctx,
		tmux.WithPath(path),
		tmux.WithSocketPath(socket),
		tmux.WithConfigFile(config),
		tmux.WithSessionName(session),
		tmux.WithCreateSession(true),
		tmux.WithShutdownTimeout(2*time.Second),
	)
	if err != nil {
		return fmt.Errorf("start tmux control client: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = client.Close(cleanupCtx)
	}()

	messageResp, err := client.Exec(ctx, tmux.DisplayMessage, tmux.RawArg("-p"), tmux.StringArg("hello from pkg/tmux"))
	if err != nil {
		return fmt.Errorf("display message: %w", err)
	}
	message := strings.Join(messageResp.Lines, "\n")

	panesResp, err := client.Exec(ctx, tmux.ListPanes, tmux.RawArg("-a"), tmux.RawArg("-F"), tmux.StringArg("#{session_name}:#{window_index}.#{pane_index}"))
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}
	if len(panesResp.Lines) == 0 {
		return fmt.Errorf("list panes returned no panes")
	}

	fmt.Fprintf(out, "session: %s\n", session)
	fmt.Fprintf(out, "message: %s\n", message)
	fmt.Fprintf(out, "panes: %d\n", len(panesResp.Lines))
	fmt.Fprintf(out, "first pane: %s\n", panesResp.Lines[0])
	return nil
}
