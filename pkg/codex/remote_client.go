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
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// RemoteConfig connects to an already-running app-server endpoint.
//
// RemoteConfig is intentionally separate from [Config.Listen]. Listen controls
// where a child codex process should listen; RemoteConfig.URL controls where
// this Go process dials without launching codex.
type RemoteConfig struct {
	// URL is the remote app-server websocket endpoint. Supported schemes are
	// ws://, wss://, unix://, and unix://PATH.
	URL string

	// BearerToken is sent as the websocket Authorization bearer token for ws://
	// and wss:// endpoints when allowed by URL validation.
	BearerToken string

	// BearerTokenFile is read and sent as the websocket Authorization bearer
	// token for ws:// and wss:// endpoints when allowed by URL validation.
	BearerTokenFile string

	// DialTimeout bounds the websocket dial attempt. A zero value inherits the
	// caller's context deadline.
	DialTimeout time.Duration

	// AllowInsecureRemoteWebSocket allows bearer-token auth over non-loopback
	// ws:// endpoints. Leave false unless the caller has provided an external
	// security layer and accepts plaintext websocket bearer-token exposure.
	AllowInsecureRemoteWebSocket bool

	ClientName    string
	ClientTitle   string
	ClientVersion string

	ExperimentalAPI *bool
}

// NewRemoteClient connects to an already-running app-server endpoint without
// launching a local codex process. Call [Client.Initialize] before issuing
// protocol requests that require the app-server initialize handshake.
func NewRemoteClient(ctx context.Context, config *RemoteConfig, approvalHandler ApprovalHandler) (*Client, error) {
	client := NewClient(remoteClientConfig(config), approvalHandler)
	if err := client.ConnectRemote(ctx, config); err != nil {
		return nil, err
	}
	return client, nil
}

// NewRemoteCodex connects to and initializes an already-running app-server
// endpoint without launching a local codex process.
func NewRemoteCodex(ctx context.Context, config *RemoteConfig) (*Codex, error) {
	client, err := NewRemoteClient(ctx, config, nil)
	if err != nil {
		return nil, err
	}

	metadata, err := client.Initialize(ctx)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Codex{
		client:   client,
		metadata: metadata,
	}, nil
}

// ConnectRemote connects this client to an already-running app-server endpoint
// without launching a local codex process.
func (c *Client) ConnectRemote(ctx context.Context, config *RemoteConfig) error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.loadTransport() != nil {
		return nil
	}

	cfg := normalizeRemoteConfig(config)
	conn, err := dialRemoteAppServer(ctx, cfg)
	if err != nil {
		return err
	}

	c.cmd = nil
	c.cmdDone = nil
	c.stdin = nil
	c.stdout = nil
	c.stdoutCloser = nil
	c.stderr = nil
	c.rpcState = newJSONRPCClientState()
	c.turnRouter = newTurnNotificationRouter()
	c.stderrDone = make(chan struct{})
	close(c.stderrDone)
	c.readDone = make(chan struct{})
	c.storeTransport(&websocketTransport{conn: conn})

	go c.readLoop(ctx, c.loadTransport(), c.readDone)
	return nil
}

func remoteClientConfig(config *RemoteConfig) *Config {
	cfg := normalizeRemoteConfig(config)
	return &Config{
		ClientName:      cfg.ClientName,
		ClientTitle:     cfg.ClientTitle,
		ClientVersion:   cfg.ClientVersion,
		ExperimentalAPI: cfg.ExperimentalAPI,
	}
}

func normalizeRemoteConfig(config *RemoteConfig) RemoteConfig {
	if config == nil {
		return RemoteConfig{}
	}
	return *config
}

type remoteEndpointKind int

const (
	remoteEndpointWebSocket remoteEndpointKind = iota
	remoteEndpointUnixWebSocket
)

type remoteEndpoint struct {
	kind   remoteEndpointKind
	rawURL string
}

