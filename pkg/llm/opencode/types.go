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
	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// Types in this file hand-write the wrapped subset of the OpenCode HTTP API.
// Shape ground truth is testdata/openapi.json (captured from a real
// `opencode serve` — see AGENTS.md for the re-capture workflow); decoding is
// tolerant: unknown JSON fields are ignored so newer servers remain readable.

// Health is the GET /global/health response.
type Health struct {
	Healthy bool   `json:"healthy"`
	Version string `json:"version"`
}

// SessionInfo is the wire representation of an OpenCode session
// (component schema "Session").
type SessionInfo struct {
	ID          string           `json:"id"`
	Slug        string           `json:"slug"`
	ProjectID   string           `json:"projectID"`
	WorkspaceID string           `json:"workspaceID,omitzero"`
	Directory   string           `json:"directory"`
	Path        string           `json:"path,omitzero"`
	ParentID    string           `json:"parentID,omitzero"`
	Title       string           `json:"title"`
	Agent       string           `json:"agent,omitzero"`
	Model       *SessionModel    `json:"model,omitzero"`
	Version     string           `json:"version"`
	Metadata    jsontext.Value   `json:"metadata,omitzero"`
	Cost        float64          `json:"cost,omitzero"`
	Tokens      *TokenDetail     `json:"tokens,omitzero"`
	Share       *SessionShare    `json:"share,omitzero"`
	Time        SessionTime      `json:"time"`
	Permission  []PermissionRule `json:"permission,omitzero"`
	Revert      *SessionRevert   `json:"revert,omitzero"`
}

// SessionModel identifies the model a session is pinned to.
type SessionModel struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
	Variant    string `json:"variant,omitzero"`
}

// SessionShare carries the public share URL of a shared session.
type SessionShare struct {
	URL string `json:"url"`
}

// SessionTime carries session lifecycle timestamps (epoch milliseconds).
type SessionTime struct {
	Created    int64 `json:"created"`
	Updated    int64 `json:"updated"`
	Compacting int64 `json:"compacting,omitzero"`
	Archived   int64 `json:"archived,omitzero"`
}

// SessionRevert describes a staged revert point on a session.
type SessionRevert struct {
	MessageID string `json:"messageID"`
	PartID    string `json:"partID,omitzero"`
	Snapshot  string `json:"snapshot,omitzero"`
	Diff      string `json:"diff,omitzero"`
}

// TokenDetail is the token accounting shape shared by sessions and
// assistant messages.
type TokenDetail struct {
	Total     float64    `json:"total,omitzero"`
	Input     float64    `json:"input"`
	Output    float64    `json:"output"`
	Reasoning float64    `json:"reasoning"`
	Cache     TokenCache `json:"cache"`
}

// TokenCache is the cache read/write token accounting pair.
type TokenCache struct {
	Read  float64 `json:"read"`
	Write float64 `json:"write"`
}

// MessageTime carries message lifecycle timestamps (epoch milliseconds).
type MessageTime struct {
	Created   int64 `json:"created"`
	Completed int64 `json:"completed,omitzero"`
}

// MessagePath records the working directory context of an assistant message.
type MessagePath struct {
	Cwd  string `json:"cwd"`
	Root string `json:"root"`
}

// MessageError is a named turn-level error carried on an assistant message or
// a session.error event. Known names on opencode 1.18.x: ProviderAuthError,
// UnknownError, MessageOutputLengthError, MessageAbortedError,
// StructuredOutputError, ContextOverflowError, ContentFilterError, APIError.
type MessageError struct {
	Name string         `json:"name"`
	Data jsontext.Value `json:"data,omitzero"`
}

// Message extracts the human-readable message text from Data, if present.
func (e *MessageError) Message() string {
	if e == nil || len(e.Data) == 0 {
		return ""
	}
	var data struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return ""
	}
	return data.Message
}

// AssistantMessage is the wire representation of an assistant message
// (component schema "AssistantMessage").
type AssistantMessage struct {
	ID         string        `json:"id"`
	SessionID  string        `json:"sessionID"`
	Role       string        `json:"role"`
	ParentID   string        `json:"parentID,omitzero"`
	ModelID    string        `json:"modelID"`
	ProviderID string        `json:"providerID"`
	Mode       string        `json:"mode,omitzero"`
	Agent      string        `json:"agent,omitzero"`
	Path       MessagePath   `json:"path"`
	Summary    bool          `json:"summary,omitzero"`
	Cost       float64       `json:"cost"`
	Tokens     TokenDetail   `json:"tokens"`
	Error      *MessageError `json:"error,omitzero"`
	Variant    string        `json:"variant,omitzero"`
	Finish     string        `json:"finish,omitzero"`
	Time       MessageTime   `json:"time"`
}

