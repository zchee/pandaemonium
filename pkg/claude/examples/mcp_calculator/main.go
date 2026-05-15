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

// Command mcp_calculator demonstrates registering an in-process MCP server via
// [claude.NewSDKMCPServer] and [claude.Tool]. Four arithmetic tools (add,
// subtract, multiply, divide) are exposed to the claude CLI subprocess over an
// in-process goroutine bridge backed by two io.Pipe pairs.
//
// Port of examples/mcp_calculator.py from claude-agent-sdk-python.
//
// Usage:
//
//	RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/mcp_calculator
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/zchee/pandaemonium/pkg/claude"
)

// calcInput is the typed input for all four arithmetic tools.
type calcInput struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
}

func main() {
	if os.Getenv("RUN_REAL_CLAUDE_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "mcp_calculator: set RUN_REAL_CLAUDE_TESTS=1 to run against the real claude CLI.")
		return
	}

	addTool := claude.Tool("add", "Add two numbers. Returns a+b.", nil,
		func(_ context.Context, in calcInput) (claude.ToolResult, error) {
			return claude.ToolResult{Content: fmt.Sprintf("%g", in.A+in.B)}, nil
		})

	subtractTool := claude.Tool("subtract", "Subtract b from a. Returns a-b.", nil,
		func(_ context.Context, in calcInput) (claude.ToolResult, error) {
			return claude.ToolResult{Content: fmt.Sprintf("%g", in.A-in.B)}, nil
		})

	multiplyTool := claude.Tool("multiply", "Multiply two numbers. Returns a*b.", nil,
		func(_ context.Context, in calcInput) (claude.ToolResult, error) {
			return claude.ToolResult{Content: fmt.Sprintf("%g", in.A*in.B)}, nil
		})

	divideTool := claude.Tool("divide", "Divide a by b. Returns an error if b is zero.", nil,
		func(_ context.Context, in calcInput) (claude.ToolResult, error) {
			if in.B == 0 {
				return claude.ToolResult{Content: "error: division by zero", IsError: true}, nil
			}
			return claude.ToolResult{Content: fmt.Sprintf("%g", in.A/in.B)}, nil
		})

	srv := claude.NewSDKMCPServer("calculator", "1.0.0", addTool, subtractTool, multiplyTool, divideTool)

	ctx := context.Background()

	opts := &claude.Options{
		MCPServers: []claude.MCPServer{srv},
		MaxTurns:   3,
	}

	for msg, err := range claude.Query(ctx, "What is (10 + 5) * 3 - 7? Use the calculator tools step by step.", opts) {
		if err != nil {
			log.Fatal(err)
		}
		if am, ok := msg.(claude.AssistantMessage); ok {
			for _, b := range am.Content {
				if tb, ok := b.(claude.TextBlock); ok {
					fmt.Print(tb.Text)
				}
			}
		}
	}
	fmt.Println()
}
