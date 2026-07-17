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

package opencode

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
)

// TestMain doubles as the fake `opencode` binary for the spawn-path tests
// (helper-process pattern): when FAKE_OPENCODE_MODE is set, the test binary
// behaves like `opencode serve` instead of running tests.
func TestMain(m *testing.M) {
	if mode := os.Getenv("FAKE_OPENCODE_MODE"); mode != "" {
		runFakeOpencodeMain(mode)
		return
	}
	os.Exit(m.Run())
}

// runFakeOpencodeMain emulates `opencode serve` for spawn tests. Modes:
//
//   - "announce": print the unsecured-warning line (exercising line
//     scanning), bind, print the real announce line, and serve.
//   - "no-announce": bind and serve but never print the announce line; with
//     --port 0 this exercises the announce-timeout + explicit-port respawn
//     fallback end to end.
func runFakeOpencodeMain(mode string) {
	var hostname string
	port := 0
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--hostname":
			i++
			hostname = args[i]
		case "--port":
			i++
			port, _ = strconv.Atoi(args[i])
		}
	}
	if hostname == "" {
		hostname = "127.0.0.1"
	}

	fake := newFakeOpencode()
	fake.password = os.Getenv("OPENCODE_SERVER_PASSWORD")

	listener, err := net.Listen("tcp", net.JoinHostPort(hostname, strconv.Itoa(port)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake opencode: listen: %v\n", err)
		os.Exit(1)
	}

	if mode == "announce" {
		if fake.password == "" {
			fmt.Println("Warning: OPENCODE_SERVER_PASSWORD is not set; server is unsecured.")
		}
		fmt.Printf("opencode server listening on http://%s\n", listener.Addr().String())
	}

	server := &http.Server{Handler: fake.handler(), ReadHeaderTimeout: 10 * time.Second}
	_ = server.Serve(listener)
}

// permissionReply records one reply the wrapper POSTed back for a permission
// request.
type permissionReply struct {
	SessionID    string
	PermissionID string
	Response     PermissionResponse
	V2           bool
}

// requestRecord captures per-request auth evidence for AC7/AC8 assertions.
type requestRecord struct {
	Method   string
	Path     string
	HasAuth  bool
	Username string
	Password string
}

// fakeOpencode is a scriptable in-process OpenCode server covering the
// wrapped REST subset plus the /event SSE hub. The zero scripting default
// answers prompts immediately with a fixed text part.
type fakeOpencode struct {
	password string

	mu          sync.Mutex
	sessionSeq  int
	sessions    map[string]SessionInfo
	aborted     map[string]bool
	requests    []requestRecord
	permReplies []permissionReply

	// Prompt scripting.
	promptStatus  int           // non-2xx to fail prompts; 0 means 200
	promptBlock   chan struct{} // non-nil: prompt blocks until closed or aborted
	promptStarted chan string   // receives sessionID when a prompt begins

	// Permission scripting: when set, a prompt emits permission.asked and
	// blocks until the wrapper replies (mirroring the probed real-server
	// pause), then completes.
	permissionGate bool
	permissionSeq  int
	permReplied    chan permissionReply

	// SSE scripting.
	subMu          sync.Mutex
	subSeq         int
	subs           map[int]chan string
	subOpened      chan int // receives subscription ordinal on each /event dial
	failEventDials bool     // /event dials respond 503
}

func newFakeOpencode() *fakeOpencode {
	return &fakeOpencode{
		sessions:      map[string]SessionInfo{},
		aborted:       map[string]bool{},
		subs:          map[int]chan string{},
		promptStarted: make(chan string, 16),
		subOpened:     make(chan int, 16),
		permReplied:   make(chan permissionReply, 16),
	}
}

// emit broadcasts one event to every live /event subscriber.
func (f *fakeOpencode) emit(ev any) {
	encoded, err := json.Marshal(ev)
	if err != nil {
		panic(fmt.Sprintf("fake opencode: marshal event: %v", err))
	}
	f.emitRaw("data: " + string(encoded) + "\n\n")
}

