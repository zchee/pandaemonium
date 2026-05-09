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
