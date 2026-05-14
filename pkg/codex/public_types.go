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

package codexappserver

import (
	"fmt"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// Indice preserves the Python root-model name for fuzzy-search indices.
type Indice = int32

// AskForApprovalValue is a Python-parity enum arm for AskForApproval.
type AskForApprovalValue string

const (
	AskForApprovalValueUntrusted AskForApprovalValue = "untrusted"
	AskForApprovalValueOnFailure AskForApprovalValue = "on-failure"
	AskForApprovalValueOnRequest AskForApprovalValue = "on-request"
	AskForApprovalValueNever     AskForApprovalValue = "never"
)

// ApprovalMode is a high-level approval preset compatible with the Python SDK.
type ApprovalMode string

const (
	// ApprovalModeDenyAll denies all requests without routing to a reviewer.
	ApprovalModeDenyAll ApprovalMode = "deny_all"
	// ApprovalModeAutoReview routes on-request approvals to the auto-reviewer.
	ApprovalModeAutoReview ApprovalMode = "auto_review"
)

// Granular is the structured Python-parity arm for AskForApproval.
type Granular struct {
	MCPElicitations    bool  `json:"mcp_elicitations"`
	RequestPermissions *bool `json:"request_permissions,omitzero"`
	Rules              bool  `json:"rules"`
	SandboxApproval    bool  `json:"sandbox_approval"`
	SkillApproval      *bool `json:"skill_approval,omitzero"`
}

// GranularAskForApproval wraps granular approval settings for AskForApproval.
type GranularAskForApproval struct {
	Granular Granular `json:"granular"`
}

// NewAskForApprovalValue creates an AskForApproval root value from an enum arm.
func NewAskForApprovalValue(value AskForApprovalValue) (AskForApproval, error) {
	return newAskForApproval(value)
}

// NewGranularAskForApproval creates an AskForApproval root value from settings.
func NewGranularAskForApproval(value GranularAskForApproval) (AskForApproval, error) {
	return newAskForApproval(value)
}

// ApprovalModeSettings maps a high-level approval preset to protocol settings.
func ApprovalModeSettings(mode ApprovalMode) (AskForApproval, *ApprovalsReviewer, error) {
	switch mode {
	case ApprovalModeDenyAll:
		approval, err := NewAskForApprovalValue(AskForApprovalValueNever)
		if err != nil {
			return nil, nil, err
		}
		return approval, nil, nil
	case ApprovalModeAutoReview:
		approval, err := NewAskForApprovalValue(AskForApprovalValueOnRequest)
		if err != nil {
			return nil, nil, err
		}
		reviewer := ApprovalsReviewerAutoReview
		return approval, &reviewer, nil
	default:
		return nil, nil, fmt.Errorf("unsupported ApprovalMode %q", mode)
	}
}

// ApprovalModeOverrideSettings maps an optional approval preset to pointer settings.
func ApprovalModeOverrideSettings(mode *ApprovalMode) (*AskForApproval, *ApprovalsReviewer, error) {
	if mode == nil {
		return nil, nil, nil
	}
	approval, reviewer, err := ApprovalModeSettings(*mode)
	if err != nil {
		return nil, nil, err
	}
	return &approval, reviewer, nil
}

func newAskForApproval(value any) (AskForApproval, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal AskForApproval: %w", err)
	}
	return AskForApproval(jsontext.Value(raw)), nil
}
