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

package claude

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ── NewSDKMCPServer / ToolDefinition accessors ────────────────────────────────

func TestNewSDKMCPServer_Accessors(t *testing.T) {
	t.Parallel()

	srv := NewSDKMCPServer("my-tools", "1.2.3")

	if got := srv.Name(); got != "my-tools" {
		t.Errorf("Name() = %q, want %q", got, "my-tools")
	}
	if got := srv.Version(); got != "1.2.3" {
		t.Errorf("Version() = %q, want %q", got, "1.2.3")
	}
	if got := srv.Mode(); got != MCPServerModeInProcess {
		t.Errorf("Mode() = %q, want %q", got, MCPServerModeInProcess)
	}
}

// ── ToolDefinition accessors ─────────────────────────────────────────────────

func TestToolDefinition_Accessors(t *testing.T) {
	t.Parallel()

	type in struct{ N int }
	def := Tool("add-one", "increments N", nil, func(_ context.Context, i in) (ToolResult, error) {
		return ToolResult{Content: "ok"}, nil
	})

	if def.Name() != "add-one" {
		t.Errorf("Name() = %q, want %q", def.Name(), "add-one")
	}
	if def.Description() != "increments N" {
		t.Errorf("Description() = %q, want %q", def.Description(), "increments N")
	}
	if def.Schema() != nil {
		t.Errorf("Schema() = %v, want nil", def.Schema())
	}
}

// ── inProcessMCPServer.start / Close ─────────────────────────────────────────

func TestInProcessMCPServer_StartClose(t *testing.T) {
	t.Parallel()

	srv := NewSDKMCPServer("test-srv", "0.1.0").(*inProcessMCPServer)

	cliR, cliW, err := srv.start(t.Context())
	if err != nil {
		t.Fatalf("start() error = %v", err)
	}
	if cliR == nil {
		t.Fatal("start() cliR is nil")
	}
	if cliW == nil {
		t.Fatal("start() cliW is nil")
	}
	// Close must drain the goroutine without hanging.
	if err := cliW.Close(); err != nil {
		t.Errorf("cliW.Close() error = %v", err)
	}
	cliR.Close()
	if err := srv.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestInProcessMCPServer_CloseIdempotent(t *testing.T) {
	t.Parallel()

	srv := NewSDKMCPServer("test-srv", "0.1.0").(*inProcessMCPServer)
	cliR, cliW, err := srv.start(t.Context())
	if err != nil {
		t.Fatalf("start() error = %v", err)
	}
	cliW.Close()
	cliR.Close()

	// First Close stops the goroutine.
	if err := srv.Close(); err != nil {
		t.Errorf("Close() #1 error = %v", err)
	}
	// Second Close must be a no-op (idempotent).
	if err := srv.Close(); err != nil {
		t.Errorf("Close() #2 error = %v", err)
	}
}

func TestInProcessMCPServer_CloseBeforeStart(t *testing.T) {
	t.Parallel()

	// Close on a server that was never started must not panic or error.
	srv := NewSDKMCPServer("test-srv", "0.1.0")
	if err := srv.Close(); err != nil {
		t.Errorf("Close() before start error = %v", err)
	}
}

// ── tool dispatch via MCP protocol ───────────────────────────────────────────

// echoInput is the JSON-unmarshalable input for the echo tool used in tests.
type echoInput struct {
	Msg string `json:"msg"`
}

// TestInProcessMCPServer_ToolDispatch connects a real gomcp.Client to the
// pipe bridge returned by start() and verifies that a registered tool is
// invoked correctly end-to-end.
func TestInProcessMCPServer_ToolDispatch(t *testing.T) {
	t.Parallel()

	type testCase struct {
		toolName    string
		args        map[string]string
		wantContent string
		wantIsError bool
	}
	tests := map[string]testCase{
		"success: echo tool returns msg": {
			toolName:    "echo",
			args:        map[string]string{"msg": "hello from test"},
			wantContent: "hello from test",
			wantIsError: false,
		},
		"success: empty args": {
			toolName:    "echo",
			args:        nil,
			wantContent: "",
			wantIsError: false,
		},
	}

	echoTool := Tool("echo", "echoes the msg field", nil, func(_ context.Context, i echoInput) (ToolResult, error) {
		return ToolResult{Content: i.Msg}, nil
	})

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := NewSDKMCPServer("test-srv", "0.1.0", echoTool).(*inProcessMCPServer)
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)

			cliR, cliW, err := srv.start(ctx)
			if err != nil {
				t.Fatalf("start() error = %v", err)
			}
			t.Cleanup(func() {
				cliW.Close()
				cliR.Close()
				srv.Close()
			})

			// Build a real MCP client over the pipe bridge.
			client := gomcp.NewClient(
				&gomcp.Implementation{Name: "test-client", Version: "1.0.0"},
				nil,
			)
			clientTransport := &gomcp.IOTransport{Reader: cliR, Writer: cliW}
			cs, err := client.Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatalf("client.Connect() error = %v", err)
			}
			t.Cleanup(func() { cs.Close() })

			// Call the tool.
			var args any
			if tc.args != nil {
				args = tc.args
			}
			result, err := cs.CallTool(ctx, &gomcp.CallToolParams{
				Name:      tc.toolName,
				Arguments: args,
			})
			if err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}

			if result.IsError != tc.wantIsError {
				t.Errorf("IsError = %v, want %v", result.IsError, tc.wantIsError)
			}

			var gotContent string
			if len(result.Content) > 0 {
				if tc, ok := result.Content[0].(*gomcp.TextContent); ok {
					gotContent = tc.Text
				}
			}
			if diff := cmp.Diff(tc.wantContent, gotContent); diff != "" {
				t.Errorf("content mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestInProcessMCPServer_ToolError verifies that a tool returning an error
// is surfaced as IsError=true in the CallToolResult.
func TestInProcessMCPServer_ToolError(t *testing.T) {
	t.Parallel()

	errTool := Tool("fail", "always fails", nil, func(_ context.Context, _ struct{}) (ToolResult, error) {
		return ToolResult{}, &CLIConnectionError{Message: "intentional tool error"}
	})

	srv := NewSDKMCPServer("test-srv", "0.1.0", errTool).(*inProcessMCPServer)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	cliR, cliW, err := srv.start(ctx)
	if err != nil {
		t.Fatalf("start() error = %v", err)
	}
	t.Cleanup(func() {
		cliW.Close()
		cliR.Close()
		srv.Close()
	})

	client := gomcp.NewClient(&gomcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	cs, err := client.Connect(ctx, &gomcp.IOTransport{Reader: cliR, Writer: cliW}, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	result, err := cs.CallTool(ctx, &gomcp.CallToolParams{Name: "fail"})
	if err != nil {
		t.Fatalf("CallTool() unexpected transport error = %v", err)
	}
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if len(result.Content) == 0 {
		t.Fatal("Content is empty, want error message")
	}
	tc, ok := result.Content[0].(*gomcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] type = %T, want *gomcp.TextContent", result.Content[0])
	}
	// CLIConnectionError.Error() wraps the message; just check it's present.
	const wantSubstr = "intentional tool error"
	if !strings.Contains(tc.Text, wantSubstr) {
		t.Errorf("Content[0].Text = %q, want to contain %q", tc.Text, wantSubstr)
	}
}
