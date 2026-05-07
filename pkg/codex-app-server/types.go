// Copyright 2026 The omxx Authors.
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

package codexappserver

import (
	"github.com/go-json-experiment/json/jsontext"
)

// Value is an arbitrary JSON value exchanged with the Codex app-server.
type Value = any

// Object is a JSON object exchanged with the Codex app-server.
type Object = map[string]any

// Notification is a server notification with its method and raw params.
type Notification struct {
	Method string         `json:"method"`
	Params jsontext.Value `json:"params,omitzero"`
}

// ServerInfo describes the app-server process returned by initialize.
type ServerInfo struct {
	Name    string `json:"name,omitzero"`
	Version string `json:"version,omitzero"`
}

// InitializeResponse is the metadata returned by the app-server initialize method.
type InitializeResponse struct {
	UserAgent  string         `json:"userAgent,omitzero"`
	ServerInfo *ServerInfo    `json:"serverInfo,omitzero"`
	Raw        jsontext.Value `json:",inline"`
}

// ThreadRef is the stable identity of a Codex thread.
type ThreadRef struct {
	ID  string         `json:"id"`
	Raw jsontext.Value `json:",inline"`
}

// TurnStatus is the app-server turn status string.
type TurnStatus string

const (
	// TurnStatusCompleted means the turn completed successfully.
	TurnStatusCompleted TurnStatus = "completed"
	// TurnStatusFailed means the turn failed.
	TurnStatusFailed TurnStatus = "failed"
	// TurnStatusCancelled means the turn was cancelled.
	TurnStatusCancelled TurnStatus = "cancelled"
	// TurnStatusInProgress means the turn is still running.
	TurnStatusInProgress TurnStatus = "inProgress"
)

// TurnError describes a failed turn.
type TurnError struct {
	Message string         `json:"message,omitzero"`
	Raw     jsontext.Value `json:",inline"`
}

// Turn is the stable identity and status of an app-server turn.
type Turn struct {
	ID     string         `json:"id"`
	Status TurnStatus     `json:"status,omitzero"`
	Error  *TurnError     `json:"error,omitzero"`
	Raw    jsontext.Value `json:",inline"`
}

// ThreadItem is a forward-compatible app-server thread item.
type ThreadItem struct {
	Type  string         `json:"type,omitzero"`
	Text  string         `json:"text,omitzero"`
	Phase string         `json:"phase,omitzero"`
	Raw   jsontext.Value `json:",inline"`
}

// TokenUsageBreakdown is one token accounting snapshot.
type TokenUsageBreakdown struct {
	CachedInputTokens     int64 `json:"cachedInputTokens"`
	InputTokens           int64 `json:"inputTokens"`
	OutputTokens          int64 `json:"outputTokens"`
	ReasoningOutputTokens int64 `json:"reasoningOutputTokens"`
	TotalTokens           int64 `json:"totalTokens"`
}

// ThreadTokenUsage contains last and cumulative token usage for a thread.
type ThreadTokenUsage struct {
	Last               TokenUsageBreakdown `json:"last"`
	Total              TokenUsageBreakdown `json:"total"`
	ModelContextWindow *int64              `json:"modelContextWindow,omitzero"`
}

// ThreadStartResponse is returned by thread/start.
type ThreadStartResponse struct {
	Thread ThreadRef      `json:"thread"`
	Raw    jsontext.Value `json:",inline"`
}

// ThreadResumeResponse is returned by thread/resume.
type ThreadResumeResponse = ThreadStartResponse

// ThreadForkResponse is returned by thread/fork.
type ThreadForkResponse = ThreadStartResponse

// ThreadUnarchiveResponse is returned by thread/unarchive.
type ThreadUnarchiveResponse = ThreadStartResponse

// ThreadArchiveResponse is returned by thread/archive.
type ThreadArchiveResponse struct {
	Raw jsontext.Value `json:",inline"`
}

// ThreadSetNameResponse is returned by thread/name/set.
type ThreadSetNameResponse struct {
	Raw jsontext.Value `json:",inline"`
}

// ThreadCompactStartResponse is returned by thread/compact/start.
type ThreadCompactStartResponse struct {
	Raw jsontext.Value `json:",inline"`
}

// ThreadReadResponse is returned by thread/read.
type ThreadReadResponse struct {
	Thread ThreadRef      `json:"thread"`
	Raw    jsontext.Value `json:",inline"`
}

// TurnStartResponse is returned by turn/start.
type TurnStartResponse struct {
	Turn Turn           `json:"turn"`
	Raw  jsontext.Value `json:",inline"`
}

// TurnSteerResponse is returned by turn/steer.
type TurnSteerResponse struct {
	Raw jsontext.Value `json:",inline"`
}

// TurnInterruptResponse is returned by turn/interrupt.
type TurnInterruptResponse struct {
	Raw jsontext.Value `json:",inline"`
}

// TurnCompletedNotification is the typed payload for turn/completed.
type TurnCompletedNotification struct {
	Turn Turn           `json:"turn"`
	Raw  jsontext.Value `json:",inline"`
}

// ItemCompletedNotification is the typed payload for item/completed.
type ItemCompletedNotification struct {
	ThreadID string         `json:"threadId,omitzero"`
	TurnID   string         `json:"turnId"`
	Item     ThreadItem     `json:"item"`
	Raw      jsontext.Value `json:",inline"`
}

