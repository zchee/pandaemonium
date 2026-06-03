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
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/zchee/pandaemonium/pkg/codex"
	"github.com/zchee/pandaemonium/pkg/codex/examples/internal/exampleutil"
)

func main() {
	ctx, cancel := exampleutil.NewContext()
	defer cancel()

	config, err := exampleutil.RemoteConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	remote, err := codex.NewRemoteCodex(ctx, config)
	if err != nil {
		log.Fatal(err)
	}
	defer remote.Close()

	cwd, command, err := processCommand()
	if err != nil {
		log.Fatal(err)
	}
	processHandle := fmt.Sprintf("codex-example-%d", time.Now().UnixNano())
	handle, err := remote.Client().SpawnProcess(ctx, &codex.ProcessSpawnParams{
		ProcessHandle:      processHandle,
		Command:            command,
		Cwd:                cwd,
		StreamStdoutStderr: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = handle.Close() }()

	fmt.Println("process.handle:", handle.ID())
	fmt.Println("process.command:", strings.Join(command, " "))
	for event, err := range handle.Stream(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		if event.OutputDelta != nil {
			decoded, err := base64.StdEncoding.DecodeString(event.OutputDelta.DeltaBase64)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%v> %s", event.OutputDelta.Stream, string(decoded))
		}
		if event.Exited != nil {
			fmt.Println("process.exit_code:", event.Exited.ExitCode)
		}
	}
}

func processCommand() (string, []string, error) {
	cwd := strings.TrimSpace(os.Getenv("CODEX_REMOTE_PROCESS_CWD"))
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", nil, err
		}
	}

	command := os.Args[1:]
	if len(command) > 0 && command[0] == "--" {
		command = command[1:]
	}
	if len(command) == 0 {
		command = []string{"sh", "-c", "printf 'hello from process/spawn\\n'"}
	}
	return cwd, command, nil
}
