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

package codex_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-json-experiment/json/jsontext"

	"github.com/zchee/pandaemonium/pkg/codex"
)

const realAppServerIntegrationEnv = "RUN_REAL_CODEX_TESTS"

func TestRealAppServerIntegrationInitializeAndModelListPort(t *testing.T) {
	sdk, ctx := newRealIntegrationCodex(t, 45*time.Second)

	assertRealInitializedMetadata(t, sdk.Metadata())

	includeHidden := true
	models, err := sdk.Models(ctx, &codex.ModelListParams{IncludeHidden: &includeHidden})
	if err != nil {
		t.Fatalf("Models(includeHidden=true) real app-server error = %v", err)
	}
	if models.Data == nil {
		t.Fatalf("Models(includeHidden=true).Data = nil, want list shape")
	}
	for index, model := range models.Data {
		if strings.TrimSpace(model.ID) == "" {
			t.Fatalf("Models().Data[%d].ID is empty: %#v", index, model)
		}
	}
}

func TestRealAppServerIntegrationThreadTurnAndRunPort(t *testing.T) {
	sdk, ctx := newRealIntegrationCodex(t, 3*time.Minute)

	thread, err := sdk.ThreadStart(ctx, realIntegrationThreadParams())
	if err != nil {
		t.Fatalf("ThreadStart() real app-server error = %v", err)
	}
	if strings.TrimSpace(thread.ID()) == "" {
		t.Fatalf("ThreadStart().ID() is empty")
	}

	turn, err := thread.Turn(ctx, codex.TextInput{Text: "Say ok in one short sentence."}, nil)
	if err != nil {
		t.Fatalf("Thread.Turn() real app-server error = %v", err)
	}
	result, err := turn.Run(ctx)
	if err != nil {
		t.Fatalf("TurnHandle.Run() real app-server error = %v", err)
	}
	assertRealCompletedTurn(t, result)

	includeTurns := true
	persisted, err := thread.Read(ctx, &codex.ThreadReadParams{IncludeTurns: &includeTurns})
	if err != nil {
		t.Fatalf("Thread.Read(includeTurns=true) real app-server error = %v", err)
	}
	if strings.TrimSpace(persisted.Thread.ID) != thread.ID() {
		t.Fatalf("Thread.Read().Thread.ID = %q, want %q", persisted.Thread.ID, thread.ID())
	}
	if persistedTurn, ok := realIntegrationFindTurn(persisted.Thread.Turns, result.Turn.ID); ok {
		if strings.TrimSpace(persistedTurn.ID) == "" {
			t.Fatalf("persisted turn has empty ID: %#v", persistedTurn)
		}
	}
}

func TestRealAppServerIntegrationThreadRunConveniencePort(t *testing.T) {
	sdk, ctx := newRealIntegrationCodex(t, 3*time.Minute)

	thread, err := sdk.ThreadStart(ctx, realIntegrationThreadParams())
	if err != nil {
		t.Fatalf("ThreadStart() real app-server error = %v", err)
	}
	result, err := thread.Run(ctx, "Say ok.", nil)
	if err != nil {
		t.Fatalf("Thread.Run() real app-server error = %v", err)
	}
	assertRealCompletedRunResult(t, result)
}

