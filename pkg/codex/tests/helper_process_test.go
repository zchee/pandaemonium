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
	"maps"
	"os"
	"os/signal"
	"strings"
	"sync"
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
	closeMarker := os.Getenv("CODEX_PORT_HELPER_CLOSE_MARKER")
	if closeMarker != "" {
		signal.Ignore(os.Interrupt)
	}
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
				if closeMarker != "" {
					_ = os.WriteFile(closeMarker, []byte("closed\n"), 0o600)
				}
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
	reader         *bufio.Reader
	writer         *bufio.Writer
	writeMu        sync.Mutex
	scenario       string
	threads        map[string]codex.Object
	approvalStates map[string]helperApprovalState
	lifecycle      map[string]*helperLifecycleThread

	turnCount          int
	retryCount         int
	modelListActive    int
	modelListMaxActive int
}

func (s *helperState) handle(req helperRequest) error {
	if req.Method == codex.RequestMethodInitialize {
		if s.scenario == "initialize_invalid_metadata" {
			s.writeResult(req.ID, codex.Object{})
			return nil
		}
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
	case "streaming_port":
		return s.handleStreamingPort(req)
	case "turn_controls":
		return s.handleTurnControls(req)
	case "client_routing_retry":
		return s.handleClientRoutingRetry(req)
	case "approval_persistence":
		return s.handleApprovalPersistence(req)
	case "input_capture":
		return s.handleInputCapture(req)
	case "lifecycle_persistence":
		return s.handleLifecyclePersistence(req)
	case "run_result":
		return s.handleRunResult(req)
	case "async_client_behavior":
		return s.handleAsyncClientBehavior(req)
	default:
		s.writeError(req.ID, -32601, "unexpected scenario "+s.scenario, nil)
		return nil
	}
}

func (s *helperState) handleAsyncClientBehavior(req helperRequest) error {
	switch req.Method {
	case codex.RequestMethodThreadStart:
		threadID := "thread-async-client-behavior"
		thread := helperThread(threadID)
		s.threads[threadID] = thread
		s.writeResult(req.ID, helperThreadState(thread, req.Params))
	case codex.RequestMethodModelList:
		go s.writeDelayedOverlappingModelList(req.ID)
	case codex.RequestMethodTurnStart:
		s.turnCount++
		turnID := fmt.Sprintf("turn-async-client-behavior-%d", s.turnCount)
		threadID := stringParam(req.Params, "threadId")
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		s.writeNotification("unknown/direct", codex.Object{"turnId": turnID})
		s.writeNotification("unknown/nested", codex.Object{"turn": codex.Object{"id": turnID}})
		s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": threadID, "turn": codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}})
		s.writeNotification("unknown/global", codex.Object{"scope": "global"})
	default:
		s.writeError(req.ID, -32601, "unexpected async client behavior method "+req.Method, nil)
	}
	return nil
}

