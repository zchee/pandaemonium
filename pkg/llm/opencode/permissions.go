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
	"context"
)

// respondPermission implements the v1 always-respond permission policy. It
// runs for every permission.asked / permission.v2.asked event routed by the
// client-lifetime bus — for sync and async turns alike, because a
// permission-gated tool call pauses the server-side turn (and therefore the
// blocking prompt request) until somebody replies. The policy: reply "once"
// when Config.PermissionAuto, "reject" otherwise; the wrapper never leaves a
// permission request unanswered. Replies are counted in Counters
// (PermissionsAutoApproved / PermissionsRejected).
//
// The event-driven PermissionHandler (interactive approvals) is a deferred
// follow-up; it will replace this fixed policy.
func (c *Client) respondPermission(ev Event) {
	props, ok := ev.PermissionAsked()
	if !ok || props.ID == "" {
		return
	}

	response := PermissionReject
	if c.config.PermissionAuto {
		response = PermissionOnce
	}

	_ = c.goWork(func() {
		ctx, cancel := context.WithTimeout(c.lifetime, c.config.DialTimeout)
		defer cancel()

		var err error
		if ev.Type == EventTypePermissionV2Asked {
			_, err = c.PermissionReplyV2(ctx, props.ID, response)
		} else {
			_, err = c.PermissionRespond(ctx, props.SessionID, props.ID, response)
		}
		if err != nil {
			// The reply failed (server gone, request already replied, ...).
			// The turn either proceeds via another replier or surfaces its own
			// failure; nothing to propagate from this consumer.
			return
		}
		if response == PermissionOnce {
			c.counters.permissionsAutoApproved.Add(1)
		} else {
			c.counters.permissionsRejected.Add(1)
		}
	})
}
