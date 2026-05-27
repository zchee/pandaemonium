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
	client   *Client
	metadata InitializeResponse
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

	metadata, err := client.initializeServer(ctx)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &ExecServer{
		client:   client,
		metadata: metadata,
	}, nil
}

// Metadata returns initialize metadata validated during construction.
func (c *ExecServer) Metadata() InitializeResponse {
	if c == nil {
		return InitializeResponse{}
	}
	return c.metadata
}

// CommandExec runs a standalone command (argv vector).
func (c *ExecServer) CommandExec(ctx context.Context, params *CommandExecParams) (CommandExecResponse, error) {
	if c == nil {
		return CommandExecResponse{}, nil
	}
	return c.client.CommandExec(ctx, params)
}

// CommandExecWrite writes stdin bytes to a running command/exec session.
func (c *ExecServer) CommandExecWrite(ctx context.Context, params *CommandExecWriteParams) (CommandExecWriteResponse, error) {
	if c == nil {
		return CommandExecWriteResponse{}, nil
	}
	return c.client.CommandExecWrite(ctx, params)
}

// CommandExecTerminate terminates a running command/exec session.
func (c *ExecServer) CommandExecTerminate(ctx context.Context, params *CommandExecTerminateParams) (CommandExecTerminateResponse, error) {
	if c == nil {
		return CommandExecTerminateResponse{}, nil
	}
	return c.client.CommandExecTerminate(ctx, params)
}

// CommandExecResize resizes a running command/exec PTY-backed session.
func (c *ExecServer) CommandExecResize(ctx context.Context, params *CommandExecResizeParams) (CommandExecResizeResponse, error) {
	if c == nil {
		return CommandExecResizeResponse{}, nil
	}
	return c.client.CommandExecResize(ctx, params)
}

// Client exposes the lower-level JSON-RPC client.
func (c *ExecServer) Client() *Client {
	if c == nil {
		return nil
	}
	return c.client
}

// Close terminates the exec-server process.
func (c *ExecServer) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
