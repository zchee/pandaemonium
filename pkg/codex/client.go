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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

const (
	sdkVersion       = "0.131.0a4-go"
	defaultListenURL = "stdio://"
)

// ApprovalHandler answers app-server requests initiated during JSON-RPC processing.
type ApprovalHandler func(method string, params jsontext.Value) (Object, error)

// WebSocketAuthMode selects the app-server WebSocket authentication mode.
type WebSocketAuthMode string

const (
	// WebSocketAuthNone disables websocket authentication flags.
	WebSocketAuthNone WebSocketAuthMode = ""
	// WebSocketAuthCapabilityToken configures capability-token websocket authentication.
	WebSocketAuthCapabilityToken WebSocketAuthMode = "capability-token"
	// WebSocketAuthSignedBearerToken configures signed-bearer-token websocket authentication.
	WebSocketAuthSignedBearerToken WebSocketAuthMode = "signed-bearer-token"
)

// ListenConfig controls the app-server listen endpoint and transport auth.
type ListenConfig struct {
	// URL is passed directly as the app-server listen endpoint.
	// An empty value means "stdio://".
	URL string
	// WebSocket enables websocket launch and auth configuration when URL is a ws
	// or wss endpoint.
	WebSocket *WebSocketConfig

	// AllowInsecureRemoteWebSocket disables the explicit localhost-only guard for
	// ws:// urls.
	AllowInsecureRemoteWebSocket bool
}

// WebSocketConfig carries websocket authentication and bearer configuration.
type WebSocketConfig struct {
	AuthMode              WebSocketAuthMode
	TokenFile             string
	TokenSHA256           string
	SharedSecretFile      string
	Issuer                string
	Audience              string
	MaxClockSkewSeconds   *int
	ClientBearerToken     string
	ClientBearerTokenFile string
	DialTimeout           time.Duration
}

// Config controls app-server process startup and client metadata.
// Defaults preserve stdio behavior unless Listen.URL is set explicitly.
type Config struct {
	CodexBin           string
	LaunchArgsOverride []string
	Listen             ListenConfig
	ConfigOverrides    []string
	Cwd                string
	Env                map[string]string
	ClientName         string
	ClientTitle        string
	ClientVersion      string
	ExperimentalAPI    *bool
}

// Client is a process-backed JSON-RPC v2 client for codex app-server over stdio.
type Client struct {
	config          Config
	approvalHandler ApprovalHandler

	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	stderr    io.ReadCloser
	transport transport
	cmdDone   chan error

	writeMu       sync.Mutex
	closeMu       sync.Mutex
	responseMu    sync.Mutex
	responses     map[string]chan responseWait
	notifications chan Notification
	turnRouter    *turnNotificationRouter
	stderrMu      sync.Mutex
	stderrLines   []string
	stderrDone    chan struct{}
	readDone      chan struct{}

	requestSeq atomic.Uint64
}

// NewClient creates a client. Call Start or use higher-level NewCodex to initialize it.
func NewClient(config *Config, approvalHandler ApprovalHandler) *Client {
	cfg := Config{}
	if config != nil {
		cfg = *config
	}
	if cfg.ClientName == "" {
		cfg.ClientName = "codex_go_sdk"
	}
	if cfg.ClientTitle == "" {
		cfg.ClientTitle = "Codex Go SDK"
	}
	if cfg.ClientVersion == "" {
		cfg.ClientVersion = sdkVersion
	}
	if approvalHandler == nil {
		approvalHandler = defaultApprovalHandler
	}
	return &Client{
		config:          cfg,
		approvalHandler: approvalHandler,
		responses:       map[string]chan responseWait{},
		notifications:   make(chan Notification, notificationQueueCapacity),
		turnRouter:      newTurnNotificationRouter(),
		stderrDone:      make(chan struct{}),
		readDone:        make(chan struct{}),
	}
}

// DefaultCodexHome returns the default ~/.codex home directory location.
func DefaultCodexHome() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), ".codex")
	}
	return filepath.Join(home, ".codex")
}

