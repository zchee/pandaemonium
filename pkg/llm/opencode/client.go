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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-json-experiment/json"

	"github.com/zchee/pandaemonium/pkg/llm"
)

const (
	defaultHostname    = "127.0.0.1"
	defaultDialTimeout = 30 * time.Second
	// defaultDrainWindow bounds how long a turn keeps draining session events
	// after the prompt goroutine has returned, waiting for the session-scoped
	// terminal event. A missing terminal event ends the stream cleanly and
	// increments Counters.StreamsWithoutTerminal instead of hanging.
	defaultDrainWindow = 2 * time.Second
	// maxResponseBody caps decoded HTTP response bodies (defense against a
	// misbehaving endpoint; the largest legitimate response is a full message
	// listing well under this bound).
	maxResponseBody = 64 << 20
	// closeReapTimeout bounds how long Close waits for turn goroutines after
	// canceling the client lifetime context.
	closeReapTimeout = 10 * time.Second
)

var errClientClosed = errors.New("opencode client is closed")

// Config controls `opencode serve` process startup and client behavior.
type Config struct {
	// OpencodeBin is the opencode binary to spawn. Empty means "opencode"
	// resolved via PATH.
	OpencodeBin string

	// Hostname is the listen host passed to `opencode serve --hostname`.
	// Empty means 127.0.0.1. The spawned server is loopback-only by default.
	Hostname string

	// Port is passed to `opencode serve --port`. Zero requests an
	// OS-assigned port; the bound port is always read back from the server's
	// announce line because a config file's server.port overrides the flag
	// default (verified against opencode 1.18.3).
	Port int

	// Password enables HTTP basic auth. It is handed to the child via the
	// OPENCODE_SERVER_PASSWORD environment variable only — never argv — and
	// is attached to requests via the Authorization header only.
	Password string

	// Cwd is the child working directory (the project the server manages).
	Cwd string

	// Env is merged over the inherited environment of the spawned server.
	Env map[string]string

	// Retry bounds HTTP overload retries (RetryOnOverload) and SSE bus
	// auto-reconnect attempts.
	Retry llm.RetryConfig

	// DialTimeout bounds server readiness (announce line + health) and the
	// event bus dial (HTTP 200 + first server.connected). Zero means 30s.
	DialTimeout time.Duration

	// DrainWindow bounds the post-prompt event drain while a turn waits for
	// its session-scoped terminal event. Zero means 2s.
	DrainWindow time.Duration

	// PermissionAuto selects the v1 permission policy applied by the
	// client-lifetime permission consumer: reply "once" to every
	// permission.asked event when true, "reject" when false. The wrapper
	// always responds so a permission-gated tool call can never stall a turn.
	PermissionAuto bool

	// HTTPClient optionally supplies the base HTTP client (its Transport is
	// wrapped with basic auth; its Timeout is cleared because the same client
	// serves the long-lived SSE stream). Nil means a fresh client on
	// http.DefaultTransport.
	HTTPClient *http.Client
}

// setDefaults replaces zero-valued fields with their defaults.
func (cfg *Config) setDefaults() {
	if cfg.OpencodeBin == "" {
		cfg.OpencodeBin = "opencode"
	}
	if cfg.Hostname == "" {
		cfg.Hostname = defaultHostname
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = defaultDialTimeout
	}
	if cfg.DrainWindow == 0 {
		cfg.DrainWindow = defaultDrainWindow
	}
}

// CountersSnapshot is a point-in-time copy of the client's observability
// counters.
type CountersSnapshot struct {
	SSEReconnects             uint64
	GapNotifications          uint64
	StreamsWithoutTerminal    uint64
	UnattributedSessionErrors uint64
	PermissionsAutoApproved   uint64
	PermissionsRejected       uint64
}

// counters holds the client's atomic observability counters.
type counters struct {
	sseReconnects             atomic.Uint64
	gapNotifications          atomic.Uint64
	streamsWithoutTerminal    atomic.Uint64
	unattributedSessionErrors atomic.Uint64
	permissionsAutoApproved   atomic.Uint64
	permissionsRejected       atomic.Uint64
}

func (c *counters) snapshot() CountersSnapshot {
	return CountersSnapshot{
		SSEReconnects:             c.sseReconnects.Load(),
		GapNotifications:          c.gapNotifications.Load(),
		StreamsWithoutTerminal:    c.streamsWithoutTerminal.Load(),
		UnattributedSessionErrors: c.unattributedSessionErrors.Load(),
		PermissionsAutoApproved:   c.permissionsAutoApproved.Load(),
		PermissionsRejected:       c.permissionsRejected.Load(),
	}
}

