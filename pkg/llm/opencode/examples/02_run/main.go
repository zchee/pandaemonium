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

// Command run demonstrates the sync surface: providers discovery, an
// explicit-model run, session titling, and reading back session history.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/zchee/pandaemonium/pkg/llm/opencode"
)

func main() {
	ctx := context.Background()

	oc, err := opencode.NewOpencode(ctx, &opencode.Config{PermissionAuto: true})
	if err != nil {
		log.Fatalf("start opencode: %v", err)
	}
	defer oc.Close()

	providers, err := oc.Providers(ctx)
	if err != nil {
		log.Fatalf("providers: %v", err)
	}
	var model *opencode.ModelRef
	for providerID, modelID := range providers.Default {
		model = &opencode.ModelRef{ProviderID: providerID, ModelID: modelID}
		break
	}
	if model == nil {
		log.Fatal("no configured providers; run `opencode auth login` first")
	}
	fmt.Printf("using %s/%s\n", model.ProviderID, model.ModelID)

	session, err := oc.SessionStart(ctx, nil)
	if err != nil {
		log.Fatalf("session start: %v", err)
	}
	defer oc.SessionDelete(ctx, session.ID())

	if _, err := session.SetTitle(ctx, "sync run example"); err != nil {
		log.Fatalf("set title: %v", err)
	}

	result, err := session.Run(ctx, "In one short sentence: what is a goroutine?", &opencode.PromptParams{Model: model})
	if err != nil {
		log.Fatalf("run: %v", err)
	}
	fmt.Printf("assistant: %s\n", result.FinalResponse)

	read, err := session.Read(ctx)
	if err != nil {
		log.Fatalf("read: %v", err)
	}
	fmt.Printf("session %q holds %d messages\n", read.Info.Title, len(read.Messages))
}