// Start launches the app-server process if it is not already running.
func (c *Client) Start(ctx context.Context) error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.transport != nil {
		return nil
	}
	args, err := c.launchArgs()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if c.config.Cwd != "" {
		cmd.Dir = c.config.Cwd
	}
	cmd.Env = os.Environ()
	for key, value := range c.config.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	c.stderrDone = make(chan struct{})
	c.readDone = make(chan struct{})
	c.notifications = make(chan Notification, notificationQueueCapacity)
	c.turnRouter = newTurnNotificationRouter()
	c.responses = map[string]chan responseWait{}
	var stderr io.ReadCloser
	listenCfg := c.effectiveListenConfig()
	listenURL := strings.TrimSpace(listenCfg.URL)
	if listenURL == "" {
		listenURL = defaultListenURL
	}
	isWebSocket := strings.HasPrefix(listenURL, "ws://") || strings.HasPrefix(listenURL, "wss://")
	if isWebSocket {
		stderr, err = cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("create app-server stderr: %w", err)
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start app-server: %w", err)
		}
		cmdDone := waitForCommand(cmd)
		go c.drainStderr(stderr, c.stderrDone)
		c.stderr = stderr
		conn, err := dialWebSocketWithWait(ctx, cmdDone, listenURL, listenCfg.WebSocket)
		if err != nil {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-cmdDone
			return fmt.Errorf("dial app-server websocket: %w", err)
		}
		c.cmd = cmd
		c.cmdDone = cmdDone
		c.transport = &websocketTransport{conn: conn}
	} else {
		var stdin io.WriteCloser
		var stdout io.ReadCloser
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("create app-server stdin: %w", err)
		}
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("create app-server stdout: %w", err)
		}
		stderr, err = cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("create app-server stderr: %w", err)
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start app-server: %w", err)
		}
		c.cmdDone = waitForCommand(cmd)
		c.cmd = cmd
		c.stderr = stderr
		c.stdin = stdin
		c.stdout = bufio.NewReader(stdout)
		c.transport = &stdioTransport{stdin: stdin, stdout: c.stdout}
		go c.drainStderr(stderr, c.stderrDone)
	}
	go c.readLoop(ctx, c.transport, c.readDone)
	return nil
}

// Close closes the transport and terminates the app-server process.
func (c *Client) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.transport == nil {
		return nil
	}
	cmd := c.cmd
	cmdDone := c.cmdDone
	transport := c.transport
	readDone := c.readDone
	stderrDone := c.stderrDone
	c.cmd = nil
	c.cmdDone = nil
	c.turnRouter.close(&TransportClosedError{Message: "app-server closed"})
	c.failPending(&TransportClosedError{Message: "app-server closed"})

	c.writeMu.Lock()
	c.transport = nil
	c.stdin = nil
	if transport != nil {
		_ = transport.Close()
	}
	c.writeMu.Unlock()

	if cmd != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		done := cmdDone
		if done == nil {
			done = waitForCommand(cmd)
		}
		select {
		case <-done:
			// Close initiated termination, so process exit status is not actionable.
		case <-time.After(2 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done
		}
	}
	select {
	case <-readDone:
	case <-time.After(500 * time.Millisecond):
	}
	select {
	case <-stderrDone:
	case <-time.After(500 * time.Millisecond):
	}
	return nil
}

// Initialize performs the initialize/initialized handshake.
func (c *Client) Initialize(ctx context.Context) (InitializeResponse, error) {
	experimentalAPI := c.experimentalAPI()
	clientTitle := c.config.ClientTitle
	params := InitializeParams{
		ClientInfo: ClientInfo{
			Name:    c.config.ClientName,
			Title:   &clientTitle,
			Version: c.config.ClientVersion,
		},
		Capabilities: &InitializeCapabilities{
			ExperimentalAPI: &experimentalAPI,
		},
	}
	resp, err := Request[InitializeResponse](ctx, c, RequestMethodInitialize, params)
	if err != nil {
		return InitializeResponse{}, err
	}
	if err := c.Notify(ctx, "initialized", nil); err != nil {
		return InitializeResponse{}, err
	}
	return validateInitialize(resp)
}

// Request sends a typed request to the app-server.
func Request[T any](ctx context.Context, c *Client, method string, params any) (T, error) {
	var zero T
	if c == nil {
		return zero, fmt.Errorf("codex client is nil")
	}
	raw, err := c.RequestRaw(ctx, method, params)
	if err != nil {
		return zero, err
	}
	return decodeRequestResult[T](method, raw)
}

func decodeRequestResult[T any](method string, raw jsontext.Value) (T, error) {
	var zero T
	if len(raw) == 0 || string(raw) == "null" {
		return zero, nil
	}
	var got T
	if method == RequestMethodAccountLoginStart {
		if target, ok := any(&got).(*LoginAccountResponse); ok {
			decoded, err := decodeGeneratedLoginAccountResponse(raw)
			if err != nil {
				return zero, fmt.Errorf("decode %s response: %w", method, err)
			}
			*target = decoded
			return got, nil
		}
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		return zero, fmt.Errorf("decode %s response: %w", method, err)
	}
	return got, nil
}

