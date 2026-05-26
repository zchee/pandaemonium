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
	"slices"
	"strings"
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"
)

func TestNotificationLoginIDExtractsCompletionLoginID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		notification Notification
		wantID       string
		wantScoped   bool
	}{
		"success: top-level login id": {
			notification: Notification{
				Method: NotificationMethodAccountLoginCompleted,
				Params: mustJSON(t, Object{"loginId": "login-1", "success": true}),
			},
			wantID:     "login-1",
			wantScoped: true,
		},
		"success: escaped login id falls back to decoder": {
			notification: Notification{
				Method: NotificationMethodAccountLoginCompleted,
				Params: []byte(`{"login\u0049d":"login-escaped","success":true}`),
			},
			wantID:     "login-escaped",
			wantScoped: true,
		},
		"success: missing login id remains login scoped for global routing": {
			notification: Notification{
				Method: NotificationMethodAccountLoginCompleted,
				Params: mustJSON(t, Object{"turnId": "turn-stray", "success": true}),
			},
			wantScoped: true,
		},
		"error: malformed completion remains login scoped for global routing": {
			notification: Notification{
				Method: NotificationMethodAccountLoginCompleted,
				Params: []byte(`{"loginId":`),
			},
			wantScoped: true,
		},
		"success: non-login method is not login scoped": {
			notification: Notification{
				Method: NotificationMethodTurnCompleted,
				Params: mustJSON(t, Object{"loginId": "login-ignored"}),
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotID, gotScoped := notificationLoginID(tt.notification)
			if gotID != tt.wantID || gotScoped != tt.wantScoped {
				t.Fatalf("notificationLoginID() = (%q, %t), want (%q, %t)", gotID, gotScoped, tt.wantID, tt.wantScoped)
			}
		})
	}
}

func TestLoginNotificationRouterQueuesPendingBeforeWaiter(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	const loginID = "login-pending"
	if err := client.routeNotification(loginCompletedNotification(t, loginID)); err != nil {
		t.Fatalf("routeNotification() error = %v", err)
	}

	completed, err := client.WaitForLoginCompleted(t.Context(), loginID)
	if err != nil {
		t.Fatalf("WaitForLoginCompleted() error = %v", err)
	}
	if completed.LoginID == nil || *completed.LoginID != loginID || !completed.Success {
		t.Fatalf("completed = %#v, want successful login %q", completed, loginID)
	}
	if got := activeLoginConsumers(client); len(got) != 0 {
		t.Fatalf("active login consumers = %v, want none after wait", got)
	}
	if got := pendingLoginNotifications(client, loginID); len(got) != 0 {
		t.Fatalf("pending login notifications = %d, want 0 after wait", len(got))
	}
}

func TestLoginNotificationRouterIsolatesAttempts(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	for _, loginID := range []string{"login-other", "login-target"} {
		if err := client.routeNotification(loginCompletedNotification(t, loginID)); err != nil {
			t.Fatalf("routeNotification(%s) error = %v", loginID, err)
		}
	}

	completed, err := client.WaitForLoginCompleted(t.Context(), "login-target")
	if err != nil {
		t.Fatalf("WaitForLoginCompleted(login-target) error = %v", err)
	}
	if completed.LoginID == nil || *completed.LoginID != "login-target" {
		t.Fatalf("target completion login id = %#v, want login-target", completed.LoginID)
	}
	if pending := pendingLoginNotifications(client, "login-other"); len(pending) != 1 {
		t.Fatalf("login-other pending len = %d, want 1", len(pending))
	}

	other, err := client.WaitForLoginCompleted(t.Context(), "login-other")
	if err != nil {
		t.Fatalf("WaitForLoginCompleted(login-other) error = %v", err)
	}
	if other.LoginID == nil || *other.LoginID != "login-other" {
		t.Fatalf("other completion login id = %#v, want login-other", other.LoginID)
	}
}

func TestLoginNotificationRouterKeepsMalformedCompletionGlobal(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	if _, err := client.openTurnConsumer("turn-stray"); err != nil {
		t.Fatalf("openTurnConsumer() error = %v", err)
	}
	defer client.releaseTurnConsumer("turn-stray")

	notification := Notification{
		Method: NotificationMethodAccountLoginCompleted,
		Params: mustJSON(t, Object{"turnId": "turn-stray", "success": true}),
	}
	if err := client.routeNotification(notification); err != nil {
		t.Fatalf("routeNotification() error = %v", err)
	}
	got, err := client.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification() error = %v", err)
	}
	if got.Method != NotificationMethodAccountLoginCompleted {
		t.Fatalf("NextNotification() method = %q, want %s", got.Method, NotificationMethodAccountLoginCompleted)
	}
}

