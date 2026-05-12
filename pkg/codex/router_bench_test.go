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

package codexappserver

import (
	"testing"

	"github.com/go-json-experiment/json/jsontext"
)

var (
	benchmarkTopLevelTurnNotification = Notification{
		Method: NotificationMethodItemCompleted,
		Params: jsontext.Value(`{"threadId":"thread-router-bench","turnId":"turn-router-bench","item":{"type":"agentMessage","text":"hello","meta":{"nested":[1,2,3]}}}`),
	}
	benchmarkNestedTurnNotification = Notification{
		Method: NotificationMethodTurnCompleted,
		Params: jsontext.Value(`{"threadId":"thread-router-bench","turn":{"id":"turn-router-bench","status":"completed","items":[{"type":"agentMessage","text":"hello"}],"itemsView":{"mode":"all"}}}`),
	}
	benchmarkGlobalNotification = Notification{
		Method: "custom/global",
		Params: jsontext.Value(`{"threadId":"thread-router-bench","phase":"unscoped","payload":{"nested":[1,2,3]}}`),
	}
)

func BenchmarkNotificationTurnIDTopLevel(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if got := notificationTurnID(benchmarkTopLevelTurnNotification); got != "turn-router-bench" {
			b.Fatalf("notificationTurnID() = %q, want turn-router-bench", got)
		}
	}
}

func BenchmarkNotificationTurnIDNestedTurn(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if got := notificationTurnID(benchmarkNestedTurnNotification); got != "turn-router-bench" {
			b.Fatalf("notificationTurnID() = %q, want turn-router-bench", got)
		}
	}
}

func BenchmarkNotificationTurnIDGlobal(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if got := notificationTurnID(benchmarkGlobalNotification); got != "" {
			b.Fatalf("notificationTurnID() = %q, want empty string", got)
		}
	}
}

func BenchmarkNotificationRingSteadyWrap(b *testing.B) {
	ring := newNotificationRing(notificationQueueCapacity)
	for range notificationQueueCapacity {
		if ok := ring.push(benchmarkTopLevelTurnNotification); !ok {
			b.Fatal("initial push ok = false, want true")
		}
	}
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, ok := ring.pop(); !ok {
			b.Fatal("pop ok = false, want true")
		}
		if ok := ring.push(benchmarkTopLevelTurnNotification); !ok {
			b.Fatal("push ok = false, want true")
		}
	}
}

func BenchmarkNotificationRingAppendPop128(b *testing.B) {
	pending := make([]Notification, notificationQueueCapacity)
	for i := range pending {
		pending[i] = benchmarkTopLevelTurnNotification
	}
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		ring := newNotificationRing(notificationQueueCapacity)
		if ok := ring.appendAll(pending); !ok {
			b.Fatal("appendAll ok = false, want true")
		}
		for range pending {
			if _, ok := ring.pop(); !ok {
				b.Fatal("pop ok = false, want true")
			}
		}
	}
}

func BenchmarkTurnNotificationQueuePushPop(b *testing.B) {
	queue := &turnNotificationQueue{
		notifications: newNotificationRing(notificationQueueCapacity),
		notify:        make(chan struct{}, 1),
	}
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := queue.push(benchmarkTopLevelTurnNotification); err != nil {
			b.Fatalf("push() error = %v", err)
		}
		if got, ok := queue.pop(); !ok || got.Method != benchmarkTopLevelTurnNotification.Method {
			b.Fatalf("pop() = (%q, %t), want (%q, true)", got.Method, ok, benchmarkTopLevelTurnNotification.Method)
		}
	}
}

func BenchmarkTurnNotificationRouterRouteActiveQueue(b *testing.B) {
	router := newTurnNotificationRouter()
	queue, err := router.register("turn-router-bench")
	if err != nil {
		b.Fatalf("register() error = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := router.route(benchmarkTopLevelTurnNotification); err != nil {
			b.Fatalf("route() error = %v", err)
		}
		if got, ok := queue.pop(); !ok || got.Method != benchmarkTopLevelTurnNotification.Method {
			b.Fatalf("pop() = (%q, %t), want (%q, true)", got.Method, ok, benchmarkTopLevelTurnNotification.Method)
		}
	}
}

func BenchmarkTurnNotificationRouterRouteGlobal(b *testing.B) {
	router := newTurnNotificationRouter()
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := router.route(benchmarkGlobalNotification); err != nil {
			b.Fatalf("route() error = %v", err)
		}
		select {
		case got := <-router.global:
			if got.Method != benchmarkGlobalNotification.Method {
				b.Fatalf("global notification method = %q, want %q", got.Method, benchmarkGlobalNotification.Method)
			}
		default:
			b.Fatal("global queue empty, want routed notification")
		}
	}
}