// RequestRaw sends a request and returns the raw result JSON.
func (c *Client) RequestRaw(ctx context.Context, method string, params any) (jsontext.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := c.nextRequestID()
	response := make(chan responseWait, 1)
	c.registerResponse(id, response)
	if err := c.writeMessage(ctx, Object{"id": id, "method": method, "params": paramsOrEmpty(params)}); err != nil {
		c.unregisterResponse(id)
		return nil, err
	}
	select {
	case <-ctx.Done():
		c.unregisterResponse(id)
		return nil, ctx.Err()
	case got := <-response:
		if got.err != nil {
			return nil, got.err
		}
		if got.msg.Error != nil {
			return nil, mapJSONRPCError(got.msg.Error.Code, got.msg.Error.Message, got.msg.Error.Data)
		}
		return got.msg.Result, nil
	}
}

// RequestWithRetryOnOverload sends a request and retries retryable overload responses.
func (c *Client) RequestWithRetryOnOverload(ctx context.Context, method string, params any, cfg RetryConfig) (jsontext.Value, error) {
	return RetryOnOverload(ctx, cfg, func() (jsontext.Value, error) {
		return c.RequestRaw(ctx, method, params)
	})
}

// RequestWithRetryOnOverload sends a typed request and retries retryable overload responses.
func RequestWithRetryOnOverload[T any](ctx context.Context, c *Client, method string, params any, cfg RetryConfig) (T, error) {
	var zero T
	if c == nil {
		return zero, fmt.Errorf("codex client is nil")
	}
	raw, err := c.RequestWithRetryOnOverload(ctx, method, params, cfg)
	if err != nil {
		return zero, err
	}
	return decodeRequestResult[T](method, raw)
}

// Notify sends a JSON-RPC notification to the app-server.
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.writeMessage(ctx, Object{"method": method, "params": params})
}

// NextNotification returns the next server notification exactly as received.
//
// The caller owns any decoding or routing decision for the returned payload.
// Unknown methods and future schema additions are preserved in the raw
// Notification so higher-level consumers can forward or inspect them without
// losing information.
func (c *Client) NextNotification(ctx context.Context) (Notification, error) {
	return c.turnRouter.nextGlobal(ctx, c.notifications)
}

// WaitForTurnCompleted waits for a matching turn/completed notification.
func (c *Client) WaitForTurnCompleted(ctx context.Context, turnID string) (TurnCompletedNotification, error) {
	if err := c.acquireTurnConsumer(turnID); err != nil {
		return TurnCompletedNotification{}, err
	}
	defer c.releaseTurnConsumer(turnID)
	for {
		notification, err := c.nextTurnNotification(ctx, turnID)
		if err != nil {
			return TurnCompletedNotification{}, err
		}
		completed, ok, err := notification.TurnCompleted()
		if err != nil {
			return TurnCompletedNotification{}, err
		}
		if !ok || completed.Turn.ID != turnID {
			continue
		}
		c.clearTurnPending(turnID)
		return completed, nil
	}
}

func (c *Client) openTurnConsumer(turnID string) (*turnNotificationQueue, error) {
	return c.turnRouter.register(turnID)
}

func (c *Client) acquireTurnConsumer(turnID string) error {
	_, err := c.openTurnConsumer(turnID)
	return err
}

// StreamUntilMethods blocks until one of the methods in methods is observed.
func (c *Client) StreamUntilMethods(ctx context.Context, methods ...string) ([]Notification, error) {
	if len(methods) == 0 {
		return nil, fmt.Errorf("stream until methods: no methods specified")
	}
	methodSet := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		methodSet[method] = struct{}{}
	}
	var notifications []Notification
	for {
		notification, err := c.NextNotification(ctx)
		if err != nil {
			return notifications, err
		}
		notifications = append(notifications, notification)
		if _, ok := methodSet[notification.Method]; ok {
			return notifications, nil
		}
	}
}

// StreamText starts a turn and yields agent message delta notifications.
func (c *Client) StreamText(ctx context.Context, threadID, text string, params *TurnStartParams) iter.Seq2[AgentMessageDeltaNotification, error] {
	started, err := c.TurnStart(ctx, threadID, text, params)
	if err != nil {
		return func(yield func(AgentMessageDeltaNotification, error) bool) {
			yield(AgentMessageDeltaNotification{}, err)
		}
	}
	expectedTurnID := started.Turn.ID
	return func(yield func(AgentMessageDeltaNotification, error) bool) {
		if err := c.acquireTurnConsumer(expectedTurnID); err != nil {
			yield(AgentMessageDeltaNotification{}, err)
			return
		}
		defer c.releaseTurnConsumer(expectedTurnID)
		for {
			notification, err := c.nextTurnNotification(ctx, expectedTurnID)
			if err != nil {
				yield(AgentMessageDeltaNotification{}, err)
				return
			}
			switch notification.Method {
			case NotificationMethodTurnCompleted:
				completed, ok, err := notification.TurnCompleted()
				if err != nil {
					yield(AgentMessageDeltaNotification{}, err)
					return
				}
				if ok && completed.Turn.ID == expectedTurnID {
					c.clearTurnPending(expectedTurnID)
					return
				}
			case NotificationMethodAgentMessageDelta:
				delta, ok, err := notification.AgentMessageDelta()
				if err != nil {
					yield(AgentMessageDeltaNotification{}, err)
					return
				}
				if !ok || delta.TurnID != expectedTurnID {
					continue
				}
				if !yield(delta, nil) {
					return
				}
			}
		}
	}
}