func TestLoginNotificationRouterCloseUnblocksWaiter(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	result := waitForLoginCompletedAsync(client, t.Context(), "login-close")
	waitForActiveLoginConsumer(t, client, "login-close")

	client.turnRouter.close(&TransportClosedError{Message: "login test closed"})

	err := (<-result).err
	if _, ok := errors.AsType[*TransportClosedError](err); !ok {
		t.Fatalf("WaitForLoginCompleted() error = %v (%T), want *TransportClosedError", err, err)
	}
}

func TestLoginNotificationRouterContextCancellationReleasesWaiter(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	ctx, cancel := context.WithCancel(t.Context())
	result := waitForLoginCompletedAsync(client, ctx, "login-cancel-context")
	waitForActiveLoginConsumer(t, client, "login-cancel-context")
	cancel()

	err := (<-result).err
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForLoginCompleted() error = %v, want context.Canceled", err)
	}
	if got := activeLoginConsumers(client); len(got) != 0 {
		t.Fatalf("active login consumers after cancel = %v, want none", got)
	}

	if err := client.routeNotification(loginCompletedNotification(t, "login-cancel-context")); err != nil {
		t.Fatalf("routeNotification(after cancel) error = %v", err)
	}
	completed, err := client.WaitForLoginCompleted(t.Context(), "login-cancel-context")
	if err != nil {
		t.Fatalf("second WaitForLoginCompleted() error = %v", err)
	}
	if completed.LoginID == nil || *completed.LoginID != "login-cancel-context" {
		t.Fatalf("second completion login id = %#v, want login-cancel-context", completed.LoginID)
	}
}

func TestLoginNotificationRouterRejectsDuplicateWaiter(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	const loginID = "login-duplicate"
	first := waitForLoginCompletedAsync(client, t.Context(), loginID)
	waitForActiveLoginConsumer(t, client, loginID)

	duplicateCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := client.WaitForLoginCompleted(duplicateCtx, loginID); err == nil || !strings.Contains(err.Error(), "already active") {
		t.Fatalf("duplicate WaitForLoginCompleted() error = %v, want already active", err)
	}

	if err := client.routeNotification(loginCompletedNotification(t, loginID)); err != nil {
		t.Fatalf("routeNotification() error = %v", err)
	}
	if err := (<-first).err; err != nil {
		t.Fatalf("first WaitForLoginCompleted() error = %v", err)
	}
}

func TestLoginNotificationRouterOverflowReportsLoginDrop(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)
	const loginID = "login-overflow"
	for range notificationQueueCapacity + 1 {
		if err := client.routeNotification(loginCompletedNotification(t, loginID)); err != nil {
			t.Fatalf("routeNotification() error = %v", err)
		}
	}

	_, err := client.WaitForLoginCompleted(t.Context(), loginID)
	var dropErr *LoginNotificationDroppedError
	if !errors.As(err, &dropErr) {
		t.Fatalf("WaitForLoginCompleted() error = %v (%T), want *LoginNotificationDroppedError", err, err)
	}
	if dropErr.LoginID != loginID {
		t.Fatalf("LoginNotificationDroppedError.LoginID = %q, want %s", dropErr.LoginID, loginID)
	}
	if dropErr.Dropped != 1 {
		t.Fatalf("LoginNotificationDroppedError.Dropped = %d, want 1", dropErr.Dropped)
	}

	global := Notification{Method: "custom/global-after-login-overflow"}
	if err := client.routeNotification(global); err != nil {
		t.Fatalf("routeNotification(global) error = %v", err)
	}
	got, err := client.NextNotification(t.Context())
	if err != nil {
		t.Fatalf("NextNotification() after login overflow error = %v", err)
	}
	if got.Method != global.Method {
		t.Fatalf("NextNotification() method = %q, want %q", got.Method, global.Method)
	}
}

