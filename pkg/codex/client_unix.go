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
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const unixListenPrefix = "unix://"

type listenTransportKind int

const (
	listenTransportStdio listenTransportKind = iota
	listenTransportWebSocket
	listenTransportUnixWebSocket
)

func classifyListenTransport(listenURL string) (listenTransportKind, error) {
	listenURL = strings.TrimSpace(listenURL)
	if listenURL == "" || listenURL == defaultListenURL {
		return listenTransportStdio, nil
	}
	if strings.HasPrefix(listenURL, unixListenPrefix) {
		if err := validateUnixListenURL(listenURL); err != nil {
			return 0, err
		}
		return listenTransportUnixWebSocket, nil
	}

	parsed, err := url.Parse(listenURL)
	if err != nil {
		return listenTransportStdio, err
	}
	switch parsed.Scheme {
	case "ws", "wss":
		return listenTransportWebSocket, nil
	case "unix":
		return listenTransportStdio, fmt.Errorf("invalid listen URL %q: use unix:// for unix websocket endpoints", listenURL)
	default:
		return listenTransportStdio, nil
	}
}

func websocketListenMode(listenURL string, env map[string]string, cwd string) (string, string, error) {
	kind, err := classifyListenTransport(listenURL)
	if err != nil {
		return "", "", err
	}
	switch kind {
	case listenTransportUnixWebSocket:
		socketPath, err := unixSocketPathFromListenURL(listenURL, env, cwd)
		if err != nil {
			return "", "", err
		}
		return "unix websocket", socketPath, nil
	case listenTransportWebSocket:
		return "websocket", "", nil
	default:
		return "stdio", "", nil
	}
}

func validateUnixListenURL(listenURL string) error {
	listenURL = strings.TrimSpace(listenURL)
	if !strings.HasPrefix(listenURL, unixListenPrefix) {
		return fmt.Errorf("invalid unix listen URL %q: unix listen endpoints must use unix:// prefix", listenURL)
	}
	if strings.Contains(listenURL, "%") {
		return fmt.Errorf("invalid unix listen URL %q: percent-encoded unix socket paths are not supported", listenURL)
	}
	return nil
}

func unixSocketPathFromListenURL(listenURL string, env map[string]string, cwd string) (string, error) {
	listenURL = strings.TrimSpace(listenURL)
	if !strings.HasPrefix(listenURL, unixListenPrefix) {
		return "", fmt.Errorf("invalid unix listen URL %q: unix listen endpoints must use unix:// prefix", listenURL)
	}
	suffix := strings.TrimPrefix(listenURL, unixListenPrefix)
	if strings.Contains(suffix, "%") {
		return "", fmt.Errorf("invalid unix listen URL %q: percent-encoded unix socket paths are not supported", listenURL)
	}
	if suffix == "" {
		codexHome := ""
		if env != nil {
			codexHome = strings.TrimSpace(env["CODEX_HOME"])
		}
		if codexHome == "" {
			codexHome = DefaultCodexHome()
		}
		return filepath.Join(codexHome, "app-server-control", "app-server-control.sock"), nil
	}
	if strings.HasPrefix(suffix, "/") {
		return filepath.Clean(suffix), nil
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve relative unix socket path %q: %w", listenURL, err)
		}
	}
	return filepath.Clean(filepath.Join(cwd, suffix)), nil
}