func TestRealAppServerIntegrationStreamingAndInterruptPort(t *testing.T) {
	sdk, ctx := newRealIntegrationCodex(t, 4*time.Minute)

	streamThread, err := sdk.StreamThreadStart(ctx, realIntegrationThreadParams())
	if err != nil {
		t.Fatalf("StreamThreadStart() real app-server error = %v", err)
	}
	if strings.TrimSpace(streamThread.ID()) == "" {
		t.Fatalf("StreamThreadStart().ID() is empty")
	}

	sawCompleted := false
	for event, err := range streamThread.RunStream(ctx, codex.TextInput{Text: "Reply with one short sentence."}, nil) {
		if err != nil {
			t.Fatalf("StreamThread.RunStream() real app-server error = %v", err)
		}
		completed, ok, err := event.TurnCompleted()
		if err != nil {
			t.Fatalf("Notification.TurnCompleted() error = %v", err)
		}
		if ok {
			if completed.Turn.Status != codex.TurnStatusCompleted {
				t.Fatalf("stream completed status = %q, want completed", completed.Turn.Status)
			}
			sawCompleted = true
		}
	}
	if !sawCompleted {
		t.Fatal("RunStream() ended without turn/completed notification")
	}

	interruptTurn, err := streamThread.Turn(ctx, codex.TextInput{Text: "Count from 1 to 200 with commas."}, nil)
	if err != nil {
		t.Fatalf("StreamThread.Turn(interrupt) real app-server error = %v", err)
	}
	if _, err := interruptTurn.Interrupt(ctx); err != nil {
		t.Fatalf("StreamTurnHandle.Interrupt() real app-server error = %v", err)
	}

	followUpTurn, err := streamThread.Turn(ctx, codex.TextInput{Text: "Say ok only."}, nil)
	if err != nil {
		t.Fatalf("StreamThread.Turn(follow-up) real app-server error = %v", err)
	}
	sawFollowUpTerminal := false
	for event, err := range followUpTurn.Stream(ctx) {
		if err != nil {
			t.Fatalf("follow-up Stream() real app-server error = %v", err)
		}
		completed, ok, err := event.TurnCompleted()
		if err != nil {
			t.Fatalf("follow-up Notification.TurnCompleted() error = %v", err)
		}
		if !ok {
			continue
		}
		sawFollowUpTerminal = true
		if completed.Turn.Status != codex.TurnStatusCompleted && completed.Turn.Status != codex.TurnStatusFailed {
			t.Fatalf("follow-up status = %q, want completed or failed", completed.Turn.Status)
		}
	}
	if !sawFollowUpTerminal {
		t.Fatal("follow-up stream ended without turn/completed notification")
	}
}

func TestRealAppServerIntegrationExamplesRunPort(t *testing.T) {
	requireRealCodexBinary(t)

	repoRoot := realIntegrationRepoRoot(t)
	cases := []string{
		"01_quickstart_constructor",
		"02_turn_run",
		"03_turn_stream_events",
		"04_models_and_metadata",
		"05_existing_thread",
		"06_thread_lifecycle_and_controls",
		"07_image_and_text",
		"08_local_image_and_text",
		"09_stream_parity",
		"10_error_handling_and_retry",
		"11_cli_mini_app",
		"12_turn_params_kitchen_sink",
		"13_model_select_and_turn_params",
		"14_turn_controls",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			t.Cleanup(cancel)

			cmd := exec.CommandContext(ctx, "go", "run", "./pkg/codex/examples/"+name)
			cmd.Dir = repoRoot
			cmd.Env = os.Environ()
			if name == "11_cli_mini_app" {
				cmd.Stdin = strings.NewReader("Give 3 short bullets on SIMD.\nNow rewrite that as 1 short sentence.\n/exit\n")
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go run ./pkg/codex/examples/%s error = %v\nOUTPUT:\n%s", name, err, output)
			}
			assertRealExampleOutput(t, name, string(output))
		})
	}
}