func TestCodexLoginAPIKeyAccountAndLogoutUseAccountMethods(t *testing.T) {
	t.Parallel()

	client := newHelperClient(t, "login_account")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()
	sdk := &Codex{client: client}

	if err := sdk.LoginAPIKey(t.Context(), "sk-sdk-login-test"); err != nil {
		t.Fatalf("LoginAPIKey() error = %v", err)
	}
	refresh := true
	account, err := sdk.Account(t.Context(), &GetAccountParams{RefreshToken: &refresh})
	if err != nil {
		t.Fatalf("Account() error = %v", err)
	}
	if _, ok := account.Account.(APIKeyAccount); !ok {
		t.Fatalf("Account().Account = %#v (%T), want APIKeyAccount", account.Account, account.Account)
	}
	if err := sdk.Logout(t.Context()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
}

func TestCodexLoginHandlesUseAccountMethods(t *testing.T) {
	t.Parallel()

	client := newHelperClient(t, "login_handles")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()
	sdk := &Codex{client: client}

	browser, err := sdk.LoginChatGPT(t.Context())
	if err != nil {
		t.Fatalf("LoginChatGPT() error = %v", err)
	}
	if diff := gocmp.Diff([]string{"login-browser", "https://example.test/login"}, []string{browser.LoginID(), browser.AuthURL()}); diff != "" {
		t.Fatalf("browser handle mismatch (-want +got):\n%s", diff)
	}
	if err := client.routeNotification(loginCompletedNotification(t, browser.LoginID())); err != nil {
		t.Fatalf("route browser completion error = %v", err)
	}
	browserCompleted, err := browser.Wait(t.Context())
	if err != nil {
		t.Fatalf("browser Wait() error = %v", err)
	}
	if browserCompleted.LoginID == nil || *browserCompleted.LoginID != browser.LoginID() {
		t.Fatalf("browser completion login id = %#v, want %s", browserCompleted.LoginID, browser.LoginID())
	}
	browserCancel, err := browser.Cancel(t.Context())
	if err != nil {
		t.Fatalf("browser Cancel() error = %v", err)
	}
	if browserCancel.Status != CancelLoginAccountStatusCanceled {
		t.Fatalf("browser Cancel().Status = %q, want canceled", browserCancel.Status)
	}

	device, err := sdk.LoginChatGPTDeviceCode(t.Context())
	if err != nil {
		t.Fatalf("LoginChatGPTDeviceCode() error = %v", err)
	}
	gotDevice := []string{device.LoginID(), device.VerificationURL(), device.UserCode()}
	wantDevice := []string{"login-device", "https://example.test/device", "CODE-123"}
	if diff := gocmp.Diff(wantDevice, gotDevice); diff != "" {
		t.Fatalf("device handle mismatch (-want +got):\n%s", diff)
	}
	if err := client.routeNotification(loginCompletedNotification(t, device.LoginID())); err != nil {
		t.Fatalf("route device completion error = %v", err)
	}
	deviceCompleted, err := device.Wait(t.Context())
	if err != nil {
		t.Fatalf("device Wait() error = %v", err)
	}
	if deviceCompleted.LoginID == nil || *deviceCompleted.LoginID != device.LoginID() {
		t.Fatalf("device completion login id = %#v, want %s", deviceCompleted.LoginID, device.LoginID())
	}
	deviceCancel, err := device.Cancel(t.Context())
	if err != nil {
		t.Fatalf("device Cancel() error = %v", err)
	}
	if deviceCancel.Status != CancelLoginAccountStatusCanceled {
		t.Fatalf("device Cancel().Status = %q, want canceled", deviceCancel.Status)
	}
}

type loginWaitResult struct {
	completed AccountLoginCompletedNotification
	err       error
}

func waitForLoginCompletedAsync(client *Client, ctx context.Context, loginID string) <-chan loginWaitResult {
	result := make(chan loginWaitResult, 1)
	go func() {
		completed, err := client.WaitForLoginCompleted(ctx, loginID)
		result <- loginWaitResult{completed: completed, err: err}
	}()
	return result
}

func loginCompletedNotification(t *testing.T, loginID string) Notification {
	t.Helper()
	return Notification{
		Method: NotificationMethodAccountLoginCompleted,
		Params: mustJSON(t, AccountLoginCompletedNotification{
			LoginID: &loginID,
			Success: true,
		}),
	}
}

func pendingLoginNotifications(client *Client, loginID string) []Notification {
	if client == nil || client.turnRouter == nil {
		return nil
	}
	client.turnRouter.mu.Lock()
	defer client.turnRouter.mu.Unlock()
	return append([]Notification(nil), client.turnRouter.pendingLogin[loginID]...)
}

func activeLoginConsumers(client *Client) []string {
	if client == nil || client.turnRouter == nil {
		return nil
	}
	client.turnRouter.mu.Lock()
	defer client.turnRouter.mu.Unlock()
	got := make([]string, 0, len(client.turnRouter.loginQueues))
	for loginID := range client.turnRouter.loginQueues {
		got = append(got, loginID)
	}
	slices.Sort(got)
	return got
}

func waitForActiveLoginConsumer(t *testing.T, client *Client, loginID string) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if got := activeLoginConsumers(client); len(got) == 1 && got[0] == loginID {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("active login consumers = %v, want [%s]", activeLoginConsumers(client), loginID)
		case <-ticker.C:
		}
	}
}
