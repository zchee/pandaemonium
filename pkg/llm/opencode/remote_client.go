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
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RemoteConfig attaches to an already-running `opencode serve` without
// spawning a process. Auth is HTTP basic (username "opencode") carried in
// the Authorization header; there is no pairing protocol.
type RemoteConfig struct {
	// BaseURL is the server root, e.g. "http://127.0.0.1:4096". It must not
	// carry userinfo — the password never appears in URLs.
	BaseURL string

	// Password is the server's OPENCODE_SERVER_PASSWORD value, if set.
	Password string

	// Insecure permits plain http to a non-loopback host. Without it, remote
	// attachment requires https or a loopback address so basic-auth
	// credentials are never sent in cleartext off-host.
	Insecure bool

	// Retry, DialTimeout, DrainWindow, PermissionAuto, and HTTPClient behave
	// exactly as their Config counterparts.
	Retry          RetryConfig
	DialTimeout    time.Duration
	DrainWindow    time.Duration
	PermissionAuto bool
	HTTPClient     *http.Client
}

// NewRemoteClient creates a client attached to a running server. It
// validates the base URL (scheme, no userinfo, loopback-or-Insecure for
// plain http) but performs no I/O; use NewRemoteOpencode for a validated
// handshake.
func NewRemoteClient(config *RemoteConfig) (*Client, error) {
	if config == nil {
		return nil, errors.New("opencode: remote config is required")
	}
	baseURL, err := validateRemoteBaseURL(config.BaseURL, config.Insecure)
	if err != nil {
		return nil, err
	}

	client := NewClient(&Config{
		Password:       config.Password,
		Retry:          config.Retry,
		DialTimeout:    config.DialTimeout,
		DrainWindow:    config.DrainWindow,
		PermissionAuto: config.PermissionAuto,
		HTTPClient:     config.HTTPClient,
	})
	client.mu.Lock()
	client.baseURL = baseURL
	client.mu.Unlock()
	return client, nil
}

// NewRemoteOpencode attaches to a running server, performs a Health
// handshake, and eagerly dials the shared event bus (failing fast, exactly
// like NewOpencode).
func NewRemoteOpencode(ctx context.Context, config *RemoteConfig) (*Opencode, error) {
	client, err := NewRemoteClient(config)
	if err != nil {
		return nil, err
	}
	healthCtx, cancel := context.WithTimeout(ctx, client.config.DialTimeout)
	defer cancel()
	if _, err := client.Health(healthCtx); err != nil {
		_ = client.Close()
		return nil, err
	}
	if _, err := client.ensureBus(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Opencode{client: client}, nil
}

// validateRemoteBaseURL enforces the remote attachment invariants and
// normalizes the URL (no trailing slash).
func validateRemoteBaseURL(rawURL string, insecure bool) (string, error) {
	if rawURL == "" {
		return "", errors.New("opencode: remote base URL is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("opencode: invalid remote base URL: %w", err)
	}
	if parsed.User != nil {
		return "", errors.New("opencode: remote base URL must not contain userinfo; pass the password via RemoteConfig.Password")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("opencode: remote base URL %q has no host", rawURL)
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !insecure && !isLoopbackHost(parsed.Hostname()) {
			return "", fmt.Errorf("opencode: plain http to non-loopback host %q requires RemoteConfig.Insecure", parsed.Hostname())
		}
	default:
		return "", fmt.Errorf("opencode: unsupported remote base URL scheme %q", parsed.Scheme)
	}

	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// isLoopbackHost reports whether host is a loopback address without
// performing DNS resolution: "localhost" or a literal loopback IP.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
