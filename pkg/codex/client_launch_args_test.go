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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientBuildAppServerArgsUsesStdioByDefault(t *testing.T) {
	client := &Client{config: Config{CodexBin: os.Args[0]}}
	args, err := client.buildAppServerArgs(ListenConfig{})
	if err != nil {
		t.Fatalf("buildAppServerArgs() error = %v", err)
	}
	got := strings.Join(args, " ")
	if want := os.Args[0] + " app-server --listen stdio://"; !strings.Contains(got, want) {
		t.Fatalf("buildAppServerArgs() = %q, want contain %q", got, want)
	}
}

func TestClientLaunchArgsOverrideTakesPriority(t *testing.T) {
	client := &Client{
		config: Config{
			LaunchArgsOverride: []string{"override", "--flag"},
		},
	}
	args, err := client.launchArgs()
	if err != nil {
		t.Fatalf("launchArgs() error = %v", err)
	}
	if got := strings.Join(args, " "); got != "override --flag" {
		t.Fatalf("launchArgs() = %q, want override --flag", got)
	}
}

func TestClientBuildAppServerArgsWebSocketNoAuthOmitsAuthFlags(t *testing.T) {
	tests := map[string]struct {
		listen ListenConfig
		want   []string
	}{
		"success: nil websocket config emits listen only": {
			listen: ListenConfig{URL: "ws://127.0.0.1:49815"},
			want:   []string{os.Args[0], "app-server", "--listen", "ws://127.0.0.1:49815"},
		},
		"success: explicit auth none emits listen only": {
			listen: ListenConfig{
				URL:       "ws://127.0.0.1:49815",
				WebSocket: &WebSocketConfig{AuthMode: WebSocketAuthNone},
			},
			want: []string{os.Args[0], "app-server", "--listen", "ws://127.0.0.1:49815"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client := &Client{config: Config{CodexBin: os.Args[0]}}
			args, err := client.buildAppServerArgs(tt.listen)
			if err != nil {
				t.Fatalf("buildAppServerArgs() error = %v", err)
			}
			if diff := compareStringSlice(args, tt.want); diff != "" {
				t.Fatalf("buildAppServerArgs() mismatch: %s", diff)
			}
		})
	}
}

