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
	fmt.Println("Server:", exampleutil.ServerLabel(client.Metadata()))

	thread, err := client.StreamThreadStart(ctx, exampleutil.DefaultThreadParams())
	if err != nil {
		log.Fatal(err)
	}
	turn, err := thread.Turn(ctx, codex.TextInput{Text: "Say hello in one sentence."}, nil)
	if err != nil {
		log.Fatal(err)
	}
	result, err := turn.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
	persisted, err := thread.Read(ctx, true)
	if err != nil {
		log.Fatal(err)
	}
	persistedTurn := exampleutil.FindTurnByID(persisted.Thread.Turns, result.Turn.ID)
	fmt.Println("Thread:", thread.ID())
	fmt.Println("Turn:", result.Turn.ID)
	fmt.Println("Text:", strings.TrimSpace(exampleutil.AssistantTextFromTurn(persistedTurn)))
}
