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

package claude

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/go-json-experiment/json/jsontext"
)

// Hook is a callback invoked by the SDK at specific lifecycle events during a
// claude CLI session. The event Kind identifies which lifecycle point fired.
//
// Return a zero [HookDecision] and nil error to let the CLI proceed unchanged.
// Return a non-zero [HookDecision] to inject system messages, override
// permission decisions, or supply additional context.
//
// The hook dispatcher invokes registered hooks in registration order and stops
// at the first [PermissionDeny] decision for tool-use events.
//
// Hooks must not block indefinitely; respect ctx.Done() for cancellation.
type Hook func(ctx context.Context, event HookEvent) (HookDecision, error)

// HookRegistration pairs a [Hook] function with an event kind filter and an
// optional tool-name glob matcher. Registered via [Options].Hooks.
//
// ToolGlob is matched against [HookEvent].ToolName using filepath.Match
// semantics. An empty ToolGlob matches all tool names for PreToolUse and
// PostToolUse events.
type HookRegistration struct {
	// Kind is the event kind this registration responds to.
	Kind HookEventKind

	// ToolGlob is an optional glob matched against the tool name for PreToolUse
	// and PostToolUse events. Empty means "match all tools".
	ToolGlob string

	// Fn is the hook callback to invoke when Kind and ToolGlob match.
	Fn Hook
}

// CanUseTool is a permission callback invoked before every tool call. It
// receives the tool name and raw JSON-encoded input, and returns a
// [PermissionDecision] with an optional error.
//
// Return [PermissionAllow] to permit the call, [PermissionDeny] to block it,
// or [PermissionAsk] (the zero value) to fall through to the CLI's configured
// permission_mode.
//
// The callback must not block indefinitely; respect ctx.Done().
type CanUseTool func(ctx context.Context, toolName string, input jsontext.Value) (PermissionDecision, error)

// ── dispatcher ───────────────────────────────────────────────────────────────

// dispatchHooks invokes every [HookRegistration] in regs whose Kind matches
// event.Kind (and whose ToolGlob, if set, matches event.ToolName). Hooks are
// called in registration order. The resulting [HookDecision] values are merged:
// SystemMessage and AdditionalContext fields are concatenated with "\n", and
// the first [PermissionDeny] short-circuits further hook invocations.
//
// An invalid ToolGlob pattern returns a [CLIConnectionError].
func dispatchHooks(ctx context.Context, regs []HookRegistration, event HookEvent) (HookDecision, error) {
	var merged HookDecision
	for _, reg := range regs {
		if reg.Kind != event.Kind {
			continue
		}
		if reg.ToolGlob != "" {
			ok, err := filepath.Match(reg.ToolGlob, event.ToolName)
			if err != nil {
				return HookDecision{}, &CLIConnectionError{
					Message: fmt.Sprintf("invalid hook ToolGlob %q: %v", reg.ToolGlob, err),
				}
			}
			if !ok {
				continue
			}
		}
		if reg.Fn == nil {
			continue
		}
		decision, err := reg.Fn(ctx, event)
		if err != nil {
			return HookDecision{}, err
		}
		// Merge system messages.
		if decision.SystemMessage != "" {
			if merged.SystemMessage != "" {
				merged.SystemMessage += "\n" + decision.SystemMessage
			} else {
				merged.SystemMessage = decision.SystemMessage
			}
		}
		// Merge additional context.
		if decision.AdditionalContext != "" {
			if merged.AdditionalContext != "" {
				merged.AdditionalContext += "\n" + decision.AdditionalContext
			} else {
				merged.AdditionalContext = decision.AdditionalContext
			}
		}
		// Propagate permission decision; deny is sticky and stops iteration.
		if decision.HookSpecificOutput.PermissionDecision != PermissionAsk {
			merged.HookSpecificOutput.PermissionDecision = decision.HookSpecificOutput.PermissionDecision
			if decision.HookSpecificOutput.PermissionDecisionReason != "" {
				merged.HookSpecificOutput.PermissionDecisionReason = decision.HookSpecificOutput.PermissionDecisionReason
			}
		}
		if merged.HookSpecificOutput.PermissionDecision == PermissionDeny {
			return merged, nil
		}
	}
	return merged, nil
}

// applyCanUseTool wraps the [CanUseTool] callback as a [HookDecision]. It
// returns a zero decision when fn is nil or event is not [HookEventPreToolUse].
func applyCanUseTool(ctx context.Context, fn CanUseTool, event HookEvent) (HookDecision, error) {
	if fn == nil || event.Kind != HookEventPreToolUse {
		return HookDecision{}, nil
	}
	perm, err := fn(ctx, event.ToolName, event.ToolInput)
	if err != nil {
		return HookDecision{}, err
	}
	return HookDecision{
		HookSpecificOutput: HookSpecificOutput{
			PermissionDecision: perm,
		},
	}, nil
}

// applyPermissions combines [dispatchHooks] and [applyCanUseTool] into a
// single permission decision for the given event. Hook registrations are
// evaluated first; if any produces [PermissionDeny] the CanUseTool callback
// is skipped. Otherwise, the CanUseTool decision is merged: a deny overrides
// any prior allow.
//
// A nil opts is treated as no registrations and no callback (always allow).
func applyPermissions(ctx context.Context, opts *Options, event HookEvent) (HookDecision, error) {
	if opts == nil {
		return HookDecision{}, nil
	}
	hookDec, err := dispatchHooks(ctx, opts.Hooks, event)
	if err != nil {
		return HookDecision{}, err
	}
	if hookDec.HookSpecificOutput.PermissionDecision == PermissionDeny {
		return hookDec, nil
	}
	canUseDec, err := applyCanUseTool(ctx, opts.CanUseTool, event)
	if err != nil {
		return HookDecision{}, err
	}
	if canUseDec.HookSpecificOutput.PermissionDecision == PermissionDeny {
		hookDec.HookSpecificOutput.PermissionDecision = PermissionDeny
		if canUseDec.HookSpecificOutput.PermissionDecisionReason != "" {
			hookDec.HookSpecificOutput.PermissionDecisionReason = canUseDec.HookSpecificOutput.PermissionDecisionReason
		}
	} else if canUseDec.HookSpecificOutput.PermissionDecision != PermissionAsk {
		hookDec.HookSpecificOutput.PermissionDecision = canUseDec.HookSpecificOutput.PermissionDecision
	}
	return hookDec, nil
}