func TestClientBuildAppServerArgsUnixWebSocketHonorsListenAndDialTimeout(t *testing.T) {
	client := &Client{config: Config{CodexBin: os.Args[0]}}
	args, err := client.buildAppServerArgs(ListenConfig{
		URL: "unix:///tmp/codex.sock",
		WebSocket: &WebSocketConfig{
			AuthMode:    WebSocketAuthNone,
			DialTimeout: 5 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("buildAppServerArgs() error = %v", err)
	}
	want := []string{os.Args[0], "app-server", "--listen", "unix:///tmp/codex.sock"}
	if diff := compareStringSlice(args, want); diff != "" {
		t.Fatalf("buildAppServerArgs() mismatch: %s", diff)
	}
}

func TestClientBuildAppServerArgsRejectsUnsupportedListenURL(t *testing.T) {
	tests := map[string]struct {
		listenURL string
		wantErr   string
	}{
		"error: bare off is not a process-backed transport": {
			listenURL: "off",
			wantErr:   "off disables the app-server transport",
		},
		"error: bare stdio without scheme is unsupported": {
			listenURL: "stdio",
			wantErr:   "unsupported app-server listen URL",
		},
		"error: http scheme is not an app-server listen transport": {
			listenURL: "http://127.0.0.1:49815",
			wantErr:   "unsupported app-server listen URL",
		},
		"error: wss scheme is not accepted by Rust app-server": {
			listenURL: "wss://codex.example.test:49815",
			wantErr:   "unsupported app-server listen URL",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client := &Client{config: Config{CodexBin: os.Args[0]}}
			_, err := client.buildAppServerArgs(ListenConfig{URL: tt.listenURL})
			if err == nil {
				t.Fatalf("buildAppServerArgs(%q) error = nil, want unsupported listen URL rejection", tt.listenURL)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("buildAppServerArgs(%q) error = %v, want contain %q", tt.listenURL, err, tt.wantErr)
			}
		})
	}
}

func TestClientBuildAppServerArgsUnixWebSocketRejectsAuthFields(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "capability.token")
	if err := os.WriteFile(tokenFile, []byte("capability-token\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", tokenFile, err)
	}
	secretFile := filepath.Join(t.TempDir(), "shared-secret")
	if err := os.WriteFile(secretFile, []byte("signed-secret\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", secretFile, err)
	}
	clockSkew := 15

	tests := map[string]struct {
		websocket *WebSocketConfig
	}{
		"error: capability token auth mode": {
			websocket: &WebSocketConfig{
				AuthMode: WebSocketAuthCapabilityToken,
			},
		},
		"error: client bearer token": {
			websocket: &WebSocketConfig{
				ClientBearerToken: "jwt-token-should-not-leak",
			},
		},
		"error: client bearer token file": {
			websocket: &WebSocketConfig{
				ClientBearerTokenFile: tokenFile,
			},
		},
		"error: shared secret auth material": {
			websocket: &WebSocketConfig{
				SharedSecretFile: secretFile,
			},
		},
		"error: signed bearer auth mode": {
			websocket: &WebSocketConfig{
				AuthMode:          WebSocketAuthSignedBearerToken,
				SharedSecretFile:  secretFile,
				ClientBearerToken: "jwt-token-should-not-leak",
			},
		},
		"error: token file auth material": {
			websocket: &WebSocketConfig{
				TokenFile: tokenFile,
			},
		},
		"error: token digest auth material": {
			websocket: &WebSocketConfig{
				TokenSHA256: strings.Repeat("a", 64),
			},
		},
		"error: issuer audience auth material": {
			websocket: &WebSocketConfig{
				Issuer:   "issuer",
				Audience: "audience",
			},
		},
		"error: max clock skew auth material": {
			websocket: &WebSocketConfig{
				MaxClockSkewSeconds: &clockSkew,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client := &Client{config: Config{CodexBin: os.Args[0]}}
			_, err := client.buildAppServerArgs(ListenConfig{
				URL:       "unix:///tmp/codex.sock",
				WebSocket: tt.websocket,
			})
			if err == nil {
				t.Fatal("buildAppServerArgs() error = nil, want unix websocket auth rejection")
			}
			if !strings.Contains(err.Error(), "unix websocket listen does not support websocket auth fields") {
				t.Fatalf("buildAppServerArgs() error = %v, want unix websocket auth rejection", err)
			}
			if strings.Contains(err.Error(), "jwt-token-should-not-leak") {
				t.Fatal("error leaks client bearer token")
			}
		})
	}
}

func TestClientBuildAppServerArgsRejectsInsecureRemoteWebSocket(t *testing.T) {
	client := &Client{config: Config{CodexBin: os.Args[0]}}
	_, err := client.buildAppServerArgs(ListenConfig{URL: "ws://codex.example.test:49815"})
	if err == nil {
		t.Fatal("buildAppServerArgs() error = nil, want insecure remote websocket rejection")
	}
	if !strings.Contains(err.Error(), "allowed only for loopback hosts") {
		t.Fatalf("buildAppServerArgs() error = %v, want loopback guard", err)
	}
}

