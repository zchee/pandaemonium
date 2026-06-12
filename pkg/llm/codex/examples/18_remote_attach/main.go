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
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/zchee/pandaemonium/pkg/llm/codex"
	"github.com/zchee/pandaemonium/pkg/llm/codex/examples/internal/exampleutil"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	// log.Fatal would skip deferred Close calls and orphan the ws-listening
	// app-server child, so all work happens behind an error return.
	if err := run(); err != nil {
		log.Fatal(fmt.Errorf("%w", err))
	}
}

func run() error {
	transport := flag.String("transport", "ws", `listen transport: "ws" (loopback websocket) or "unix" (websocket over a unix domain socket)`)
	flag.Parse()

	// The attached TUI is interactive, so no wall-clock deadline here:
	// exampleutil.NewContext's 2-minute timeout would interrupt the app-server
	// and the TUI mid-session. Ctrl-C cancels both children instead.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var listenURL string
	switch *transport {
	case "ws":
		// The app-server rejects ws://...:0, so reserve a concrete loopback
		// port before launching.
		port, err := codex.ReserveLoopbackPort()
		if err != nil {
			return err
		}
		listenURL = fmt.Sprintf("ws://127.0.0.1:%d", port)
	case "unix":
		// An explicit SDK-owned socket: server.Close removes the socket file
		// and the deferred RemoveAll the directory. The bare unix:// default
		// would instead bind $CODEX_HOME's control socket, which is shared
		// with daemon tooling.
		dir, err := os.MkdirTemp("", "codex-ex18-*")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(dir) }()
		listenURL = "unix://" + filepath.Join(dir, "codex.sock")
	default:
		return fmt.Errorf("unsupported -transport %q: expected ws or unix", *transport)
	}

	server, err := codex.LaunchRemoteAppServer(ctx, &codex.RemoteAppServerConfig{
		AppServerBin: strings.TrimSpace(os.Getenv("CODEX_APP_SERVER_BIN")),
		CodexBin:     strings.TrimSpace(os.Getenv("CODEX_BIN")),
		Listen:       codex.ListenConfig{URL: listenURL},
		Stderr:       os.Stderr,
	})
	if err != nil {
		return err
	}
	defer func() { _ = server.Close() }()

	fmt.Println("remote.endpoint:", server.Endpoint())

	attach, err := server.CodexCommand(ctx)
	if err != nil {
		return err
	}
	fmt.Println("attach.command:", strings.Join(attach.Args, " "))

	client, err := codex.NewRemoteCodex(ctx, server.RemoteConfig())
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	fmt.Println("remote.server:", exampleutil.ServerLabel(client.Metadata()))
	// The app-server requires a params object on model/list, so send
	// explicit empty params instead of nil.
	models, err := client.Models(ctx, &codex.ModelListParams{})
	if err != nil {
		return err
	}
	for _, model := range models.Data {
		fmt.Printf("model: %#v\n", model)
	}

	if !exampleutil.BoolEnv("CODEX_EXAMPLE_RUN_TUI") {
		fmt.Println("tui.skipped: set CODEX_EXAMPLE_RUN_TUI=1 to attach the interactive codex TUI")
		return nil
	}
	tui, err := server.StartCodex(ctx, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}
	return tui.Wait()
}
