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
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-json-experiment/json/jsontext"

	"github.com/zchee/pandaemonium/pkg/llm/codex"
)

const realAppServerIntegrationEnv = "RUN_REAL_CODEX_TESTS"

func TestRealAppServerIntegrationInitializeAndModelListPort(t *testing.T) {
	sdk, ctx := newRealIntegrationCodex(t, 45*time.Second)

	assertRealInitializedMetadata(t, sdk.Metadata())

	includeHidden := true
	models, err := sdk.Models(ctx, &codex.ModelListParams{IncludeHidden: includeHidden})
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

	turn, err := thread.Turn(ctx, "Say ok in one short sentence.", nil)
	if err != nil {
		t.Fatalf("Thread.Turn() real app-server error = %v", err)
	}
	result, err := turn.Run(ctx)
	if err != nil {
		t.Fatalf("TurnHandle.Run() real app-server error = %v", err)
	}
	assertRealCompletedTurn(t, result)

	includeTurns := true
	persisted, err := thread.Read(ctx, &codex.ThreadReadParams{IncludeTurns: includeTurns})
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
	for event, err := range streamThread.RunStream(ctx, "Reply with one short sentence.", nil) {
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

	interruptTurn, err := streamThread.Turn(ctx, "Count from 1 to 200 with commas.", nil)
	if err != nil {
		t.Fatalf("StreamThread.Turn(interrupt) real app-server error = %v", err)
	}
	if _, err := interruptTurn.Interrupt(ctx); err != nil {
		// The real service can finish this intentionally simple prompt before
		// the interrupt request arrives. That still exercises the transport and
		// should not fail the follow-up turn regression below.
		if !strings.Contains(err.Error(), "no active turn to interrupt") {
			t.Fatalf("StreamTurnHandle.Interrupt() real app-server error = %v", err)
		}
	}
	interruptCtx, interruptCancel := context.WithTimeout(ctx, 45*time.Second)
	defer interruptCancel()
	sawInterruptTerminal := false
	for event, err := range interruptTurn.Stream(interruptCtx) {
		if err != nil {
			t.Fatalf("interrupted Stream() real app-server error = %v", err)
		}
		completed, ok, err := event.TurnCompleted()
		if err != nil {
			t.Fatalf("interrupted Notification.TurnCompleted() error = %v", err)
		}
		if !ok {
			continue
		}
		sawInterruptTerminal = true
		if completed.Turn.Status != codex.TurnStatusCompleted && completed.Turn.Status != codex.TurnStatusFailed {
			t.Fatalf("interrupted status = %q, want completed or failed", completed.Turn.Status)
		}
		break
	}
	if !sawInterruptTerminal {
		t.Fatal("interrupted stream ended without turn/completed notification")
	}

	followUpTurn, err := streamThread.Turn(ctx, "Say ok only.", nil)
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

func TestRealAppServerIntegrationUnixWebSocketPort(t *testing.T) {
	codexBin := requireRealCodexBinary(t)
	if runtime.GOOS == "windows" {
		t.Skip("Unix-domain sockets are not supported on windows")
	}

	socketPath := filepath.Join(t.TempDir(), "codex.sock")
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	t.Cleanup(cancel)
	sdk, err := codex.NewCodex(ctx, &codex.Config{
		CodexBin: codexBin,
		Cwd:      t.TempDir(),
		Listen: codex.ListenConfig{
			URL: "unix://" + socketPath,
			WebSocket: &codex.WebSocketConfig{
				AuthMode:    codex.WebSocketAuthNone,
				DialTimeout: 500 * time.Millisecond,
			},
		},
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unix") || strings.Contains(strings.ToLower(err.Error()), "listen") {
			t.Skipf("real Codex binary does not support unix websocket listen: %v", err)
		}
		t.Fatalf("NewCodex() real unix websocket error = %v", err)
	}
	t.Cleanup(func() {
		if err := sdk.Close(); err != nil {
			t.Fatalf("Codex.Close() real unix websocket error = %v", err)
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
	expectedVersion := realIntegrationGeneratedSourceBinary(t)
	codexBin, probes := findCodexBinaryForGeneratedProvenance(t.Context(), expectedVersion)
	if codexBin == "" {
		if len(probes) == 0 {
			t.Skip("real Codex app-server integration requires codex binary on PATH or in the local standalone release store")
		}
		t.Fatalf(
			"real Codex app-server integration requires generated provenance %q; checked binaries:\n%s",
			expectedVersion,
			formatCodexBinaryProbes(probes),
		)
	}
	return codexBin
}

func realIntegrationGeneratedSourceBinary(t *testing.T) string {
	t.Helper()

	generatedPath := filepath.Join(realIntegrationRepoRoot(t), "pkg", "llm", "codex", "protocol_gen.go")
	generated, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", generatedPath, err)
	}
	for line := range strings.SplitSeq(string(generated), "\n") {
		version, ok := strings.CutPrefix(line, "// Source binary: ")
		if ok {
			version = strings.TrimSpace(version)
			if version == "" {
				break
			}
			return version
		}
	}
	t.Fatalf("%s is missing generated Source binary provenance", generatedPath)
	return ""
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
		Model: model,
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
	return filepath.Clean(filepath.Join(cwd, "..", "..", "..", ".."))
}