// emitRaw broadcasts a raw SSE chunk (may contain comments, multi-line data,
// CRLF endings) to every live subscriber.
func (f *fakeOpencode) emitRaw(chunk string) {
	f.subMu.Lock()
	defer f.subMu.Unlock()
	for _, sub := range f.subs {
		select {
		case sub <- chunk:
		case <-time.After(5 * time.Second):
			panic("fake opencode: subscriber stuck")
		}
	}
}

// event builds an Event payload with properties.
func fakeEvent(eventType string, props map[string]any) map[string]any {
	return map[string]any{
		"id":         "evt_fake",
		"type":       eventType,
		"properties": props,
	}
}

// closeSubscribers terminates all live /event streams (simulating a server
// -side disconnect).
func (f *fakeOpencode) closeSubscribers() {
	f.subMu.Lock()
	defer f.subMu.Unlock()
	for id, sub := range f.subs {
		close(sub)
		delete(f.subs, id)
	}
}

// setFailEventDials makes subsequent /event dials fail with 503.
func (f *fakeOpencode) setFailEventDials(fail bool) {
	f.subMu.Lock()
	f.failEventDials = fail
	f.subMu.Unlock()
}

// recordedRequests returns a copy of the request log.
func (f *fakeOpencode) recordedRequests() []requestRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]requestRecord(nil), f.requests...)
}

// permissionRepliesSeen returns a copy of the recorded permission replies.
func (f *fakeOpencode) permissionRepliesSeen() []permissionReply {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]permissionReply(nil), f.permReplies...)
}

func (f *fakeOpencode) defaultPromptResponse(sessionID string, err *MessageError) PromptResponse {
	message := AssistantMessage{
		ID:         "msg_fake_assistant",
		SessionID:  sessionID,
		Role:       "assistant",
		ParentID:   "msg_fake_user",
		ModelID:    "fake-model",
		ProviderID: "fake-provider",
		Mode:       "build",
		Agent:      "build",
		Path:       MessagePath{Cwd: "/tmp", Root: "/tmp"},
		Cost:       0.001,
		Tokens: TokenDetail{
			Total: 42, Input: 10, Output: 30, Reasoning: 2,
			Cache: TokenCache{Read: 5, Write: 1},
		},
		Error:  err,
		Finish: "stop",
		Time:   MessageTime{Created: 1, Completed: 2},
	}
	return PromptResponse{
		Info: message,
		Parts: []Part{
			{ID: "prt_1", SessionID: sessionID, MessageID: message.ID, Type: "step-start"},
			{ID: "prt_2", SessionID: sessionID, MessageID: message.ID, Type: "text", Text: "hello from fake opencode"},
			{ID: "prt_3", SessionID: sessionID, MessageID: message.ID, Type: "step-finish"},
		},
	}
}

