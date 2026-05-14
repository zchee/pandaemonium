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

package examples_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-json-experiment/json"
	"github.com/zchee/pandaemonium/pkg/codex"
	"github.com/zchee/pandaemonium/pkg/codex/examples/internal/exampleutil"
)

var upstreamExampleNames = []string{
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

func TestExamplesReadmeIndexAndPublicImports(t *testing.T) {
	t.Parallel()

	root := examplesRoot(t)
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read examples README: %v", err)
	}
	if !bytes.Contains(readme, []byte("rust-v0.131.0-alpha.9")) {
		t.Fatalf("README missing upstream tag provenance")
	}
	for _, name := range upstreamExampleNames {
		mainPath := filepath.Join(root, name, "main.go")
		source, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("read %s: %v", mainPath, err)
		}
		if !bytes.Contains(readme, []byte(name+"/")) {
			t.Fatalf("README missing example %s", name)
		}
		if !bytes.Contains(source, []byte("package main")) {
			t.Fatalf("%s is not a main package", mainPath)
		}
		for _, forbidden := range []string{
			"github.com/zchee/pandaemonium/pkg/codex/internal",
			"protocol_gen.go",
		} {
			if bytes.Contains(source, []byte(forbidden)) {
				t.Fatalf("%s imports or references forbidden implementation detail %q", mainPath, forbidden)
			}
		}
	}
}

func TestExamplesPublicAppServerLifecyclePort(t *testing.T) {
	t.Parallel()

	client := newMockCodex(t)
	defer func() { _ = client.Close() }()

	if got := exampleutil.ServerLabel(client.Metadata()); got != "codex-examples-test 1.2.3" {
		t.Fatalf("metadata label = %q, want codex-examples-test 1.2.3", got)
	}
	models, err := client.Models(t.Context(), true)
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	selected, ok := exampleutil.PickHighestModel(models.Data, exampleutil.DefaultModel())
	if !ok || selected.Model != exampleutil.DefaultModel() {
		t.Fatalf("PickHighestModel() = (%#v, %v), want default model", selected, ok)
	}

	thread, err := client.ThreadStart(t.Context(), exampleutil.DefaultThreadParams())
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	if got, want := thread.ID(), "thr_examples"; got != want {
		t.Fatalf("Thread.ID() = %q, want %q", got, want)
	}

	runResult, err := thread.Run(t.Context(), codex.TextInput{Text: "run lifecycle turn"}, nil)
	if err != nil {
		t.Fatalf("Thread.Run() error = %v", err)
	}
	if runResult.FinalResponse != "final lifecycle text" {
		t.Fatalf("RunResult.FinalResponse = %q, want final lifecycle text", runResult.FinalResponse)
	}
	if runResult.Usage == nil || runResult.Usage.Total.TotalTokens != 6 {
		t.Fatalf("RunResult.Usage = %#v, want total tokens 6", runResult.Usage)
	}

	reading, err := thread.Read(t.Context(), true)
	if err != nil {
		t.Fatalf("Thread.Read() error = %v", err)
	}
	persistedTurn := exampleutil.FindTurnByID(reading.Thread.Turns, runResult.Turn.ID)
	if got := exampleutil.AssistantTextFromTurn(persistedTurn); got != "persisted lifecycle text" {
		t.Fatalf("persisted assistant text = %q, want persisted lifecycle text", got)
	}
	if _, err := thread.SetName(t.Context(), "sdk-lifecycle-demo"); err != nil {
		t.Fatalf("SetName() error = %v", err)
	}
	if _, err := client.ThreadArchive(t.Context(), thread.ID()); err != nil {
		t.Fatalf("ThreadArchive() error = %v", err)
	}
	if listing, err := client.ThreadList(t.Context(), nil); err != nil {
		t.Fatalf("ThreadList() error = %v", err)
	} else if len(listing.Data) != 1 || listing.Data[0].ID != thread.ID() {
		t.Fatalf("ThreadList() = %#v, want one example thread", listing.Data)
	}
	unarchived, err := client.ThreadUnarchive(t.Context(), thread.ID())
	if err != nil {
		t.Fatalf("ThreadUnarchive() error = %v", err)
	}
	if unarchived.ID() != thread.ID() {
		t.Fatalf("unarchived ID = %q, want %q", unarchived.ID(), thread.ID())
	}
	resumed, err := client.ThreadResume(t.Context(), thread.ID(), nil)
	if err != nil {
		t.Fatalf("ThreadResume() error = %v", err)
	}
	if resumed.ID() != thread.ID() {
		t.Fatalf("resumed ID = %q, want %q", resumed.ID(), thread.ID())
	}
	forked, err := client.ThreadFork(t.Context(), thread.ID(), nil)
	if err != nil {
		t.Fatalf("ThreadFork() error = %v", err)
	}
	if forked.ID() == thread.ID() {
		t.Fatalf("forked ID = original ID %q, want distinct thread", forked.ID())
	}
	if _, err := thread.Compact(t.Context()); err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
}

