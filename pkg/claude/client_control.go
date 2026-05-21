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
	"time"

	"github.com/go-json-experiment/json/jsontext"
)

// defaultControlTimeout bounds how long an outbound control request waits for
// its response, matching upstream's _send_control_request default of 60s.
const defaultControlTimeout = 60 * time.Second

// controlRequest snapshots the control protocol under closeMu and sends an
// outbound control_request{subtype} carrying payload, blocking until the
// response arrives, ctx is cancelled, or the effective timeout elapses.
//
// The effective timeout is the larger of [defaultControlTimeout] and the time
// remaining on ctx's deadline: a caller that sets a deadline beyond 60s gets
// that longer window, while a shorter ctx deadline (or cancellation) still
// aborts the wait early. A ctx with no deadline uses the 60s default.
//
// The snapshot is the same race-safety discipline used for the transport: the
// returned controlProtocol outlives the lock, but its sends route through
// writeMessage (writeMu-symmetric with Close's transport clear), so a Close
// racing this call surfaces as a CLIConnectionError rather than a data race.
// A nil control protocol (client never started, or already closed) is reported
// as a not-running error.
func (c *ClaudeSDKClient) controlRequest(ctx context.Context, subtype string, payload map[string]any) (jsontext.Value, error) {
	c.closeMu.Lock()
	cp := c.cp
	c.closeMu.Unlock()
	if cp == nil {
		return nil, &CLIConnectionError{Message: "CLI is not running"}
	}
	timeout := defaultControlTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > timeout {
			timeout = remaining
		}
	}
	return cp.sendControlRequest(ctx, subtype, payload, timeout)
}

// Interrupt asks the CLI to cancel the current operation by sending a
// control_request{subtype:"interrupt"} and waiting for its acknowledgement.
//
// This is the control-protocol interrupt, not an OS signal: it requires the
// initialize handshake to have completed so the CLI is reading control
// requests. It returns a [CLIConnectionError] if the client is not running, the
// CLI replies with an error, or the request times out.
//
// Behavior change: earlier revisions sent an OS SIGINT to the subprocess, which
// worked before any [ClaudeSDKClient.Query] and returned immediately. Interrupt
// now requires an active, initialized session and blocks for the CLI's
// acknowledgement (bounded by ctx; see [controlRequest]). Callers that relied
// on the fire-and-forget signal must now hold a started client and handle the
// returned error.
func (c *ClaudeSDKClient) Interrupt(ctx context.Context) error {
	_, err := c.controlRequest(ctx, "interrupt", nil)
	return err
}

// SetModel changes the model the CLI uses for subsequent turns. An empty model
// resets the CLI to its configured default (sent as JSON null, matching
// upstream set_model(None)).
func (c *ClaudeSDKClient) SetModel(ctx context.Context, model string) error {
	var modelValue any
	if model != "" {
		modelValue = model
	}
	_, err := c.controlRequest(ctx, "set_model", map[string]any{"model": modelValue})
	return err
}

// SetPermissionMode changes the CLI's permission mode for subsequent tool
// calls. Pass one of the [PermissionMode] constants (e.g.
// [PermissionModeAcceptEdits], [PermissionModePlan]); the value is sent
// verbatim and the CLI validates it.
func (c *ClaudeSDKClient) SetPermissionMode(ctx context.Context, mode PermissionMode) error {
	_, err := c.controlRequest(ctx, "set_permission_mode", map[string]any{"mode": string(mode)})
	return err
}

// GetServerInfo returns the CLI's initialize response as raw JSON: the object
// the CLI sent in reply to the initialize handshake, carrying fields such as
// the available slash commands and the active output style. The shape is not
// modelled as a Go type (forward compatibility); decode it into your own shape.
//
// It returns the cached result from the initialize handshake without issuing a
// new control request, and a [CLIConnectionError] if initialize has not
// completed (the client has not been started or has been closed).
func (c *ClaudeSDKClient) GetServerInfo(ctx context.Context) (jsontext.Value, error) {
	_ = ctx // No request is sent; the result is cached from initialize.
	c.closeMu.Lock()
	cp := c.cp
	c.closeMu.Unlock()
	if cp == nil {
		return nil, &CLIConnectionError{Message: "CLI is not running"}
	}
	return cp.serverInfoResult()
}

// GetMCPStatus returns the CLI's MCP server connection status as raw JSON.
// The shape is not modelled as a Go type (forward compatibility); decode it
// into your own shape. Returns a [CLIConnectionError] on a not-running client,
// a CLI error response, or timeout.
func (c *ClaudeSDKClient) GetMCPStatus(ctx context.Context) (jsontext.Value, error) {
	return c.controlRequest(ctx, "mcp_status", nil)
}

// GetContextUsage returns a breakdown of the current context-window usage by
// category as raw JSON. The shape is not modelled as a Go type (forward
// compatibility); decode it into your own shape. Returns a [CLIConnectionError]
// on a not-running client, a CLI error response, or timeout.
func (c *ClaudeSDKClient) GetContextUsage(ctx context.Context) (jsontext.Value, error) {
	return c.controlRequest(ctx, "get_context_usage", nil)
}
