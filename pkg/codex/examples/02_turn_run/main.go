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
	handle, err := thread.Turn(ctx, codex.TextInput{Text: "Give 3 bullets about SIMD."}, nil)
	if err != nil {
		log.Fatal(err)
	}
	result, err := handle.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
	persisted, err := thread.Read(ctx, true)
	if err != nil {
		log.Fatal(err)
	}
	persistedTurn := exampleutil.FindTurnByID(persisted.Thread.Turns, result.Turn.ID)

	fmt.Println("thread_id:", thread.ID())
	fmt.Println("turn_id:", result.Turn.ID)
	fmt.Println("status:", result.Turn.Status)
	if result.Turn.Error != nil {
		fmt.Println("error:", result.Turn.Error)
	}
	fmt.Println("text:", exampleutil.AssistantTextFromTurn(persistedTurn))
	if persistedTurn == nil {
		fmt.Println("persisted.items.count:", 0)
	} else {
		fmt.Println("persisted.items.count:", len(persistedTurn.Items))
	}
}