func TestExamplesPublicStreamingControlsAndInputsPort(t *testing.T) {
	t.Parallel()

	client := newMockCodex(t)
	defer func() { _ = client.Close() }()

	thread, err := client.ThreadStart(t.Context(), exampleutil.DefaultThreadParams())
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	imagePath, cleanup, err := exampleutil.TemporarySampleImagePath()
	if err != nil {
		t.Fatalf("TemporarySampleImagePath() error = %v", err)
	}
	defer cleanup()

	streamTurn, err := thread.Turn(t.Context(), []codex.InputItem{
		codex.TextInput{Text: "stream multimodal turn"},
		codex.ImageInput{URL: "https://example.test/image.png"},
		codex.LocalImageInput{Path: imagePath},
		codex.SkillInput{Name: "demo-skill", Path: "/tmp/demo-skill/SKILL.md"},
	}, nil)
	if err != nil {
		t.Fatalf("Turn(stream) error = %v", err)
	}
	var deltas []string
	var completedStatus codex.TurnStatus
	for event, err := range streamTurn.Stream(t.Context()) {
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		if delta, ok, err := event.ItemAgentMessageDelta(); err != nil {
			t.Fatalf("ItemAgentMessageDelta() error = %v", err)
		} else if ok {
			deltas = append(deltas, delta.Delta)
		}
		if completed, ok, err := event.TurnCompleted(); err != nil {
			t.Fatalf("TurnCompleted() error = %v", err)
		} else if ok {
			completedStatus = completed.Turn.Status
		}
	}
	if got := strings.Join(deltas, ""); got != "streamed text" {
		t.Fatalf("stream deltas = %q, want streamed text", got)
	}
	if completedStatus != codex.TurnStatusCompleted {
		t.Fatalf("completed status = %q, want completed", completedStatus)
	}

	steerTurn, err := thread.Turn(t.Context(), codex.TextInput{Text: "wait for steer"}, nil)
	if err != nil {
		t.Fatalf("Turn(steer) error = %v", err)
	}
	if _, err := steerTurn.Steer(t.Context(), codex.TextInput{Text: "follow-up steer input"}); err != nil {
		t.Fatalf("Steer() error = %v", err)
	}
	if status := collectCompletedStatus(t, steerTurn); status != codex.TurnStatusCompleted {
		t.Fatalf("steer stream status = %q, want completed", status)
	}

	interruptTurn, err := thread.Turn(t.Context(), codex.TextInput{Text: "wait for interrupt"}, nil)
	if err != nil {
		t.Fatalf("Turn(interrupt) error = %v", err)
	}
	if _, err := interruptTurn.Interrupt(t.Context()); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}
	if status := collectCompletedStatus(t, interruptTurn); status != codex.TurnStatusInterrupted {
		t.Fatalf("interrupt stream status = %q, want interrupted", status)
	}
}

func TestExampleMockAppServerProcess(t *testing.T) {
	if os.Getenv("CODEX_EXAMPLES_WANT_MOCK_SERVER") != "1" {
		return
	}
	runMockAppServer()
}

func newMockCodex(t *testing.T) *codex.Codex {
	t.Helper()

	client, err := codex.NewCodex(t.Context(), &codex.Config{
		LaunchArgsOverride: []string{os.Args[0], "-test.run=TestExampleMockAppServerProcess", "--"},
		Env: map[string]string{
			"CODEX_EXAMPLES_WANT_MOCK_SERVER": "1",
		},
	})
	if err != nil {
		t.Fatalf("NewCodex(mock) error = %v", err)
	}
	return client
}

func collectCompletedStatus(t *testing.T, turn *codex.TurnHandle) codex.TurnStatus {
	t.Helper()

	var status codex.TurnStatus
	for event, err := range turn.Stream(t.Context()) {
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		if completed, ok, err := event.TurnCompleted(); err != nil {
			t.Fatalf("TurnCompleted() error = %v", err)
		} else if ok {
			status = completed.Turn.Status
		}
	}
	return status
}

func examplesRoot(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	return cwd
}

