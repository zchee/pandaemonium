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
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/google/go-cmp/cmp"

	"github.com/zchee/pandaemonium/pkg/codex"
)

type responseJSON = map[string]any

type capturedResponsesRequest struct {
	method  string
	path    string
	headers map[string]string
	body    []byte
}

func (r capturedResponsesRequest) bodyJSON(t *testing.T) responseJSON {
	t.Helper()
	var body responseJSON
	if err := json.Unmarshal(r.body, &body); err != nil {
		t.Fatalf("json.Unmarshal(captured body) error = %v; body = %s", err, r.body)
	}
	return body
}

func (r capturedResponsesRequest) input(t *testing.T) []responseJSON {
	t.Helper()
	body := r.bodyJSON(t)
	value, ok := body["input"].([]any)
	if !ok {
		t.Fatalf("captured request input = %T, want array; body = %s", body["input"], r.body)
	}
	input := make([]responseJSON, 0, len(value))
	for index, item := range value {
		object, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("captured request input[%d] = %T, want object; body = %s", index, item, r.body)
		}
		input = append(input, responseJSON(object))
	}
	return input
}

func (r capturedResponsesRequest) messageInputTexts(t *testing.T, role string) []string {
	t.Helper()
	var texts []string
	for _, item := range r.input(t) {
		if item["type"] != "message" || item["role"] != role {
			continue
		}
		switch content := item["content"].(type) {
		case string:
			texts = append(texts, content)
		case []any:
			for _, span := range content {
				object, ok := span.(map[string]any)
				if !ok || object["type"] != "input_text" {
					continue
				}
				if text, ok := object["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
	}
	return texts
}

func (r capturedResponsesRequest) messageContentItems(t *testing.T, role string) []responseJSON {
	t.Helper()
	var items []responseJSON
	for _, item := range r.input(t) {
		if item["type"] != "message" || item["role"] != role {
			continue
		}
		content, ok := item["content"].([]any)
		if !ok {
			continue
		}
		for _, part := range content {
			object, ok := part.(map[string]any)
			if ok {
				items = append(items, responseJSON(object))
			}
		}
	}
	return items
}

func (r capturedResponsesRequest) messageImageURLs(t *testing.T, role string) []string {
	t.Helper()
	var urls []string
	for _, item := range r.messageContentItems(t, role) {
		if item["type"] != "input_image" {
			continue
		}
		if imageURL, ok := item["image_url"].(string); ok {
			urls = append(urls, imageURL)
		}
	}
	return urls
}

func (r capturedResponsesRequest) header(name string) string {
	return r.headers[strings.ToLower(name)]
}

type mockSSEResponse struct {
	body               string
	delayBetweenEvents time.Duration
}

func (r mockSSEResponse) chunks() [][]byte {
	var chunks [][]byte
	for part := range strings.SplitSeq(r.body, "\n\n") {
		if part == "" {
			continue
		}
		chunks = append(chunks, []byte(part+"\n\n"))
	}
	return chunks
}

type mockResponsesServer struct {
	server *httptest.Server

	mu               sync.Mutex
	responses        []mockSSEResponse
	capturedRequests []capturedResponsesRequest
}

func newMockResponsesServer() *mockResponsesServer {
	mock := &mockResponsesServer{}
	mock.server = httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	return mock
}

func (s *mockResponsesServer) close() {
	s.server.Close()
}

func (s *mockResponsesServer) url() string {
	return s.server.URL
}

func (s *mockResponsesServer) enqueueSSE(body string, delayBetweenEvents time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = append(s.responses, mockSSEResponse{body: body, delayBetweenEvents: delayBetweenEvents})
}

func (s *mockResponsesServer) enqueueAssistantMessage(text string, responseID string) {
	if responseID == "" {
		responseID = "resp-1"
	}
	s.enqueueSSE(sse([]responseJSON{
		evResponseCreated(responseID),
		evAssistantMessage("msg-"+responseID, text),
		evCompleted(responseID),
	}), 0)
}

func (s *mockResponsesServer) requests() []capturedResponsesRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.capturedRequests)
}

func (s *mockResponsesServer) singleRequest(t *testing.T) capturedResponsesRequest {
	t.Helper()
	requests := s.requests()
	if len(requests) != 1 {
		t.Fatalf("captured request count = %d, want 1", len(requests))
	}
	return requests[0]
}

func (s *mockResponsesServer) waitForRequests(t *testing.T, count int, timeout time.Duration) []capturedResponsesRequest {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		requests := s.requests()
		if len(requests) >= count {
			return requests
		}
		time.Sleep(10 * time.Millisecond)
	}
	requests := s.requests()
	t.Fatalf("captured request count = %d, want at least %d", len(requests), count)
	return nil
}