func (c *Client) nextTurnNotification(ctx context.Context, turnID string) (Notification, error) {
	return c.turnRouter.next(ctx, turnID)
}

func (c *Client) releaseTurnConsumer(turnID string) {
	c.turnRouter.unregister(turnID)
}

func (c *Client) clearTurnPending(turnID string) {
	c.turnRouter.clearPending(turnID)
}

func (c *Client) routeNotification(notification Notification) error {
	return c.turnRouter.route(notification)
}

func (c *Client) experimentalAPI() bool {
	if c.config.ExperimentalAPI == nil {
		return true
	}
	return *c.config.ExperimentalAPI
}

func (c *Client) launchArgs() ([]string, error) {
	if len(c.config.LaunchArgsOverride) > 0 {
		return slices.Clone(c.config.LaunchArgsOverride), nil
	}
	return c.appServerArgs(c.effectiveListenConfig())
}

func (c *Client) appServerArgs(listenCfg ListenConfig) ([]string, error) {
	return c.buildAppServerArgs(listenCfg)
}

func (c *Client) buildAppServerArgs(listenCfg ListenConfig) ([]string, error) {
	listenURL := strings.TrimSpace(listenCfg.URL)
	if listenURL == "" {
		listenURL = defaultListenURL
	}
	clientBearerSource := websocketHasClientBearerToken(listenCfg.WebSocket)

	parsed, err := url.Parse(listenURL)
	if err != nil {
		return nil, fmt.Errorf("invalid listen URL %q: %w", listenURL, err)
	}
	if parsed.Scheme == "ws" || parsed.Scheme == "wss" {
		if parsed.Host == "" {
			return nil, fmt.Errorf("websocket listen URL %q is missing host", listenURL)
		}
		if parsed.Port() == "0" {
			return nil, fmt.Errorf("websocket listen URL %q uses unsupported :0 port", listenURL)
		}
		if err := ensureWebSocketListenAllowed(parsed, listenCfg); err != nil {
			return nil, err
		}
		if err := validateWebSocketConfig(listenCfg.WebSocket, clientBearerSource); err != nil {
			return nil, err
		}
	}

	codexBin := c.config.CodexBin
	if codexBin == "" {
		path, err := exec.LookPath("codex")
		if err != nil {
			return nil, fmt.Errorf("locate codex binary: %w", err)
		}
		codexBin = path
	}
	if !filepath.IsAbs(codexBin) {
		resolved, err := exec.LookPath(codexBin)
		if err != nil {
			return nil, fmt.Errorf("locate codex binary %q: %w", codexBin, err)
		}
		codexBin = resolved
	}
	if _, err := os.Stat(codexBin); err != nil {
		return nil, fmt.Errorf("codex binary not found at %s: %w", codexBin, err)
	}
	args := []string{codexBin}
	for _, override := range c.config.ConfigOverrides {
		args = append(args, "--config", override)
	}
	if listenURL == defaultListenURL {
		args = append(args, "app-server", "--listen", defaultListenURL)
		return args, nil
	}
	args = append(args, "app-server", "--listen", listenURL)
	args = append(args, wsLaunchArgs(listenCfg.WebSocket)...)
	return args, nil
}

func waitForCommand(cmd *exec.Cmd) chan error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		close(done)
	}()
	return done
}

func (c *Client) effectiveListenConfig() ListenConfig {
	return c.config.Listen
}

func (c *Client) nextRequestID() string {
	return fmt.Sprintf("go-sdk-%d", c.requestSeq.Add(1))
}

func (c *Client) writeMessage(ctx context.Context, payload any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	line, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode JSON-RPC payload: %w", err)
	}
	if c.transport == nil {
		return &TransportClosedError{Message: "app-server is not running"}
	}
	return c.transport.WriteJSON(ctx, line)
}