// ThreadTokenUsageUpdatedNotification is the typed payload for thread/tokenUsage/updated.
type ThreadTokenUsageUpdatedNotification struct {
	ThreadID   string           `json:"threadId"`
	TurnID     string           `json:"turnId"`
	TokenUsage ThreadTokenUsage `json:"tokenUsage"`
	Raw        jsontext.Value   `json:",inline"`
}

// AgentMessageDeltaNotification is the typed payload for item/agentMessage/delta.
type AgentMessageDeltaNotification struct {
	ThreadID string         `json:"threadId,omitzero"`
	TurnID   string         `json:"turnId"`
	Delta    string         `json:"delta,omitzero"`
	Raw      jsontext.Value `json:",inline"`
}

// ModelListResponse is returned by model/list.
type ModelListResponse struct {
	Data []Model        `json:"data"`
	Raw  jsontext.Value `json:",inline"`
}

// Model describes a selectable Codex model.
type Model struct {
	ID                        string                  `json:"id"`
	Model                     string                  `json:"model"`
	DisplayName               string                  `json:"displayName"`
	Description               string                  `json:"description"`
	Hidden                    bool                    `json:"hidden"`
	IsDefault                 bool                    `json:"isDefault"`
	DefaultReasoningEffort    string                  `json:"defaultReasoningEffort"`
	SupportedReasoningEfforts []ReasoningEffortOption `json:"supportedReasoningEfforts"`
	Upgrade                   *string                 `json:"upgrade,omitzero"`
	Raw                       jsontext.Value          `json:",inline"`
}

// ReasoningEffortOption describes one supported effort for a model.
type ReasoningEffortOption struct {
	ReasoningEffort string         `json:"reasoningEffort"`
	Raw             jsontext.Value `json:",inline"`
}

// ThreadStartParams are params for thread/start.
type ThreadStartParams struct {
	ApprovalPolicy        any    `json:"approvalPolicy,omitzero"`
	ApprovalsReviewer     string `json:"approvalsReviewer,omitzero"`
	BaseInstructions      string `json:"baseInstructions,omitzero"`
	Config                Object `json:"config,omitzero"`
	Cwd                   string `json:"cwd,omitzero"`
	DeveloperInstructions string `json:"developerInstructions,omitzero"`
	Ephemeral             *bool  `json:"ephemeral,omitzero"`
	Model                 string `json:"model,omitzero"`
	ModelProvider         string `json:"modelProvider,omitzero"`
	Personality           string `json:"personality,omitzero"`
	Sandbox               any    `json:"sandbox,omitzero"`
	ServiceName           string `json:"serviceName,omitzero"`
	ServiceTier           string `json:"serviceTier,omitzero"`
	SessionStartSource    string `json:"sessionStartSource,omitzero"`
}

// ThreadResumeParams are params for thread/resume.
type ThreadResumeParams struct {
	ApprovalPolicy        any    `json:"approvalPolicy,omitzero"`
	ApprovalsReviewer     string `json:"approvalsReviewer,omitzero"`
	BaseInstructions      string `json:"baseInstructions,omitzero"`
	Config                Object `json:"config,omitzero"`
	Cwd                   string `json:"cwd,omitzero"`
	DeveloperInstructions string `json:"developerInstructions,omitzero"`
	Model                 string `json:"model,omitzero"`
	ModelProvider         string `json:"modelProvider,omitzero"`
	Personality           string `json:"personality,omitzero"`
	Sandbox               any    `json:"sandbox,omitzero"`
	ServiceTier           string `json:"serviceTier,omitzero"`
}

// ThreadForkParams are params for thread/fork.
type ThreadForkParams struct {
	ThreadResumeParams
	Ephemeral *bool `json:"ephemeral,omitzero"`
}

// ThreadListParams are params for thread/list.
type ThreadListParams struct {
	Archived       *bool    `json:"archived,omitzero"`
	Cursor         string   `json:"cursor,omitzero"`
	Cwd            any      `json:"cwd,omitzero"`
	Limit          *int     `json:"limit,omitzero"`
	ModelProviders []string `json:"modelProviders,omitzero"`
	SearchTerm     string   `json:"searchTerm,omitzero"`
	SortDirection  string   `json:"sortDirection,omitzero"`
	SortKey        string   `json:"sortKey,omitzero"`
	SourceKinds    []string `json:"sourceKinds,omitzero"`
	UseStateDBOnly *bool    `json:"useStateDbOnly,omitzero"`
}

// ThreadListResponse is returned by thread/list.
type ThreadListResponse struct {
	Data []ThreadRef    `json:"data"`
	Raw  jsontext.Value `json:",inline"`
}

// TurnStartParams are optional params for turn/start.
type TurnStartParams struct {
	ApprovalPolicy    any    `json:"approvalPolicy,omitzero"`
	ApprovalsReviewer string `json:"approvalsReviewer,omitzero"`
	Cwd               string `json:"cwd,omitzero"`
	Effort            string `json:"effort,omitzero"`
	Model             string `json:"model,omitzero"`
	OutputSchema      any    `json:"outputSchema,omitzero"`
	Personality       string `json:"personality,omitzero"`
	SandboxPolicy     any    `json:"sandboxPolicy,omitzero"`
	ServiceTier       string `json:"serviceTier,omitzero"`
	Summary           string `json:"summary,omitzero"`
}