func (s *mockResponsesServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGET(w, r)
	case http.MethodPost:
		s.handlePOST(w, r)
	default:
		http.Error(w, "unexpected method "+r.Method, http.StatusNotFound)
	}
}

func (s *mockResponsesServer) handleGET(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/v1/models") || strings.HasSuffix(r.URL.Path, "/models") {
		s.writeJSON(w, responseJSON{
			"object": "list",
			"data": []responseJSON{{
				"id":       "mock-model",
				"object":   "model",
				"created":  float64(0),
				"owned_by": "openai",
			}},
		})
		return
	}
	http.Error(w, "unexpected GET "+r.URL.Path, http.StatusNotFound)
}

func (s *mockResponsesServer) handlePOST(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read request body: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.recordRequest(r, body)
	if !strings.HasSuffix(r.URL.Path, "/v1/responses") && !strings.HasSuffix(r.URL.Path, "/responses") {
		http.Error(w, "unexpected POST "+r.URL.Path, http.StatusNotFound)
		return
	}
	response, ok := s.nextResponse()
	if !ok {
		http.Error(w, "no queued SSE response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for _, chunk := range response.chunks() {
		_, _ = w.Write(chunk)
		if flusher != nil {
			flusher.Flush()
		}
		if response.delayBetweenEvents > 0 {
			time.Sleep(response.delayBetweenEvents)
		}
	}
}

func (s *mockResponsesServer) recordRequest(r *http.Request, body []byte) {
	headers := make(map[string]string, len(r.Header))
	for key, values := range r.Header {
		headers[strings.ToLower(key)] = strings.Join(values, ",")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capturedRequests = append(s.capturedRequests, capturedResponsesRequest{
		method:  r.Method,
		path:    r.URL.Path,
		headers: headers,
		body:    slices.Clone(body),
	})
}

func (s *mockResponsesServer) nextResponse() (mockSSEResponse, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.responses) == 0 {
		return mockSSEResponse{}, false
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, true
}

func (s *mockResponsesServer) writeJSON(w http.ResponseWriter, payload responseJSON) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "marshal JSON response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.Header().Set("content-length", fmt.Sprint(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

type appServerHarness struct {
	t          *testing.T
	codexHome  string
	workspace  string
	responses  *mockResponsesServer
	configPath string
}

func newAppServerHarness(t *testing.T) *appServerHarness {
	t.Helper()
	tmpPath := t.TempDir()
	return &appServerHarness{
		t:         t,
		codexHome: filepath.Join(tmpPath, "codex-home"),
		workspace: filepath.Join(tmpPath, "workspace"),
	}
}

func (h *appServerHarness) start() *appServerHarness {
	h.t.Helper()
	if err := os.Mkdir(h.codexHome, 0o755); err != nil {
		h.t.Fatalf("os.Mkdir(codexHome) error = %v", err)
	}
	if err := os.Mkdir(h.workspace, 0o755); err != nil {
		h.t.Fatalf("os.Mkdir(workspace) error = %v", err)
	}
	h.responses = newMockResponsesServer()
	h.t.Cleanup(h.close)
	h.writeConfig()
	return h
}

func (h *appServerHarness) close() {
	if h.responses != nil {
		h.responses.close()
		h.responses = nil
	}
}

func (h *appServerHarness) appServerConfig() *codex.Config {
	return &codex.Config{
		Cwd: h.workspace,
		Env: map[string]string{
			"CODEX_HOME": h.codexHome,
			"CODEX_APP_SERVER_DISABLE_MANAGED_CONFIG": "1",
			"RUST_LOG": "warn",
		},
	}
}

func (h *appServerHarness) writeConfig() {
	h.t.Helper()
	h.configPath = filepath.Join(h.codexHome, "config.toml")
	config := fmt.Sprintf(`model = "mock-model"
approval_policy = "never"
sandbox_mode = "read-only"

model_provider = "mock_provider"

[model_providers.mock_provider]
name = "Mock provider for Go SDK tests"
base_url = %q
wire_api = "responses"
request_max_retries = 0
stream_max_retries = 0
`, h.responses.url()+"/v1")
	if err := os.WriteFile(h.configPath, []byte(config), 0o644); err != nil {
		h.t.Fatalf("os.WriteFile(config.toml) error = %v", err)
	}
}

func sse(events []responseJSON) string {
	chunks := make([]string, 0, len(events))
	for _, event := range events {
		eventType, ok := event["type"].(string)
		if !ok || eventType == "" {
			panic(fmt.Sprintf("SSE event has no string type: %#v", event))
		}
		body, err := json.Marshal(event)
		if err != nil {
			panic(fmt.Sprintf("marshal SSE event: %v", err))
		}
		chunks = append(chunks, fmt.Sprintf("event: %s\ndata: %s\n", eventType, body))
	}
	return strings.Join(chunks, "\n") + "\n"
}

func evResponseCreated(responseID string) responseJSON {
	return responseJSON{"type": "response.created", "response": responseJSON{"id": responseID}}
}

func evCompleted(responseID string) responseJSON {
	return responseJSON{
		"type": "response.completed",
		"response": responseJSON{
			"id": responseID,
			"usage": responseJSON{
				"input_tokens":          float64(1),
				"input_tokens_details":  nil,
				"output_tokens":         float64(1),
				"output_tokens_details": nil,
				"total_tokens":          float64(2),
			},
		},
	}
}

func evCompletedWithUsage(responseID string, inputTokens, cachedInputTokens, outputTokens, reasoningOutputTokens, totalTokens int) responseJSON {
	return responseJSON{
		"type": "response.completed",
		"response": responseJSON{
			"id": responseID,
			"usage": responseJSON{
				"input_tokens": float64(inputTokens),
				"input_tokens_details": responseJSON{
					"cached_tokens": float64(cachedInputTokens),
				},
				"output_tokens": float64(outputTokens),
				"output_tokens_details": responseJSON{
					"reasoning_tokens": float64(reasoningOutputTokens),
				},
				"total_tokens": float64(totalTokens),
			},
		},
	}
}

func evAssistantMessage(itemID string, text string) responseJSON {
	return responseJSON{
		"type": "response.output_item.done",
		"item": responseJSON{
			"type":    "message",
			"role":    "assistant",
			"id":      itemID,
			"content": []responseJSON{{"type": "output_text", "text": text}},
		},
	}
}

func evMessageItemAdded(itemID string, text string) responseJSON {
	return responseJSON{
		"type": "response.output_item.added",
		"item": responseJSON{
			"type":    "message",
			"role":    "assistant",
			"id":      itemID,
			"content": []responseJSON{{"type": "output_text", "text": text}},
		},
	}
}

func evOutputTextDelta(delta string) responseJSON {
	return responseJSON{"type": "response.output_text.delta", "delta": delta}
}

func evFunctionCall(callID string, name string, arguments string) responseJSON {
	return responseJSON{
		"type": "response.output_item.done",
		"item": responseJSON{
			"type":      "function_call",
			"call_id":   callID,
			"name":      name,
			"arguments": arguments,
		},
	}
}

func evFailed(responseID string, message string) responseJSON {
	return responseJSON{
		"type": "response.failed",
		"response": responseJSON{
			"id": responseID,
			"error": responseJSON{
				"code":    "server_error",
				"message": message,
			},
		},
	}
}

func TestAppServerHarnessConfigAndMockResponsesServerPort(t *testing.T) {
	harness := newAppServerHarness(t).start()
	config := harness.appServerConfig()
	if config.Cwd != harness.workspace {
		t.Fatalf("Config.Cwd = %q, want workspace %q", config.Cwd, harness.workspace)
	}
	wantEnv := map[string]string{
		"CODEX_HOME": harness.codexHome,
		"CODEX_APP_SERVER_DISABLE_MANAGED_CONFIG": "1",
		"RUST_LOG": "warn",
	}
	if diff := cmp.Diff(wantEnv, config.Env); diff != "" {
		t.Fatalf("Config.Env mismatch (-want +got):\n%s", diff)
	}
	configTOML, err := os.ReadFile(harness.configPath)
	if err != nil {
		t.Fatalf("os.ReadFile(config.toml) error = %v", err)
	}
	for _, want := range []string{
		`model = "mock-model"`,
		`base_url = "` + harness.responses.url() + `/v1"`,
		`wire_api = "responses"`,
		`request_max_retries = 0`,
		`stream_max_retries = 0`,
	} {
		if !strings.Contains(string(configTOML), want) {
			t.Fatalf("config.toml missing %q:\n%s", want, configTOML)
		}
	}

	events := []responseJSON{
		evResponseCreated("resp-1"),
		evMessageItemAdded("msg-1", ""),
		evOutputTextDelta("hel"),
		evOutputTextDelta("lo"),
		evAssistantMessage("msg-1", "hello"),
		evFunctionCall("call-1", "demo", `{"ok":true}`),
		evCompletedWithUsage("resp-1", 3, 1, 5, 2, 8),
		evFailed("resp-failed", "ignored helper event"),
	}
	harness.responses.enqueueSSE(sse(events), 0)
	requestBody := []byte(`{
		"input":[
			{"type":"message","role":"system","content":"system prompt"},
			{"type":"message","role":"user","content":[
				{"type":"input_text","text":"hello"},
				{"type":"input_image","image_url":"https://example.com/image.png"}
			]}
		]
	}`)
	response, err := http.Post(harness.responses.url()+"/v1/responses", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("POST /v1/responses error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST /v1/responses status = %d, want 200; body = %s", response.StatusCode, body)
	}
	if got := response.Header.Get("content-type"); got != "text/event-stream" {
		t.Fatalf("POST /v1/responses content-type = %q, want text/event-stream", got)
	}
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("io.ReadAll(SSE response) error = %v", err)
	}
	for _, want := range []string{"event: response.created", "event: response.output_text.delta", "event: response.completed", "event: response.failed"} {
		if !strings.Contains(string(responseBody), want) {
			t.Fatalf("SSE response missing %q:\n%s", want, responseBody)
		}
	}

	captured := harness.responses.singleRequest(t)
	if captured.method != http.MethodPost || captured.path != "/v1/responses" {
		t.Fatalf("captured request = %s %s, want POST /v1/responses", captured.method, captured.path)
	}
	if got := captured.header("Content-Type"); got != "application/json" {
		t.Fatalf("captured Content-Type = %q, want application/json", got)
	}
	if got := captured.messageInputTexts(t, "system"); !cmp.Equal(got, []string{"system prompt"}) {
		t.Fatalf("system message texts mismatch (-want +got):\n%s", cmp.Diff([]string{"system prompt"}, got))
	}
	if got := captured.messageInputTexts(t, "user"); !cmp.Equal(got, []string{"hello"}) {
		t.Fatalf("user message texts mismatch (-want +got):\n%s", cmp.Diff([]string{"hello"}, got))
	}
	if got := captured.messageImageURLs(t, "user"); !cmp.Equal(got, []string{"https://example.com/image.png"}) {
		t.Fatalf("user image URLs mismatch (-want +got):\n%s", cmp.Diff([]string{"https://example.com/image.png"}, got))
	}
	if got := len(captured.messageContentItems(t, "user")); got != 2 {
		t.Fatalf("user message content item count = %d, want 2", got)
	}

	harness.responses.enqueueAssistantMessage("second", "resp-2")
	secondResponse, err := http.Post(harness.responses.url()+"/responses", "application/json", strings.NewReader(`{"input":[]}`))
	if err != nil {
		t.Fatalf("POST /responses error = %v", err)
	}
	defer secondResponse.Body.Close()
	if secondResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(secondResponse.Body)
		t.Fatalf("POST /responses status = %d, want 200; body = %s", secondResponse.StatusCode, body)
	}
	_ = harness.responses.waitForRequests(t, 2, time.Second)

	modelsResponse, err := http.Get(harness.responses.url() + "/v1/models")
	if err != nil {
		t.Fatalf("GET /v1/models error = %v", err)
	}
	defer modelsResponse.Body.Close()
	if modelsResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(modelsResponse.Body)
		t.Fatalf("GET /v1/models status = %d, want 200; body = %s", modelsResponse.StatusCode, body)
	}
	var models responseJSON
	if err := json.UnmarshalRead(modelsResponse.Body, &models); err != nil {
		t.Fatalf("json.UnmarshalRead(models) error = %v", err)
	}
	wantModels := responseJSON{"object": "list", "data": []any{map[string]any{"id": "mock-model", "object": "model", "created": float64(0), "owned_by": "openai"}}}
	if diff := cmp.Diff(wantModels, models); diff != "" {
		t.Fatalf("models response mismatch (-want +got):\n%s", diff)
	}
}

