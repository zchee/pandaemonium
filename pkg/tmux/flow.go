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

package tmux

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// DetachClient is the tmux detach-client command; its alias is detach.
	DetachClient Command = "detach-client"
	// DisplayMessage is the tmux display-message command; its alias is display.
	DisplayMessage Command = "display-message"
	// ListPanes is the tmux list-panes command; its alias is lsp.
	ListPanes Command = "list-panes"
	// RefreshClient is the tmux refresh-client command; its alias is refresh.
	RefreshClient Command = "refresh-client"
)

// PaneID is a stable tmux pane ID such as `%0`.
type PaneID string

// WindowID is a stable tmux window ID such as `@1`.
type WindowID string

// SessionID is a stable tmux session ID such as `$2`.
type SessionID string

// ClientFlag is a refresh-client flag value.
type ClientFlag string

const (
	// ClientFlagNoOutput disables `%output` notifications for the client.
	ClientFlagNoOutput ClientFlag = "no-output"
	// ClientFlagWaitExit asks tmux to wait for an empty line after `%exit`.
	ClientFlagWaitExit ClientFlag = "wait-exit"
)

// SubscriptionTarget is the target part of a refresh-client format subscription.
type SubscriptionTarget string

const (
	// SubscriptionAttachedSession subscribes to the attached session.
	SubscriptionAttachedSession SubscriptionTarget = ""
	// SubscriptionAllPanes subscribes to all panes in the attached session.
	SubscriptionAllPanes SubscriptionTarget = "%*"
	// SubscriptionAllWindows subscribes to all windows in the attached session.
	SubscriptionAllWindows SubscriptionTarget = "@*"
)

// RefreshClientSize sets the control client size with `refresh-client -C XxY`.
func (c *Client) RefreshClientSize(ctx context.Context, width, height int) (Response, error) {
	if width <= 0 || height <= 0 {
		return Response{}, fmt.Errorf("tmux: client size must be positive")
	}
	return c.Exec(ctx, RefreshClient, RawArg("-C"), StringArg(strconv.Itoa(width)+"x"+strconv.Itoa(height)))
}

// SetClientFlags sets control-client flags with `refresh-client -f`.
func (c *Client) SetClientFlags(ctx context.Context, flags ...ClientFlag) (Response, error) {
	if len(flags) == 0 {
		return Response{}, fmt.Errorf("tmux: at least one client flag is required")
	}
	values := make([]string, 0, len(flags))
	for _, flag := range flags {
		value := string(flag)
		if err := validateRefreshFragment(value, "client flag"); err != nil {
			return Response{}, err
		}
		values = append(values, value)
	}
	return c.Exec(ctx, RefreshClient, RawArg("-f"), StringArg(strings.Join(values, ",")))
}

// SetPauseAfter enables flow control with `refresh-client -f pause-after=N`.
//
// The tmux wire format is integer seconds; d must be a whole number of seconds
// because tmux cannot express sub-second precision.
func (c *Client) SetPauseAfter(ctx context.Context, d time.Duration) (Response, error) {
	if d <= 0 {
		return Response{}, fmt.Errorf("tmux: pause-after duration must be positive")
	}
	if d%time.Second != 0 {
		return Response{}, fmt.Errorf("tmux: pause-after duration %s must be a whole number of seconds", d)
	}
	seconds := int64(d / time.Second)
	return c.SetClientFlags(ctx, ClientFlag("pause-after="+strconv.FormatInt(seconds, 10)))
}

// PausePane asks tmux to pause pane output with `refresh-client -A pane:pause`.
func (c *Client) PausePane(ctx context.Context, pane PaneID) (Response, error) {
	return c.paneFlow(ctx, pane, "pause")
}

// ContinuePane asks tmux to continue pane output with `refresh-client -A pane:continue`.
func (c *Client) ContinuePane(ctx context.Context, pane PaneID) (Response, error) {
	return c.paneFlow(ctx, pane, "continue")
}

// DisablePaneOutput tells tmux this client no longer needs output from pane.
func (c *Client) DisablePaneOutput(ctx context.Context, pane PaneID) (Response, error) {
	return c.paneFlow(ctx, pane, "off")
}

// EnablePaneOutput tells tmux this client needs output from pane.
func (c *Client) EnablePaneOutput(ctx context.Context, pane PaneID) (Response, error) {
	return c.paneFlow(ctx, pane, "on")
}

// SubscribeFormat subscribes to a tmux format with `refresh-client -B`.
func (c *Client) SubscribeFormat(ctx context.Context, name string, target SubscriptionTarget, format string) (Response, error) {
	if err := validateSubscriptionName(name); err != nil {
		return Response{}, err
	}
	if strings.ContainsAny(string(target), "\r\n:") {
		return Response{}, fmt.Errorf("tmux: subscription target %q must not contain newline or colon", target)
	}
	if strings.ContainsAny(format, "\r\n") {
		return Response{}, fmt.Errorf("tmux: subscription format must not contain a newline")
	}
	arg := name + ":" + string(target) + ":" + format
	return c.Exec(ctx, RefreshClient, RawArg("-B"), StringArg(arg))
}

// UnsubscribeFormat removes a tmux format subscription with `refresh-client -B`.
func (c *Client) UnsubscribeFormat(ctx context.Context, name string) (Response, error) {
	if err := validateSubscriptionName(name); err != nil {
		return Response{}, err
	}
	return c.Exec(ctx, RefreshClient, RawArg("-B"), StringArg(name))
}

func (c *Client) paneFlow(ctx context.Context, pane PaneID, state string) (Response, error) {
	if err := validatePaneID(pane); err != nil {
		return Response{}, err
	}
	return c.Exec(ctx, RefreshClient, RawArg("-A"), StringArg(string(pane)+":"+state))
}

func validatePaneID(pane PaneID) error {
	value := string(pane)
	if len(value) < 2 || value[0] != '%' {
		return fmt.Errorf("tmux: pane ID %q must have %% prefix", value)
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return fmt.Errorf("tmux: pane ID %q must contain decimal digits after %%", value)
		}
	}
	return nil
}

func validateRefreshFragment(value, name string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("tmux: %s must not be empty", name)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("tmux: %s must not contain a newline", name)
	}
	return nil
}

func validateSubscriptionName(name string) error {
	if err := validateRefreshFragment(name, "subscription name"); err != nil {
		return err
	}
	if strings.Contains(name, ":") {
		return fmt.Errorf("tmux: subscription name %q must not contain colon", name)
	}
	return nil
}