func dialRemoteAppServer(ctx context.Context, cfg RemoteConfig) (*websocket.Conn, error) {
	endpoint, err := validateRemoteConfig(cfg)
	if err != nil {
		return nil, err
	}

	dialCtx := ctx
	var cancel context.CancelFunc
	if cfg.DialTimeout > 0 {
		dialCtx, cancel = context.WithTimeout(ctx, cfg.DialTimeout)
	}
	if cancel != nil {
		defer cancel()
	}

	switch endpoint.kind {
	case remoteEndpointUnixWebSocket:
		return dialWebSocket(dialCtx, endpoint.rawURL, &WebSocketConfig{
			AuthMode:    WebSocketAuthNone,
			DialTimeout: cfg.DialTimeout,
		}, nil, "")
	default:
		return dialRemoteWebSocketURL(dialCtx, endpoint.rawURL, &WebSocketConfig{
			ClientBearerToken:     cfg.BearerToken,
			ClientBearerTokenFile: cfg.BearerTokenFile,
		})
	}
}

func validateRemoteConfig(cfg RemoteConfig) (remoteEndpoint, error) {
	if cfg.DialTimeout < 0 {
		return remoteEndpoint{}, fmt.Errorf("remote app-server dial timeout must be non-negative")
	}
	if err := validateRemoteBearerConfig(cfg); err != nil {
		return remoteEndpoint{}, err
	}

	rawURL := strings.TrimSpace(cfg.URL)
	if rawURL == "" {
		return remoteEndpoint{}, fmt.Errorf("remote app-server URL is required")
	}

	if strings.HasPrefix(rawURL, unixListenPrefix) {
		if err := validateUnixListenURL(rawURL); err != nil {
			return remoteEndpoint{}, err
		}
		if remoteBearerConfigured(cfg) {
			return remoteEndpoint{}, fmt.Errorf("unix remote app-server URLs use socket permissions and do not support bearer auth")
		}
		return remoteEndpoint{kind: remoteEndpointUnixWebSocket, rawURL: rawURL}, nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return remoteEndpoint{}, fmt.Errorf("invalid remote app-server URL: %w", err)
	}
	if parsed.User != nil {
		return remoteEndpoint{}, fmt.Errorf("remote app-server URL must not include user info")
	}
	switch parsed.Scheme {
	case "ws", "wss":
	default:
		return remoteEndpoint{}, fmt.Errorf("unsupported remote app-server URL scheme %q: expected ws://, wss://, or unix://", parsed.Scheme)
	}
	if parsed.Host == "" {
		return remoteEndpoint{}, fmt.Errorf("remote app-server %s URL is missing host", parsed.Scheme)
	}
	if parsed.Scheme == "ws" && remoteBearerConfigured(cfg) && !remoteHostIsLoopback(parsed.Hostname()) && !cfg.AllowInsecureRemoteWebSocket {
		return remoteEndpoint{}, fmt.Errorf("refusing bearer auth over non-loopback ws:// remote app-server URL; use wss:// or explicitly allow insecure remote websocket auth")
	}

	return remoteEndpoint{
		kind:   remoteEndpointWebSocket,
		rawURL: rawURL,
	}, nil
}

func validateRemoteBearerConfig(cfg RemoteConfig) error {
	bearerTokenSet := cfg.BearerToken != ""
	bearerTokenFileSet := cfg.BearerTokenFile != ""
	if bearerTokenSet && strings.TrimSpace(cfg.BearerToken) == "" {
		return fmt.Errorf("remote app-server bearer token is empty")
	}
	if bearerTokenFileSet && strings.TrimSpace(cfg.BearerTokenFile) == "" {
		return fmt.Errorf("remote app-server bearer token file is empty")
	}
	if bearerTokenSet && bearerTokenFileSet {
		return fmt.Errorf("remote app-server bearer token and bearer token file are mutually exclusive")
	}
	return nil
}

func remoteBearerConfigured(cfg RemoteConfig) bool {
	return strings.TrimSpace(cfg.BearerToken) != "" || strings.TrimSpace(cfg.BearerTokenFile) != ""
}

func remoteHostIsLoopback(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func dialRemoteWebSocketURL(ctx context.Context, endpointURL string, cfg *WebSocketConfig) (*websocket.Conn, error) {
	token, err := websocketBearerToken(cfg)
	if err != nil {
		return nil, err
	}
	opts := &websocket.DialOptions{}
	if token != "" {
		opts.HTTPHeader = http.Header{
			"Authorization": {fmt.Sprintf("Bearer %s", token)},
		}
	}
	conn, resp, err := websocket.Dial(ctx, endpointURL, opts)
	if err != nil {
		return nil, websocketDialError("remote app-server websocket dial failed", resp, err)
	}
	return conn, nil
}