func TestMockResponsesServerReportsMissingQueuedResponse(t *testing.T) {
	server := newMockResponsesServer()
	t.Cleanup(server.close)
	response, err := http.Post(server.url()+"/v1/responses", "application/json", strings.NewReader(`{"input":[]}`))
	if err != nil {
		t.Fatalf("POST /v1/responses error = %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("io.ReadAll(missing response body) error = %v", err)
	}
	if response.StatusCode != http.StatusInternalServerError || !strings.Contains(string(body), "no queued SSE response") {
		t.Fatalf("POST /v1/responses = status %d body %q, want 500 no queued SSE response", response.StatusCode, body)
	}
	captured := server.singleRequest(t)
	if captured.path != "/v1/responses" {
		t.Fatalf("captured request path = %q, want /v1/responses", captured.path)
	}
}

func TestMockResponsesServerRejectsUnexpectedPaths(t *testing.T) {
	server := newMockResponsesServer()
	t.Cleanup(server.close)
	for name, run := range map[string]func(t *testing.T) *http.Response{
		"GET": func(t *testing.T) *http.Response {
			t.Helper()
			response, err := http.Get(server.url() + "/v1/unknown")
			if err != nil {
				t.Fatalf("GET /v1/unknown error = %v", err)
			}
			return response
		},
		"POST": func(t *testing.T) *http.Response {
			t.Helper()
			response, err := http.Post(server.url()+"/v1/unknown", "application/json", strings.NewReader(`{"input":[]}`))
			if err != nil {
				t.Fatalf("POST /v1/unknown error = %v", err)
			}
			return response
		},
	} {
		t.Run(name, func(t *testing.T) {
			response := run(t)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("io.ReadAll(%s body) error = %v", name, err)
			}
			if response.StatusCode != http.StatusNotFound || !strings.Contains(string(body), "unexpected "+name) {
				t.Fatalf("%s /v1/unknown = status %d body %q, want 404 unexpected method", name, response.StatusCode, body)
			}
		})
	}
}