// handler serves the wrapped REST subset.
func (f *fakeOpencode) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, pass, hasAuth := r.BasicAuth()
		f.mu.Lock()
		f.requests = append(f.requests, requestRecord{
			Method:   r.Method,
			Path:     r.URL.Path,
			HasAuth:  hasAuth,
			Username: username,
			Password: pass,
		})
		f.mu.Unlock()

		if f.password != "" {
			if !hasAuth || username != basicAuthUsername || pass != f.password {
				w.Header().Set("WWW-Authenticate", `Basic realm="Secure Area"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/global/health":
			writeJSON(w, http.StatusOK, Health{Healthy: true, Version: "fake-1.18.3"})

		case r.Method == http.MethodGet && r.URL.Path == "/event":
			f.serveEvents(w, r)

		case r.Method == http.MethodPost && r.URL.Path == "/session":
			var params SessionNewParams
			decodeJSONBody(r, &params)
			f.mu.Lock()
			f.sessionSeq++
			info := SessionInfo{
				ID:        fmt.Sprintf("ses_fake%04d", f.sessionSeq),
				Slug:      "fake-session",
				ProjectID: "prj_fake",
				Directory: "/tmp",
				ParentID:  params.ParentID,
				Title:     params.Title,
				Version:   "fake-1.18.3",
				Time:      SessionTime{Created: 1, Updated: 1},
			}
			f.sessions[info.ID] = info
			f.mu.Unlock()
			writeJSON(w, http.StatusOK, info)

		case r.Method == http.MethodGet && r.URL.Path == "/session":
			f.mu.Lock()
			sessions := make([]SessionInfo, 0, len(f.sessions))
			for _, info := range f.sessions {
				sessions = append(sessions, info)
			}
			f.mu.Unlock()
			writeJSON(w, http.StatusOK, sessions)

		case r.Method == http.MethodGet && r.URL.Path == "/config/providers":
			writeJSON(w, http.StatusOK, ProvidersResponse{
				Providers: []Provider{{
					ID: "fake-provider", Name: "Fake Provider", Source: "config",
					Models: map[string]Model{"fake-model": {ID: "fake-model", ProviderID: "fake-provider", Name: "Fake Model"}},
				}},
				Default: map[string]string{"fake-provider": "fake-model"},
			})

		case len(segments) >= 2 && segments[0] == "session":
			f.serveSession(w, r, segments[1], segments[2:])

		case len(segments) == 3 && segments[0] == "permission" && segments[2] == "reply":
			var body struct {
				Reply PermissionResponse `json:"reply"`
			}
			decodeJSONBody(r, &body)
			reply := permissionReply{PermissionID: segments[1], Response: body.Reply, V2: true}
			f.mu.Lock()
			f.permReplies = append(f.permReplies, reply)
			f.mu.Unlock()
			f.permReplied <- reply
			writeJSON(w, http.StatusOK, true)

		default:
			writeError(w, http.StatusNotFound, "NotFoundError", "no route for "+r.URL.Path)
		}
	})
}

// serveSession handles /session/{id}/... routes.
func (f *fakeOpencode) serveSession(w http.ResponseWriter, r *http.Request, sessionID string, rest []string) {
	f.mu.Lock()
	info, exists := f.sessions[sessionID]
	f.mu.Unlock()
	if !exists {
		writeError(w, http.StatusNotFound, "NotFoundError", "Session not found: "+sessionID)
		return
	}

	route := strings.Join(rest, "/")
	switch {
	case r.Method == http.MethodGet && route == "":
		writeJSON(w, http.StatusOK, info)

	case r.Method == http.MethodDelete && route == "":
		f.mu.Lock()
		delete(f.sessions, sessionID)
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, true)

	case r.Method == http.MethodPatch && route == "":
		var params SessionUpdateParams
		decodeJSONBody(r, &params)
		f.mu.Lock()
		if params.Title != "" {
			info.Title = params.Title
		}
		f.sessions[sessionID] = info
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, info)

	case r.Method == http.MethodPost && route == "fork":
		f.mu.Lock()
		f.sessionSeq++
		fork := info
		fork.ID = fmt.Sprintf("ses_fake%04d", f.sessionSeq)
		fork.ParentID = sessionID
		f.sessions[fork.ID] = fork
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, fork)

	case r.Method == http.MethodGet && route == "message":
		writeJSON(w, http.StatusOK, []MessageWithParts{})

	case r.Method == http.MethodPost && route == "message":
		f.servePrompt(w, r, sessionID)

	case r.Method == http.MethodPost && route == "abort":
		f.mu.Lock()
		f.aborted[sessionID] = true
		block := f.promptBlock
		f.mu.Unlock()
		if block != nil {
			select {
			case <-block:
			default:
				close(block)
			}
		}
		writeJSON(w, http.StatusOK, true)

	case r.Method == http.MethodPost && route == "summarize":
		writeJSON(w, http.StatusOK, true)

	case r.Method == http.MethodPost && route == "revert":
		writeJSON(w, http.StatusOK, info)

	case r.Method == http.MethodPost && route == "unrevert":
		writeJSON(w, http.StatusOK, info)

	case route == "share" && (r.Method == http.MethodPost || r.Method == http.MethodDelete):
		if r.Method == http.MethodPost {
			info.Share = &SessionShare{URL: "https://opencode.ai/s/fake"}
		} else {
			info.Share = nil
		}
		f.mu.Lock()
		f.sessions[sessionID] = info
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, info)

	case r.Method == http.MethodPost && route == "shell":
		resp := f.defaultPromptResponse(sessionID, nil)
		writeJSON(w, http.StatusOK, MessageWithParts{
			Info: Message{
				ID: resp.Info.ID, SessionID: sessionID, Role: "assistant",
				Time: MessageTime{Created: 1, Completed: 2},
			},
			Parts: resp.Parts,
		})

	case r.Method == http.MethodPost && route == "command":
		writeJSON(w, http.StatusOK, f.defaultPromptResponse(sessionID, nil))

	case len(rest) == 2 && rest[0] == "permissions" && r.Method == http.MethodPost:
		var body struct {
			Response PermissionResponse `json:"response"`
		}
		decodeJSONBody(r, &body)
		reply := permissionReply{SessionID: sessionID, PermissionID: rest[1], Response: body.Response}
		f.mu.Lock()
		f.permReplies = append(f.permReplies, reply)
		f.mu.Unlock()
		f.permReplied <- reply
		writeJSON(w, http.StatusOK, true)

	default:
		writeError(w, http.StatusNotFound, "NotFoundError", "no route for "+r.URL.Path)
	}
}

// servePrompt implements POST /session/{id}/message with scripting hooks.
func (f *fakeOpencode) servePrompt(w http.ResponseWriter, r *http.Request, sessionID string) {
	var params PromptParams
	decodeJSONBody(r, &params)

	select {
	case f.promptStarted <- sessionID:
	default:
	}

	f.mu.Lock()
	status := f.promptStatus
	block := f.promptBlock
	gate := f.permissionGate
	f.mu.Unlock()

	if gate {
		f.mu.Lock()
		f.permissionSeq++
		permID := fmt.Sprintf("per_fake%04d", f.permissionSeq)
		f.mu.Unlock()
		f.emit(fakeEvent(EventTypePermissionAsked, map[string]any{
			"id":         permID,
			"sessionID":  sessionID,
			"permission": "bash",
			"patterns":   []string{"echo probe"},
			"metadata":   map[string]any{},
			"always":     []string{"echo *"},
		}))
		// Pause the turn until the wrapper replies (probed real-server
		// behavior); a missing reply hangs the prompt exactly like 1.18.3.
		select {
		case <-f.permReplied:
		case <-r.Context().Done():
			return
		}
	}

	if block != nil {
		select {
		case <-block:
		case <-r.Context().Done():
			return
		}
	}

	if status != 0 && (status < 200 || status > 299) {
		writeError(w, status, "UnknownError", "scripted prompt failure")
		return
	}

	f.mu.Lock()
	wasAborted := f.aborted[sessionID]
	f.aborted[sessionID] = false
	f.mu.Unlock()

	if wasAborted {
		aborted := &MessageError{Name: errorNameMessageAborted, Data: []byte(`{"message":"aborted by request"}`)}
		writeJSON(w, http.StatusOK, f.defaultPromptResponse(sessionID, aborted))
		return
	}
	writeJSON(w, http.StatusOK, f.defaultPromptResponse(sessionID, nil))
}

// serveEvents implements the GET /event SSE hub: every connection gets
// server.connected first, then broadcast events until the subscriber channel
// closes or the client goes away.
func (f *fakeOpencode) serveEvents(w http.ResponseWriter, r *http.Request) {
	f.subMu.Lock()
	if f.failEventDials {
		f.subMu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "UnknownError", "scripted event dial failure")
		return
	}
	f.subSeq++
	id := f.subSeq
	sub := make(chan string, 64)
	f.subs[id] = sub
	f.subMu.Unlock()

	defer func() {
		f.subMu.Lock()
		if _, live := f.subs[id]; live {
			delete(f.subs, id)
			close(sub)
		}
		f.subMu.Unlock()
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "UnknownError", "no flusher")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "data: {\"id\":\"evt_connected%d\",\"type\":\"server.connected\",\"properties\":{}}\n\n", id)
	flusher.Flush()

	select {
	case f.subOpened <- id:
	default:
	}

	for {
		select {
		case chunk, open := <-sub:
			if !open {
				return
			}
			_, _ = io.WriteString(w, chunk)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("fake opencode: marshal response: %v", err))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

func writeError(w http.ResponseWriter, status int, name, message string) {
	writeJSON(w, status, map[string]any{
		"name": name,
		"data": map[string]any{"message": message},
	})
}

func decodeJSONBody(r *http.Request, out any) {
	payload, err := io.ReadAll(r.Body)
	if err != nil || len(payload) == 0 {
		return
	}
	_ = json.Unmarshal(payload, out)
}
