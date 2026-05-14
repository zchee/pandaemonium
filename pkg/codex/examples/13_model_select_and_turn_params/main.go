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
	"os"

	"github.com/zchee/pandaemonium/pkg/codex"
	"github.com/zchee/pandaemonium/pkg/codex/examples/internal/exampleutil"

	"github.com/go-json-experiment/json/jsontext"
)

func main() {
	ctx, cancel := exampleutil.NewContext()
	defer cancel()

	client, err := exampleutil.NewCodex(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	models, err := client.Models(ctx, true)
	if err != nil {
		log.Fatal(err)
	}
	selectedModel, ok := exampleutil.PickHighestModel(models.Data, exampleutil.DefaultModel())
	if !ok {
		log.Fatal("no models available")
	}
	selectedEffort := exampleutil.PickHighestTurnEffort(selectedModel)
	fmt.Println("selected.model:", selectedModel.Model)
	fmt.Println("selected.effort:", selectedEffort)

	thread, err := client.ThreadStart(ctx, &codex.ThreadStartParams{
		Model: &selectedModel.Model,
		Config: map[string]jsontext.Value{
			"model_reasoning_effort": exampleutil.MustJSONValue(selectedEffort),
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	first, err := thread.Run(ctx,
		codex.TextInput{Text: "Give one short sentence about reliable production releases."},
		&codex.TurnStartParams{Model: &selectedModel.Model, Effort: &selectedEffort},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("agent.message:", first.FinalResponse)
	fmt.Println("items:", len(first.Items))

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	personality := codex.PersonalityPragmatic
	summary := codex.ReasoningSummary(codex.ReasoningSummaryValueConcise)
	sandbox := exampleutil.ReadOnlySandboxPolicy()
	second, err := thread.Run(ctx,
		codex.TextInput{Text: "Return JSON for a safe feature-flag rollout plan."},
		&codex.TurnStartParams{
			Cwd:           &cwd,
			Effort:        &selectedEffort,
			Model:         &selectedModel.Model,
			OutputSchema:  exampleutil.OutputSchema(),
			Personality:   &personality,
			SandboxPolicy: &sandbox,
			Summary:       &summary,
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("agent.message.params:", second.FinalResponse)
	plan, err := exampleutil.ParseRolloutPlan(second.FinalResponse)
	if err != nil {
		log.Fatalf("expected structured output matching schema: %v", err)
	}
	fmt.Println("summary.params:", plan.Summary)
	fmt.Println("actions.params:", len(plan.Actions))
	fmt.Println("items.params:", len(second.Items))
}
