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
	"context"
	"errors"
)

var errExecServerNil = errors.New("codex exec server is nil")

// ServerMode selects whether the SDK expects an app-server or command-exec server.
type ServerMode string

const (
	// ServerModeAppServer configures the legacy thread/turn server flow.
	ServerModeAppServer ServerMode = "app-server"
	// ServerModeExecServer configures the command/exec server flow.
	ServerModeExecServer ServerMode = "exec-server"

	// NotificationMethodInitialized is sent by servers after initialize completion.
	NotificationMethodInitialized = "initialized"
)

// ExecServer is the high-level synchronous Go SDK surface for command-exec server flows.
type ExecServer struct {
	client    *Client
	metadata  InitializeResponse
	sessionID string
}

// NewExecServer starts and initializes an exec-server client.
func NewExecServer(ctx context.Context, config *Config) (*ExecServer, error) {
	cfg := Config{}
	if config != nil {
		cfg = *config
	}
	cfg.ServerMode = ServerModeExecServer
	client := NewClient(&cfg, nil)
	if err := client.Start(ctx); err != nil {
		return nil, err
	}

	metadata, execMeta, err := client.initializeServer(ctx)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &ExecServer{
		client:    client,
		metadata:  metadata,
		sessionID: execMeta.SessionID,
	}, nil
}

// Metadata returns initialize metadata validated during construction.
func (es *ExecServer) Metadata() InitializeResponse {
	if es == nil {
		return InitializeResponse{}
	}
	return es.metadata
}

// SessionID returns the session id assigned by the exec-server during
// initialization, or the empty string if the server did not assign one.
func (es *ExecServer) SessionID() string {
	if es == nil {
		return ""
	}
	return es.sessionID
}

// CommandExec runs a standalone command (argv vector).
func (es *ExecServer) CommandExec(ctx context.Context, params *CommandExecParams) (CommandExecResponse, error) {
	if es == nil || es.client == nil {
		return CommandExecResponse{}, errExecServerNil
	}
	return es.client.CommandExec(ctx, params)
}

// CommandExecWrite writes stdin bytes to a running command/exec session.
func (es *ExecServer) CommandExecWrite(ctx context.Context, params *CommandExecWriteParams) (CommandExecWriteResponse, error) {
	if es == nil || es.client == nil {
		return CommandExecWriteResponse{}, errExecServerNil
	}
	return es.client.CommandExecWrite(ctx, params)
}

// CommandExecTerminate terminates a running command/exec session.
func (es *ExecServer) CommandExecTerminate(ctx context.Context, params *CommandExecTerminateParams) (CommandExecTerminateResponse, error) {
	if es == nil || es.client == nil {
		return CommandExecTerminateResponse{}, errExecServerNil
	}
	return es.client.CommandExecTerminate(ctx, params)
}

// CommandExecResize resizes a running command/exec PTY-backed session.
func (es *ExecServer) CommandExecResize(ctx context.Context, params *CommandExecResizeParams) (CommandExecResizeResponse, error) {
	if es == nil || es.client == nil {
		return CommandExecResizeResponse{}, errExecServerNil
	}
	return es.client.CommandExecResize(ctx, params)
}

// Client exposes the lower-level JSON-RPC client.
func (es *ExecServer) Client() *Client {
	if es == nil {
		return nil
	}
	return es.client
}

// Close terminates the exec-server process.
func (es *ExecServer) Close() error {
	if es == nil || es.client == nil {
		return nil
	}
	return es.client.Close()
}
