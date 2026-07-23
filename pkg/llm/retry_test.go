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

package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

// errTransient stands in for a package-specific retryable error.
var errTransient = errors.New("transient overload")

func isTransient(err error) bool { return errors.Is(err, errTransient) }

func TestRetryConfigWithDefaults(t *testing.T) {
	t.Parallel()

	cfg := RetryConfig{}.WithDefaults()
	if cfg.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 250*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 250ms", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 2*time.Second {
		t.Errorf("MaxDelay = %v, want 2s", cfg.MaxDelay)
	}
	if cfg.JitterRatio != 0.2 {
		t.Errorf("JitterRatio = %v, want 0.2", cfg.JitterRatio)
	}

	negative := RetryConfig{JitterRatio: -1}.WithDefaults()
	if negative.JitterRatio != 0 {
		t.Errorf("negative JitterRatio = %v, want 0 (disabled)", negative.JitterRatio)
	}
}

func TestRetryOn(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cfg       RetryConfig
		results   []error // successive op outcomes; nil = success
		wantCalls int
		wantErr   bool
	}{
		"success: first attempt needs no retry": {
			cfg:       RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond},
			results:   []error{nil},
			wantCalls: 1,
		},
		"success: retryable failures then success": {
			cfg:       RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond},
			results:   []error{errTransient, errTransient, nil},
			wantCalls: 3,
		},
		"error: non-retryable error stops immediately": {
			cfg:       RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond},
			results:   []error{errors.New("boom")},
			wantCalls: 1,
			wantErr:   true,
		},
		"error: attempts exhausted returns last error": {
			cfg:       RetryConfig{MaxAttempts: 2, InitialDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond},
			results:   []error{errTransient, errTransient},
			wantCalls: 2,
			wantErr:   true,
		},
		"error: invalid jitter ratio rejected": {
			cfg:       RetryConfig{JitterRatio: 2},
			results:   []error{nil},
			wantCalls: 0,
			wantErr:   true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			got, err := RetryOn(t.Context(), tt.cfg, isTransient, func() (string, error) {
				result := tt.results[calls]
				calls++
				if result != nil {
					return "", result
				}
				return "ok", nil
			})
			if calls != tt.wantCalls {
				t.Errorf("op called %d times, want %d", calls, tt.wantCalls)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %t", err, tt.wantErr)
			}
			if !tt.wantErr && got != "ok" {
				t.Errorf("result = %q, want ok", got)
			}
		})
	}
}

func TestRetryOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	_, err := RetryOn(ctx, RetryConfig{MaxAttempts: 5, InitialDelay: time.Hour, MaxDelay: time.Hour}, isTransient, func() (int, error) {
		calls++
		cancel() // cancel while the retry loop would sleep
		return 0, errTransient
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("op called %d times, want 1 (canceled during backoff)", calls)
	}
}

func TestRetryBackoffDoublesAndClamps(t *testing.T) {
	t.Parallel()

	cfg := RetryConfig{
		MaxAttempts:  4,
		InitialDelay: time.Millisecond,
		MaxDelay:     4 * time.Millisecond,
		JitterRatio:  -1, // deterministic: jitter disabled
	}.WithDefaults()

	delay := cfg.InitialDelay
	var observed []time.Duration
	for range 3 {
		start := time.Now()
		next, err := cfg.SleepDelay(t.Context(), delay)
		if err != nil {
			t.Fatalf("SleepDelay: %v", err)
		}
		observed = append(observed, time.Since(start))
		delay = next
	}

	// 1ms -> 2ms -> 4ms doubling, clamped at MaxDelay for the follow-up.
	if delay != cfg.MaxDelay {
		t.Errorf("final delay = %v, want clamped %v", delay, cfg.MaxDelay)
	}
	for i, slept := range observed {
		if slept > 500*time.Millisecond {
			t.Errorf("sleep %d took %v; backoff not bounded", i, slept)
		}
	}
}