func runMockAppServer() {
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer func() { _ = writer.Flush() }()

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		var req map[string]any
		if err := json.Unmarshal(line, &req); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		method, _ := req["method"].(string)
		id, _ := req["id"].(string)
		params, _ := req["params"].(map[string]any)
		if err := handleMockRequest(writer, id, method, params); err != nil {
			if id != "" {
				writeJSON(writer, codex.Object{"id": id, "error": codex.Object{"code": -32602, "message": err.Error()}})
				continue
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
}

func handleMockRequest(writer *bufio.Writer, id, method string, params map[string]any) error {
	switch method {
	case codex.RequestMethodInitialize:
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{"userAgent": "codex-examples-test/1.2.3", "serverInfo": codex.Object{"name": "codex-examples-test", "version": "1.2.3"}}})
	case "initialized":
	case codex.RequestMethodModelList:
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{"data": []codex.Object{
			{"id": "gpt-5.3", "model": "gpt-5.3", "displayName": "GPT 5.3", "description": "old", "hidden": false, "isDefault": false, "defaultReasoningEffort": "medium", "supportedReasoningEfforts": []codex.Object{{"reasoningEffort": "medium"}}, "upgrade": "gpt-5.4"},
			{"id": "gpt-5.4", "model": "gpt-5.4", "displayName": "GPT 5.4", "description": "preferred", "hidden": false, "isDefault": true, "defaultReasoningEffort": "high", "supportedReasoningEfforts": []codex.Object{{"reasoningEffort": "low"}, {"reasoningEffort": "xhigh"}}},
			{"id": "hidden", "model": "gpt-hidden", "displayName": "Hidden", "description": "hidden", "hidden": true, "isDefault": false, "defaultReasoningEffort": "medium", "supportedReasoningEfforts": []codex.Object{{"reasoningEffort": "medium"}}},
		}}})
	case codex.RequestMethodThreadStart:
		if got := stringParam(params, "model"); got != exampleutil.DefaultModel() {
			return fmt.Errorf("thread/start model = %q, want %q", got, exampleutil.DefaultModel())
		}
		writeJSON(writer, codex.Object{"id": id, "result": threadResponse("thr_examples")})
	case codex.RequestMethodThreadRead:
		if !boolParam(params, "includeTurns") {
			return errors.New("thread/read includeTurns = false, want true")
		}
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{"thread": threadPayload("thr_examples", []codex.Object{completedTurn("turn_run", "persisted lifecycle text")})}})
	case codex.RequestMethodThreadNameSet:
		if got := stringParam(params, "name"); got != "sdk-lifecycle-demo" {
			return fmt.Errorf("thread/name/set name = %q, want sdk-lifecycle-demo", got)
		}
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{}})
	case codex.RequestMethodThreadArchive, codex.RequestMethodThreadCompactStart:
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{}})
	case codex.RequestMethodThreadList:
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{"data": []codex.Object{threadPayload("thr_examples", nil)}}})
	case codex.RequestMethodThreadUnarchive, codex.RequestMethodThreadResume:
		writeJSON(writer, codex.Object{"id": id, "result": threadResponse("thr_examples")})
	case codex.RequestMethodThreadFork:
		writeJSON(writer, codex.Object{"id": id, "result": threadResponse("thr_examples_fork")})
	case codex.RequestMethodTurnStart:
		return handleMockTurnStart(writer, id, params)
	case codex.RequestMethodTurnSteer:
		if got := stringParam(params, "expectedTurnId"); got != "turn_steer" {
			return fmt.Errorf("turn/steer expectedTurnId = %q, want turn_steer", got)
		}
		if err := requireInputTypes(params, "text"); err != nil {
			return err
		}
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{"turnId": "turn_steer"}})
		writeJSON(writer, turnCompletedNotification("thr_examples", "turn_steer", "completed", "steered text"))
	case codex.RequestMethodTurnInterrupt:
		if got := stringParam(params, "turnId"); got != "turn_interrupt" {
			return fmt.Errorf("turn/interrupt turnId = %q, want turn_interrupt", got)
		}
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{}})
		writeJSON(writer, turnCompletedNotification("thr_examples", "turn_interrupt", "interrupted", "interrupted text"))
	default:
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{}})
	}
	return nil
}

