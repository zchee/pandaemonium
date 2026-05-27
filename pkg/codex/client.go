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
	"maps"
	"net/url"
	"os"
	"os/exec"
	"os/user"
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

var errUnsupportedServerMode = errors.New("unsupported codex server mode")

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

	// WebSocket configures websocket launch/auth for ws:// endpoints and dial
	// behavior for unix:// websocket endpoints. Unix sockets do not use
	// websocket auth fields.
	WebSocket *WebSocketConfig

	// AllowInsecureRemoteWebSocket disables the explicit localhost-only guard for
	// ws:// urls.
	AllowInsecureRemoteWebSocket bool
}

// WebSocketConfig carries TCP websocket authentication and shared dial options.
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

func (cfg WebSocketConfig) Is() bool {
	return websocketAuthFieldsSet(&cfg)
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
	ServerMode         ServerMode
	ClientName         string
	ClientTitle        string
	ClientVersion      string
	ExperimentalAPI    *bool
}

// Client is a process-backed JSON-RPC v2 client for codex app-server over stdio.
type Client struct {
	config          Config
	approvalHandler ApprovalHandler

	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       *bufio.Reader
	stdoutCloser io.Closer // raw stdout pipe; Close() closes this to unblock ReadJSON on ctx cancel
	stderr       io.ReadCloser
	transport    atomic.Pointer[Transport]
	cmdDone      chan error

	closeMu     sync.Mutex
	rpcState    *jsonRPCClientState
	turnRouter  *turnNotificationRouter
	stderrMu    sync.Mutex
	stderrLines []string
	stderrDone  chan struct{}
	readDone    chan struct{}
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
	if cfg.ServerMode == "" {
		cfg.ServerMode = ServerModeAppServer
	}
	if approvalHandler == nil {
		approvalHandler = defaultApprovalHandler
	}
	return &Client{
		config:          cfg,
		approvalHandler: approvalHandler,
		rpcState:        newJSONRPCClientState(),
		turnRouter:      newTurnNotificationRouter(),
		stderrDone:      make(chan struct{}),
		readDone:        make(chan struct{}),
	}
}

var userHomeDir = sync.OnceValues(func() (string, error) {
	return os.UserHomeDir()
})

// expandUser mimics Python's os.path.expanduser function.
// It replaces "~" or "~username" at the start of a path with the corresponding user's home directory.
// If expansion fails or the user is not found, it returns the original path unchanged.
func expandUser(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}

	// 1: Case: path is exactly "~"
	if path == "~" {
		home, err := userHomeDir()
		if err != nil {
			return path
		}
		return home
	}

	// 2: path starts with "~/" or "~\" (current user's home directory)
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := userHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}

	// 3: path starts with "~username" or "~username/path"
	sep := strings.IndexAny(path[1:], "/\\")
	var username string
	var rest string

	if sep == -1 {
		username = path[1:]
	} else {
		username = path[1 : 1+sep]
		rest = path[1+sep:]
	}

	u, err := user.Lookup(username)
	if err != nil {
		// If lookup fails, return the original path unchanged
		return path
	}

	return filepath.Clean(u.HomeDir + rest)
}

// HomeDir returns the default path to the codex home directory,
// which is used for config and other state by default.
//
// It can be overridden by setting the CODEX_HOME environment variable. If the
// user home directory cannot be determined, it falls back to a temporary directory.
func HomeDir() string {
	// fast path for tests and users who set CODEX_HOME explicitly
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		return expandUser(codexHome)
	}

	// fall back to user home directory or temp directory
	home, err := userHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), ".codex")
	}
	return filepath.Join(home, ".codex")
}