func (s *helperState) writeDelayedOverlappingModelList(id string) {
	s.writeMu.Lock()
	s.modelListActive++
	if s.modelListActive > s.modelListMaxActive {
		s.modelListMaxActive = s.modelListActive
	}
	snapshot := s.modelListMaxActive
	s.writeMu.Unlock()

	time.Sleep(75 * time.Millisecond)

	s.writeMu.Lock()
	if s.modelListMaxActive > snapshot {
		snapshot = s.modelListMaxActive
	}
	s.modelListActive--
	s.writeMu.Unlock()

	modelID := fmt.Sprintf("gpt-overlap-%d", snapshot)
	s.writeResult(id, codex.Object{"data": []codex.Object{{"id": modelID, "model": modelID, "displayName": "GPT Overlap", "description": "overlap test", "hidden": false, "isDefault": true}}})
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
			s.writeNotification(codex.NotificationMethodItemAgentMessageDelta, codex.Object{"threadId": "thread-stream", "turnId": turnID, "itemId": "delta-" + turnID, "delta": "he"})
			s.writeNotification(codex.NotificationMethodItemAgentMessageDelta, codex.Object{"threadId": "thread-stream", "turnId": turnID, "itemId": "delta-" + turnID, "delta": "llo"})
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

func (s *helperState) handleStreamingPort(req helperRequest) error {
	switch req.Method {
	case codex.RequestMethodThreadStart:
		threadID := fmt.Sprintf("thread-streaming-%d", len(s.threads)+1)
		thread := helperThread(threadID)
		s.threads[threadID] = thread
		s.writeResult(req.ID, helperThreadState(thread, req.Params))
	case codex.RequestMethodModelList:
		s.writeResult(req.ID, codex.Object{"data": []codex.Object{{"id": "gpt-stream", "model": "gpt-stream", "displayName": "GPT Stream", "description": "streaming test model", "hidden": false, "isDefault": true}}})
	case codex.RequestMethodTurnStart:
		if err := validateTurnInput(req.Params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return nil
		}
		s.turnCount++
		turnID := fmt.Sprintf("turn-streaming-%d", s.turnCount)
		threadID := stringParam(req.Params, "threadId")
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		s.writeStreamingPortTurn(threadID, turnID, textInput(req.Params))
	default:
		s.writeError(req.ID, -32601, "unexpected streaming method "+req.Method, nil)
	}
	return nil
}

func (s *helperState) writeStreamingPortTurn(threadID, turnID, input string) {
	switch input {
	case "stream please":
		s.writeStreamingPortDeltas(threadID, turnID, "he", "llo")
		s.writeStreamingPortAgentMessage(threadID, turnID, "hello")
		s.writeStreamingPortCompleted(threadID, turnID)
	case "complete this turn":
		s.writeStreamingPortCompleted(threadID, turnID)
	case "async stream please":
		s.writeStreamingPortDeltas(threadID, turnID, "as", "ync")
		s.writeStreamingPortAgentMessage(threadID, turnID, "async")
		s.writeStreamingPortCompleted(threadID, turnID)
	case "low-level sync":
		s.writeStreamingPortDeltas(threadID, turnID, "fir", "st")
		s.writeStreamingPortCompleted(threadID, turnID)
	case "low-level async":
		s.writeStreamingPortDeltas(threadID, turnID, "one", "two", "three")
		s.writeStreamingPortCompleted(threadID, turnID)
	case "first":
		s.writeStreamingPortDeltas(threadID, turnID, "one-", "done")
		s.writeStreamingPortAgentMessage(threadID, turnID, "one-done")
		s.writeStreamingPortCompleted(threadID, turnID)
	case "second":
		s.writeStreamingPortDeltas(threadID, turnID, "two-", "done")
		s.writeStreamingPortAgentMessage(threadID, turnID, "two-done")
		s.writeStreamingPortCompleted(threadID, turnID)
	case "async first":
		s.writeStreamingPortDeltas(threadID, turnID, "a1", "-done")
		s.writeStreamingPortAgentMessage(threadID, turnID, "a1-done")
		s.writeStreamingPortCompleted(threadID, turnID)
	case "async second":
		s.writeStreamingPortDeltas(threadID, turnID, "a2", "-done")
		s.writeStreamingPortAgentMessage(threadID, turnID, "a2-done")
		s.writeStreamingPortCompleted(threadID, turnID)
	default:
		s.writeStreamingPortDeltas(threadID, turnID, "streaming response for "+input)
		s.writeStreamingPortCompleted(threadID, turnID)
	}
}

func (s *helperState) writeStreamingPortDeltas(threadID, turnID string, deltas ...string) {
	for index, delta := range deltas {
		s.writeNotification(codex.NotificationMethodItemAgentMessageDelta, codex.Object{"threadId": threadID, "turnId": turnID, "itemId": fmt.Sprintf("delta-%s-%d", turnID, index+1), "delta": delta})
	}
}

func (s *helperState) writeStreamingPortAgentMessage(threadID, turnID, text string) {
	s.writeNotification(codex.NotificationMethodItemCompleted, codex.Object{"threadId": threadID, "turnId": turnID, "item": codex.Object{"id": "msg-" + turnID, "type": "agentMessage", "phase": "final_answer", "text": text}})
}

func (s *helperState) writeStreamingPortCompleted(threadID, turnID string) {
	s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": threadID, "turn": codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}})
}

func (s *helperState) handleTurnControls(req helperRequest) error {
	switch req.Method {
	case codex.RequestMethodThreadStart:
		thread := helperThread("thread-turn-controls")
		s.threads["thread-turn-controls"] = thread
		s.writeResult(req.ID, helperThreadState(thread, req.Params))
	case codex.RequestMethodTurnStart:
		if err := validateTurnInput(req.Params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return nil
		}
		if err := validateTurnControlsStart(req.Params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return nil
		}
		s.turnCount++
		turnID := fmt.Sprintf("turn-control-%d", s.turnCount)
		threadID := stringParam(req.Params, "threadId")
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		s.writeTurnControlsStart(threadID, turnID, textInput(req.Params))
	case codex.RequestMethodTurnSteer:
		if err := validateTurnControlsSteer(req.Params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return nil
		}
		turnID := stringParam(req.Params, "expectedTurnId")
		threadID := stringParam(req.Params, "threadId")
		s.writeResult(req.ID, codex.Object{"turnId": turnID})
		s.writeRunResultAgentMessage(threadID, turnID, "msg-after-steer-"+turnID, "after steer", "final_answer")
		s.writeTurnControlsCompleted(threadID, turnID, codex.TurnStatusCompleted)
	case codex.RequestMethodTurnInterrupt:
		turnID := stringParam(req.Params, "turnId")
		if turnID == "" {
			s.writeError(req.ID, -32602, "missing interrupt turnId", nil)
			return nil
		}
		threadID := stringParam(req.Params, "threadId")
		s.writeResult(req.ID, codex.Object{})
		s.writeTurnControlsCompleted(threadID, turnID, codex.TurnStatusInterrupted)
	default:
		s.writeError(req.ID, -32601, "unexpected turn controls method "+req.Method, nil)
	}
	return nil
}

func (s *helperState) writeTurnControlsStart(threadID, turnID, input string) {
	switch input {
	case "Start a steerable turn.":
		s.writeRunResultAgentMessage(threadID, turnID, "msg-before-steer-"+turnID, "before steer", "final_answer")
	case "Start a long turn.":
		s.writeNotification(codex.NotificationMethodItemAgentMessageDelta, codex.Object{"threadId": threadID, "turnId": turnID, "itemId": "delta-" + turnID, "delta": "still "})
		s.writeNotification(codex.NotificationMethodItemAgentMessageDelta, codex.Object{"threadId": threadID, "turnId": turnID, "itemId": "delta-" + turnID, "delta": "running"})
	case "Continue after the interrupt.":
		s.writeRunResultAgentMessage(threadID, turnID, "msg-after-interrupt-"+turnID, "after interrupt", "final_answer")
		s.writeTurnControlsCompleted(threadID, turnID, codex.TurnStatusCompleted)
	default:
		panic("validated turn controls input reached default case: " + input)
	}
}

func validateTurnControlsStart(params codex.Object) error {
	switch got := textInput(params); got {
	case "Start a steerable turn.", "Start a long turn.", "Continue after the interrupt.":
		return nil
	default:
		return fmt.Errorf("turn controls input = %q, want a known turn-control test input", got)
	}
}

func validateTurnControlsSteer(params codex.Object) error {
	if expectedTurnID := stringParam(params, "expectedTurnId"); expectedTurnID == "" {
		return fmt.Errorf("missing expectedTurnId")
	}
	if got := textInput(params); got != "Use this steering input." {
		return fmt.Errorf("steer input = %q, want Use this steering input.", got)
	}
	return nil
}

func (s *helperState) writeTurnControlsCompleted(threadID, turnID string, status codex.TurnStatus) {
	s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": threadID, "turn": codex.Object{"id": turnID, "status": string(status), "items": []codex.Object{}}})
}

type helperApprovalState struct {
	thread   codex.Object
	approval any
	reviewer any
}

func (s *helperState) handleApprovalPersistence(req helperRequest) error {
	s.ensureApprovalState()
	switch req.Method {
	case codex.RequestMethodThreadStart:
		threadID := fmt.Sprintf("thread-approval-%d", len(s.approvalStates)+1)
		state := helperApprovalState{thread: helperThread(threadID), approval: "on-request", reviewer: "auto_review"}
		state = state.withExplicitApproval(req.Params)
		s.approvalStates[threadID] = state
		s.writeResult(req.ID, state.response())
	case codex.RequestMethodThreadResume:
		threadID := stringParam(req.Params, "threadId")
		state := s.approvalState(threadID)
		state = state.withExplicitApproval(req.Params)
		s.approvalStates[threadID] = state
		s.writeResult(req.ID, state.response())
	case codex.RequestMethodThreadFork:
		sourceID := stringParam(req.Params, "threadId")
		source := s.approvalState(sourceID)
		forkID := fmt.Sprintf("%s-fork-%d", sourceID, len(s.approvalStates)+1)
		fork := helperApprovalState{thread: helperThread(forkID), approval: source.approval, reviewer: source.reviewer}
		fork = fork.withExplicitApproval(req.Params)
		s.approvalStates[forkID] = fork
		s.writeResult(req.ID, fork.response())
	case codex.RequestMethodTurnStart:
		if err := validateTurnInput(req.Params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return nil
		}
		threadID := stringParam(req.Params, "threadId")
		state := s.approvalState(threadID).withExplicitApproval(req.Params)
		s.approvalStates[threadID] = state
		s.turnCount++
		turnID := fmt.Sprintf("turn-approval-%d", s.turnCount)
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		s.writeNotification(codex.NotificationMethodItemCompleted, codex.Object{"threadId": threadID, "turnId": turnID, "item": codex.Object{"id": "approval-item-" + turnID, "type": "agentMessage", "phase": "final_answer", "text": approvalFinalResponse(textInput(req.Params))}})
		s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": threadID, "turn": codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}})
	default:
		s.writeError(req.ID, -32601, "unexpected approval method "+req.Method, nil)
	}
	return nil
}

func (s *helperState) ensureApprovalState() {
	if s.approvalStates == nil {
		s.approvalStates = map[string]helperApprovalState{}
	}
}

func (s *helperState) approvalState(threadID string) helperApprovalState {
	if state, ok := s.approvalStates[threadID]; ok {
		return state
	}
	state := helperApprovalState{thread: helperThread(threadID), approval: "on-request", reviewer: "auto_review"}
	s.approvalStates[threadID] = state
	return state
}

func (s helperApprovalState) withExplicitApproval(params codex.Object) helperApprovalState {
	if value, ok := params["approvalPolicy"]; ok {
		s.approval = value
	}
	if value, ok := params["approvalsReviewer"]; ok {
		s.reviewer = value
	}
	return s
}

func (s helperApprovalState) response() codex.Object {
	return codex.Object{
		"approvalPolicy":    s.approval,
		"approvalsReviewer": s.reviewer,
		"cwd":               "/tmp",
		"model":             "gpt-test",
		"modelProvider":     "openai",
		"sandbox":           codex.Object{"type": "dangerFullAccess"},
		"thread":            s.thread,
	}
}

func approvalFinalResponse(input string) string {
	switch input {
	case "seed the source rollout":
		return "source seeded"
	case "deny this and later turns":
		return "turn override"
	case "inherit previous approval mode":
		return "turn inherited"
	case "keep approvals denied":
		return "locked down"
	case "allow auto review now":
		return "reviewable"
	default:
		return "approval response for " + input
	}
}

func (s *helperState) handleInputCapture(req helperRequest) error {
	switch req.Method {
	case codex.RequestMethodThreadStart:
		threadID := fmt.Sprintf("thread-input-%d", len(s.threads)+1)
		thread := helperThread(threadID)
		s.threads[threadID] = thread
		s.writeResult(req.ID, helperThreadState(thread, req.Params))
	case codex.RequestMethodTurnStart:
		if err := validateInputCapture(req.Params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return nil
		}
		s.turnCount++
		turnID := fmt.Sprintf("turn-input-%d", s.turnCount)
		threadID := stringParam(req.Params, "threadId")
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		s.writeNotification(codex.NotificationMethodItemCompleted, codex.Object{"threadId": threadID, "turnId": turnID, "item": codex.Object{"id": "input-item-" + turnID, "type": "agentMessage", "phase": "final_answer", "text": inputCaptureFinalResponse(textInput(req.Params))}})
		s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": threadID, "turn": codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}})
	default:
		s.writeError(req.ID, -32601, "unexpected input capture method "+req.Method, nil)
	}
	return nil
}

func validateInputCapture(params codex.Object) error {
	input, ok := params["input"].([]any)
	if !ok || len(input) != 2 {
		return fmt.Errorf("input capture expected two input items, got %T len=%d", params["input"], len(input))
	}
	textItem, ok := input[0].(map[string]any)
	if !ok || textItem["type"] != "text" {
		return fmt.Errorf("first input item = %#v, want text item", input[0])
	}
	text, _ := textItem["text"].(string)
	second, ok := input[1].(map[string]any)
	if !ok {
		return fmt.Errorf("second input item = %T, want object", input[1])
	}
	switch text {
	case "Describe the remote image.":
		if second["type"] != "image" || second["url"] != "https://example.com/codex.png" {
			return fmt.Errorf("remote image input = %#v, want image URL", second)
		}
	case "Describe the local image.":
		path, _ := second["path"].(string)
		if second["type"] != "localImage" || path == "" {
			return fmt.Errorf("local image input = %#v, want localImage path", second)
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("local image path %q is not readable: %w", path, err)
		}
	case "Use the selected skill.":
		path, _ := second["path"].(string)
		if second["type"] != "skill" || second["name"] != "demo" || path == "" {
			return fmt.Errorf("skill input = %#v, want demo skill path", second)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read skill file %q: %w", path, err)
		}
		if !strings.Contains(string(body), "Use the word cobalt.") {
			return fmt.Errorf("skill file %q missing expected body", path)
		}
	default:
		return fmt.Errorf("unexpected input capture text %q", text)
	}
	return nil
}

func inputCaptureFinalResponse(input string) string {
	switch input {
	case "Describe the remote image.":
		return "remote image received"
	case "Describe the local image.":
		return "local image received"
	case "Use the selected skill.":
		return "skill received"
	default:
		return "input response for " + input
	}
}

type helperLifecycleThread struct {
	thread   codex.Object
	archived bool
	turns    []codex.Object
}

func (s *helperState) handleLifecyclePersistence(req helperRequest) error {
	s.ensureLifecycleState()
	switch req.Method {
	case codex.RequestMethodThreadStart:
		threadID := fmt.Sprintf("thread-life-%d", len(s.lifecycle)+1)
		state := &helperLifecycleThread{thread: helperThread(threadID)}
		s.lifecycle[threadID] = state
		s.writeResult(req.ID, helperThreadState(state.payload(false), req.Params))
	case codex.RequestMethodTurnStart:
		if err := validateTurnInput(req.Params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return nil
		}
		threadID := stringParam(req.Params, "threadId")
		state := s.lifecycleThread(threadID)
		s.turnCount++
		turnID := fmt.Sprintf("turn-life-persist-%d", s.turnCount)
		text := textInput(req.Params)
		final := lifecycleFinalResponse(text)
		state.turns = append(state.turns, helperLifecycleTurn(turnID, text, final))
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		s.writeNotification(codex.NotificationMethodItemCompleted, codex.Object{"threadId": threadID, "turnId": turnID, "item": codex.Object{"id": "agent-" + turnID, "type": "agentMessage", "phase": "final_answer", "text": final}})
		s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": threadID, "turn": codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}})
	case codex.RequestMethodThreadNameSet:
		threadID := stringParam(req.Params, "threadId")
		name := stringParam(req.Params, "name")
		state := s.lifecycleThread(threadID)
		if name == "" {
			delete(state.thread, "name")
		} else {
			state.thread["name"] = name
		}
		s.writeResult(req.ID, codex.Object{})
	case codex.RequestMethodThreadRead:
		threadID := stringParam(req.Params, "threadId")
		state := s.lifecycleThread(threadID)
		includeTurns, _ := req.Params["includeTurns"].(bool)
		s.writeResult(req.ID, codex.Object{"thread": state.payload(includeTurns)})
	case codex.RequestMethodThreadResume:
		threadID := stringParam(req.Params, "threadId")
		state := s.lifecycleThread(threadID)
		s.writeResult(req.ID, helperThreadState(state.payload(true), req.Params))
	case codex.RequestMethodThreadFork:
		sourceID := stringParam(req.Params, "threadId")
		source := s.lifecycleThread(sourceID)
		forkID := fmt.Sprintf("%s-fork-%d", sourceID, len(s.lifecycle)+1)
		fork := &helperLifecycleThread{thread: helperThread(forkID), turns: append([]codex.Object(nil), source.turns...)}
		if name, ok := source.thread["name"]; ok {
			fork.thread["name"] = name
		}
		fork.thread["forkedFromId"] = sourceID
		s.lifecycle[forkID] = fork
		s.writeResult(req.ID, helperThreadState(fork.payload(true), req.Params))
	case codex.RequestMethodThreadArchive:
		threadID := stringParam(req.Params, "threadId")
		s.lifecycleThread(threadID).archived = true
		s.writeResult(req.ID, codex.Object{})
	case codex.RequestMethodThreadUnarchive:
		threadID := stringParam(req.Params, "threadId")
		state := s.lifecycleThread(threadID)
		state.archived = false
		s.writeResult(req.ID, codex.Object{"thread": state.payload(true)})
	case codex.RequestMethodThreadList:
		archived, _ := req.Params["archived"].(bool)
		threads := make([]codex.Object, 0, len(s.lifecycle))
		for _, state := range s.lifecycle {
			if state.archived == archived {
				threads = append(threads, state.payload(false))
			}
		}
		s.writeResult(req.ID, codex.Object{"data": threads})
	case codex.RequestMethodModelList:
		s.writeResult(req.ID, codex.Object{"data": []codex.Object{{"id": "gpt-life", "model": "gpt-life", "displayName": "GPT Lifecycle", "description": "lifecycle test model", "hidden": false, "isDefault": true}}})
	case codex.RequestMethodThreadCompactStart:
		threadID := stringParam(req.Params, "threadId")
		if _, ok := s.lifecycle[threadID]; !ok {
			s.writeError(req.ID, -32602, "unknown compact thread "+threadID, nil)
			return nil
		}
		s.writeResult(req.ID, codex.Object{})
	default:
		s.writeError(req.ID, -32601, "unexpected lifecycle persistence method "+req.Method, nil)
	}
	return nil
}

func (s *helperState) ensureLifecycleState() {
	if s.lifecycle == nil {
		s.lifecycle = map[string]*helperLifecycleThread{}
	}
}

func (s *helperState) lifecycleThread(threadID string) *helperLifecycleThread {
	if state, ok := s.lifecycle[threadID]; ok {
		return state
	}
	state := &helperLifecycleThread{thread: helperThread(threadID)}
	s.lifecycle[threadID] = state
	return state
}

func (s *helperLifecycleThread) payload(includeTurns bool) codex.Object {
	thread := cloneObject(s.thread)
	if includeTurns {
		thread["turns"] = append([]codex.Object(nil), s.turns...)
	} else {
		thread["turns"] = []codex.Object{}
	}
	return thread
}

func helperLifecycleTurn(turnID, userText, agentText string) codex.Object {
	return codex.Object{
		"id":     turnID,
		"status": "completed",
		"items": []codex.Object{
			{
				"id":      "user-" + turnID,
				"type":    "userMessage",
				"content": []codex.Object{{"type": "text", "text": userText}},
			},
			{
				"id":   "agent-" + turnID,
				"type": "agentMessage",
				"text": agentText,
			},
		},
	}
}

func lifecycleFinalResponse(input string) string {
	switch input {
	case "keep this listed":
		return "active"
	case "archive this":
		return "archived"
	case "first question":
		return "first answer"
	case "second question":
		return "second answer"
	case "materialize async thread":
		return "async materialized"
	case "materialize this thread before fork", "materialize this thread before archive":
		return "materialized"
	case "create history":
		return "history"
	default:
		return "lifecycle response for " + input
	}
}

func cloneObject(in codex.Object) codex.Object {
	out := make(codex.Object, len(in))
	maps.Copy(out, in)
	return out
}

func (s *helperState) handleRunResult(req helperRequest) error {
	switch req.Method {
	case codex.RequestMethodThreadStart:
		threadID := fmt.Sprintf("thread-run-%d", len(s.threads)+1)
		thread := helperThread(threadID)
		s.threads[threadID] = thread
		s.writeResult(req.ID, helperThreadState(thread, req.Params))
	case codex.RequestMethodTurnStart:
		if err := validateRunResultParams(req.Params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return nil
		}
		s.turnCount++
		turnID := fmt.Sprintf("turn-run-%d", s.turnCount)
		threadID := stringParam(req.Params, "threadId")
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		s.writeRunResultNotifications(threadID, turnID, textInput(req.Params))
	default:
		s.writeError(req.ID, -32601, "unexpected run result method "+req.Method, nil)
	}
	return nil
}

func validateRunResultParams(params codex.Object) error {
	if err := validateTurnInput(params); err != nil {
		return err
	}
	if textInput(params) == "use overrides" {
		if model, _ := params["model"].(string); model != "mock-model-override" {
			return fmt.Errorf("model override = %q, want mock-model-override", model)
		}
	}
	return nil
}

func (s *helperState) writeRunResultNotifications(threadID, turnID, input string) {
	switch input {
	case "hello":
		s.writeRunResultUsage(threadID, turnID, helperUsageFull(1, 3, 1, 2))
		s.writeRunResultAgentMessage(threadID, turnID, "msg-"+turnID, "Hello from the mock.", "")
		s.writeRunResultCompleted(threadID, turnID, nil)
	case "use overrides":
		s.writeRunResultUsage(threadID, turnID, helperUsageFull(11, 7, 5, 18))
		s.writeRunResultAgentMessage(threadID, turnID, "msg-"+turnID, "overrides applied", "")
		s.writeRunResultCompleted(threadID, turnID, nil)
	case "async hello":
		s.writeRunResultAgentMessage(threadID, turnID, "msg-"+turnID, "Hello async.", "")
		s.writeRunResultCompleted(threadID, turnID, nil)
	case "case: last unknown phase wins":
		s.writeRunResultAgentMessage(threadID, turnID, "msg-first-"+turnID, "First message", "")
		s.writeRunResultAgentMessage(threadID, turnID, "msg-second-"+turnID, "Second message", "")
		s.writeRunResultCompleted(threadID, turnID, nil)
	case "case: empty last message":
		s.writeRunResultAgentMessage(threadID, turnID, "msg-nonempty-"+turnID, "First message", "")
		s.writeRunResultAgentMessage(threadID, turnID, "msg-empty-"+turnID, "", "")
		s.writeRunResultCompleted(threadID, turnID, nil)
	case "case: commentary only":
		s.writeRunResultAgentMessage(threadID, turnID, "msg-commentary-"+turnID, "Commentary", "commentary")
		s.writeRunResultCompleted(threadID, turnID, nil)
	case "case: async last unknown phase":
		s.writeRunResultAgentMessage(threadID, turnID, "msg-async-first-"+turnID, "First async message", "")
		s.writeRunResultAgentMessage(threadID, turnID, "msg-async-second-"+turnID, "Second async message", "")
		s.writeRunResultCompleted(threadID, turnID, nil)
	case "case: async commentary only":
		s.writeRunResultAgentMessage(threadID, turnID, "msg-async-commentary-"+turnID, "Async commentary", "commentary")
		s.writeRunResultCompleted(threadID, turnID, nil)
	case "trigger failure":
		s.writeRunResultCompleted(threadID, turnID, codex.Object{"message": "boom from mock model"})
	case "choose final answer":
		s.writeRunResultAgentMessage(threadID, turnID, "msg-commentary-"+turnID, "Commentary", "commentary")
		s.writeRunResultAgentMessage(threadID, turnID, "msg-final-"+turnID, "Final answer", "final_answer")
		s.writeRunResultCompleted(threadID, turnID, nil)
	default:
		s.writeRunResultAgentMessage(threadID, turnID, "msg-"+turnID, "run response for "+input, "")
		s.writeRunResultCompleted(threadID, turnID, nil)
	}
}

func (s *helperState) writeRunResultUsage(threadID, turnID string, usage codex.Object) {
	s.writeNotification(codex.NotificationMethodThreadTokenUsageUpdated, codex.Object{"threadId": threadID, "turnId": turnID, "tokenUsage": usage})
}

func (s *helperState) writeRunResultAgentMessage(threadID, turnID, itemID, text, phase string) {
	item := codex.Object{"id": itemID, "type": "agentMessage", "text": text}
	if phase != "" {
		item["phase"] = phase
	}
	s.writeNotification(codex.NotificationMethodItemCompleted, codex.Object{"threadId": threadID, "turnId": turnID, "item": item})
}

func (s *helperState) writeRunResultCompleted(threadID, turnID string, failure codex.Object) {
	turn := codex.Object{"id": turnID, "status": "completed", "items": []codex.Object{}}
	if failure != nil {
		turn["status"] = "failed"
		turn["error"] = failure
	}
	s.writeNotification(codex.NotificationMethodTurnCompleted, codex.Object{"threadId": threadID, "turn": turn})
}

func helperUsageFull(inputTokens, outputTokens, reasoningOutputTokens, totalTokens int64) codex.Object {
	breakdown := codex.Object{
		"cachedInputTokens":     int64(3),
		"inputTokens":           inputTokens,
		"outputTokens":          outputTokens,
		"reasoningOutputTokens": reasoningOutputTokens,
		"totalTokens":           totalTokens,
	}
	return codex.Object{"last": breakdown, "total": breakdown}
}

func (s *helperState) handleClientRoutingRetry(req helperRequest) error {
	switch req.Method {
	case codex.RequestMethodTurnStart:
		turnID := "turn-route"
		s.writeResult(req.ID, codex.Object{"turn": codex.Object{"id": turnID, "status": "inProgress"}})
		s.writeNotification(codex.NotificationMethodItemAgentMessageDelta, codex.Object{"threadId": stringParam(req.Params, "threadId"), "turnId": "other-turn", "itemId": "other", "delta": "ignored"})
		s.writeNotification(codex.NotificationMethodItemAgentMessageDelta, codex.Object{"threadId": stringParam(req.Params, "threadId"), "turnId": turnID, "itemId": "target", "delta": "first"})
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
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
