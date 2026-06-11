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

	config, err := exampleutil.RemoteConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	client, err := codex.NewRemoteCodex(ctx, config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("remote.server:", exampleutil.ServerLabel(client.Metadata()))
	status, err := client.RemoteControl().Status(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("remote-control.status:", status.Status)
	fmt.Println("remote-control.server:", status.ServerName)
	if status.EnvironmentID != "" {
		fmt.Println("remote-control.environment:", status.EnvironmentID)
	}

	// app-server 0.140.0-alpha.4 requires a params object on model/list, so
	// send explicit empty params instead of nil.
	models, err := client.Models(ctx, &codex.ModelListParams{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("models.count:", len(models.Data))
}