// Start launches the configured server process if it is not already running.
func (c *Client) Start(ctx context.Context) error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.loadTransport() != nil {
		return nil
	}

	args, err := c.launchArgs()
	if err != nil {
		return err
	}
	serverName := c.serverProcessNameForErrors()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if c.config.Cwd != "" {
		cmd.Dir = c.config.Cwd
	}
	effectiveEnv := c.effectiveEnv()
	cmd.Env = make([]string, 0, len(effectiveEnv))
	for key, value := range effectiveEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	c.stderrDone = make(chan struct{})
	c.readDone = make(chan struct{})
	c.turnRouter = newTurnNotificationRouter()
	c.rpcState = newJSONRPCClientState()

	var stderr io.ReadCloser
	listenCfg := c.effectiveListenConfig()
	listenURL := strings.TrimSpace(listenCfg.URL)
	if listenURL == "" {
		listenURL = defaultListenURL
	}
	kind, err := parseListenTransport(listenURL)
	if err != nil {
		return err
	}
	if err := validateListenConfig(listenCfg, kind, listenURL); err != nil {
		return err
	}
	switch kind {
	case listenTransportWebSocket, listenTransportUnixWebSocket:
		stderr, err = cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("create %s stderr: %w", serverName, err)
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start %s: %w", serverName, err)
		}
		cmdDone := waitForCommand(cmd)
		go c.drainStderr(stderr, c.stderrDone)
		c.stderr = stderr
		conn, err := dialWebSocketWithWait(ctx, cmdDone, listenURL, listenCfg.WebSocket, effectiveEnv, c.config.Cwd)
		if err != nil {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-cmdDone
			return fmt.Errorf("dial %s websocket: %w", serverName, err)
		}
		c.cmd = cmd
		c.cmdDone = cmdDone
		c.storeTransport(&websocketTransport{conn: conn})

	default:
		var stdin io.WriteCloser
		var stdout io.ReadCloser
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("create %s stdin: %w", serverName, err)
		}
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("create %s stdout: %w", serverName, err)
		}
		stderr, err = cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("create %s stderr: %w", serverName, err)
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start %s: %w", serverName, err)
		}
		c.cmdDone = waitForCommand(cmd)
		c.cmd = cmd
		c.stderr = stderr
		c.stdin = stdin
		c.stdout = bufio.NewReader(stdout)
		c.stdoutCloser = stdout
		c.storeTransport(&stdioTransport{stdin: stdin, stdout: c.stdout})
		go c.drainStderr(stderr, c.stderrDone)
	}

	go c.readLoop(ctx, c.loadTransport(), c.readDone)
	return nil
}

// Close closes the transport and terminates the app-server process.
func (c *Client) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.loadTransport() == nil {
		return nil
	}

	cmd := c.cmd
	cmdDone := c.cmdDone
	readDone := c.readDone
	stderrDone := c.stderrDone
	c.cmd = nil
	c.cmdDone = nil
	c.turnRouter.close(&TransportClosedError{Message: "app-server closed"})
	c.failPending(&TransportClosedError{Message: "app-server closed"})

	c.rpcState.lockWrite()
	transport := c.loadTransport()
	c.storeTransport(nil)
	c.stdin = nil
	stdoutCloser := c.stdoutCloser
	c.stdoutCloser = nil
	if transport != nil {
		_ = transport.Close()
	}
	c.rpcState.unlockWrite()

	if stdoutCloser != nil {
		_ = stdoutCloser.Close()
	}

	if cmd != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		done := cmdDone
		if done == nil {
			done = waitForCommand(cmd)
		}
		killTimer := time.NewTimer(2 * time.Second)
		select {
		case <-done:
			// Close initiated termination, so process exit status is not actionable.
			killTimer.Stop()
		case <-killTimer.C:
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done
		}
	}

	readTimer := time.NewTimer(500 * time.Millisecond)
	select {
	case <-readDone:
		readTimer.Stop()
	case <-readTimer.C:
	}

	stderrTimer := time.NewTimer(500 * time.Millisecond)
	select {
	case <-stderrDone:
		stderrTimer.Stop()
	case <-stderrTimer.C:
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
		Capabilities: InitializeCapabilities{
			ExperimentalAPI: experimentalAPI,
		},
	}
	resp, err := Request[InitializeResponse](ctx, c, RequestMethodInitialize, params)
	if err != nil {
		return InitializeResponse{}, err
	}

	if err := c.Notify(ctx, NotificationMethodInitialized, nil); err != nil {
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
	return c.rpcState.requestRaw(ctx, method, params, c.writeMessage)
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
	return c.rpcState.notify(ctx, method, params, c.writeMessage)
}