func TestClientBuildAppServerArgsCapabilityTokenFileMode(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "capability.token")
	if err := os.WriteFile(tokenFile, []byte("capability-token\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", tokenFile, err)
	}

	client := &Client{
		config: Config{
			CodexBin: os.Args[0],
			Listen: ListenConfig{
				URL: "ws://127.0.0.1:49815",
				WebSocket: &WebSocketConfig{
					AuthMode:            WebSocketAuthCapabilityToken,
					TokenFile:           tokenFile,
					ClientBearerToken:   "unused-in-tests",
					MaxClockSkewSeconds: new(15),
					DialTimeout:         5 * time.Second,
				},
			},
		},
	}
	args, err := client.buildAppServerArgs(client.config.Listen)
	if err != nil {
		t.Fatalf("buildAppServerArgs() error = %v", err)
	}
	want := []string{
		os.Args[0], "app-server", "--listen", "ws://127.0.0.1:49815",
		"--ws-auth", "capability-token", "--ws-token-file", tokenFile,
		"--ws-max-clock-skew-seconds", "15",
	}
	if got := strings.Join(args, " "); got == "" {
		t.Fatal("got empty args")
	}
	if diff := compareStringSlice(args, want); diff != "" {
		t.Fatalf("buildAppServerArgs() mismatch: %s", diff)
	}
}

func TestClientBuildAppServerArgsCapabilityTokenSHA256Mode(t *testing.T) {
	client := &Client{
		config: Config{
			CodexBin: os.Args[0],
			Listen: ListenConfig{
				URL: "ws://127.0.0.1:49815",
				WebSocket: &WebSocketConfig{
					AuthMode:            WebSocketAuthCapabilityToken,
					TokenSHA256:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					ClientBearerToken:   "jwt-from-env",
					MaxClockSkewSeconds: new(3),
				},
			},
		},
	}
	args, err := client.buildAppServerArgs(client.config.Listen)
	if err != nil {
		t.Fatalf("buildAppServerArgs() error = %v", err)
	}
	for _, want := range []string{
		"--ws-auth", "capability-token",
		"--ws-token-sha256", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"--ws-max-clock-skew-seconds", "3",
	} {
		if !strings.Contains(strings.Join(args, " "), want) {
			t.Fatalf("buildAppServerArgs() = %q, want to contain %q", strings.Join(args, " "), want)
		}
	}
}

func TestClientBuildAppServerArgsCapabilityTokenRejectsMutualExclusion(t *testing.T) {
	client := &Client{
		config: Config{
			CodexBin: os.Args[0],
			Listen: ListenConfig{
				URL: "ws://127.0.0.1:49815",
				WebSocket: &WebSocketConfig{
					AuthMode:          WebSocketAuthCapabilityToken,
					TokenFile:         "/tmp/a",
					TokenSHA256:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					ClientBearerToken: "token",
				},
			},
		},
	}
	if _, err := client.buildAppServerArgs(client.config.Listen); err == nil {
		t.Fatal("buildAppServerArgs() error = nil, want auth-field conflict error")
	}
}

func TestClientBuildAppServerArgsSignedBearerTokenMode(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secretFile, []byte("signed-secret\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", secretFile, err)
	}
	bearerFile := filepath.Join(t.TempDir(), "bearer")
	if err := os.WriteFile(bearerFile, []byte("jwt-token\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", bearerFile, err)
	}

	client := &Client{
		config: Config{
			CodexBin: os.Args[0],
			Listen: ListenConfig{
				URL: "ws://127.0.0.1:49815",
				WebSocket: &WebSocketConfig{
					AuthMode:              WebSocketAuthSignedBearerToken,
					SharedSecretFile:      secretFile,
					ClientBearerTokenFile: bearerFile,
					Issuer:                "issuer",
					Audience:              "audience",
				},
			},
		},
	}
	args, err := client.buildAppServerArgs(client.config.Listen)
	if err != nil {
		t.Fatalf("buildAppServerArgs() error = %v", err)
	}
	for _, want := range []string{
		"--ws-auth", "signed-bearer-token",
		"--ws-shared-secret-file", secretFile,
		"--ws-issuer", "issuer",
		"--ws-audience", "audience",
	} {
		if !strings.Contains(strings.Join(args, " "), want) {
			t.Fatalf("buildAppServerArgs() = %q, want contain %q", strings.Join(args, " "), want)
		}
	}
}

