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
	first, err := thread.Run(ctx, codex.TextInput{Text: "One sentence about structured planning."}, nil)
	if err != nil {
		log.Fatal(err)
	}
	second, err := thread.Run(ctx, codex.TextInput{Text: "Now restate it for a junior engineer."}, nil)
	if err != nil {
		log.Fatal(err)
	}

	reopened, err := client.ThreadResume(ctx, thread.ID(), nil)
	if err != nil {
		log.Fatal(err)
	}
	limit := int32(20)
	archivedFalse := false
	listingActive, err := client.ThreadList(ctx, &codex.ThreadListParams{Limit: &limit, Archived: &archivedFalse})
	if err != nil {
		log.Fatal(err)
	}
	includeTurns := true
	reading, err := reopened.Read(ctx, &codex.ThreadReadParams{IncludeTurns: &includeTurns})
	if err != nil {
		log.Fatal(err)
	}
	_, _ = reopened.SetName(ctx, "sdk-lifecycle-demo")
	_, _ = client.ThreadArchive(ctx, reopened.ID())
	archivedTrue := true
	listingArchived, err := client.ThreadList(ctx, &codex.ThreadListParams{Limit: &limit, Archived: &archivedTrue})
	if err != nil {
		log.Fatal(err)
	}
	unarchived, err := client.ThreadUnarchive(ctx, reopened.ID())
	if err != nil {
		log.Fatal(err)
	}

	resumedInfo := "n/a"
	if resumed, err := client.ThreadResume(ctx, unarchived.ID(), &codex.ThreadResumeParams{Model: exampleutil.DefaultThreadParams().Model, Config: exampleutil.DefaultThreadParams().Config}); err == nil {
		if result, err := resumed.Run(ctx, codex.TextInput{Text: "Continue in one short sentence."}, nil); err == nil {
			resumedInfo = fmt.Sprintf("%s %s", result.Turn.ID, result.Turn.Status)
		} else {
			resumedInfo = fmt.Sprintf("skipped(%T)", err)
		}
	} else {
		resumedInfo = fmt.Sprintf("skipped(%T)", err)
	}

	forkedInfo := "n/a"
	if forked, err := client.ThreadFork(ctx, unarchived.ID(), &codex.ThreadForkParams{Model: exampleutil.DefaultThreadParams().Model}); err == nil {
		if result, err := forked.Run(ctx, codex.TextInput{Text: "Take a different angle in one short sentence."}, nil); err == nil {
			forkedInfo = fmt.Sprintf("%s %s", result.Turn.ID, result.Turn.Status)
		} else {
			forkedInfo = fmt.Sprintf("skipped(%T)", err)
		}
	} else {
		forkedInfo = fmt.Sprintf("skipped(%T)", err)
	}

	compactInfo := "sent"
	if _, err := unarchived.Compact(ctx); err != nil {
		compactInfo = fmt.Sprintf("skipped(%T)", err)
	}

	fmt.Println("Lifecycle OK:", thread.ID())
	fmt.Println("first:", first.Turn.ID, first.Turn.Status)
	fmt.Println("second:", second.Turn.ID, second.Turn.Status)
	fmt.Println("read.turns:", len(reading.Thread.Turns))
	fmt.Println("list.active:", len(listingActive.Data))
	fmt.Println("list.archived:", len(listingArchived.Data))
	fmt.Println("resumed:", resumedInfo)
	fmt.Println("forked:", forkedInfo)
	fmt.Println("compact:", compactInfo)
}
