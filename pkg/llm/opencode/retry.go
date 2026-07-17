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

package opencode

import (
	"context"
	"fmt"
	rand "math/rand/v2"
	"time"
)

// RetryConfig controls RetryOnOverload behavior and bounds SSE bus
// auto-reconnect attempts. This wrapper is the only retry layer: the
// hand-built HTTP client never retries on its own.
type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	JitterRatio  float64
}

// withDefaults returns a copy of cfg with zero-valued fields replaced by their
// defaults. A negative JitterRatio disables jitter.
func (cfg RetryConfig) withDefaults() RetryConfig {
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialDelay == 0 {
		cfg.InitialDelay = 250 * time.Millisecond
	}
	if cfg.MaxDelay == 0 {
		cfg.MaxDelay = 2 * time.Second
	}
	if cfg.JitterRatio == 0 {
		cfg.JitterRatio = 0.2
	} else if cfg.JitterRatio < 0 {
		cfg.JitterRatio = 0 // negative disables jitter
	}
	return cfg
}

// validate reports whether cfg holds usable retry parameters.
func (cfg RetryConfig) validate() error {
	if cfg.JitterRatio > 1 {
		return fmt.Errorf("jitter ratio %g out of range [0, 1]", cfg.JitterRatio)
	}
	if cfg.MaxAttempts < 1 {
		return fmt.Errorf("max attempts must be >= 1")
	}
	return nil
}

// sleepDelay blocks for the jittered, clamped backoff delay derived from
// delay, honoring ctx cancellation, and returns the next (doubled) delay.
func (cfg RetryConfig) sleepDelay(ctx context.Context, delay time.Duration) (time.Duration, error) {
	// Clamp delay before computing jitter so jitter range is bounded by MaxDelay.
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}
	sleepFor := delay
	if cfg.JitterRatio > 0 {
		jitter := float64(delay) * cfg.JitterRatio
		sleepFor = time.Duration(float64(delay) - jitter + rand.Float64()*2*jitter) //nolint:gosec // G404: non-cryptographic jitter for retry backoff
	}
	sleepFor = min(max(0, sleepFor), cfg.MaxDelay)
	if sleepFor > 0 {
		timer := time.NewTimer(sleepFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}

	delay *= 2
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}
	return delay, nil
}

// RetryOnOverload retries op when it returns a retryable overload error.
func RetryOnOverload[T any](ctx context.Context, cfg RetryConfig, op func() (T, error)) (T, error) {
	cfg = cfg.withDefaults()

	var zero T
	if err := cfg.validate(); err != nil {
		return zero, err
	}

	delay := cfg.InitialDelay
	for attempt := 1; ; attempt++ {
		result, err := op()
		if err == nil {
			return result, nil
		}
		if attempt >= cfg.MaxAttempts || !isRetryableOp(err) {
			return zero, err
		}

		delay, err = cfg.sleepDelay(ctx, delay)
		if err != nil {
			return zero, err
		}
	}
}

// isRetryableOp reports whether err should be retried. It checks the internal
// retryable() marker interface first (used by tests), then falls back to the
// public IsRetryableError which recognizes HTTP overload responses.
func isRetryableOp(err error) bool {
	if _, ok := err.(interface{ retryable() }); ok {
		return true
	}
	return IsRetryableError(err)
}