// Message is the wire union of user and assistant messages (component schema
// "Message"). Role discriminates: "user" or "assistant". Assistant-only
// fields (ModelID, ProviderID, Path, Cost, Tokens, Error, Finish) and
// user-only fields (Model, System, Tools) are populated per role; the rest
// are shared.
type Message struct {
	ID        string      `json:"id"`
	SessionID string      `json:"sessionID"`
	Role      string      `json:"role"`
	Agent     string      `json:"agent,omitzero"`
	Time      MessageTime `json:"time"`

	// Assistant-only fields.
	ParentID   string        `json:"parentID,omitzero"`
	ModelID    string        `json:"modelID,omitzero"`
	ProviderID string        `json:"providerID,omitzero"`
	Mode       string        `json:"mode,omitzero"`
	Path       *MessagePath  `json:"path,omitzero"`
	Cost       float64       `json:"cost,omitzero"`
	Tokens     *TokenDetail  `json:"tokens,omitzero"`
	Error      *MessageError `json:"error,omitzero"`
	Variant    string        `json:"variant,omitzero"`
	Finish     string        `json:"finish,omitzero"`

	// User-only fields.
	Model  *UserMessageModel `json:"model,omitzero"`
	System string            `json:"system,omitzero"`
	Tools  map[string]bool   `json:"tools,omitzero"`
}

// UserMessageModel is the model reference recorded on a user message.
type UserMessageModel struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
	Variant    string `json:"variant,omitzero"`
}

// MessageWithParts pairs a message with its parts, as returned by
// GET /session/{id}/message and the shell/command endpoints.
type MessageWithParts struct {
	Info  Message `json:"info"`
	Parts []Part  `json:"parts"`
}

// PromptResponse is the POST /session/{id}/message response: the completed
// assistant message and its parts.
type PromptResponse struct {
	Info  AssistantMessage `json:"info"`
	Parts []Part           `json:"parts"`
}

// Part is the wire union of message parts (component schema "Part"; 12 types
// on opencode 1.18.x: text, subtask, reasoning, file, tool, step-start,
// step-finish, snapshot, patch, agent, retry, compaction). Fields are
// populated per type; unknown part types decode Type plus whatever fields
// match.
type Part struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
	MessageID string `json:"messageID"`
	Type      string `json:"type"`

	Text      string `json:"text,omitzero"`      // text, reasoning
	Synthetic bool   `json:"synthetic,omitzero"` // text
	Ignored   bool   `json:"ignored,omitzero"`   // text

	Mime     string `json:"mime,omitzero"`     // file
	Filename string `json:"filename,omitzero"` // file
	URL      string `json:"url,omitzero"`      // file

	Tool   string         `json:"tool,omitzero"`   // tool
	CallID string         `json:"callID,omitzero"` // tool
	State  jsontext.Value `json:"state,omitzero"`  // tool (status/input/output union)

	Name string `json:"name,omitzero"` // agent

	Metadata jsontext.Value `json:"metadata,omitzero"`
}

// PartInput is one input part of a prompt request
// (TextPartInput | FilePartInput | AgentPartInput wire shapes).
// Construct values via the RunInput contract in input.go; Type discriminates.
type PartInput struct {
	ID   string `json:"id,omitzero"`
	Type string `json:"type"`

	Text string `json:"text,omitzero"` // text (required for type "text")

	Mime     string `json:"mime,omitzero"`     // file (required for type "file")
	Filename string `json:"filename,omitzero"` // file
	URL      string `json:"url,omitzero"`      // file (required for type "file")

	Name string `json:"name,omitzero"` // agent (required for type "agent")
}

// ModelRef selects a provider/model pair for prompt-shaped requests.
type ModelRef struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

// PermissionRule is one entry of a session permission ruleset
// (component schema "PermissionRule"). Action is one of "allow", "deny",
// or "ask".
type PermissionRule struct {
	Permission string `json:"permission"`
	Pattern    string `json:"pattern"`
	Action     string `json:"action"`
}

// SessionNewParams is the POST /session request body.
type SessionNewParams struct {
	ParentID   string           `json:"parentID,omitzero"`
	Title      string           `json:"title,omitzero"`
	Agent      string           `json:"agent,omitzero"`
	Model      *SessionModel    `json:"model,omitzero"`
	Permission []PermissionRule `json:"permission,omitzero"`
}

