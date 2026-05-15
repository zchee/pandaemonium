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
	"context"
	"errors"
	"testing"
	"time"
)

// busyError returns a *JSONRPCError with Kind="server_busy", which IsRetryableError accepts.
func busyError() error {
	return &JSONRPCError{Kind: "server_busy"}
}

// TestRetryRejectsBadJitter verifies RetryOnOverload's JitterRatio validation (AC-5.1):
//   - JitterRatio > 1 must return a validation error before calling op.
//   - JitterRatio < 0 is silently clamped to 0 (jitter disabled) — NOT an error.
//   - JitterRatio = 0 applies the default (0.2) and succeeds.
//   - JitterRatio = 1 is the maximum valid value and succeeds.
func TestRetryRejectsBadJitter(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	op := func() (int, error) { return 1, nil }

	// JitterRatio > 1 must return a validation error without calling op.
	called := false
	_, err := RetryOnOverload[int](ctx, RetryConfig{JitterRatio: 1.01}, func() (int, error) {
		called = true
		return 0, nil
	})
	if err == nil {
		t.Fatal("RetryOnOverload(JitterRatio=1.01) err = nil, want validation error")
	}
	if called {
		t.Fatal("RetryOnOverload(JitterRatio=1.01) called op before validation error")
	}

	// JitterRatio < 0 is clamped to 0 (jitter disabled) — op IS called, no error.
	called = false
	_, err = RetryOnOverload[int](ctx, RetryConfig{JitterRatio: -0.5}, func() (int, error) {
		called = true
		return 7, nil
	})
	if err != nil {
		t.Fatalf("RetryOnOverload(JitterRatio=-0.5) err = %v, want nil (negative clamped to 0)", err)
	}
	if !called {
		t.Fatal("RetryOnOverload(JitterRatio=-0.5) did not call op")
	}

	// JitterRatio = 0 triggers the default (0.2) — succeeds.
	_, err = RetryOnOverload[int](ctx, RetryConfig{JitterRatio: 0}, op)
	if err != nil {
		t.Fatalf("RetryOnOverload(JitterRatio=0) err = %v, want nil", err)
	}

	// JitterRatio = 1 is the boundary — succeeds.
	_, err = RetryOnOverload[int](ctx, RetryConfig{JitterRatio: 1}, op)
	if err != nil {
		t.Fatalf("RetryOnOverload(JitterRatio=1) err = %v, want nil", err)
	}
}

// TestRetryJitterMeanAtCap verifies that with JitterRatio=1 and MinDelay=MaxDelay,
// the clamp-before-jitter and max(0,sleepFor) guards prevent sleepFor from going
// negative or exceeding MaxDelay (AC-5.2). The test completes all maxAttempts
// within a short timeout because sleepFor is bounded to ≤1ns.
func TestRetryJitterMeanAtCap(t *testing.T) {
	t.Parallel()

	const maxAttempts = 50
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	var attempts int
	busy := busyError()
	_, err := RetryOnOverload[int](ctx, RetryConfig{
		MaxAttempts:  maxAttempts,
		InitialDelay: time.Nanosecond,
		MaxDelay:     time.Nanosecond,
		JitterRatio:  1.0, // maximum jitter: range [0, 2*delay] clamped to [0, MaxDelay]
	}, func() (int, error) {
		attempts++
		if attempts < maxAttempts {
			return 0, busy
		}
		return 42, nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("RetryOnOverload timed out after %d attempts — sleepFor likely exceeded MaxDelay", attempts)
		}
		t.Fatalf("RetryOnOverload() err = %v, want nil", err)
	}
	if attempts != maxAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, maxAttempts)
	}
}
