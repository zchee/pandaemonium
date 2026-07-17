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

// Command quickstart spawns `opencode serve`, starts a session, and runs one
// blocking prompt. The server-side default model from your opencode
// configuration is used; set OPENCODE_MODEL="providerID/modelID" to override.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/zchee/pandaemonium/pkg/llm/opencode"
)

// modelFromEnv parses OPENCODE_MODEL ("providerID/modelID") when set.
func modelFromEnv() *opencode.ModelRef {
	providerID, modelID, ok := strings.Cut(os.Getenv("OPENCODE_MODEL"), "/")
	if !ok || providerID == "" || modelID == "" {
		return nil
	}
	return &opencode.ModelRef{ProviderID: providerID, ModelID: modelID}
}

func main() {
	ctx := context.Background()

	oc, err := opencode.NewOpencode(ctx, &opencode.Config{PermissionAuto: true})
	if err != nil {
		log.Fatalf("start opencode: %v", err)
	}
	defer oc.Close()

	health, err := oc.Health(ctx)
	if err != nil {
		log.Fatalf("health: %v", err)
	}
	fmt.Printf("opencode %s healthy=%t\n", health.Version, health.Healthy)

	session, err := oc.SessionStart(ctx, &opencode.SessionNewParams{Title: "quickstart"})
	if err != nil {
		log.Fatalf("session start: %v", err)
	}
	defer oc.SessionDelete(ctx, session.ID())

	result, err := session.Run(ctx, "Reply with exactly one word: pong", &opencode.PromptParams{Model: modelFromEnv()})
	if err != nil {
		log.Fatalf("run: %v", err)
	}
	fmt.Printf("assistant: %s\n", result.FinalResponse)
	fmt.Printf("tokens: in=%.0f out=%.0f cost=%.5f\n", result.Usage.Input, result.Usage.Output, result.Usage.Cost)
}
