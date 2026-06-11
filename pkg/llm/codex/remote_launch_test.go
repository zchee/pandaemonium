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
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"
)

func TestBuildRemoteAppServerArgs(t *testing.T) {
	const serverBin = "/opt/local/codex/bin/codex-app-server"
	const codexBin = "/opt/local/codex/bin/codex"
	disabled := false
	enabled := true

	tests := map[string]struct {
		cfg       *RemoteAppServerConfig
		bin       string
		endpoint  string
		useSubcmd bool
		want      []string
	}{
		"success: standalone default enables remote-control before listen": {
			cfg:      &RemoteAppServerConfig{},
			bin:      serverBin,
			endpoint: "ws://127.0.0.1:49815",
			want: []string{
				serverBin, "--remote-control", "--listen", "ws://127.0.0.1:49815",
			},
		},
		"success: standalone nil RemoteControl is treated as enabled": {
			cfg:      &RemoteAppServerConfig{RemoteControl: nil},
			bin:      serverBin,
			endpoint: "unix:///tmp/app.sock",
			want: []string{
				serverBin, "--remote-control", "--listen", "unix:///tmp/app.sock",
			},
		},
		"success: explicit true RemoteControl keeps the flag": {
			cfg:      &RemoteAppServerConfig{RemoteControl: &enabled},
			bin:      serverBin,
			endpoint: "unix://",
			want: []string{
				serverBin, "--remote-control", "--listen", "unix://",
			},
		},
		"success: RemoteControl pointing at false omits the flag": {
			cfg:      &RemoteAppServerConfig{RemoteControl: &disabled},
			bin:      serverBin,
			endpoint: "ws://127.0.0.1:49815",
			want: []string{
				serverBin, "--listen", "ws://127.0.0.1:49815",
			},
		},
		"success: config overrides precede listen on standalone binary": {
			cfg: &RemoteAppServerConfig{
				ConfigOverrides: []string{"sandbox_mode=danger-full-access", "model=o3"},
			},
			bin:      serverBin,
			endpoint: "ws://127.0.0.1:49815",
			want: []string{
				serverBin, "--remote-control",
				"--config", "sandbox_mode=danger-full-access",
				"--config", "model=o3",
				"--listen", "ws://127.0.0.1:49815",
			},
		},
		"success: websocket auth args follow the listen URL": {
			cfg: &RemoteAppServerConfig{
				Listen: ListenConfig{
					WebSocket: &WebSocketConfig{
						AuthMode:  WebSocketAuthCapabilityToken,
						TokenFile: "/tmp/cap.token",
					},
				},
			},
			bin:      serverBin,
			endpoint: "ws://127.0.0.1:49815",
			want: []string{
				serverBin, "--remote-control", "--listen", "ws://127.0.0.1:49815",
				"--ws-auth", "capability-token", "--ws-token-file", "/tmp/cap.token",
			},
		},
		"success: unix endpoint omits websocket auth args even when set": {
			cfg: &RemoteAppServerConfig{
				Listen: ListenConfig{
					WebSocket: &WebSocketConfig{
						AuthMode:  WebSocketAuthCapabilityToken,
						TokenFile: "/tmp/cap.token",
					},
				},
			},
			bin:      serverBin,
			endpoint: "unix:///tmp/app.sock",
			want: []string{
				serverBin, "--remote-control", "--listen", "unix:///tmp/app.sock",
			},
		},
		"success: extra args are appended verbatim last": {
			cfg: &RemoteAppServerConfig{
				ExtraArgs: []string{"--session-source", "exec"},
			},
			bin:      serverBin,
			endpoint: "ws://127.0.0.1:49815",
			want: []string{
				serverBin, "--remote-control", "--listen", "ws://127.0.0.1:49815",
				"--session-source", "exec",
			},
		},
		"success: subcommand fallback places config before app-server and flag after": {
			cfg: &RemoteAppServerConfig{
				ConfigOverrides: []string{"model=o3"},
			},
			bin:       codexBin,
			endpoint:  "ws://127.0.0.1:49815",
			useSubcmd: true,
			want: []string{
				codexBin, "--config", "model=o3", "app-server", "--remote-control",
				"--listen", "ws://127.0.0.1:49815",
			},
		},
		"success: subcommand fallback omits flag when RemoteControl is false": {
			cfg:       &RemoteAppServerConfig{RemoteControl: &disabled},
			bin:       codexBin,
			endpoint:  "unix:///tmp/app.sock",
			useSubcmd: true,
			want: []string{
				codexBin, "app-server", "--listen", "unix:///tmp/app.sock",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := buildRemoteAppServerArgs(tt.cfg, tt.bin, tt.endpoint, tt.useSubcmd)
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("buildRemoteAppServerArgs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveRemoteEndpointRejectsNonRemoteListen(t *testing.T) {
	tests := map[string]struct {
		listenURL string
		wantErrIs error
		wantErr   string
	}{
		"error: empty listen URL requires a remote endpoint": {
			listenURL: "",
			wantErrIs: errRemoteListenRequired,
		},
		"error: stdio scheme cannot accept remote attachments": {
			listenURL: "stdio://",
			wantErrIs: errRemoteListenRequired,
		},
		"error: bare stdio token is rejected": {
			listenURL: "stdio",
			wantErrIs: errRemoteListenRequired,
		},
		"error: off disables the transport": {
			listenURL: "off",
			wantErrIs: errRemoteListenRequired,
		},
		"error: websocket :0 port is unsupported": {
			listenURL: "ws://127.0.0.1:0",
			wantErr:   "unsupported :0 port",
		},
		"error: non-loopback websocket without opt-in is rejected": {
			listenURL: "ws://0.0.0.0:8123",
			wantErr:   "allowed only for loopback hosts",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := &RemoteAppServerConfig{Listen: ListenConfig{URL: tt.listenURL}}
			_, _, _, err := resolveRemoteEndpoint(cfg)
			if err == nil {
				t.Fatalf("resolveRemoteEndpoint(%q) error = nil, want rejection", tt.listenURL)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Fatalf("resolveRemoteEndpoint(%q) error = %v, want %v", tt.listenURL, err, tt.wantErrIs)
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("resolveRemoteEndpoint(%q) error = %v, want contain %q", tt.listenURL, err, tt.wantErr)
			}
		})
	}
}

func TestResolveRemoteEndpointAcceptsValidListen(t *testing.T) {
	codexHome := filepath.Join(shortTempDir(t), "codex-home")

	tests := map[string]struct {
		cfg            *RemoteAppServerConfig
		wantEndpoint   string
		wantKind       listenTransportKind
		wantSocketPath string
	}{
		"success: loopback websocket resolves to tcp probe with empty socket path": {
			cfg:          &RemoteAppServerConfig{Listen: ListenConfig{URL: "ws://127.0.0.1:49815"}},
			wantEndpoint: "ws://127.0.0.1:49815",
			wantKind:     listenTransportWebSocket,
		},
		"success: non-loopback websocket allowed with explicit opt-in": {
			cfg: &RemoteAppServerConfig{Listen: ListenConfig{
				URL:                          "ws://10.0.0.5:8123",
				AllowInsecureRemoteWebSocket: true,
			}},
			wantEndpoint: "ws://10.0.0.5:8123",
			wantKind:     listenTransportWebSocket,
		},
		"success: custom unix path resolves to that socket path": {
			cfg:            &RemoteAppServerConfig{Listen: ListenConfig{URL: "unix:///tmp/codex-remote.sock"}},
			wantEndpoint:   "unix:///tmp/codex-remote.sock",
			wantKind:       listenTransportUnixWebSocket,
			wantSocketPath: filepath.Clean("/tmp/codex-remote.sock"),
		},
		"success: default unix socket resolves under CODEX_HOME from env": {
			cfg: &RemoteAppServerConfig{
				Listen: ListenConfig{URL: "unix://"},
				Env:    map[string]string{"CODEX_HOME": codexHome},
			},
			wantEndpoint:   "unix://",
			wantKind:       listenTransportUnixWebSocket,
			wantSocketPath: filepath.Join(codexHome, "app-server-control", "app-server-control.sock"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			endpoint, kind, socketPath, err := resolveRemoteEndpoint(tt.cfg)
			if err != nil {
				t.Fatalf("resolveRemoteEndpoint() error = %v", err)
			}
			if endpoint != tt.wantEndpoint {
				t.Fatalf("endpoint = %q, want %q", endpoint, tt.wantEndpoint)
			}
			if kind != tt.wantKind {
				t.Fatalf("kind = %v, want %v", kind, tt.wantKind)
			}
			if socketPath != tt.wantSocketPath {
				t.Fatalf("socketPath = %q, want %q", socketPath, tt.wantSocketPath)
			}
		})
	}
}

func TestResolveRemoteEndpointRejectsOverlongUnixPath(t *testing.T) {
	longSocket := "/tmp/" + strings.Repeat("p", unixSocketPathLimit) + ".sock"
	cfg := &RemoteAppServerConfig{Listen: ListenConfig{URL: "unix://" + longSocket}}

	_, _, _, err := resolveRemoteEndpoint(cfg)
	if err == nil {
		t.Fatal("resolveRemoteEndpoint() error = nil, want unix socket path length rejection")
	}
	for _, want := range []string{"exceeding", "sun_path", "104"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("resolveRemoteEndpoint() error = %v, want contain %q", err, want)
		}
	}
}

func TestRemoteEndpointHasExplicitPath(t *testing.T) {
	tests := map[string]struct {
		listenURL string
		want      bool
	}{
		"success: bare default control socket is not explicit": {
			listenURL: "unix://",
			want:      false,
		},
		"success: absolute custom path is explicit": {
			listenURL: "unix:///tmp/app.sock",
			want:      true,
		},
		"success: relative custom path is explicit": {
			listenURL: "unix://relative/app.sock",
			want:      true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := remoteEndpointHasExplicitPath(tt.listenURL); got != tt.want {
				t.Fatalf("remoteEndpointHasExplicitPath(%q) = %v, want %v", tt.listenURL, got, tt.want)
			}
		})
	}
}

func TestRemoteHostPort(t *testing.T) {
	tests := map[string]struct {
		endpoint string
		want     string
	}{
		"success: loopback host and port": {
			endpoint: "ws://127.0.0.1:49815",
			want:     "127.0.0.1:49815",
		},
		"success: trailing path is stripped": {
			endpoint: "ws://127.0.0.1:49815/",
			want:     "127.0.0.1:49815",
		},
		"success: ipv6 host preserved": {
			endpoint: "ws://[::1]:8123",
			want:     "[::1]:8123",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := remoteHostPort(tt.endpoint); got != tt.want {
				t.Fatalf("remoteHostPort(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestRemoteAppServerCodexCommandArgv(t *testing.T) {
	const codexBin = "/opt/local/codex/bin/codex"

	tests := map[string]struct {
		server  *RemoteAppServer
		args    []string
		want    []string
		wantErr bool
	}{
		"success: resume last injects single --remote before caller args": {
			server: &RemoteAppServer{codexBin: codexBin, endpoint: "unix:///tmp/app.sock"},
			args:   []string{"resume", "--last"},
			want:   []string{codexBin, "--remote=unix:///tmp/app.sock", "resume", "--last"},
		},
		"success: no caller args yields only the remote flag": {
			server: &RemoteAppServer{codexBin: codexBin, endpoint: "ws://127.0.0.1:49815"},
			args:   nil,
			want:   []string{codexBin, "--remote=ws://127.0.0.1:49815"},
		},
		"success: bearer token wires remote-auth-token-env after remote flag": {
			server: &RemoteAppServer{codexBin: codexBin, endpoint: "ws://127.0.0.1:49815", bearerToken: "jwt-token-should-not-leak"},
			args:   []string{"exec", "hello"},
			want: []string{
				codexBin, "--remote=ws://127.0.0.1:49815",
				"--remote-auth-token-env", "CODEX_REMOTE_AUTH_TOKEN",
				"exec", "hello",
			},
		},
		"error: caller args containing --remote are rejected": {
			server:  &RemoteAppServer{codexBin: codexBin, endpoint: "ws://127.0.0.1:49815"},
			args:    []string{"--remote", "ws://other"},
			wantErr: true,
		},
		"error: caller args containing --remote= form are rejected": {
			server:  &RemoteAppServer{codexBin: codexBin, endpoint: "ws://127.0.0.1:49815"},
			args:    []string{"--remote=ws://other"},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cmd, err := tt.server.CodexCommand(t.Context(), tt.args...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("CodexCommand() error = nil, want --remote duplication rejection")
				}
				if strings.Contains(err.Error(), "jwt-token-should-not-leak") {
					t.Fatal("CodexCommand() error leaks bearer token")
				}
				return
			}
			if err != nil {
				t.Fatalf("CodexCommand() error = %v", err)
			}
			if diff := gocmp.Diff(tt.want, cmd.Args); diff != "" {
				t.Fatalf("CodexCommand() argv mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRemoteAppServerCodexCommandBearerNeverInArgv(t *testing.T) {
	const token = "super-secret-bearer-token"
	server := &RemoteAppServer{
		codexBin:    "/opt/local/codex/bin/codex",
		endpoint:    "ws://127.0.0.1:49815",
		bearerToken: token,
		env:         map[string]string{"CODEX_HOME": "/tmp/home"},
	}

	cmd, err := server.CodexCommand(t.Context(), "exec", "do-thing")
	if err != nil {
		t.Fatalf("CodexCommand() error = %v", err)
	}

	for _, arg := range cmd.Args {
		if strings.Contains(arg, token) {
			t.Fatalf("CodexCommand() argv leaks bearer token in %q", arg)
		}
	}

	if !slices.Contains(cmd.Env, remoteAuthTokenEnv+"="+token) {
		t.Fatalf("CodexCommand() env missing %s=<token> entry", remoteAuthTokenEnv)
	}
}

func TestRemoteAppServerCodexCommandPropagatesCodexHome(t *testing.T) {
	codexHome := filepath.Join(t.TempDir(), "codex-home")
	server := &RemoteAppServer{
		codexBin: "/opt/local/codex/bin/codex",
		endpoint: "unix:///tmp/app.sock",
		env:      remoteEffectiveEnv(map[string]string{"CODEX_HOME": codexHome}),
	}

	cmd, err := server.CodexCommand(t.Context())
	if err != nil {
		t.Fatalf("CodexCommand() error = %v", err)
	}

	want := "CODEX_HOME=" + codexHome
	if !slices.Contains(cmd.Env, want) {
		t.Fatalf("CodexCommand() env missing %q\nenv: %v", want, cmd.Env)
	}
}

func TestRemoteAppServerRemoteConfig(t *testing.T) {
	tests := map[string]struct {
		server          *RemoteAppServer
		wantURL         string
		wantBearerToken string
	}{
		"success: websocket endpoint propagates bearer token": {
			server: &RemoteAppServer{
				endpoint:    "ws://127.0.0.1:49815",
				bearerToken: "bearer-token-value",
			},
			wantURL:         "ws://127.0.0.1:49815",
			wantBearerToken: "bearer-token-value",
		},
		"success: unix endpoint never carries a bearer token": {
			server: &RemoteAppServer{
				endpoint:    "unix:///tmp/app.sock",
				socketPath:  "/tmp/app.sock",
				bearerToken: "ignored-for-unix",
			},
			wantURL:         "unix:///tmp/app.sock",
			wantBearerToken: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := tt.server.RemoteConfig()
			if cfg.URL != tt.wantURL {
				t.Fatalf("RemoteConfig().URL = %q, want %q", cfg.URL, tt.wantURL)
			}
			if cfg.BearerToken != tt.wantBearerToken {
				t.Fatalf("RemoteConfig().BearerToken = %q, want %q", cfg.BearerToken, tt.wantBearerToken)
			}
			if cfg.DialTimeout != 5*time.Second {
				t.Fatalf("RemoteConfig().DialTimeout = %v, want 5s", cfg.DialTimeout)
			}
		})
	}
}

func TestRemoteAttachBearerToken(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "bearer.token")
	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", tokenFile, err)
	}
	emptyFile := filepath.Join(t.TempDir(), "empty.token")
	if err := os.WriteFile(emptyFile, []byte("  \n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", emptyFile, err)
	}

	tests := map[string]struct {
		cfg       *WebSocketConfig
		wantToken string
		wantErr   bool
	}{
		"success: nil config yields no token": {
			cfg:       nil,
			wantToken: "",
		},
		"success: inline token is trimmed": {
			cfg:       &WebSocketConfig{ClientBearerToken: "  inline-token  "},
			wantToken: "inline-token",
		},
		"success: token file is read and trimmed": {
			cfg:       &WebSocketConfig{ClientBearerTokenFile: tokenFile},
			wantToken: "file-token",
		},
		"error: empty token file is rejected": {
			cfg:     &WebSocketConfig{ClientBearerTokenFile: emptyFile},
			wantErr: true,
		},
		"error: missing token file is rejected": {
			cfg:     &WebSocketConfig{ClientBearerTokenFile: filepath.Join(t.TempDir(), "absent.token")},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := remoteAttachBearerToken(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("remoteAttachBearerToken() error = nil, want rejection")
				}
				return
			}
			if err != nil {
				t.Fatalf("remoteAttachBearerToken() error = %v", err)
			}
			if got != tt.wantToken {
				t.Fatalf("remoteAttachBearerToken() = %q, want %q", got, tt.wantToken)
			}
		})
	}
}

func TestRemoteControlEnabled(t *testing.T) {
	enabled := true
	disabled := false
	tests := map[string]struct {
		ptr  *bool
		want bool
	}{
		"success: nil defaults to enabled":    {ptr: nil, want: true},
		"success: explicit true is enabled":   {ptr: &enabled, want: true},
		"success: explicit false is disabled": {ptr: &disabled, want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := remoteControlEnabled(tt.ptr); got != tt.want {
				t.Fatalf("remoteControlEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoteAppServerCloseIdempotentWithoutProcess(t *testing.T) {
	server := &RemoteAppServer{endpoint: "ws://127.0.0.1:49815"}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() first call error = %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() second call error = %v", err)
	}
}

func TestReserveLoopbackPortReturnsUsablePort(t *testing.T) {
	port, err := ReserveLoopbackPort()
	if err != nil {
		t.Fatalf("ReserveLoopbackPort() error = %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Fatalf("ReserveLoopbackPort() = %d, want a valid TCP port", port)
	}
}