// Client is a typed HTTP + SSE client for one OpenCode server. It optionally
// owns a spawned `opencode serve` process (Start) or attaches to a running
// server (NewRemoteClient). Client is safe for concurrent use.
type Client struct {
	config     Config
	httpClient *http.Client

	// lifetime outlives every caller context: prompt goroutines and the SSE
	// bus bind to it so async work survives a canceled caller ctx and stops
	// on Close.
	lifetime       context.Context //nolint:containedctx // client-lifetime binding is the design: turn goroutines must outlive caller contexts and stop on Close.
	lifetimeCancel context.CancelCauseFunc

	counters counters
	turnWG   sync.WaitGroup

	mu          sync.Mutex
	baseURL     string // http://host:port, no trailing slash; set by Start/attach
	proc        *serverProcess
	bus         *eventBus
	activeTurns map[string]struct{} // sessionID set: one active turn per session
	closed      bool
}

// NewClient creates a client that will spawn `opencode serve` on Start. Use
// NewOpencode for the high-level facade, or NewRemoteClient to attach to a
// running server without spawning.
func NewClient(config *Config) *Client {
	cfg := Config{}
	if config != nil {
		cfg = *config
	}
	cfg.setDefaults()

	lifetime, cancel := context.WithCancelCause(context.Background())
	return &Client{
		config:         cfg,
		httpClient:     newAuthHTTPClient(cfg.HTTPClient, cfg.Password),
		lifetime:       lifetime,
		lifetimeCancel: cancel,
		activeTurns:    map[string]struct{}{},
	}
}

// Start spawns `opencode serve`, parses the announced base URL, and confirms
// readiness via GET /global/health. It is a no-op error to Start twice or to
// Start a remote-attached client.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errClientClosed
	}
	if c.baseURL != "" {
		c.mu.Unlock()
		return errors.New("opencode client already started")
	}
	c.mu.Unlock()

	proc, baseURL, err := startServerProcess(ctx, &c.config)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.proc = proc
	c.baseURL = baseURL
	c.mu.Unlock()

	healthCtx, cancel := context.WithTimeout(ctx, c.config.DialTimeout)
	defer cancel()
	if _, err := c.waitHealthy(healthCtx); err != nil {
		_ = c.Close()
		return err
	}
	return nil
}

// BaseURL returns the server base URL, or "" before Start/attach.
func (c *Client) BaseURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL
}

// Counters returns a snapshot of the client's observability counters.
func (c *Client) Counters() CountersSnapshot {
	return c.counters.snapshot()
}

// waitHealthy polls GET /global/health until it succeeds or ctx expires.
func (c *Client) waitHealthy(ctx context.Context) (Health, error) {
	var lastErr error
	for {
		health, err := c.Health(ctx)
		if err == nil {
			return health, nil
		}
		lastErr = err

		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Health{}, fmt.Errorf("opencode server did not become healthy: %w (last error: %w)", ctx.Err(), lastErr)
		case <-timer.C:
		}
	}
}

// Close shuts the client down in the documented order: refuse new turns,
// cancel the client lifetime context (aborting in-flight prompt requests),
// close the event bus, reap turn goroutines with a bounded wait, then
// terminate the spawned server (if any) and drain its pipes.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	bus := c.bus
	proc := c.proc
	c.mu.Unlock()

	c.lifetimeCancel(errClientClosed)
	if bus != nil {
		bus.close(&TransportClosedError{Message: "opencode event bus closed: client closed"})
	}

	reaped := make(chan struct{})
	go func() {
		c.turnWG.Wait()
		close(reaped)
	}()
	timer := time.NewTimer(closeReapTimeout)
	select {
	case <-reaped:
		timer.Stop()
	case <-timer.C:
	}

	if proc != nil {
		return proc.Close()
	}
	return nil
}

// goWork runs fn on a tracked goroutine unless the client is already
// closing, and reports whether fn was accepted. Checking closed under mu
// orders every WaitGroup Add strictly before Close's Wait (Close flips
// closed inside the same mutex before waiting), eliminating the
// Add-during-Wait panic window.
func (c *Client) goWork(fn func()) bool {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	c.turnWG.Add(1)
	c.mu.Unlock()
	go func() {
		defer c.turnWG.Done()
		fn()
	}()
	return true
}

