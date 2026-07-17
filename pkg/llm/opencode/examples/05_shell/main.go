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

// Command shell runs a shell command inside a session context
// (POST /session/{id}/shell) and prints the tool output parts.
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

	session, err := oc.SessionStart(ctx, &opencode.SessionNewParams{Title: "shell example"})
	if err != nil {
		log.Fatalf("session start: %v", err)
	}
	defer oc.SessionDelete(ctx, session.ID())

	result, err := session.Shell(ctx, "build", "git status --short")
	if err != nil {
		log.Fatalf("shell: %v", err)
	}
	for _, part := range result.Parts {
		switch part.Type {
		case "text":
			fmt.Println(part.Text)
		case "tool":
			fmt.Printf("[tool %s call %s]\n", part.Tool, part.CallID)
		}
	}
}
