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
)

// ChatGPTLoginHandle controls one browser-based ChatGPT login attempt.
type ChatGPTLoginHandle struct {
	client  *Client
	loginID string
	authURL string
}

// DeviceCodeLoginHandle controls one ChatGPT device-code login attempt.
type DeviceCodeLoginHandle struct {
	client          *Client
	loginID         string
	verificationURL string
	userCode        string
}

// LoginAPIKey authenticates the app-server with an API key.
func (c *Codex) LoginAPIKey(ctx context.Context, apiKey string) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("codex client is nil")
	}
	_, err := c.client.AccountLoginStart(ctx, NewLoginAccountParamsAPIKey(apiKey))
	return err
}

// LoginChatGPT starts browser-based ChatGPT login and returns its live handle.
func (c *Codex) LoginChatGPT(ctx context.Context) (*ChatGPTLoginHandle, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("codex client is nil")
	}
	response, err := c.client.AccountLoginStart(ctx, NewLoginAccountParamsChatGPT())
	if err != nil {
		return nil, err
	}
	typed, ok := response.(ChatGPTv2LoginAccountResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected ChatGPT login response %T", response)
	}
	if typed.LoginID == "" {
		return nil, fmt.Errorf("unexpected ChatGPT login response with empty login id")
	}
	return &ChatGPTLoginHandle{
		client:  c.client,
		loginID: typed.LoginID,
		authURL: typed.AuthURL,
	}, nil
}

// LoginChatGPTDeviceCode starts ChatGPT device-code login and returns its live handle.
func (c *Codex) LoginChatGPTDeviceCode(ctx context.Context) (*DeviceCodeLoginHandle, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("codex client is nil")
	}
	response, err := c.client.AccountLoginStart(ctx, NewLoginAccountParamsHeadless())
	if err != nil {
		return nil, err
	}
	typed, ok := response.(ChatGPTDeviceCodev2LoginAccountResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected ChatGPT device-code login response %T", response)
	}
	if typed.LoginID == "" {
		return nil, fmt.Errorf("unexpected ChatGPT device-code login response with empty login id")
	}
	return &DeviceCodeLoginHandle{
		client:          c.client,
		loginID:         typed.LoginID,
		verificationURL: typed.VerificationURL,
		userCode:        typed.UserCode,
	}, nil
}

// Account reads the current app-server account state.
func (c *Codex) Account(ctx context.Context, params *GetAccountParams) (GetAccountResponse, error) {
	if c == nil || c.client == nil {
		return GetAccountResponse{}, fmt.Errorf("codex client is nil")
	}
	return c.client.AccountRead(ctx, params)
}

// Logout clears the current app-server account session.
func (c *Codex) Logout(ctx context.Context) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("codex client is nil")
	}
	_, err := c.client.AccountLogout(ctx)
	return err
}

// LoginID returns the app-server login attempt id.
func (h *ChatGPTLoginHandle) LoginID() string {
	if h == nil {
		return ""
	}
	return h.loginID
}

// AuthURL returns the browser URL for this ChatGPT login attempt.
func (h *ChatGPTLoginHandle) AuthURL() string {
	if h == nil {
		return ""
	}
	return h.authURL
}

// Wait blocks until this login attempt emits account/login/completed.
func (h *ChatGPTLoginHandle) Wait(ctx context.Context) (AccountLoginCompletedNotification, error) {
	if h == nil || h.client == nil {
		return AccountLoginCompletedNotification{}, fmt.Errorf("chatgpt login handle is nil")
	}
	return h.client.WaitForLoginCompleted(ctx, h.loginID)
}

// Cancel cancels this browser-based ChatGPT login attempt.
func (h *ChatGPTLoginHandle) Cancel(ctx context.Context) (CancelLoginAccountResponse, error) {
	if h == nil || h.client == nil {
		return CancelLoginAccountResponse{}, fmt.Errorf("chatgpt login handle is nil")
	}
	return h.client.AccountLoginCancel(ctx, &CancelLoginAccountParams{LoginID: h.loginID})
}

// LoginID returns the app-server login attempt id.
func (h *DeviceCodeLoginHandle) LoginID() string {
	if h == nil {
		return ""
	}
	return h.loginID
}

// VerificationURL returns the URL where the user enters the device code.
func (h *DeviceCodeLoginHandle) VerificationURL() string {
	if h == nil {
		return ""
	}
	return h.verificationURL
}

// UserCode returns the one-time device code for this login attempt.
func (h *DeviceCodeLoginHandle) UserCode() string {
	if h == nil {
		return ""
	}
	return h.userCode
}

// Wait blocks until this login attempt emits account/login/completed.
func (h *DeviceCodeLoginHandle) Wait(ctx context.Context) (AccountLoginCompletedNotification, error) {
	if h == nil || h.client == nil {
		return AccountLoginCompletedNotification{}, fmt.Errorf("device-code login handle is nil")
	}
	return h.client.WaitForLoginCompleted(ctx, h.loginID)
}

// Cancel cancels this device-code login attempt.
func (h *DeviceCodeLoginHandle) Cancel(ctx context.Context) (CancelLoginAccountResponse, error) {
	if h == nil || h.client == nil {
		return CancelLoginAccountResponse{}, fmt.Errorf("device-code login handle is nil")
	}
	return h.client.AccountLoginCancel(ctx, &CancelLoginAccountParams{LoginID: h.loginID})
}
