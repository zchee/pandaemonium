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
	"fmt"
	"iter"
	"sync"
)

// AppServerProcessNotification is a typed process/spawn notification with its
// original raw notification preserved.
type AppServerProcessNotification struct {
	Notification Notification
	OutputDelta  *ProcessOutputDeltaNotification
	Exited       *ProcessExitedNotification
}

// AppServerProcessHandle is a high-level handle for a process/spawn session.
type AppServerProcessHandle struct {
	client        *Client
	processHandle string
	releaseOnce   sync.Once
}

// SpawnProcess calls process/spawn and returns a process notification handle.
func (c *Client) SpawnProcess(ctx context.Context, params *ProcessSpawnParams) (*AppServerProcessHandle, error) {
	if c == nil {
		return nil, errClientNil
	}
	if params == nil {
		return nil, fmt.Errorf("process spawn params are required")
	}
	if params.ProcessHandle == "" {
		return nil, fmt.Errorf("process spawn process handle is required")
	}
	if _, err := c.turnRouter.registerProcess(params.ProcessHandle); err != nil {
		return nil, err
	}

	spawnParams := *params
	if _, err := c.ProcessSpawn(ctx, &spawnParams); err != nil {
		c.releaseProcessConsumer(params.ProcessHandle)
		return nil, err
	}

	return &AppServerProcessHandle{
		client:        c,
		processHandle: params.ProcessHandle,
	}, nil
}

// ID returns the process handle.
func (h *AppServerProcessHandle) ID() string {
	if h == nil {
		return ""
	}
	return h.processHandle
}

// Close releases the local process notification consumer.
func (h *AppServerProcessHandle) Close() error {
	if h == nil || h.client == nil {
		return errProcessHandleNil
	}
	h.release()
	return nil
}

// WriteStdin writes base64-encoded stdin bytes, closes stdin, or both.
func (h *AppServerProcessHandle) WriteStdin(ctx context.Context, deltaBase64 *string, closeStdin bool) (ProcessWriteStdinResponse, error) {
	if h == nil || h.client == nil || h.processHandle == "" {
		return ProcessWriteStdinResponse{}, errProcessHandleNil
	}
	return h.client.ProcessWriteStdin(ctx, &ProcessWriteStdinParams{
		ProcessHandle: h.processHandle,
		DeltaBase64:   deltaBase64,
		CloseStdin:    closeStdin,
	})
}

// ResizePty resizes the process PTY.
func (h *AppServerProcessHandle) ResizePty(ctx context.Context, size ProcessTerminalSize) (ProcessResizePtyResponse, error) {
	if h == nil || h.client == nil || h.processHandle == "" {
		return ProcessResizePtyResponse{}, errProcessHandleNil
	}
	return h.client.ProcessResizePty(ctx, &ProcessResizePtyParams{
		ProcessHandle: h.processHandle,
		Size:          size,
	})
}

// Kill terminates the running process.
func (h *AppServerProcessHandle) Kill(ctx context.Context) (ProcessKillResponse, error) {
	if h == nil || h.client == nil || h.processHandle == "" {
		return ProcessKillResponse{}, errProcessHandleNil
	}
	return h.client.ProcessKill(ctx, &ProcessKillParams{ProcessHandle: h.processHandle})
}

// NextNotification waits for the next typed process notification.
func (h *AppServerProcessHandle) NextNotification(ctx context.Context) (AppServerProcessNotification, error) {
	if h == nil || h.client == nil || h.processHandle == "" {
		return AppServerProcessNotification{}, errProcessHandleNil
	}
	notification, err := h.client.nextProcessNotification(ctx, h.processHandle)
	if err != nil {
		return AppServerProcessNotification{}, err
	}
	typed, err := decodeAppServerProcessNotification(notification)
	if err != nil {
		return AppServerProcessNotification{}, err
	}
	if typed.Exited != nil {
		h.release()
	}
	return typed, nil
}

// Stream yields typed process notifications until error, cancellation, or exit.
func (h *AppServerProcessHandle) Stream(ctx context.Context) iter.Seq2[AppServerProcessNotification, error] {
	return func(yield func(AppServerProcessNotification, error) bool) {
		for {
			notification, err := h.NextNotification(ctx)
			if err != nil {
				yield(AppServerProcessNotification{}, err)
				return
			}
			if !yield(notification, nil) {
				return
			}
			if notification.Exited != nil {
				return
			}
		}
	}
}

func (h *AppServerProcessHandle) release() {
	h.releaseOnce.Do(func() {
		h.client.releaseProcessConsumer(h.processHandle)
	})
}

func decodeAppServerProcessNotification(notification Notification) (AppServerProcessNotification, error) {
	switch notification.Method {
	case NotificationMethodProcessOutputDelta:
		output, ok, err := notification.ProcessOutputDelta()
		if err != nil {
			return AppServerProcessNotification{}, err
		}
		if !ok {
			return AppServerProcessNotification{}, fmt.Errorf("process notification method mismatch: %s", notification.Method)
		}
		return AppServerProcessNotification{
			Notification: notification,
			OutputDelta:  &output,
		}, nil
	case NotificationMethodProcessExited:
		exited, ok, err := notification.ProcessExited()
		if err != nil {
			return AppServerProcessNotification{}, err
		}
		if !ok {
			return AppServerProcessNotification{}, fmt.Errorf("process notification method mismatch: %s", notification.Method)
		}
		return AppServerProcessNotification{
			Notification: notification,
			Exited:       &exited,
		}, nil
	default:
		return AppServerProcessNotification{}, fmt.Errorf("unexpected process notification method %q", notification.Method)
	}
}
