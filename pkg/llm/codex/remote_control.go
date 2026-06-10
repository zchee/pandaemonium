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

import "context"

// RemoteControl is the high-level SDK handle for app-server remote-control
// management requests.
type RemoteControl struct {
	client *Client
}

// RemoteControl returns a high-level remote-control handle.
func (c *Codex) RemoteControl() *RemoteControl {
	if c == nil {
		return &RemoteControl{}
	}
	return c.client.RemoteControl()
}

// RemoteControl returns a high-level remote-control handle.
func (c *Client) RemoteControl() *RemoteControl {
	return &RemoteControl{client: c}
}

// Enable calls remoteControl/enable.
func (r *RemoteControl) Enable(ctx context.Context) (RemoteControlEnableResponse, error) {
	if r == nil || r.client == nil {
		return RemoteControlEnableResponse{}, errClientNil
	}
	return r.client.RemoteControlEnable(ctx)
}

// Disable calls remoteControl/disable.
func (r *RemoteControl) Disable(ctx context.Context) (RemoteControlDisableResponse, error) {
	if r == nil || r.client == nil {
		return RemoteControlDisableResponse{}, errClientNil
	}
	return r.client.RemoteControlDisable(ctx)
}

// Status calls remoteControl/status/read.
func (r *RemoteControl) Status(ctx context.Context) (RemoteControlStatusReadResponse, error) {
	if r == nil || r.client == nil {
		return RemoteControlStatusReadResponse{}, errClientNil
	}
	return r.client.RemoteControlStatusRead(ctx)
}

// PairingStart calls remoteControl/pairing/start.
func (r *RemoteControl) PairingStart(ctx context.Context, params *RemoteControlPairingStartParams) (RemoteControlPairingStartResponse, error) {
	if r == nil || r.client == nil {
		return RemoteControlPairingStartResponse{}, errClientNil
	}
	return r.client.RemoteControlPairingStart(ctx, params)
}

// PairingStatus calls remoteControl/pairing/status.
func (r *RemoteControl) PairingStatus(ctx context.Context, params *RemoteControlPairingStatusParams) (RemoteControlPairingStatusResponse, error) {
	if r == nil || r.client == nil {
		return RemoteControlPairingStatusResponse{}, errClientNil
	}
	return r.client.RemoteControlPairingStatus(ctx, params)
}

// Clients lists paired remote-control clients.
func (r *RemoteControl) Clients(ctx context.Context, params *RemoteControlClientsListParams) (RemoteControlClientsListResponse, error) {
	if r == nil || r.client == nil {
		return RemoteControlClientsListResponse{}, errClientNil
	}
	return r.client.RemoteControlClientList(ctx, params)
}

// RevokeClient revokes a paired remote-control client.
func (r *RemoteControl) RevokeClient(ctx context.Context, params *RemoteControlClientsRevokeParams) (RemoteControlClientsRevokeResponse, error) {
	if r == nil || r.client == nil {
		return RemoteControlClientsRevokeResponse{}, errClientNil
	}
	return r.client.RemoteControlClientRevoke(ctx, params)
}
