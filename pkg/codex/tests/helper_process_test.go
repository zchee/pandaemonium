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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/go-json-experiment/json"

	"github.com/zchee/pandaemonium/pkg/codex"
)

const helperProcessEnv = "PANDAEMONIUM_CODEX_TEST_HELPER_PROCESS"

func helperCodexConfig(t *testing.T, scenario string) *codex.Config {
	t.Helper()
	return &codex.Config{
		LaunchArgsOverride: []string{os.Args[0], "-test.run=TestCodexPortHelperProcess", "--"},
		Cwd:                t.TempDir(),
		Env: map[string]string{
			helperProcessEnv:             "1",
			"CODEX_PORT_HELPER_SCENARIO": scenario,
		},
	}
}

func newHelperClient(t *testing.T, scenario string) *codex.Client {
	t.Helper()
	client := codex.NewClient(helperCodexConfig(t, scenario), nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Client.Start(%q) error = %v", scenario, err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Client.Close(%q) error = %v", scenario, err)
		}
	})
	return client
}

func newHelperCodex(t *testing.T, scenario string) *codex.Codex {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	client, err := codex.NewCodex(ctx, helperCodexConfig(t, scenario))
	if err != nil {
		t.Fatalf("NewCodex(%q) error = %v", scenario, err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Codex.Close(%q) error = %v", scenario, err)
		}
	})
	return client
}

