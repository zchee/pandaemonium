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

package codex

import (
	"fmt"
)

// ApprovalMode is a high-level approval preset compatible with the Python SDK.
type ApprovalMode string

const (
	// ApprovalModeDenyAll denies all requests without routing to a reviewer.
	ApprovalModeDenyAll ApprovalMode = "deny_all"
	// ApprovalModeAutoReview routes on-request approvals to the auto-reviewer.
	ApprovalModeAutoReview ApprovalMode = "auto_review"
)

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
	switch value := value.(type) {
	case AskForApprovalValue:
		return value, nil
	case GranularAskForApproval:
		return value, nil
	case RawAskForApproval:
		return value, nil
	case AskForApproval:
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported AskForApproval value %T", value)
	}
}
