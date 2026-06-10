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
	"testing"

	"github.com/go-json-experiment/json"
)

func TestRemoteControlFacadeDelegatesToClient(t *testing.T) {
	client := newRemoteControlFacadeTestClient(t)

	got, err := client.RemoteControl().Status(t.Context())
	if err != nil {
		t.Fatalf("Client.RemoteControl().Status() error = %v", err)
	}
	if got.Status != RemoteControlConnectionStatusConnected || got.ServerName != "facade-server" {
		t.Fatalf("Client.RemoteControl().Status() = %#v, want connected facade-server", got)
	}

	got, err = (&Codex{client: client}).RemoteControl().Status(t.Context())
	if err != nil {
		t.Fatalf("Codex.RemoteControl().Status() error = %v", err)
	}
	if got.Status != RemoteControlConnectionStatusConnected || got.ServerName != "facade-server" {
		t.Fatalf("Codex.RemoteControl().Status() = %#v, want connected facade-server", got)
	}
}

func TestRemoteControlFacadePairingStatusDelegatesToClient(t *testing.T) {
	client := newRemoteControlPairingStatusFacadeTestClient(t)

	got, err := client.RemoteControl().PairingStatus(t.Context(), &RemoteControlPairingStatusParams{
		PairingCode:       "pair-facade",
		ManualPairingCode: "manual-facade",
	})
	if err != nil {
		t.Fatalf("Client.RemoteControl().PairingStatus() error = %v", err)
	}
	if !got.Claimed {
		t.Fatalf("Client.RemoteControl().PairingStatus() = %#v, want claimed", got)
	}
}

func newRemoteControlPairingStatusFacadeTestClient(t *testing.T) *Client {
	t.Helper()

	tr := newScriptTransport()
	tr.onWrite = func(data []byte, tr *scriptTransport) error {
		var req rpcMessage
		if err := json.Unmarshal(data, &req); err != nil {
			return err
		}
		if req.Method != RequestMethodRemoteControlPairingStatus {
			t.Fatalf("request method = %q, want %s", req.Method, RequestMethodRemoteControlPairingStatus)
		}
		var params RemoteControlPairingStatusParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return err
		}
		if params.PairingCode != "pair-facade" || params.ManualPairingCode != "manual-facade" {
			t.Fatalf("request params = %#v, want facade pairing codes", params)
		}
		return tr.enqueueJSON(Object{
			"id":     req.ID,
			"result": RemoteControlPairingStatusResponse{Claimed: true},
		})
	}

	return newScriptedClient(t, tr)
}

func newRemoteControlFacadeTestClient(t *testing.T) *Client {
	t.Helper()

	tr := newScriptTransport()
	tr.onWrite = func(data []byte, tr *scriptTransport) error {
		var req rpcMessage
		if err := json.Unmarshal(data, &req); err != nil {
			return err
		}
		if req.Method != RequestMethodRemoteControlStatusRead {
			t.Fatalf("request method = %q, want %s", req.Method, RequestMethodRemoteControlStatusRead)
		}
		return tr.enqueueJSON(Object{
			"id": req.ID,
			"result": RemoteControlStatusReadResponse{
				ServerName:     "facade-server",
				InstallationID: "facade-installation",
				Status:         RemoteControlConnectionStatusConnected,
			},
		})
	}

	return newScriptedClient(t, tr)
}
