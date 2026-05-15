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

func TestClientAppServerArgsUsesStdioByDefault(t *testing.T) {
	client := &Client{config: Config{CodexBin: os.Args[0]}}
	args, err := client.appServerArgs(ListenConfig{})
	if err != nil {
		t.Fatalf("appServerArgs() error = %v", err)
	}
	got := strings.Join(args, " ")
	if want := os.Args[0] + " app-server --listen stdio://"; !strings.Contains(got, want) {
		t.Fatalf("appServerArgs() = %q, want contain %q", got, want)
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

func TestClientAppServerArgsWebSocketNoAuthOmitsAuthFlags(t *testing.T) {
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
		"success: remote wss does not require insecure ws opt-in": {
			listen: ListenConfig{URL: "wss://codex.example.test:49815"},
			want:   []string{os.Args[0], "app-server", "--listen", "wss://codex.example.test:49815"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client := &Client{config: Config{CodexBin: os.Args[0]}}
			args, err := client.appServerArgs(tt.listen)
			if err != nil {
				t.Fatalf("appServerArgs() error = %v", err)
			}
			if diff := compareStringSlice(args, tt.want); diff != "" {
				t.Fatalf("appServerArgs() mismatch: %s", diff)
			}
		})
	}
}

func TestClientAppServerArgsRejectsInsecureRemoteWebSocket(t *testing.T) {
	client := &Client{config: Config{CodexBin: os.Args[0]}}
	_, err := client.appServerArgs(ListenConfig{URL: "ws://codex.example.test:49815"})
	if err == nil {
		t.Fatal("appServerArgs() error = nil, want insecure remote websocket rejection")
	}
	if !strings.Contains(err.Error(), "allowed only for loopback hosts") {
		t.Fatalf("appServerArgs() error = %v, want loopback guard", err)
	}
}

func TestClientAppServerArgsCapabilityTokenFileMode(t *testing.T) {
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
	args, err := client.appServerArgs(client.config.Listen)
	if err != nil {
		t.Fatalf("appServerArgs() error = %v", err)
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
		t.Fatalf("appServerArgs() mismatch: %s", diff)
	}
}

func TestClientAppServerArgsCapabilityTokenSHA256Mode(t *testing.T) {
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
	args, err := client.appServerArgs(client.config.Listen)
	if err != nil {
		t.Fatalf("appServerArgs() error = %v", err)
	}
	for _, want := range []string{
		"--ws-auth", "capability-token",
		"--ws-token-sha256", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"--ws-max-clock-skew-seconds", "3",
	} {
		if !strings.Contains(strings.Join(args, " "), want) {
			t.Fatalf("appServerArgs() = %q, want to contain %q", strings.Join(args, " "), want)
		}
	}
}

func TestClientAppServerArgsCapabilityTokenRejectsMutualExclusion(t *testing.T) {
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
	if _, err := client.appServerArgs(client.config.Listen); err == nil {
		t.Fatal("appServerArgs() error = nil, want auth-field conflict error")
	}
}

func TestClientAppServerArgsSignedBearerTokenMode(t *testing.T) {
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
	args, err := client.appServerArgs(client.config.Listen)
	if err != nil {
		t.Fatalf("appServerArgs() error = %v", err)
	}
	for _, want := range []string{
		"--ws-auth", "signed-bearer-token",
		"--ws-shared-secret-file", secretFile,
		"--ws-issuer", "issuer",
		"--ws-audience", "audience",
	} {
		if !strings.Contains(strings.Join(args, " "), want) {
			t.Fatalf("appServerArgs() = %q, want contain %q", strings.Join(args, " "), want)
		}
	}
}

func TestClientAppServerArgsSignedBearerRequiresBearerToken(t *testing.T) {
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
	_, err := client.appServerArgs(client.config.Listen)
	if err == nil {
		t.Fatal("appServerArgs() error = nil, want missing bearer source error")
	}
}

func TestClientAppServerArgsRejectsMalformedTokenSHA256(t *testing.T) {
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
			_, err := client.appServerArgs(client.config.Listen)
			if err == nil {
				t.Fatal("appServerArgs() error = nil, want malformed token digest error")
			}
		})
	}
}

func TestClientAppServerArgsRejectsAuthNoneWithAuthFields(t *testing.T) {
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
	_, err := client.appServerArgs(client.config.Listen)
	if err == nil {
		t.Fatal("appServerArgs() error = nil, want auth-none rejection")
	}
}

func TestClientAppServerArgsRejectsNegativeClockSkew(t *testing.T) {
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
	_, err := client.appServerArgs(client.config.Listen)
	if err == nil {
		t.Fatal("appServerArgs() error = nil, want negative clock-skew error")
	}
}

func TestClientAppServerArgsSkipsBearerMaterialInErrors(t *testing.T) {
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
	_, err := client.appServerArgs(client.config.Listen)
	if err == nil {
		t.Fatal("appServerArgs() error = nil, want malformed digest error")
	}
	if strings.Contains(err.Error(), "jwt-token-should-not-leak") {
		t.Fatal("error leaks client bearer token")
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