func handleMockTurnStart(writer *bufio.Writer, id string, params map[string]any) error {
	inputItems, err := inputItems(params)
	if err != nil {
		return err
	}
	if len(inputItems) == 0 {
		return errors.New("turn/start input is empty")
	}
	text, _ := inputItems[0]["text"].(string)
	switch {
	case text == "run lifecycle turn":
		if err := requireInputTypes(params, "text"); err != nil {
			return err
		}
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{"turn": turnPayload("turn_run", "inProgress", nil)}})
		writeJSON(writer, codex.Object{"method": codex.NotificationMethodItemCompleted, "params": codex.Object{"threadId": "thr_examples", "turnId": "turn_run", "item": agentMessage("draft lifecycle text")}})
		writeJSON(writer, codex.Object{"method": codex.NotificationMethodThreadTokenUsageUpdated, "params": codex.Object{"threadId": "thr_examples", "turnId": "turn_run", "tokenUsage": codex.Object{"last": tokenUsage(1), "total": tokenUsage(6)}}})
		writeJSON(writer, codex.Object{"method": codex.NotificationMethodItemCompleted, "params": codex.Object{"threadId": "thr_examples", "turnId": "turn_run", "item": finalAgentMessage("final lifecycle text")}})
		writeJSON(writer, turnCompletedNotification("thr_examples", "turn_run", "completed", "final lifecycle text"))
	case text == "stream multimodal turn":
		if err := requireInputTypes(params, "text", "image", "localImage", "skill"); err != nil {
			return err
		}
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{"turn": turnPayload("turn_stream", "inProgress", nil)}})
		writeJSON(writer, codex.Object{"method": codex.NotificationMethodItemAgentMessageDelta, "params": codex.Object{"threadId": "thr_examples", "turnId": "turn_stream", "delta": "streamed "}})
		writeJSON(writer, codex.Object{"method": codex.NotificationMethodItemAgentMessageDelta, "params": codex.Object{"threadId": "thr_examples", "turnId": "turn_stream", "delta": "text"}})
		writeJSON(writer, turnCompletedNotification("thr_examples", "turn_stream", "completed", "streamed text"))
	case text == "wait for steer":
		if err := requireInputTypes(params, "text"); err != nil {
			return err
		}
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{"turn": turnPayload("turn_steer", "inProgress", nil)}})
	case text == "wait for interrupt":
		if err := requireInputTypes(params, "text"); err != nil {
			return err
		}
		writeJSON(writer, codex.Object{"id": id, "result": codex.Object{"turn": turnPayload("turn_interrupt", "inProgress", nil)}})
	default:
		return fmt.Errorf("unexpected turn/start text %q", text)
	}
	return nil
}

func requireInputTypes(params map[string]any, want ...string) error {
	items, err := inputItems(params)
	if err != nil {
		return err
	}
	got := make([]string, len(items))
	for i, item := range items {
		got[i], _ = item["type"].(string)
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("input types = %v, want %v", got, want)
	}
	return nil
}

func inputItems(params map[string]any) ([]map[string]any, error) {
	rawItems, ok := params["input"].([]any)
	if !ok {
		return nil, errors.New("params.input is not an array")
	}
	items := make([]map[string]any, len(rawItems))
	for i, rawItem := range rawItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("input[%d] is %T, want object", i, rawItem)
		}
		items[i] = item
	}
	return items, nil
}

func stringParam(params map[string]any, key string) string {
	value, _ := params[key].(string)
	return value
}

func boolParam(params map[string]any, key string) bool {
	value, _ := params[key].(bool)
	return value
}

func threadResponse(threadID string) codex.Object {
	return codex.Object{
		"approvalPolicy":    "never",
		"approvalsReviewer": "auto_review",
		"cwd":               "/tmp/codex-examples",
		"model":             exampleutil.DefaultModel(),
		"modelProvider":     "openai",
		"sandbox":           codex.Object{"type": "readOnly"},
		"thread":            threadPayload(threadID, nil),
	}
}

func threadPayload(threadID string, turns []codex.Object) codex.Object {
	return codex.Object{
		"cliVersion":    "0.131.0-alpha.9",
		"createdAt":     int64(1),
		"cwd":           "/tmp/codex-examples",
		"ephemeral":     false,
		"id":            threadID,
		"modelProvider": "openai",
		"preview":       "example preview",
		"sessionId":     "session-examples",
		"source":        "app_server",
		"status":        "idle",
		"turns":         turns,
		"updatedAt":     int64(2),
	}
}

func completedTurn(turnID, text string) codex.Object {
	return turnPayload(turnID, "completed", []codex.Object{agentMessage(text)})
}

func turnPayload(turnID, status string, items []codex.Object) codex.Object {
	return codex.Object{"id": turnID, "status": status, "items": items}
}

func turnCompletedNotification(threadID, turnID, status, text string) codex.Object {
	return codex.Object{"method": codex.NotificationMethodTurnCompleted, "params": codex.Object{"threadId": threadID, "turn": turnPayload(turnID, status, []codex.Object{agentMessage(text)})}}
}

func agentMessage(text string) codex.Object {
	return codex.Object{"id": "msg-" + strings.ReplaceAll(text, " ", "-"), "type": "agentMessage", "text": text}
}

func finalAgentMessage(text string) codex.Object {
	message := agentMessage(text)
	message["phase"] = "final_answer"
	return message
}

func tokenUsage(total int64) codex.Object {
	return codex.Object{"cachedInputTokens": int64(0), "inputTokens": total, "outputTokens": int64(0), "reasoningOutputTokens": int64(0), "totalTokens": total}
}

func writeJSON(writer *bufio.Writer, payload any) {
	line, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	_, _ = writer.Write(line)
	_, _ = writer.WriteString("\n")
	_ = writer.Flush()
}