// beginTurn reserves the one-active-turn slot for sessionID.
func (c *Client) beginTurn(sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errClientClosed
	}
	if _, active := c.activeTurns[sessionID]; active {
		return fmt.Errorf("opencode: session %s already has an active turn", sessionID)
	}
	c.activeTurns[sessionID] = struct{}{}
	return nil
}

// endTurn releases the one-active-turn slot for sessionID.
func (c *Client) endTurn(sessionID string) {
	c.mu.Lock()
	delete(c.activeTurns, sessionID)
	c.mu.Unlock()
}

// doRequest performs one JSON round-trip against the server. in is marshaled
// as the request body when non-nil; a non-2xx response is mapped through
// mapHTTPError. path must start with "/" and contain pre-escaped segments.
func doRequest[T any](ctx context.Context, c *Client, method, path string, in any) (T, error) {
	var zero T

	c.mu.Lock()
	baseURL := c.baseURL
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return zero, errClientClosed
	}
	if baseURL == "" {
		return zero, errors.New("opencode client is not started")
	}

	var body io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return zero, fmt.Errorf("opencode: marshal %s %s request: %w", method, path, err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return zero, fmt.Errorf("opencode: build %s %s request: %w", method, path, err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("opencode: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return zero, fmt.Errorf("opencode: read %s %s response: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return zero, mapHTTPError(resp.StatusCode, method, path, payload)
	}

	if err := json.Unmarshal(payload, &zero); err != nil {
		return zero, fmt.Errorf("opencode: decode %s %s response: %w", method, path, err)
	}
	return zero, nil
}

// sessionPath builds "/session/{id}" + suffix with the id path-escaped.
func sessionPath(sessionID, suffix string) string {
	return "/session/" + url.PathEscape(sessionID) + suffix
}

// Health fetches GET /global/health.
func (c *Client) Health(ctx context.Context) (Health, error) {
	return doRequest[Health](ctx, c, http.MethodGet, "/global/health", nil)
}

// SessionNew creates a session (POST /session).
func (c *Client) SessionNew(ctx context.Context, params *SessionNewParams) (SessionInfo, error) {
	if params == nil {
		params = &SessionNewParams{}
	}
	return doRequest[SessionInfo](ctx, c, http.MethodPost, "/session", params)
}

// SessionGet fetches a session (GET /session/{id}).
func (c *Client) SessionGet(ctx context.Context, sessionID string) (SessionInfo, error) {
	return doRequest[SessionInfo](ctx, c, http.MethodGet, sessionPath(sessionID, ""), nil)
}

// SessionList lists sessions (GET /session).
func (c *Client) SessionList(ctx context.Context) ([]SessionInfo, error) {
	return doRequest[[]SessionInfo](ctx, c, http.MethodGet, "/session", nil)
}

// SessionDelete deletes a session (DELETE /session/{id}).
func (c *Client) SessionDelete(ctx context.Context, sessionID string) (bool, error) {
	return doRequest[bool](ctx, c, http.MethodDelete, sessionPath(sessionID, ""), nil)
}

// SessionUpdate updates session attributes (PATCH /session/{id}).
func (c *Client) SessionUpdate(ctx context.Context, sessionID string, params *SessionUpdateParams) (SessionInfo, error) {
	if params == nil {
		params = &SessionUpdateParams{}
	}
	return doRequest[SessionInfo](ctx, c, http.MethodPatch, sessionPath(sessionID, ""), params)
}

// SessionFork forks a session (POST /session/{id}/fork), optionally at a
// specific message.
func (c *Client) SessionFork(ctx context.Context, sessionID string, params *SessionForkParams) (SessionInfo, error) {
	if params == nil {
		params = &SessionForkParams{}
	}
	return doRequest[SessionInfo](ctx, c, http.MethodPost, sessionPath(sessionID, "/fork"), params)
}

// SessionMessages lists a session's messages with parts
// (GET /session/{id}/message).
func (c *Client) SessionMessages(ctx context.Context, sessionID string) ([]MessageWithParts, error) {
	return doRequest[[]MessageWithParts](ctx, c, http.MethodGet, sessionPath(sessionID, "/message"), nil)
}

// Prompt sends a prompt and blocks until the turn completes
// (POST /session/{id}/message). Most callers want Session.Run or
// Session.Turn instead.
func (c *Client) Prompt(ctx context.Context, sessionID string, params *PromptParams) (PromptResponse, error) {
	if params == nil {
		return PromptResponse{}, errors.New("opencode: prompt params are required")
	}
	return doRequest[PromptResponse](ctx, c, http.MethodPost, sessionPath(sessionID, "/message"), params)
}

// Command runs a configured slash command (POST /session/{id}/command).
func (c *Client) Command(ctx context.Context, sessionID string, params *CommandParams) (PromptResponse, error) {
	if params == nil {
		return PromptResponse{}, errors.New("opencode: command params are required")
	}
	return doRequest[PromptResponse](ctx, c, http.MethodPost, sessionPath(sessionID, "/command"), params)
}

// Shell runs a shell command in the session context
// (POST /session/{id}/shell).
func (c *Client) Shell(ctx context.Context, sessionID string, params *ShellParams) (MessageWithParts, error) {
	if params == nil {
		return MessageWithParts{}, errors.New("opencode: shell params are required")
	}
	return doRequest[MessageWithParts](ctx, c, http.MethodPost, sessionPath(sessionID, "/shell"), params)
}

// Abort interrupts the session's active turn (POST /session/{id}/abort).
func (c *Client) Abort(ctx context.Context, sessionID string) (bool, error) {
	return doRequest[bool](ctx, c, http.MethodPost, sessionPath(sessionID, "/abort"), nil)
}

// Summarize compacts the session history (POST /session/{id}/summarize).
// The server requires ProviderID and ModelID.
func (c *Client) Summarize(ctx context.Context, sessionID string, params *SummarizeParams) (bool, error) {
	if params == nil {
		return false, errors.New("opencode: summarize params are required")
	}
	return doRequest[bool](ctx, c, http.MethodPost, sessionPath(sessionID, "/summarize"), params)
}

// Revert stages a revert to a message (POST /session/{id}/revert).
func (c *Client) Revert(ctx context.Context, sessionID string, params *RevertParams) (SessionInfo, error) {
	if params == nil {
		return SessionInfo{}, errors.New("opencode: revert params are required")
	}
	return doRequest[SessionInfo](ctx, c, http.MethodPost, sessionPath(sessionID, "/revert"), params)
}

// Unrevert clears a staged revert (POST /session/{id}/unrevert).
func (c *Client) Unrevert(ctx context.Context, sessionID string) (SessionInfo, error) {
	return doRequest[SessionInfo](ctx, c, http.MethodPost, sessionPath(sessionID, "/unrevert"), nil)
}

// Share publishes the session (POST /session/{id}/share).
func (c *Client) Share(ctx context.Context, sessionID string) (SessionInfo, error) {
	return doRequest[SessionInfo](ctx, c, http.MethodPost, sessionPath(sessionID, "/share"), nil)
}

// Unshare unpublishes the session (DELETE /session/{id}/share).
func (c *Client) Unshare(ctx context.Context, sessionID string) (SessionInfo, error) {
	return doRequest[SessionInfo](ctx, c, http.MethodDelete, sessionPath(sessionID, "/share"), nil)
}

// Providers lists configured providers and default models
// (GET /config/providers).
func (c *Client) Providers(ctx context.Context) (ProvidersResponse, error) {
	return doRequest[ProvidersResponse](ctx, c, http.MethodGet, "/config/providers", nil)
}

// PermissionRespond replies to a permission request
// (POST /session/{id}/permissions/{permissionID}).
func (c *Client) PermissionRespond(ctx context.Context, sessionID, permissionID string, response PermissionResponse) (bool, error) {
	body := struct {
		Response PermissionResponse `json:"response"`
	}{Response: response}
	return doRequest[bool](ctx, c, http.MethodPost, sessionPath(sessionID, "/permissions/"+url.PathEscape(permissionID)), body)
}

// PermissionReplyV2 replies to a v2 permission request
// (POST /permission/{requestID}/reply).
func (c *Client) PermissionReplyV2(ctx context.Context, requestID string, reply PermissionResponse) (bool, error) {
	body := struct {
		Reply PermissionResponse `json:"reply"`
	}{Reply: reply}
	return doRequest[bool](ctx, c, http.MethodPost, "/permission/"+url.PathEscape(requestID)+"/reply", body)
}
