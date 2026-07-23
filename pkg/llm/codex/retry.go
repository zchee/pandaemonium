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

	"github.com/zchee/pandaemonium/pkg/llm"
)

// RetryOnOverload retries op when it returns a retryable overload error.
func RetryOnOverload[T any](ctx context.Context, cfg llm.RetryConfig, op func() (T, error)) (T, error) {
	return llm.RetryOn(ctx, cfg, isRetryableOp, op)
}

// isRetryableOp reports whether err should be retried. It checks the internal
// retryable() marker interface first (used by tests), then falls back to the
// public IsRetryableError which recognizes server-side JSONRPCError kinds.
func isRetryableOp(err error) bool {
	if _, ok := err.(interface{ retryable() }); ok {
		return true
	}
	return IsRetryableError(err)
}