func TestClientBuildAppServerArgsSignedBearerRequiresBearerToken(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secretFile, []byte("signed-secret\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", secretFile, err)
	}

	client := &Client{
		config: Config{
			CodexBin: os.Args[0],
			Listen: ListenConfig{
				URL: "ws://127.0.0.1:49815",
				WebSocket: &WebSocketConfig{
					AuthMode:         WebSocketAuthSignedBearerToken,
					SharedSecretFile: secretFile,
				},
			},
		},
	}
	_, err := client.buildAppServerArgs(client.config.Listen)
	if err == nil {
		t.Fatal("buildAppServerArgs() error = nil, want missing bearer source error")
	}
}

func TestClientBuildAppServerArgsRejectsMalformedTokenSHA256(t *testing.T) {
	tests := map[string]string{
		"error: non-hex digest": strings.Repeat("g", 64),
		"error: short digest":   "0123456789abcdef",
	}

	for name, tokenSHA256 := range tests {
		t.Run(name, func(t *testing.T) {
			client := &Client{
				config: Config{
					CodexBin: os.Args[0],
					Listen: ListenConfig{
						URL: "ws://127.0.0.1:49815",
						WebSocket: &WebSocketConfig{
							AuthMode:          WebSocketAuthCapabilityToken,
							TokenSHA256:       tokenSHA256,
							ClientBearerToken: "token",
						},
					},
				},
			}
			_, err := client.buildAppServerArgs(client.config.Listen)
			if err == nil {
				t.Fatal("buildAppServerArgs() error = nil, want malformed token digest error")
			}
		})
	}
}

func TestClientBuildAppServerArgsRejectsAuthNoneWithAuthFields(t *testing.T) {
	client := &Client{
		config: Config{
			CodexBin: os.Args[0],
			Listen: ListenConfig{
				URL: "ws://127.0.0.1:49815",
				WebSocket: &WebSocketConfig{
					AuthMode:         WebSocketAuthNone,
					SharedSecretFile: "ignored",
				},
			},
		},
	}
	_, err := client.buildAppServerArgs(client.config.Listen)
	if err == nil {
		t.Fatal("buildAppServerArgs() error = nil, want auth-none rejection")
	}
}

func TestClientBuildAppServerArgsRejectsNegativeClockSkew(t *testing.T) {
	client := &Client{
		config: Config{
			CodexBin: os.Args[0],
			Listen: ListenConfig{
				URL: "ws://127.0.0.1:49815",
				WebSocket: &WebSocketConfig{
					AuthMode:            WebSocketAuthCapabilityToken,
					TokenSHA256:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					ClientBearerToken:   "token",
					MaxClockSkewSeconds: new(-1),
				},
			},
		},
	}
	_, err := client.buildAppServerArgs(client.config.Listen)
	if err == nil {
		t.Fatal("buildAppServerArgs() error = nil, want negative clock-skew error")
	}
}

func TestClientBuildAppServerArgsSkipsBearerMaterialInErrors(t *testing.T) {
	client := &Client{
		config: Config{
			CodexBin: os.Args[0],
			Listen: ListenConfig{
				URL: "ws://127.0.0.1:49815",
				WebSocket: &WebSocketConfig{
					AuthMode:          WebSocketAuthCapabilityToken,
					TokenSHA256:       "bad",
					ClientBearerToken: "jwt-token-should-not-leak",
				},
			},
		},
	}
	_, err := client.buildAppServerArgs(client.config.Listen)
	if err == nil {
		t.Fatal("buildAppServerArgs() error = nil, want malformed digest error")
	}
	if strings.Contains(err.Error(), "jwt-token-should-not-leak") {
		t.Fatal("error leaks client bearer token")
	}
}

func TestClientBuildAppServerArgsRejectsMalformedUnixListenURL(t *testing.T) {
	client := &Client{config: Config{CodexBin: os.Args[0]}}
	for _, listenURL := range []string{"unix:relative.sock", "unix://%2Ftmp%2Fcodex.sock"} {
		_, err := client.buildAppServerArgs(ListenConfig{URL: listenURL})
		if err == nil {
			t.Fatalf("buildAppServerArgs(%q) error = nil, want unix URL rejection", listenURL)
		}
	}
}

