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

	original, err := client.ThreadStart(ctx, exampleutil.DefaultThreadParams())
	if err != nil {
		log.Fatal(err)
	}
	if _, err := original.Run(ctx, codex.TextInput{Text: "Tell me one fact about Saturn."}, nil); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Created thread:", original.ID())

	resumed, err := client.ThreadResume(ctx, original.ID(), nil)
	if err != nil {
		log.Fatal(err)
	}
	second, err := resumed.Run(ctx, codex.TextInput{Text: "Continue with one more fact."}, nil)
	if err != nil {
		log.Fatal(err)
	}
	persisted, err := resumed.Read(ctx, true)
	if err != nil {
		log.Fatal(err)
	}
	persistedTurn := exampleutil.FindTurnByID(persisted.Thread.Turns, second.Turn.ID)
	fmt.Println(exampleutil.AssistantTextFromTurn(persistedTurn))
}
