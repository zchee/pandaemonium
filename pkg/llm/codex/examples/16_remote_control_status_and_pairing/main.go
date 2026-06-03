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
	"fmt"
	"log"

	"github.com/zchee/pandaemonium/pkg/llm/codex"
	"github.com/zchee/pandaemonium/pkg/llm/codex/examples/internal/exampleutil"
)

func main() {
	ctx, cancel := exampleutil.NewContext()
	defer cancel()

	client, err := exampleutil.NewCodex(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	remote := client.RemoteControl()
	status, err := remote.Status(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("remote-control.status:", status.Status)
	fmt.Println("remote-control.server:", status.ServerName)

	environmentID := environmentIDFromStatus(status)
	if exampleutil.BoolEnv("CODEX_EXAMPLE_ENABLE_REMOTE_CONTROL") {
		enabled, err := remote.Enable(ctx)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("remote-control.enabled.status:", enabled.Status)
		if enabled.EnvironmentID != "" {
			environmentID = enabled.EnvironmentID
		}
	} else {
		fmt.Println("remote-control.enable: skipped (set CODEX_EXAMPLE_ENABLE_REMOTE_CONTROL=1)")
	}

	if environmentID == "" {
		fmt.Println("remote-control.clients: skipped (no environment id)")
	} else {
		limit := int32(5)
		clients, err := remote.Clients(ctx, &codex.RemoteControlClientsListParams{
			EnvironmentID: environmentID,
			Limit:         limit,
			Order:         codex.RemoteControlClientsListOrderDesc,
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("remote-control.clients.count:", len(clients.Data))
		if clients.NextCursor != "" {
			fmt.Println("remote-control.clients.next_cursor:", clients.NextCursor)
		}
	}

	if exampleutil.BoolEnv("CODEX_EXAMPLE_START_PAIRING") {
		pairing, err := remote.PairingStart(ctx, &codex.RemoteControlPairingStartParams{ManualCode: true})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("remote-control.pairing.environment:", pairing.EnvironmentID)
		fmt.Println("remote-control.pairing.expires_at:", pairing.ExpiresAt)
		fmt.Println("remote-control.pairing.code:", pairing.PairingCode)
		if pairing.ManualPairingCode != "" {
			fmt.Println("remote-control.pairing.manual_code:", pairing.ManualPairingCode)
		}
	} else {
		fmt.Println("remote-control.pairing: skipped (set CODEX_EXAMPLE_START_PAIRING=1)")
	}
}

func environmentIDFromStatus(status codex.RemoteControlStatusReadResponse) string {
	return status.EnvironmentID
}
