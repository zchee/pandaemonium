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
	"context"
	"fmt"
	rand "math/rand/v2"
	"time"
)

// RetryConfig controls RetryOnOverload behavior.
type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	JitterRatio  float64
}

// RetryOnOverload retries op when it returns a retryable overload error.
func RetryOnOverload[T any](ctx context.Context, cfg RetryConfig, op func() (T, error)) (T, error) {
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
	}
	var zero T
	if cfg.MaxAttempts < 1 {
		return zero, fmt.Errorf("max attempts must be >= 1")
	}
	delay := cfg.InitialDelay
	for attempt := 1; ; attempt++ {
		result, err := op()
		if err == nil {
			return result, nil
		}
		if attempt >= cfg.MaxAttempts || !IsRetryableError(err) {
			return zero, err
		}
		sleepFor := delay
		if cfg.JitterRatio > 0 {
			jitter := float64(delay) * cfg.JitterRatio
			sleepFor = time.Duration(float64(delay) - jitter + rand.Float64()*2*jitter)
		}
		if sleepFor > cfg.MaxDelay {
			sleepFor = cfg.MaxDelay
		}
		if sleepFor > 0 {
			timer := time.NewTimer(sleepFor)
			select {
			case <-ctx.Done():
				timer.Stop()
				return zero, ctx.Err()
			case <-timer.C:
			}
		}
		delay *= 2
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}
}
