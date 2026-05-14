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
	"log"
	"strings"

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
	steerTurn, err := thread.Turn(ctx, codex.TextInput{Text: "Count from 1 to 40 with commas, then one summary sentence."}, nil)
	if err != nil {
		log.Fatal(err)
	}
	steerResult := "sent"
	if _, err := steerTurn.Steer(ctx, codex.TextInput{Text: "Keep it brief and stop after 10 numbers."}); err != nil {
		steerResult = fmt.Sprintf("skipped %T", err)
	}
	steerEventCount, steerStatus, steerPreview := consumeControlStream(ctx, steerTurn)

	interruptTurn, err := thread.Turn(ctx, codex.TextInput{Text: "Count from 1 to 200 with commas, then one summary sentence."}, nil)
	if err != nil {
		log.Fatal(err)
	}
	interruptResult := "sent"
	if _, err := interruptTurn.Interrupt(ctx); err != nil {
		interruptResult = fmt.Sprintf("skipped %T", err)
	}
	interruptEventCount, interruptStatus, interruptPreview := consumeControlStream(ctx, interruptTurn)

	fmt.Println("steer.result:", steerResult)
	fmt.Println("steer.final.status:", steerStatus)
	fmt.Println("steer.events.count:", steerEventCount)
	fmt.Println("steer.assistant.preview:", steerPreview)
	fmt.Println("interrupt.result:", interruptResult)
	fmt.Println("interrupt.final.status:", interruptStatus)
	fmt.Println("interrupt.events.count:", interruptEventCount)
	fmt.Println("interrupt.assistant.preview:", interruptPreview)
}

func consumeControlStream(ctx context.Context, turn *codex.TurnHandle) (int, string, string) {
	eventCount := 0
	completedStatus := "unknown"
	var completedTurn *codex.Turn
	for event, err := range turn.Stream(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		eventCount++
		if completed, ok, err := event.TurnCompleted(); err != nil {
			log.Fatal(err)
		} else if ok {
			completedStatus = string(completed.Turn.Status)
			completedTurn = &completed.Turn
		}
	}
	preview := strings.TrimSpace(exampleutil.AssistantTextFromTurn(completedTurn))
	if preview == "" {
		preview = "[no assistant text]"
	}
	return eventCount, completedStatus, preview
}
