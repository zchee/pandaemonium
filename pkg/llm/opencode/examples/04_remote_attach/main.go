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

// Command remote_attach connects to an already-running `opencode serve`
// instead of spawning one. Start the server separately, e.g.:
//
//	OPENCODE_SERVER_PASSWORD=secret opencode serve --hostname 127.0.0.1 --port 4096
//
// then run this with OPENCODE_BASE_URL=http://127.0.0.1:4096 and
// OPENCODE_SERVER_PASSWORD=secret.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/zchee/pandaemonium/pkg/llm/opencode"
)

func main() {
	ctx := context.Background()

	baseURL := os.Getenv("OPENCODE_BASE_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:4096"
	}

	oc, err := opencode.NewRemoteOpencode(ctx, &opencode.RemoteConfig{
		BaseURL:        baseURL,
		Password:       os.Getenv("OPENCODE_SERVER_PASSWORD"),
		PermissionAuto: true,
	})
	if err != nil {
		log.Fatalf("attach to %s: %v", baseURL, err)
	}
	defer oc.Close()

	health, err := oc.Health(ctx)
	if err != nil {
		log.Fatalf("health: %v", err)
	}
	fmt.Printf("attached to opencode %s at %s\n", health.Version, baseURL)

	sessions, err := oc.SessionList(ctx)
	if err != nil {
		log.Fatalf("session list: %v", err)
	}
	fmt.Printf("server has %d sessions\n", len(sessions))
	for i, info := range sessions {
		if i == 5 {
			fmt.Println("...")
			break
		}
		fmt.Printf("  %s  %s\n", info.ID, info.Title)
	}
}