func TestCodexPortHelperProcess(t *testing.T) {
	if os.Getenv(helperProcessEnv) != "1" {
		return
	}
	scenario := os.Getenv("CODEX_PORT_HELPER_SCENARIO")
	state := helperState{
		reader:   bufio.NewReader(os.Stdin),
		writer:   bufio.NewWriter(os.Stdout),
		scenario: scenario,
		threads:  map[string]codex.Object{},
	}
	defer func() { _ = state.writer.Flush() }()
	for {
		var req helperRequest
		line, err := state.reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := json.Unmarshal(line, &req); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := state.handle(req); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
}

type helperRequest struct {
	ID     string       `json:"id,omitzero"`
	Method string       `json:"method"`
	Params codex.Object `json:"params,omitzero"`
}

type helperState struct {
	reader   *bufio.Reader
	writer   *bufio.Writer
	scenario string
	threads  map[string]codex.Object

	turnCount  int
	retryCount int
}

func (s *helperState) handle(req helperRequest) error {
	if req.Method == codex.RequestMethodInitialize {
		s.writeResult(req.ID, codex.Object{"userAgent": "codex-test/1.2.3"})
		return nil
	}
	if req.Method == "initialized" {
		return nil
	}
	switch s.scenario {
	case "lifecycle_inputs":
		return s.handleLifecycleInputs(req)
	case "stream_controls":
		return s.handleStreamControls(req)
	case "client_routing_retry":
		return s.handleClientRoutingRetry(req)
	default:
		s.writeError(req.ID, -32601, "unexpected scenario "+s.scenario, nil)
		return nil
	}
}

func (s *helperState) handleLifecycleInputs(req helperRequest) error {
	switch req.Method {
	case codex.RequestMethodThreadStart:
		threadID := fmt.Sprintf("thread-%d", len(s.threads)+1)
		thread := helperThread(threadID)
		s.threads[threadID] = thread
		s.writeResult(req.ID, helperThreadState(thread, req.Params))
	case codex.RequestMethodThreadResume:
		threadID := stringParam(req.Params, "threadId")
		s.writeResult(req.ID, helperThreadState(helperThread(threadID), req.Params))
	case codex.RequestMethodThreadFork:
		threadID := stringParam(req.Params, "threadId") + "-fork"
		s.writeResult(req.ID, helperThreadState(helperThread(threadID), req.Params))
	case codex.RequestMethodThreadArchive:
		s.writeResult(req.ID, codex.Object{})
	case codex.RequestMethodThreadUnarchive:
		threadID := stringParam(req.Params, "threadId")
		s.writeResult(req.ID, codex.Object{"thread": helperThread(threadID)})
	case codex.RequestMethodThreadNameSet:
		s.writeResult(req.ID, codex.Object{})
	case codex.RequestMethodThreadRead:
		threadID := stringParam(req.Params, "threadId")
		s.writeResult(req.ID, codex.Object{"thread": helperThread(threadID)})
	case codex.RequestMethodThreadList:
		s.writeResult(req.ID, codex.Object{"data": []codex.Object{helperThread("thread-1"), helperThread("thread-2")}})
	case codex.RequestMethodThreadCompactStart:
		s.writeResult(req.ID, codex.Object{})
	case codex.RequestMethodModelList:
		s.writeResult(req.ID, codex.Object{"data": []codex.Object{{"id": "gpt-test", "model": "gpt-test", "displayName": "GPT Test", "description": "test", "hidden": false, "isDefault": true}}})
	case codex.RequestMethodTurnStart:
		if err := validateTurnInput(req.Params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return nil
		}
		s.turnCount++
		turnID := fmt.Sprintf("turn-life-%d", s.turnCount)
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		s.writeNotification(codex.NotificationMethodThreadTokenUsageUpdated, codex.Object{"threadId": stringParam(req.Params, "threadId"), "turnId": turnID, "tokenUsage": helperUsage(13)})
		s.writeNotification(codex.NotificationMethodItemCompleted, codex.Object{"threadId": stringParam(req.Params, "threadId"), "turnId": turnID, "item": codex.Object{"id": "item-" + turnID, "type": "agentMessage", "phase": "final_answer", "text": "final for " + turnID}})
		s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": stringParam(req.Params, "threadId"), "turn": codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}})
	default:
		s.writeError(req.ID, -32601, "unexpected lifecycle method "+req.Method, nil)
	}
	return nil
}

func (s *helperState) handleStreamControls(req helperRequest) error {
	switch req.Method {
	case codex.RequestMethodThreadStart:
		s.writeResult(req.ID, helperThreadState(helperThread("thread-stream"), req.Params))
	case codex.RequestMethodTurnStart:
		if err := validateTurnInput(req.Params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return nil
		}
		s.turnCount++
		turnID := fmt.Sprintf("turn-stream-%d", s.turnCount)
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		switch textInput(req.Params) {
		case "fail this turn":
			s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": "thread-stream", "turn": codex.Object{"id": turnID, "status": "failed", "items": []codex.Object{}, "error": codex.Object{"message": "mock failure"}}})
		case "continue after interrupt":
			s.writeNotification(codex.NotificationMethodItemCompleted, codex.Object{"threadId": "thread-stream", "turnId": turnID, "item": codex.Object{"id": "final-" + turnID, "type": "agentMessage", "phase": "final_answer", "text": "after interrupt"}})
			s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": "thread-stream", "turn": codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}})
		case "start steerable turn", "start interrupt turn":
			// The follow-up steer/interrupt request owns completion for these turns.
		default:
			s.writeNotification(codex.NotificationMethodAgentMessageDelta, codex.Object{"threadId": "thread-stream", "turnId": turnID, "itemId": "delta-" + turnID, "delta": "he"})
			s.writeNotification(codex.NotificationMethodAgentMessageDelta, codex.Object{"threadId": "thread-stream", "turnId": turnID, "itemId": "delta-" + turnID, "delta": "llo"})
			s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": "thread-stream", "turn": codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}})
		}
	case codex.RequestMethodTurnSteer:
		s.writeResult(req.ID, codex.Object{"turnId": stringParam(req.Params, "expectedTurnId")})
		turnID := stringParam(req.Params, "expectedTurnId")
		s.writeNotification(codex.NotificationMethodItemCompleted, codex.Object{"threadId": "thread-stream", "turnId": turnID, "item": codex.Object{"id": "steered-" + turnID, "type": "agentMessage", "phase": "final_answer", "text": "steered final"}})
		s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": "thread-stream", "turn": codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}})
	case codex.RequestMethodTurnInterrupt:
		s.writeResult(req.ID, codex.Object{})
		s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": "thread-stream", "turn": codex.Object{"id": stringParam(req.Params, "turnId"), "status": "interrupted", "items": []codex.Object{}}})
	default:
		s.writeError(req.ID, -32601, "unexpected stream method "+req.Method, nil)
	}
	return nil
}

func (s *helperState) handleClientRoutingRetry(req helperRequest) error {
	switch req.Method {
	case codex.RequestMethodTurnStart:
		turnID := "turn-route"
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		s.writeNotification(codex.NotificationMethodAgentMessageDelta, codex.Object{"threadId": stringParam(req.Params, "threadId"), "turnId": "other-turn", "itemId": "other", "delta": "ignored"})
		s.writeNotification(codex.NotificationMethodAgentMessageDelta, codex.Object{"threadId": stringParam(req.Params, "threadId"), "turnId": turnID, "itemId": "target", "delta": "first"})
		s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": stringParam(req.Params, "threadId"), "turn": codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}})
	case "ping":
		if s.retryCount == 0 {
			s.retryCount++
			s.writeError(req.ID, -32000, "server busy", codex.Object{"codexErrorInfo": "server_overloaded"})
			return nil
		}
		s.writeResult(req.ID, "ok")
	default:
		s.writeResult(req.ID, codex.Object{})
	}
	return nil
}

func (s *helperState) writeResult(id string, result any) {
	s.write(codex.Object{"id": id, "result": result})
}

func (s *helperState) writeError(id string, code int64, message string, data any) {
	payload := codex.Object{"id": id, "error": codex.Object{"code": code, "message": message}}
	if data != nil {
		payload["error"].(codex.Object)["data"] = data
	}
	s.write(payload)
}

func (s *helperState) writeNotification(method string, params codex.Object) {
	s.write(codex.Object{"method": method, "params": params})
}

func (s *helperState) write(payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	_, _ = s.writer.Write(encoded)
	_, _ = s.writer.WriteString("\n")
	_ = s.writer.Flush()
}

func helperThread(id string) codex.Object {
	return codex.Object{
		"id":            id,
		"sessionId":     "session-" + id,
		"createdAt":     int64(1),
		"updatedAt":     int64(2),
		"cwd":           "/tmp",
		"modelProvider": "openai",
		"preview":       "",
		"source":        "cli",
		"status":        codex.Object{"type": "idle"},
		"turns":         []codex.Object{},
		"ephemeral":     false,
		"cliVersion":    "1.2.3",
	}
}

func helperThreadState(thread, params codex.Object) codex.Object {
	approval := any("on-request")
	reviewer := any("auto_review")
	if value, ok := params["approvalPolicy"]; ok {
		approval = value
	}
	if value, ok := params["approvalsReviewer"]; ok {
		reviewer = value
	}
	return codex.Object{
		"approvalPolicy":    approval,
		"approvalsReviewer": reviewer,
		"cwd":               "/tmp",
		"model":             "gpt-test",
		"modelProvider":     "openai",
		"sandbox":           codex.Object{"type": "dangerFullAccess"},
		"thread":            thread,
	}
}

func helperUsage(total int64) codex.Object {
	breakdown := codex.Object{
		"cachedInputTokens":     int64(3),
		"inputTokens":           total,
		"outputTokens":          int64(5),
		"reasoningOutputTokens": int64(2),
		"totalTokens":           total + 7,
	}
	return codex.Object{"last": breakdown, "total": breakdown}
}

func stringParam(params codex.Object, key string) string {
	if value, ok := params[key].(string); ok {
		return value
	}
	return ""
}

func textInput(params codex.Object) string {
	input, ok := params["input"].([]any)
	if !ok || len(input) == 0 {
		return ""
	}
	first, ok := input[0].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := first["text"].(string)
	return text
}

func validateTurnInput(params codex.Object) error {
	input, ok := params["input"].([]any)
	if !ok || len(input) == 0 {
		return fmt.Errorf("missing normalized input array")
	}
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("normalized input item is %T, want object", raw)
		}
		kind, _ := item["type"].(string)
		switch kind {
		case "text":
			if item["text"] == "" {
				return fmt.Errorf("text input missing text")
			}
		case "image":
			if item["url"] == "" {
				return fmt.Errorf("image input missing url")
			}
		case "localImage":
			if item["path"] == "" {
				return fmt.Errorf("local image input missing path")
			}
		case "skill", "mention":
			if item["name"] == "" || item["path"] == "" {
				return fmt.Errorf("%s input missing name/path", kind)
			}
		default:
			return fmt.Errorf("unexpected normalized input type %q", kind)
		}
	}
	return nil
}
