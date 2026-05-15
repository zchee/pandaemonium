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
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/go-json-experiment/json/jsontext"
)

const transportHelperEnv = "PANDAEMONIUM_CODEX_TEST_HELPER_PROCESS"

func TestCodexTransportHelperProcess(t *testing.T) {
	if os.Getenv(transportHelperEnv) != "1" {
		return
	}
	switch os.Getenv("CODEX_PORT_HELPER_SCENARIO") {
	case "websocket_roundtrip":
		runTransportHelperWebSocket()
	default:
		runTransportHelperStdio()
	}
}

func runTransportHelperStdio() {
	scanner := bufio.NewScanner(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	defer func() { _ = w.Flush() }()
	for scanner.Scan() {
		var req rpcMessage
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		switch req.Method {
		case RequestMethodInitialize:
			_ = json.NewEncoder(w).Encode(InitializeResponse{
				UserAgent:  "codex-bench/1.0",
				ServerInfo: &ServerInfo{Name: "codex-bench", Version: "1.0"},
			})
			_ = w.Flush()
		case "initialized":
		case "helper/echo":
			_ = json.NewEncoder(w).Encode(rpcMessage{ID: req.ID, Result: mustJSONValueForHelper(Object{"ok": true})})
			_ = w.Flush()
		default:
			_ = json.NewEncoder(w).Encode(rpcMessage{ID: req.ID, Error: &rpcErrorBody{Code: -32601, Message: "unexpected method"}})
			_ = w.Flush()
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func runTransportHelperWebSocket() {
	port := os.Getenv("CODEX_WEBSOCKET_LISTEN_PORT")
	if port == "" {
		port = "49815"
	}
	expectedBearer := os.Getenv("CODEX_WEBSOCKET_EXPECT_BEARER")
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if expectedBearer != "" && r.Header.Get("Authorization") != "Bearer "+expectedBearer {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		for {
			typ, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			if typ != websocket.MessageText {
				continue
			}
			var req rpcMessage
			if err := json.Unmarshal(data, &req); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}
			switch req.Method {
			case RequestMethodInitialize:
				_ = conn.Write(context.Background(), websocket.MessageText, mustJSONValueForHelper(InitializeResponse{
					UserAgent:  "codex-bench/1.0",
					ServerInfo: &ServerInfo{Name: "codex-bench", Version: "1.0"},
				}))
			case "initialized":
				_ = conn.Write(context.Background(), websocket.MessageText, mustJSONValueForHelper(rpcMessage{Method: "custom/global", Params: mustJSONValueForHelper(Object{"scope": "global"})}))
			case "helper/echo":
				_ = conn.Write(context.Background(), websocket.MessageText, mustJSONValueForHelper(rpcMessage{ID: req.ID, Result: mustJSONValueForHelper(Object{"ok": true})}))
			default:
				_ = conn.Write(context.Background(), websocket.MessageText, mustJSONValueForHelper(rpcMessage{ID: req.ID, Error: &rpcErrorBody{Code: -32601, Message: "unexpected method"}}))
			}
		}
	})}
	go func() {
		<-context.Background().Done()
		_ = srv.Close()
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func mustJSONValueForHelper(value any) jsontext.Value {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return jsontext.Value(data)
}

func helperWebSocketFrameWriter(conn net.Conn, payload []byte) error {
	var header [14]byte
	header[0] = 0x81
	payloadLen := len(payload)
	var n int
	switch {
	case payloadLen < 126:
		header[1] = byte(payloadLen)
		n = 2
	case payloadLen <= math.MaxUint16:
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:4], uint16(payloadLen))
		n = 4
	default:
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:10], uint64(payloadLen))
		n = 10
	}
	if _, err := conn.Write(header[:n]); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func helperWebSocketAcceptPath(listen string) string {
	if strings.HasPrefix(listen, "ws://") || strings.HasPrefix(listen, "wss://") {
		if u, err := url.Parse(listen); err == nil {
			return u.Host
		}
	}
	return listen
}