func (c *Client) readMessage(ctx context.Context, t transport) (rpcMessage, error) {
	if t == nil {
		return rpcMessage{}, &TransportClosedError{Message: "app-server is not running"}
	}
	line, err := t.ReadJSON(ctx)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return rpcMessage{}, &TransportClosedError{Message: "app-server closed stdout. stderr_tail=" + c.stderrTail(40)}
		}
		return rpcMessage{}, err
	}
	var msg rpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return rpcMessage{}, &AppServerError{Message: fmt.Sprintf("invalid JSON-RPC line %q: %v", string(line), err)}
	}
	return msg, nil
}

func (c *Client) handleServerRequest(msg rpcMessage) Object {
	result, err := c.approvalHandler(msg.Method, msg.Params)
	if err != nil {
		return Object{"id": msg.ID, "error": Object{"code": -32603, "message": err.Error()}}
	}
	return Object{"id": msg.ID, "result": result}
}

func (c *Client) readLoop(ctx context.Context, t transport, done chan<- struct{}) {
	defer close(done)
	defer c.turnRouter.close(&TransportClosedError{Message: "app-server notification stream closed"})
	for {
		msg, err := c.readMessage(ctx, t)
		if err != nil {
			c.failPending(err)
			return
		}
		if msg.Method != "" && msg.ID != "" {
			response := c.handleServerRequest(msg)
			if err := c.writeMessage(ctx, response); err != nil {
				c.failPending(err)
				return
			}
			continue
		}
		if msg.Method != "" {
			notification := Notification{Method: msg.Method, Params: cloneRaw(msg.Params)}
			if err := c.routeNotification(notification); err != nil {
				c.failPending(err)
				return
			}
			continue
		}
		c.deliverResponse(msg)
	}
}

func (c *Client) registerResponse(id string, response chan responseWait) {
	c.responseMu.Lock()
	defer c.responseMu.Unlock()
	c.responses[id] = response
}

func (c *Client) unregisterResponse(id string) {
	c.responseMu.Lock()
	defer c.responseMu.Unlock()
	delete(c.responses, id)
}

func (c *Client) deliverResponse(msg rpcMessage) {
	c.responseMu.Lock()
	response := c.responses[msg.ID]
	delete(c.responses, msg.ID)
	c.responseMu.Unlock()
	if response != nil {
		response <- responseWait{msg: msg}
	}
}

func (c *Client) failPending(err error) {
	c.responseMu.Lock()
	responses := c.responses
	c.responses = map[string]chan responseWait{}
	c.responseMu.Unlock()
	for _, response := range responses {
		response <- responseWait{err: err}
	}
}

func (c *Client) drainStderr(stderr io.Reader, done chan<- struct{}) {
	defer close(done)
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		c.stderrMu.Lock()
		c.stderrLines = append(c.stderrLines, line)
		if len(c.stderrLines) > 400 {
			copy(c.stderrLines, c.stderrLines[len(c.stderrLines)-400:])
			c.stderrLines = c.stderrLines[:400]
		}
		c.stderrMu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		c.stderrMu.Lock()
		c.stderrLines = append(c.stderrLines, "stderr read error: "+err.Error())
		if len(c.stderrLines) > 400 {
			copy(c.stderrLines, c.stderrLines[len(c.stderrLines)-400:])
			c.stderrLines = c.stderrLines[:400]
		}
		c.stderrMu.Unlock()
	}
}

func (c *Client) stderrTail(limit int) string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	if limit > len(c.stderrLines) {
		limit = len(c.stderrLines)
	}
	return strings.Join(c.stderrLines[len(c.stderrLines)-limit:], "\n")
}

func defaultApprovalHandler(method string, _ jsontext.Value) (Object, error) {
	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		return Object{"decision": "accept"}, nil
	default:
		return Object{}, nil
	}
}

func paramsOrEmpty(params any) any {
	if params == nil {
		return Object{}
	}
	return params
}

type responseWait struct {
	msg rpcMessage
	err error
}

type rpcMessage struct {
	ID     string         `json:"id,omitzero"`
	Method string         `json:"method,omitzero"`
	Params jsontext.Value `json:"params,omitzero"`
	Result jsontext.Value `json:"result,omitzero"`
	Error  *rpcErrorBody  `json:"error,omitzero"`
}

type rpcErrorBody struct {
	Code    int64          `json:"code"`
	Message string         `json:"message"`
	Data    jsontext.Value `json:"data,omitzero"`
}

func cloneRaw(value jsontext.Value) jsontext.Value {
	if len(value) == 0 {
		return nil
	}
	out := make(jsontext.Value, len(value))
	copy(out, value)
	return out
}