// NextNotification returns the next server notification exactly as received.
//
// The caller owns any decoding or routing decision for the returned payload.
// Unknown methods and future schema additions are preserved in the raw
// Notification so higher-level consumers can forward or inspect them without
// losing information.
func (c *Client) NextNotification(ctx context.Context) (Notification, error) {
	return c.turnRouter.nextGlobal(ctx)
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

// WaitForLoginCompleted waits for a matching account/login/completed notification.
func (c *Client) WaitForLoginCompleted(ctx context.Context, loginID string) (AccountLoginCompletedNotification, error) {
	if err := c.acquireLoginConsumer(loginID); err != nil {
		return AccountLoginCompletedNotification{}, err
	}
	defer c.releaseLoginConsumer(loginID)

	for {
		notification, err := c.nextLoginNotification(ctx, loginID)
		if err != nil {
			return AccountLoginCompletedNotification{}, err
		}

		completed, ok, err := notification.AccountLoginCompleted()
		if err != nil {
			return AccountLoginCompletedNotification{}, err
		}
		if !ok || completed.LoginID == nil || *completed.LoginID != loginID {
			continue
		}

		c.clearLoginPending(loginID)
		return completed, nil
	}
}

func (c *Client) openTurnConsumer(turnID string) (*notificationQueue, error) {
	return c.turnRouter.register(turnID)
}

func (c *Client) acquireTurnConsumer(turnID string) error {
	_, err := c.openTurnConsumer(turnID)
	return err
}

func (c *Client) openLoginConsumer(loginID string) (*notificationQueue, error) {
	return c.turnRouter.registerLogin(loginID)
}

func (c *Client) acquireLoginConsumer(loginID string) error {
	_, err := c.openLoginConsumer(loginID)
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

			case NotificationMethodItemAgentMessageDelta:
				delta, ok, err := notification.ItemAgentMessageDelta()
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

func (c *Client) nextLoginNotification(ctx context.Context, loginID string) (Notification, error) {
	return c.turnRouter.nextLogin(ctx, loginID)
}

func (c *Client) releaseTurnConsumer(turnID string) {
	c.turnRouter.unregister(turnID)
}

func (c *Client) releaseLoginConsumer(loginID string) {
	c.turnRouter.unregisterLogin(loginID)
}

func (c *Client) clearTurnPending(turnID string) {
	c.turnRouter.clearPending(turnID)
}

func (c *Client) clearLoginPending(loginID string) {
	c.turnRouter.clearLoginPending(loginID)
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
	return c.buildServerArgs(c.serverMode(), c.effectiveListenConfig())
}

func (c *Client) buildServerArgs(mode ServerMode, listenCfg ListenConfig) ([]string, error) {
	command, err := serverModeCommand(mode)
	if err != nil {
		return nil, err
	}
	return c.buildServerArgsForCommand(command, listenCfg)
}

func (c *Client) serverMode() ServerMode {
	if c.config.ServerMode == "" {
		return ServerModeAppServer
	}
	return c.config.ServerMode
}

func (c *Client) serverProcessNameForErrors() string {
	command, err := serverModeCommand(c.serverMode())
	if err != nil {
		return string(c.serverMode())
	}
	return command
}

func serverModeCommand(mode ServerMode) (string, error) {
	switch mode {
	case "", ServerModeAppServer:
		return string(ServerModeAppServer), nil
	case ServerModeExecServer:
		return string(ServerModeExecServer), nil
	default:
		return "", fmt.Errorf("%w %q", errUnsupportedServerMode, mode)
	}
}

func (c *Client) buildAppServerArgs(listenCfg ListenConfig) ([]string, error) {
	return c.buildServerArgsForCommand(string(ServerModeAppServer), listenCfg)
}

func (c *Client) buildServerArgsForCommand(command string, listenCfg ListenConfig) ([]string, error) {
	if command == "" {
		return nil, errors.New("codex server command is empty")
	}
	listenURL := strings.TrimSpace(listenCfg.URL)
	if listenURL == "" {
		listenURL = defaultListenURL
	}
	kind, err := parseListenTransport(listenURL)
	if err != nil {
		return nil, err
	}

	if err := validateListenConfig(listenCfg, kind, listenURL); err != nil {
		return nil, err
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
		args = append(args, command, "--listen", defaultListenURL)
		return args, nil
	}
	args = append(args, command, "--listen", listenURL)
	if kind == listenTransportWebSocket {
		args = append(args, wsLaunchArgs(listenCfg.WebSocket)...)
	}
	return args, nil
}

func (c *Client) initializeServer(ctx context.Context) (InitializeResponse, error) {
	switch c.config.ServerMode {
	case ServerModeExecServer:
		raw, err := c.RequestRaw(ctx, ExecServerInitializeMethod, &ExecServerInitializeParams{
			ClientName: c.config.ClientName,
		})
		if err != nil {
			return InitializeResponse{}, err
		}
		if _, err := decodeRequestResult[ExecServerInitializeResponse](ExecServerInitializeMethod, raw); err != nil {
			return InitializeResponse{}, err
		}

		var metadata InitializeResponse
		if len(raw) > 0 && string(raw) != "null" {
			var candidate InitializeResponse
			if err := json.Unmarshal(raw, &candidate); err != nil {
				return InitializeResponse{}, fmt.Errorf("decode %s metadata: %w", ExecServerInitializeMethod, err)
			}
			if strings.TrimSpace(candidate.UserAgent) != "" || candidate.ServerInfo != nil {
				metadata, err = validateInitialize(candidate)
				if err != nil {
					return InitializeResponse{}, err
				}
			}
		}

		if err := c.Notify(ctx, NotificationMethodInitialized, nil); err != nil {
			return InitializeResponse{}, err
		}
		return metadata, nil
	default:
		return c.Initialize(ctx)
	}
}

func validateListenConfig(listenCfg ListenConfig, kind listenTransportKind, listenURL string) error {
	switch kind {
	case listenTransportUnixWebSocket:
		return validateUnixWebSocketConfig(listenCfg.WebSocket)
	case listenTransportWebSocket:
		parsed, err := url.Parse(listenURL)
		if err != nil {
			return fmt.Errorf("invalid listen URL %q: %w", listenURL, err)
		}
		if parsed.Host == "" {
			return fmt.Errorf("websocket listen URL %q is missing host", listenURL)
		}
		if parsed.Port() == "0" {
			return fmt.Errorf("websocket listen URL %q uses unsupported :0 port", listenURL)
		}
		if err := ensureWebSocketListenAllowed(parsed, listenCfg); err != nil {
			return err
		}
		return validateWebSocketConfig(listenCfg.WebSocket, websocketHasClientBearerToken(listenCfg.WebSocket))
	default:
		return nil
	}
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

func (c *Client) effectiveEnv() map[string]string {
	env := make(map[string]string, len(os.Environ())+len(c.config.Env))
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		env[key] = value
	}
	maps.Copy(env, c.config.Env)
	return env
}

func (c *Client) loadTransport() Transport {
	p := c.transport.Load()
	if p == nil {
		return nil
	}
	return *p
}

func (c *Client) storeTransport(t Transport) {
	if t == nil {
		c.transport.Store(nil)
		return
	}
	c.transport.Store(&t)
}

func (c *Client) writeMessage(ctx context.Context, payload any) error {
	c.rpcState.lockWrite()
	defer c.rpcState.unlockWrite()

	line, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode JSON-RPC payload: %w", err)
	}

	t := c.loadTransport()
	if t == nil {
		return &TransportClosedError{Message: "app-server is not running"}
	}
	return t.WriteJSON(ctx, line)
}

func (c *Client) readMessage(ctx context.Context, t Transport) (rpcMessage, error) {
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

func (c *Client) readLoop(ctx context.Context, t Transport, done chan<- struct{}) {
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
	c.rpcState.registerResponse(id, response)
}

func (c *Client) unregisterResponse(id string) {
	c.rpcState.unregisterResponse(id)
}

func (c *Client) deliverResponse(msg rpcMessage) {
	c.rpcState.deliverResponse(msg)
}

func (c *Client) failPending(err error) {
	c.rpcState.failPending(err)
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