// SessionUpdateParams is the PATCH /session/{id} request body.
type SessionUpdateParams struct {
	Title string `json:"title,omitzero"`
}

// SessionForkParams is the POST /session/{id}/fork request body.
type SessionForkParams struct {
	MessageID string `json:"messageID,omitzero"`
}

// PromptParams is the POST /session/{id}/message (and prompt_async) request
// body. Parts is required; use input.go's normalizeInput to build it from a
// RunInput value.
type PromptParams struct {
	MessageID string          `json:"messageID,omitzero"`
	Model     *ModelRef       `json:"model,omitzero"`
	Agent     string          `json:"agent,omitzero"`
	NoReply   bool            `json:"noReply,omitzero"`
	Tools     map[string]bool `json:"tools,omitzero"`
	System    string          `json:"system,omitzero"`
	Variant   string          `json:"variant,omitzero"`
	Parts     []PartInput     `json:"parts"`
}

// CommandParams is the POST /session/{id}/command request body. Command and
// Arguments are required by the server; Model is a "provider/model" string
// (not a ModelRef — verified against the live OpenAPI document).
type CommandParams struct {
	MessageID string `json:"messageID,omitzero"`
	Agent     string `json:"agent,omitzero"`
	Model     string `json:"model,omitzero"`
	Command   string `json:"command"`
	Arguments string `json:"arguments"`
	Variant   string `json:"variant,omitzero"`
}

// ShellParams is the POST /session/{id}/shell request body. Agent and
// Command are required by the server.
type ShellParams struct {
	MessageID string    `json:"messageID,omitzero"`
	Agent     string    `json:"agent"`
	Model     *ModelRef `json:"model,omitzero"`
	Command   string    `json:"command"`
}

// SummarizeParams is the POST /session/{id}/summarize request body.
// ProviderID and ModelID are required by the server.
type SummarizeParams struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
	Auto       bool   `json:"auto,omitzero"`
}

// RevertParams is the POST /session/{id}/revert request body. MessageID is
// required by the server.
type RevertParams struct {
	MessageID string `json:"messageID"`
	PartID    string `json:"partID,omitzero"`
}

// PermissionResponse is a reply to a permission request: "once", "always",
// or "reject".
type PermissionResponse string

// Permission reply values accepted by POST
// /session/{id}/permissions/{permissionID} and POST /permission/{requestID}/reply.
const (
	PermissionOnce   PermissionResponse = "once"
	PermissionAlways PermissionResponse = "always"
	PermissionReject PermissionResponse = "reject"
)

// ProvidersResponse is the GET /config/providers response.
type ProvidersResponse struct {
	Providers []Provider        `json:"providers"`
	Default   map[string]string `json:"default"`
}

// Provider describes one configured model provider. Models keeps the typed
// subset the wrapper needs; the full upstream Model schema is much larger.
type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Source string           `json:"source"`
	Env    []string         `json:"env,omitzero"`
	Key    string           `json:"key,omitzero"`
	Models map[string]Model `json:"models"`
}

// Model is the typed subset of one provider model entry.
type Model struct {
	ID          string `json:"id"`
	ProviderID  string `json:"providerID"`
	Name        string `json:"name"`
	Family      string `json:"family,omitzero"`
	Status      string `json:"status,omitzero"`
	ReleaseDate string `json:"release_date,omitzero"`
}

// Event is one server-sent event from GET /event. Type discriminates the
// union (89 known types on opencode 1.18.x); Properties preserves the raw
// payload so unknown event types pass through undamaged (the live server
// emits types absent from its own OpenAPI document, e.g. server.heartbeat).
type Event struct {
	ID         string         `json:"id,omitzero"`
	Type       string         `json:"type"`
	Properties jsontext.Value `json:"properties,omitzero"`
}

// Event type constants for the load-bearing subset routed by this package.
const (
	EventTypeServerConnected    = "server.connected"
	EventTypeSessionIdle        = "session.idle"
	EventTypeSessionError       = "session.error"
	EventTypeMessageUpdated     = "message.updated"
	EventTypeMessagePartUpdated = "message.part.updated"
	EventTypeMessagePartDelta   = "message.part.delta"
	EventTypePermissionAsked    = "permission.asked"
	EventTypePermissionV2Asked  = "permission.v2.asked"

	// EventTypeGap is synthesized by this package (never by the server) and
	// delivered to stream consumers after an SSE reconnect: events emitted
	// while the bus was disconnected may have been lost. Properties is empty.
	EventTypeGap = "wrapper.events.gap"
)

