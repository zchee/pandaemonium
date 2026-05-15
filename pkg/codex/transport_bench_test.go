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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

func BenchmarkClientTransportRoundTripStdIO(b *testing.B) {
	client, cancel := benchmarkStdIOClient(b)
	defer cancel()
	benchmarkInitializeClient(b, client)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := client.RequestRaw(context.Background(), "helper/echo", Object{"hello": "world"}); err != nil {
			b.Fatalf("RequestRaw() error = %v", err)
		}
	}
}

func BenchmarkClientTransportRoundTripWebSocket(b *testing.B) {
	srv, wsURL := benchmarkWebSocketServer(b)
	b.Cleanup(srv.Close)

	client, cancel := benchmarkWebSocketClient(b, wsURL)
	defer cancel()
	benchmarkInitializeClient(b, client)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := client.RequestRaw(context.Background(), "helper/echo", Object{"hello": "world"}); err != nil {
			b.Fatalf("RequestRaw() error = %v", err)
		}
	}
}

func benchmarkStdIOClient(b *testing.B) (*Client, context.CancelFunc) {
	b.Helper()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	client := NewClient(&Config{}, nil)
	client.transport = &stdioTransport{stdin: stdinW, stdout: bufio.NewReader(stdoutR)}
	client.responses = map[string]chan responseWait{}
	client.notifications = make(chan Notification, notificationQueueCapacity)
	client.turnRouter = newTurnNotificationRouter()
	client.readDone = make(chan struct{})
	go client.readLoop(client.transport, client.readDone)
	go benchmarkJSONRPCResponder(b, ctx, stdinR, stdoutW, "stdio")
	b.Cleanup(func() {
		if err := client.Close(); err != nil {
			b.Fatalf("Client.Close() error = %v", err)
		}
	})
	return client, cancel
}

func benchmarkWebSocketClient(b *testing.B, wsURL string) (*Client, context.CancelFunc) {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		b.Fatalf("websocket.Dial() error = %v", err)
	}
	client := NewClient(&Config{}, nil)
	client.transport = &websocketTransport{conn: conn}
	client.responses = map[string]chan responseWait{}
	client.notifications = make(chan Notification, notificationQueueCapacity)
	client.turnRouter = newTurnNotificationRouter()
	client.readDone = make(chan struct{})
	go client.readLoop(client.transport, client.readDone)
	b.Cleanup(func() {
		if err := client.Close(); err != nil {
			b.Fatalf("Client.Close() error = %v", err)
		}
	})
	return client, cancel
}

func benchmarkInitializeClient(b *testing.B, client *Client) {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := client.Initialize(ctx); err != nil {
		b.Fatalf("Client.Initialize() error = %v", err)
	}
}

func benchmarkWebSocketServer(b *testing.B) (*httptest.Server, string) {
	b.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			b.Errorf("websocket.Accept() error = %v", err)
			return
		}
		go func() {
			defer conn.Close(websocket.StatusNormalClosure, "")
			ctx := context.Background()
			for {
				typ, payload, err := conn.Read(ctx)
				if err != nil {
					if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
						return
					}
					if !errors.Is(err, io.EOF) {
						b.Logf("websocket server read error: %v", err)
					}
					return
				}
				if typ != websocket.MessageText {
					continue
				}
				var msg rpcMessage
				if err := json.Unmarshal(payload, &msg); err != nil {
					b.Logf("websocket server decode error: %v", err)
					return
				}
				switch msg.Method {
				case RequestMethodInitialize:
					resp := rpcMessage{
						ID: msg.ID,
						Result: jsontextValueMust(Object{
							"userAgent":  "bench/1.0",
							"serverInfo": Object{"name": "bench", "version": "1.0"},
						}),
					}
					raw, _ := json.Marshal(resp)
					_ = conn.Write(ctx, websocket.MessageText, raw)
				case "initialized":
				case "helper/echo":
					resp := rpcMessage{ID: msg.ID, Result: jsontextValueMust(Object{"ok": "world"})}
					raw, _ := json.Marshal(resp)
					_ = conn.Write(ctx, websocket.MessageText, raw)
				}
			}
		}()
	}))
	b.Cleanup(srv.Close)
	return srv, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func benchmarkJSONRPCResponder(b *testing.B, ctx context.Context, stdinR *io.PipeReader, stdoutW *io.PipeWriter, mode string) {
	b.Helper()
	reader := bufio.NewReader(stdinR)
	for {
		select {
		case <-ctx.Done():
			_ = stdinR.Close()
			_ = stdoutW.Close()
			return
		default:
		}
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			b.Logf("%s benchmark decode error: %v", mode, err)
			return
		}
		switch msg.Method {
		case RequestMethodInitialize:
			resp := rpcMessage{
				ID: msg.ID,
				Result: jsontextValueMust(Object{
					"userAgent":  "bench/1.0",
					"serverInfo": Object{"name": "bench", "version": "1.0"},
				}),
			}
			raw, _ := json.Marshal(resp)
			if _, err := stdoutW.Write(append(raw, '\n')); err != nil {
				return
			}
		case "initialized":
		case "helper/echo":
			resp := rpcMessage{ID: msg.ID, Result: jsontextValueMust(Object{"ok": "world"})}
			raw, _ := json.Marshal(resp)
			if _, err := stdoutW.Write(append(raw, '\n')); err != nil {
				return
			}
		}
	}
}

func jsontextValueMust(v any) jsontext.Value {
	raw, _ := json.Marshal(v)
	return jsontext.Value(raw)
}
