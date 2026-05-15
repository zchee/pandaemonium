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

	imagePath, cleanup, err := exampleutil.TemporarySampleImagePath()
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	client, err := exampleutil.NewCodex(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	thread, err := client.ThreadStart(ctx, exampleutil.DefaultThreadParams())
	if err != nil {
		log.Fatal(err)
	}
	result, err := thread.Run(ctx, []codex.InputItem{
		codex.TextInput{Text: "Read this generated local image and summarize the colors/layout in 2 bullets."},
		codex.LocalImageInput{Path: imagePath},
	}, nil)
	if err != nil {
		log.Fatal(err)
	}
	includeTurns := true
	persisted, err := thread.Read(ctx, &codex.ThreadReadParams{IncludeTurns: &includeTurns})
	if err != nil {
		log.Fatal(err)
	}
	persistedTurn := exampleutil.FindTurnByID(persisted.Thread.Turns, result.Turn.ID)
	fmt.Println("Status:", result.Turn.Status)
	fmt.Println(exampleutil.AssistantTextFromTurn(persistedTurn))
}
