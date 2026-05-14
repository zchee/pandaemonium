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

	"github.com/zchee/pandaemonium/pkg/codex"
	"github.com/zchee/pandaemonium/pkg/codex/examples/internal/exampleutil"
)

func main() {
	ctx, cancel := exampleutil.NewContext()
	defer cancel()

	client, err := exampleutil.NewCodex(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	thread, err := client.ThreadStart(ctx, exampleutil.DefaultThreadParams())
	if err != nil {
		log.Fatal(err)
	}
	personality := codex.PersonalityPragmatic
	summary := codex.ReasoningSummary(codex.ReasoningSummaryValueConcise)
	result, err := thread.Run(ctx,
		codex.TextInput{Text: "Analyze a safe rollout plan for enabling a feature flag in production. Return JSON matching the requested schema."},
		&codex.TurnStartParams{
			OutputSchema: exampleutil.OutputSchema(),
			Personality:  &personality,
			Summary:      &summary,
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	plan, err := exampleutil.ParseRolloutPlan(result.FinalResponse)
	if err != nil {
		log.Fatalf("expected structured output matching schema: %v", err)
	}
	fmt.Println("Status:", result.Turn.Status)
	fmt.Println("summary:", plan.Summary)
	fmt.Println("actions:")
	for _, action := range plan.Actions {
		fmt.Println("-", action)
	}
	fmt.Println("Items:", len(result.Items))
}