// SessionID extracts properties.sessionID, best-effort. It returns "" when
// the event has no properties, no sessionID field, or malformed properties.
func (e Event) SessionID() string {
	if len(e.Properties) == 0 {
		return ""
	}
	var props struct {
		SessionID string `json:"sessionID"`
	}
	if err := json.Unmarshal(e.Properties, &props); err != nil {
		return ""
	}
	return props.SessionID
}

// SessionErrorProperties is the payload of a session.error event. Both
// fields are optional on the wire: a session.error without a SessionID is
// unroutable and is counted, never guessed (see Counters).
type SessionErrorProperties struct {
	SessionID string        `json:"sessionID,omitzero"`
	Error     *MessageError `json:"error,omitzero"`
}

// SessionError decodes a session.error event payload. ok is false when the
// event is not session.error or its properties do not decode.
func (e Event) SessionError() (SessionErrorProperties, bool) {
	if e.Type != EventTypeSessionError {
		return SessionErrorProperties{}, false
	}
	var props SessionErrorProperties
	if err := json.Unmarshal(e.Properties, &props); err != nil {
		return SessionErrorProperties{}, false
	}
	return props, true
}

// MessageUpdatedProperties is the payload of a message.updated event.
type MessageUpdatedProperties struct {
	SessionID string  `json:"sessionID"`
	Info      Message `json:"info"`
}

// MessageUpdated decodes a message.updated event payload. ok is false when
// the event is not message.updated or its properties do not decode.
func (e Event) MessageUpdated() (MessageUpdatedProperties, bool) {
	if e.Type != EventTypeMessageUpdated {
		return MessageUpdatedProperties{}, false
	}
	var props MessageUpdatedProperties
	if err := json.Unmarshal(e.Properties, &props); err != nil {
		return MessageUpdatedProperties{}, false
	}
	return props, true
}

// MessagePartUpdatedProperties is the payload of a message.part.updated event.
type MessagePartUpdatedProperties struct {
	SessionID string `json:"sessionID"`
	Part      Part   `json:"part"`
}

// MessagePartUpdated decodes a message.part.updated event payload. ok is
// false when the event is not message.part.updated or its properties do not
// decode.
func (e Event) MessagePartUpdated() (MessagePartUpdatedProperties, bool) {
	if e.Type != EventTypeMessagePartUpdated {
		return MessagePartUpdatedProperties{}, false
	}
	var props MessagePartUpdatedProperties
	if err := json.Unmarshal(e.Properties, &props); err != nil {
		return MessagePartUpdatedProperties{}, false
	}
	return props, true
}

// PermissionTool identifies the paused tool call behind a permission request.
type PermissionTool struct {
	MessageID string `json:"messageID"`
	CallID    string `json:"callID"`
}

// PermissionAskedProperties is the payload of a permission.asked event
// (legacy shape) or permission.v2.asked (v2 shape: Action/Resources instead
// of Permission/Patterns). ID is the permission id used in the reply URL.
type PermissionAskedProperties struct {
	ID         string          `json:"id"`
	SessionID  string          `json:"sessionID"`
	Permission string          `json:"permission,omitzero"`
	Patterns   []string        `json:"patterns,omitzero"`
	Action     string          `json:"action,omitzero"`
	Resources  []string        `json:"resources,omitzero"`
	Metadata   jsontext.Value  `json:"metadata,omitzero"`
	Always     []string        `json:"always,omitzero"`
	Tool       *PermissionTool `json:"tool,omitzero"`
}

// PermissionAsked decodes a permission.asked or permission.v2.asked event
// payload. ok is false for other event types or undecodable properties.
func (e Event) PermissionAsked() (PermissionAskedProperties, bool) {
	if e.Type != EventTypePermissionAsked && e.Type != EventTypePermissionV2Asked {
		return PermissionAskedProperties{}, false
	}
	var props PermissionAskedProperties
	if err := json.Unmarshal(e.Properties, &props); err != nil {
		return PermissionAskedProperties{}, false
	}
	return props, true
}

// errorEnvelope is the OpenCode HTTP error body: {"name": ..., "data":
// {"message": ...}}.
type errorEnvelope struct {
	Name string `json:"name"`
	Data struct {
		Message string `json:"message"`
	} `json:"data"`
}
