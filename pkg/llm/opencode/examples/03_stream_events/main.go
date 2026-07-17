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

// Command stream_events demonstrates an asynchronous turn: register the
// session-scoped stream, watch text deltas arrive live, and inspect the
// wrapper's observability counters afterwards. This example doubles as a
// manual smoke check for the SSE bus.
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

	session, err := oc.SessionStart(ctx, &opencode.SessionNewParams{Title: "stream example"})
	if err != nil {
		log.Fatalf("session start: %v", err)
	}
	defer oc.SessionDelete(ctx, session.ID())

	handle, err := session.Turn(ctx, "Count from 1 to 10, one number per line.", nil)
	if err != nil {
		log.Fatalf("turn: %v", err)
	}

	for ev, err := range handle.Stream(ctx) {
		if err != nil {
			log.Fatalf("stream: %v", err)
		}
		switch ev.Type {
		case opencode.EventTypeMessagePartDelta:
			fmt.Print(".")
		case opencode.EventTypeGap:
			fmt.Println("\n[gap: events may have been lost across a reconnect]")
		default:
			fmt.Printf("\n[%s]", ev.Type)
		}
	}
	fmt.Println()

	counters := oc.Client().Counters()
	fmt.Printf("reconnects=%d gaps=%d streams-without-terminal=%d\n",
		counters.SSEReconnects, counters.GapNotifications, counters.StreamsWithoutTerminal)
}
