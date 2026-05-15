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

package codex_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestAppServerAsyncClientBehaviorPortConcurrentTransportCallsOverlap(t *testing.T) {
	sdk := newHelperCodex(t, "async_client_behavior")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	const callCount = 2
	results := make([]codex.ModelListResponse, callCount)
	errs := make([]error, callCount)

	var ready sync.WaitGroup
	ready.Add(callCount)
	start := make(chan struct{})
	var done sync.WaitGroup
	done.Add(callCount)
	for i := range callCount {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			results[i], errs[i] = sdk.Models(ctx, nil)
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Models() concurrent call %d error = %v", i, err)
		}
	}
	for i, result := range results {
		if len(result.Data) != 1 {
			t.Fatalf("Models() concurrent call %d returned %d models, want 1", i, len(result.Data))
		}
		if result.Data[0].ID != "gpt-overlap-2" {
			t.Fatalf("Models() concurrent call %d model id = %q, want gpt-overlap-2", i, result.Data[0].ID)
		}
	}
}

func TestAppServerAsyncClientBehaviorPortTurnRoutingPreservesSyncSemantics(t *testing.T) {
	sdk := newHelperCodex(t, "async_client_behavior")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	thread, err := sdk.ThreadStart(ctx, nil)
	if err != nil {
		t.Fatalf("ThreadStart() error = %v", err)
	}
	started, err := sdk.Client().TurnStart(ctx, thread.ID(), "route direct turn notifications", nil)
	if err != nil {
		t.Fatalf("Client.TurnStart() error = %v", err)
	}

	completed, err := sdk.Client().WaitForTurnCompleted(ctx, started.Turn.ID)
	if err != nil {
		t.Fatalf("WaitForTurnCompleted(%q) error = %v", started.Turn.ID, err)
	}
	if completed.Turn.ID != started.Turn.ID {
		t.Fatalf("WaitForTurnCompleted() turn id = %q, want %q", completed.Turn.ID, started.Turn.ID)
	}

	global, err := sdk.Client().NextNotification(ctx)
	if err != nil {
		t.Fatalf("NextNotification() after turn completion error = %v", err)
	}
	if global.Method != "unknown/global" {
		t.Fatalf("NextNotification() method = %q, want unknown/global", global.Method)
	}
}
