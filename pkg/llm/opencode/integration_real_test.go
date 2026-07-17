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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-json-experiment/json"
)

// Real-server integration (AC9), gated behind RUN_REAL_OPENCODE_TESTS=1.
// Hermetic evidence alone never satisfies completion (plan pre-mortem 3):
// these tests exercise a real `opencode serve` on loopback, including the
// OpenAPI drift canary against testdata/openapi.json.

func realOpencodeConfig(t *testing.T) *Config {
	t.Helper()
	if os.Getenv("RUN_REAL_OPENCODE_TESTS") != "1" {
		t.Skip("set RUN_REAL_OPENCODE_TESTS=1 to run against a real opencode serve")
	}
	bin, err := exec.LookPath("opencode")
	if err != nil {
		t.Fatalf("RUN_REAL_OPENCODE_TESTS=1 but no opencode binary on PATH: %v", err)
	}
	return &Config{
		OpencodeBin: bin,
		// Isolate from user config: a config file's server.port would
		// override --port 0 and its permission rules would change probe
		// behavior. Credentials live in the separate auth store and remain
		// available.
		Env:         map[string]string{"OPENCODE_CONFIG": "/dev/null"},
		DialTimeout: 60 * time.Second,
		DrainWindow: 3 * time.Second,
	}
}

// realModel picks a provider/model pair from the server's defaults,
// preferring github-copilot (subscription-backed) when configured.
func realModel(t *testing.T, oc *Opencode) *ModelRef {
	t.Helper()
	providers, err := oc.Providers(t.Context())
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	if model, ok := providers.Default["github-copilot"]; ok {
		return &ModelRef{ProviderID: "github-copilot", ModelID: model}
	}
	for providerID, model := range providers.Default {
		return &ModelRef{ProviderID: providerID, ModelID: model}
	}
	t.Skip("no configured providers; cannot run a real turn")
	return nil
}

func TestRealOpencodeIntegration(t *testing.T) {
	oc, err := NewOpencode(t.Context(), realOpencodeConfig(t))
	if err != nil {
		t.Fatalf("NewOpencode (real): %v", err)
	}
	defer func() {
		if err := oc.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	health, err := oc.Health(t.Context())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !health.Healthy || health.Version == "" {
		t.Fatalf("health = %+v", health)
	}
	t.Logf("real opencode version: %s", health.Version)

	model := realModel(t, oc)
	t.Logf("using model %s/%s", model.ProviderID, model.ModelID)

	t.Run("turn stream and run result", func(t *testing.T) {
		session, err := oc.SessionStart(t.Context(), &SessionNewParams{Title: "pandaemonium AC9"})
		if err != nil {
			t.Fatalf("SessionStart: %v", err)
		}
		defer func() { _, _ = oc.SessionDelete(t.Context(), session.ID()) }()

		handle, err := session.Turn(t.Context(), "Reply with exactly one word: pong", &PromptParams{Model: model})
		if err != nil {
			t.Fatalf("Turn: %v", err)
		}

		eventTypes := map[string]int{}
		var streamErr error
		for ev, err := range handle.Stream(t.Context()) {
			if err != nil {
				streamErr = err
				break
			}
			eventTypes[ev.Type]++
		}
		if streamErr != nil {
			t.Fatalf("stream error against real server: %v", streamErr)
		}
		if eventTypes[EventTypeMessageUpdated] == 0 {
			t.Errorf("no message.updated observed; events: %v", eventTypes)
		}
		if eventTypes[EventTypeMessagePartUpdated]+eventTypes[EventTypeMessagePartDelta] == 0 {
			t.Errorf("no part progress events observed; events: %v", eventTypes)
		}
		t.Logf("streamed event types: %v", eventTypes)

		read, err := session.Read(t.Context())
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		var finalText string
		for _, message := range read.Messages {
			if message.Info.Role == "assistant" {
				if text := finalTextResponse(message.Parts); text != "" {
					finalText = text
				}
			}
		}
		if !strings.Contains(strings.ToLower(finalText), "pong") {
			t.Errorf("assistant text %q does not contain pong", finalText)
		}
	})

	t.Run("sync run aggregates usage", func(t *testing.T) {
		session, err := oc.SessionStart(t.Context(), &SessionNewParams{Title: "pandaemonium AC9 sync"})
		if err != nil {
			t.Fatalf("SessionStart: %v", err)
		}
		defer func() { _, _ = oc.SessionDelete(t.Context(), session.ID()) }()

		result, err := session.Run(t.Context(), "Reply with exactly one word: pong", &PromptParams{Model: model})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if result.FinalResponse == "" {
			t.Error("FinalResponse empty from real turn")
		}
		if result.Usage == nil || result.Usage.Output == 0 {
			t.Errorf("usage not aggregated: %+v", result.Usage)
		}
	})

	t.Run("interrupt aborts a real turn", func(t *testing.T) {
		session, err := oc.SessionStart(t.Context(), &SessionNewParams{Title: "pandaemonium AC9 interrupt"})
		if err != nil {
			t.Fatalf("SessionStart: %v", err)
		}
		defer func() { _, _ = oc.SessionDelete(t.Context(), session.ID()) }()

		handle, err := session.Turn(t.Context(),
			"Count from 1 to 500, one number per line. Do not stop early.",
			&PromptParams{Model: model})
		if err != nil {
			t.Fatalf("Turn: %v", err)
		}
		// Let generation begin before interrupting.
		time.Sleep(3 * time.Second)
		if _, err := handle.Interrupt(t.Context()); err != nil {
			t.Fatalf("Interrupt: %v", err)
		}

		_, err = handle.Run(t.Context())
		if _, ok := errors.AsType[*MessageAbortedError](err); !ok {
			t.Fatalf("Run after Interrupt = %v (%T), want MessageAbortedError", err, err)
		}
	})
}

// wrappedEndpoints is the REST subset this package depends on: the drift
// canary asserts each one still exists with the same required request
// fields as the committed snapshot.
var wrappedEndpoints = map[string][]string{
	"/session":                                        {"get", "post"},
	"/session/{sessionID}":                            {"get", "delete", "patch"},
	"/session/{sessionID}/message":                    {"get", "post"},
	"/session/{sessionID}/fork":                       {"post"},
	"/session/{sessionID}/abort":                      {"post"},
	"/session/{sessionID}/summarize":                  {"post"},
	"/session/{sessionID}/revert":                     {"post"},
	"/session/{sessionID}/unrevert":                   {"post"},
	"/session/{sessionID}/share":                      {"post", "delete"},
	"/session/{sessionID}/shell":                      {"post"},
	"/session/{sessionID}/command":                    {"post"},
	"/session/{sessionID}/permissions/{permissionID}": {"post"},
	"/config/providers":                               {"get"},
	"/global/health":                                  {"get"},
	"/event":                                          {"get"},
}

// openAPIDoc is the minimal OpenAPI slice the canary compares.
type openAPIDoc struct {
	Paths map[string]map[string]struct {
		RequestBody struct {
			Content map[string]struct {
				Schema struct {
					Required []string `json:"required"`
				} `json:"schema"`
			} `json:"content"`
		} `json:"requestBody"`
	} `json:"paths"`
}

func decodeOpenAPI(t *testing.T, payload []byte, source string) openAPIDoc {
	t.Helper()
	var doc openAPIDoc
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatalf("decode OpenAPI document from %s: %v", source, err)
	}
	if len(doc.Paths) == 0 {
		t.Fatalf("OpenAPI document from %s has no paths", source)
	}
	return doc
}

