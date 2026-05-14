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
	"errors"
	"fmt"
	"log"
	"time"

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
	result, err := codex.RetryOnOverload(ctx, codex.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 250 * time.Millisecond,
		MaxDelay:     2 * time.Second,
	}, func() (codex.RunResult, error) {
		return thread.Run(ctx, codex.TextInput{Text: "Summarize retry best practices in 3 bullets."}, nil)
	})
	if err != nil {
		var busy *codex.ServerBusyError
		var rpcErr *codex.JSONRPCError
		switch {
		case errors.As(err, &busy):
			fmt.Println("Server overloaded after retries:", busy.Message)
			fmt.Println("Text:")
		case errors.As(err, &rpcErr):
			fmt.Printf("JSON-RPC error %d: %s\n", rpcErr.Code, rpcErr.Message)
			fmt.Println("Text:")
		default:
			log.Fatal(err)
		}
		return
	}
	if result.Turn.Status == codex.TurnStatusFailed {
		fmt.Println("Turn failed:", result.Turn.Error)
	}
	fmt.Println("Text:", result.FinalResponse)
}
