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
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/zchee/pandaemonium/pkg/codex"
	"github.com/zchee/pandaemonium/pkg/codex/examples/internal/exampleutil"
)

func main() {
	ctx, cancel := exampleutil.NewContext()
	defer cancel()

	fmt.Println("Codex mini CLI. Type /exit to quit.")
	client, err := exampleutil.NewCodex(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	thread, err := client.ThreadStart(ctx, exampleutil.DefaultThreadParams())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Thread:", thread.ID())

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("you> ")
		if !scanner.Scan() {
			break
		}
		userInput := strings.TrimSpace(scanner.Text())
		if userInput == "" {
			continue
		}
		if userInput == "/exit" || userInput == "/quit" {
			break
		}

		turn, err := thread.Turn(ctx, codex.TextInput{Text: userInput}, nil)
		if err != nil {
			log.Fatal(err)
		}
		var usage *codex.ThreadTokenUsage
		status := "unknown"
		var turnErr *codex.TurnError
		printedDelta := false
		fmt.Print("assistant> ")
		for event, err := range turn.Stream(ctx) {
			if err != nil {
				log.Fatal(err)
			}
			if delta, ok, err := event.ItemAgentMessageDelta(); err != nil {
				log.Fatal(err)
			} else if ok && delta.Delta != "" {
				fmt.Print(delta.Delta)
				printedDelta = true
				continue
			}
			if updated, ok, err := event.ThreadTokenUsageUpdated(); err != nil {
				log.Fatal(err)
			} else if ok {
				snapshot := updated.TokenUsage
				usage = &snapshot
				continue
			}
			if completed, ok, err := event.TurnCompleted(); err != nil {
				log.Fatal(err)
			} else if ok {
				status = string(completed.Turn.Status)
				turnErr = completed.Turn.Error
			}
		}
		if printedDelta {
			fmt.Println()
		} else {
			fmt.Println("[no text]")
		}
		fmt.Println("assistant.status>", status)
		if status == string(codex.TurnStatusFailed) {
			fmt.Println("assistant.error>", turnErr)
		}
		fmt.Println(exampleutil.FormatUsage(usage))
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