func TestParseListenTransport(t *testing.T) {
	tests := map[string]struct {
		listen string
		want   listenTransportKind
	}{
		"success: empty defaults to stdio": {
			listen: "",
			want:   listenTransportStdio,
		},
		"success: stdio URL": {
			listen: "stdio://",
			want:   listenTransportStdio,
		},
		"success: unix default socket": {
			listen: "unix://",
			want:   listenTransportUnixWebSocket,
		},
		"success: unix path socket": {
			listen: "unix:///tmp/codex.sock",
			want:   listenTransportUnixWebSocket,
		},
		"success: websocket URL": {
			listen: "ws://127.0.0.1:49815",
			want:   listenTransportWebSocket,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseListenTransport(tt.listen)
			if err != nil {
				t.Fatalf("parseListenTransport(%q) error = %v", tt.listen, err)
			}
			if got != tt.want {
				t.Fatalf("parseListenTransport(%q) = %v, want %v", tt.listen, got, tt.want)
			}
		})
	}
}

func TestParseListenTransportRejectsUnsupportedListenURL(t *testing.T) {
	tests := map[string]struct {
		listen  string
		wantErr string
	}{
		"error: bare stdio": {
			listen:  "stdio",
			wantErr: "unsupported app-server listen URL",
		},
		"error: http URL": {
			listen:  "http://codex.example.test",
			wantErr: "unsupported app-server listen URL",
		},
		"error: off transport": {
			listen:  "off",
			wantErr: "off disables the app-server transport",
		},
		"error: opaque unix URL": {
			listen:  "unix:relative.sock",
			wantErr: "unix listen endpoints must use unix:// prefix",
		},
		"error: wss URL": {
			listen:  "wss://codex.example.test:49815",
			wantErr: "unsupported app-server listen URL",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseListenTransport(tt.listen)
			if err == nil {
				t.Fatalf("parseListenTransport(%q) error = nil, want rejection", tt.listen)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseListenTransport(%q) error = %v, want contain %q", tt.listen, err, tt.wantErr)
			}
		})
	}
}

func TestUnixSocketPathFromListenURL(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%s) error = %v", cwd, err)
	}
	codeXHome := filepath.Join(t.TempDir(), "codex-home")
	tests := []struct {
		name    string
		listen  string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{
			name:   "default path uses CODEX_HOME",
			listen: "unix://",
			env:    map[string]string{"CODEX_HOME": codeXHome},
			want:   filepath.Join(codeXHome, "app-server-control", "app-server-control.sock"),
		},
		{
			name:   "absolute path is preserved",
			listen: "unix:///tmp/codex.sock",
			want:   filepath.Clean("/tmp/codex.sock"),
		},
		{
			name:   "relative path resolves against cwd",
			listen: "unix://localhost/tmp/codex.sock",
			want:   filepath.Join(cwd, "localhost", "tmp", "codex.sock"),
		},
		{
			name:    "rejects encoded path",
			listen:  "unix://%2Ftmp%2Fcodex.sock",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unixSocketPathFromListenURL(tt.listen, tt.env, cwd)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("unixSocketPathFromListenURL(%q) error = nil, want error", tt.listen)
				}
				return
			}
			if err != nil {
				t.Fatalf("unixSocketPathFromListenURL(%q) error = %v", tt.listen, err)
			}
			if got != tt.want {
				t.Fatalf("unixSocketPathFromListenURL(%q) = %q, want %q", tt.listen, got, tt.want)
			}
		})
	}
}

func compareStringSlice(got, want []string) string {
	if len(got) != len(want) {
		return "len mismatch"
	}
	for i, v := range want {
		if got[i] != v {
			return "value mismatch"
		}
	}
	return ""
}