func TestRealAppServerIntegrationWebSocketCapabilityTokenPort(t *testing.T) {
	codexBin := requireRealCodexBinary(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	t.Cleanup(cancel)

	tokenFile := filepath.Join(t.TempDir(), "capability.token")
	if err := os.WriteFile(tokenFile, []byte("capability-token\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", tokenFile, err)
	}
	sdk, err := codex.NewCodex(ctx, &codex.Config{
		CodexBin: codexBin,
		Cwd:      t.TempDir(),
		Listen: codex.ListenConfig{
			URL: "ws://127.0.0.1:" + reserveRealIntegrationLoopbackPort(t),
			WebSocket: &codex.WebSocketConfig{
				AuthMode:    codex.WebSocketAuthCapabilityToken,
				TokenFile:   tokenFile,
				DialTimeout: 500 * time.Millisecond,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCodex() real websocket capability-token error = %v", err)
	}
	t.Cleanup(func() {
		if err := sdk.Close(); err != nil {
			t.Fatalf("Codex.Close() real websocket capability-token error = %v", err)
		}
	})

	assertRealInitializedMetadata(t, sdk.Metadata())
}

func TestRealAppServerIntegrationWebSocketSignedBearerTokenPort(t *testing.T) {
	codexBin := requireRealCodexBinary(t)
	sharedSecret := strings.TrimSpace(os.Getenv("CODEX_REAL_WS_SHARED_SECRET"))
	clientBearer := strings.TrimSpace(os.Getenv("CODEX_REAL_WS_BEARER_TOKEN"))
	if sharedSecret == "" || clientBearer == "" {
		t.Skip("set CODEX_REAL_WS_SHARED_SECRET and CODEX_REAL_WS_BEARER_TOKEN for signed-bearer-token real websocket coverage")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	t.Cleanup(cancel)
	sharedSecretFile := filepath.Join(t.TempDir(), "shared-secret")
	if err := os.WriteFile(sharedSecretFile, []byte(sharedSecret+"\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", sharedSecretFile, err)
	}
	sdk, err := codex.NewCodex(ctx, &codex.Config{
		CodexBin: codexBin,
		Cwd:      t.TempDir(),
		Listen: codex.ListenConfig{
			URL: "ws://127.0.0.1:" + reserveRealIntegrationLoopbackPort(t),
			WebSocket: &codex.WebSocketConfig{
				AuthMode:          codex.WebSocketAuthSignedBearerToken,
				SharedSecretFile:  sharedSecretFile,
				ClientBearerToken: clientBearer,
				DialTimeout:       500 * time.Millisecond,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCodex() real websocket signed-bearer-token error = %v", err)
	}
	t.Cleanup(func() {
		if err := sdk.Close(); err != nil {
			t.Fatalf("Codex.Close() real websocket signed-bearer-token error = %v", err)
		}
	})

	assertRealInitializedMetadata(t, sdk.Metadata())
}

func newRealIntegrationCodex(t *testing.T, timeout time.Duration) (*codex.Codex, context.Context) {
	t.Helper()

	codexBin := requireRealCodexBinary(t)
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)

	sdk, err := codex.NewCodex(ctx, &codex.Config{CodexBin: codexBin, Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("NewCodex() real app-server error = %v", err)
	}
	t.Cleanup(func() {
		if err := sdk.Close(); err != nil {
			t.Fatalf("Codex.Close() real app-server error = %v", err)
		}
	})
	return sdk, ctx
}

func assertRealInitializedMetadata(t *testing.T, metadata codex.InitializeResponse) {
	t.Helper()
	if strings.TrimSpace(metadata.UserAgent) == "" {
		t.Fatalf("Metadata().UserAgent is empty: %#v", metadata)
	}
	if metadata.ServerInfo != nil {
		if strings.TrimSpace(metadata.ServerInfo.Name) == "" {
			t.Fatalf("Metadata().ServerInfo.Name is empty: %#v", metadata.ServerInfo)
		}
		if strings.TrimSpace(metadata.ServerInfo.Version) == "" {
			t.Fatalf("Metadata().ServerInfo.Version is empty: %#v", metadata.ServerInfo)
		}
	}
}

func requireRealCodexBinary(t *testing.T) string {
	t.Helper()

	if os.Getenv(realAppServerIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run real Codex app-server integration coverage", realAppServerIntegrationEnv)
	}
	codexBin, err := exec.LookPath("codex")
	if err != nil {
		t.Skipf("real Codex app-server integration requires codex binary on PATH: %v", err)
	}
	return codexBin
}

func reserveRealIntegrationLoopbackPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen(127.0.0.1:0) error = %v", err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("net.SplitHostPort(%q) error = %v", ln.Addr().String(), err)
	}
	return port
}

func realIntegrationThreadParams() *codex.ThreadStartParams {
	model := strings.TrimSpace(os.Getenv("CODEX_REAL_TEST_MODEL"))
	if model == "" {
		model = "gpt-5.4"
	}
	return &codex.ThreadStartParams{
		Model: &model,
		Config: map[string]jsontext.Value{
			"model_reasoning_effort": jsontext.Value(`"high"`),
		},
	}
}

func assertRealCompletedTurn(t *testing.T, result codex.RunResult) {
	t.Helper()

	if strings.TrimSpace(result.Turn.ID) == "" {
		t.Fatalf("RunResult.Turn.ID is empty: %#v", result.Turn)
	}
	if result.Turn.Status != codex.TurnStatusCompleted {
		t.Fatalf("RunResult.Turn.Status = %q, want completed", result.Turn.Status)
	}
}

func assertRealCompletedRunResult(t *testing.T, result codex.RunResult) {
	t.Helper()

	assertRealCompletedTurn(t, result)
	if strings.TrimSpace(result.FinalResponse) == "" {
		t.Fatalf("RunResult.FinalResponse is empty: %#v", result)
	}
}

func realIntegrationFindTurn(turns []codex.Turn, turnID string) (codex.Turn, bool) {
	for _, turn := range turns {
		if turn.ID == turnID {
			return turn, true
		}
	}
	return codex.Turn{}, false
}

func realIntegrationRepoRoot(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", "..", ".."))
}

func assertRealExampleOutput(t *testing.T, name, output string) {
	t.Helper()

	switch name {
	case "01_quickstart_constructor":
		if !strings.Contains(output, "Server:") || !strings.Contains(output, "Items:") || !strings.Contains(output, "Text:") {
			t.Fatalf("example %s output missing constructor markers:\n%s", name, output)
		}
	case "02_turn_run":
		if !strings.Contains(output, "thread_id:") || !strings.Contains(output, "turn_id:") || !strings.Contains(output, "status:") {
			t.Fatalf("example %s output missing run markers:\n%s", name, output)
		}
	case "03_turn_stream_events":
		if !strings.Contains(output, "stream.completed:") || !strings.Contains(output, "assistant>") {
			t.Fatalf("example %s output missing streaming markers:\n%s", name, output)
		}
	case "04_models_and_metadata":
		if !strings.Contains(output, "server:") || !strings.Contains(output, "models.count:") || !strings.Contains(output, "models:") {
			t.Fatalf("example %s output missing model metadata markers:\n%s", name, output)
		}
	case "05_existing_thread":
		if !strings.Contains(output, "Created thread:") {
			t.Fatalf("example %s output missing existing-thread markers:\n%s", name, output)
		}
	case "06_thread_lifecycle_and_controls":
		if !strings.Contains(output, "Lifecycle OK:") {
			t.Fatalf("example %s output missing lifecycle markers:\n%s", name, output)
		}
	case "07_image_and_text", "08_local_image_and_text":
		if !strings.Contains(strings.ToLower(output), "completed") && !strings.Contains(output, "Status:") {
			t.Fatalf("example %s output missing image status markers:\n%s", name, output)
		}
	case "09_stream_parity":
		if !strings.Contains(output, "Thread:") || !strings.Contains(output, "Turn:") {
			t.Fatalf("example %s output missing stream parity markers:\n%s", name, output)
		}
	case "10_error_handling_and_retry":
		if !strings.Contains(output, "Text:") {
			t.Fatalf("example %s output missing retry text markers:\n%s", name, output)
		}
	case "11_cli_mini_app":
		if !strings.Contains(output, "Thread:") || strings.Count(output, "assistant>") < 2 || strings.Count(output, "usage>") < 2 {
			t.Fatalf("example %s output missing CLI markers:\n%s", name, output)
		}
	case "12_turn_params_kitchen_sink":
		if !strings.Contains(output, "Status:") || !strings.Contains(output, "summary:") || !strings.Contains(output, "actions:") {
			t.Fatalf("example %s output missing kitchen-sink markers:\n%s", name, output)
		}
	case "13_model_select_and_turn_params":
		if !strings.Contains(output, "selected.model:") || !strings.Contains(output, "agent.message.params:") || !strings.Contains(output, "items.params:") {
			t.Fatalf("example %s output missing model-select markers:\n%s", name, output)
		}
	case "14_turn_controls":
		if !strings.Contains(output, "steer.result:") || !strings.Contains(output, "interrupt.result:") {
			t.Fatalf("example %s output missing turn-control markers:\n%s", name, output)
		}
	default:
		t.Fatalf("unhandled real example %q", name)
	}
}