// TestRealOpencodeOpenAPIDriftCanary fetches GET /doc from a real server and
// asserts the wrapped endpoints and their required request fields still
// match testdata/openapi.json, naming every drifted path.
func TestRealOpencodeOpenAPIDriftCanary(t *testing.T) {
	oc, err := NewOpencode(t.Context(), realOpencodeConfig(t))
	if err != nil {
		t.Fatalf("NewOpencode (real): %v", err)
	}
	defer oc.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, oc.Client().BaseURL()+"/doc", nil)
	if err != nil {
		t.Fatalf("build /doc request: %v", err)
	}
	resp, err := oc.Client().httpClient.Do(req)
	if err != nil {
		t.Fatalf("GET /doc: %v", err)
	}
	defer resp.Body.Close()
	livePayload, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /doc: status %d, err %v", resp.StatusCode, err)
	}

	snapshotPayload, err := os.ReadFile("testdata/openapi.json")
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	live := decodeOpenAPI(t, livePayload, "live server")
	snapshot := decodeOpenAPI(t, snapshotPayload, "testdata/openapi.json")

	var drifted []string
	for path, methods := range wrappedEndpoints {
		livePath, liveOK := live.Paths[path]
		snapPath, snapOK := snapshot.Paths[path]
		if !liveOK || !snapOK {
			drifted = append(drifted, fmt.Sprintf("%s (present: live=%t snapshot=%t)", path, liveOK, snapOK))
			continue
		}
		for _, method := range methods {
			liveOp, liveOK := livePath[method]
			snapOp, snapOK := snapPath[method]
			if !liveOK || !snapOK {
				drifted = append(drifted, fmt.Sprintf("%s %s (present: live=%t snapshot=%t)", method, path, liveOK, snapOK))
				continue
			}
			liveRequired := requiredRequestFields(liveOp.RequestBody.Content)
			snapRequired := requiredRequestFields(snapOp.RequestBody.Content)
			if !slices.Equal(liveRequired, snapRequired) {
				drifted = append(drifted, fmt.Sprintf("%s %s required fields: live=%v snapshot=%v", method, path, liveRequired, snapRequired))
			}
		}
	}
	if len(drifted) > 0 {
		t.Fatalf("OpenAPI drift detected — re-capture testdata/openapi.json and re-verify types.go:\n%s",
			strings.Join(drifted, "\n"))
	}
}

func requiredRequestFields(content map[string]struct {
	Schema struct {
		Required []string `json:"required"`
	} `json:"schema"`
},
) []string {
	body, ok := content["application/json"]
	if !ok {
		return nil
	}
	required := slices.Clone(body.Schema.Required)
	slices.Sort(required)
	return required
}
