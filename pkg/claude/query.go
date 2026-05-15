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

package claude

import (
	"context"
	"errors"
	"iter"
)

// Query sends prompt to the claude CLI and returns an iterator over the
// streamed [Message] values. It is the one-shot convenience entry point; for
// multi-turn interactive sessions use [NewClient].
//
// The iterator creates and owns a private [ClaudeSDKClient]. The client is
// closed automatically when the iterator is exhausted or when the caller breaks
// early from the range loop. This is safe because defer cli.Close() is placed
// INSIDE the yield closure, not after the for-range; early break therefore
// always triggers cleanup (AC-i6).
//
// Example — consume all messages:
//
//	for msg, err := range claude.Query(ctx, "hello", nil) {
//	    if err != nil { log.Fatal(err) }
//	    fmt.Println(msg)
//	}
//
// opts may be nil; a nil Options is equivalent to a zero-value Options.
//
// The body is stubbed until Phase C. Callers may range over the returned
// iterator; the first and only value will be (nil, errors.ErrUnsupported).
func Query(ctx context.Context, prompt string, opts *Options) iter.Seq2[Message, error] {
	_, _ = prompt, opts
	return func(yield func(Message, error) bool) {
		// Phase C replaces this stub with the real implementation:
		//
		//   cli, err := NewClient(ctx, opts)
		//   if err != nil { yield(nil, err); return }
		//   defer cli.Close()  // MUST be inside the yield closure for early-break safety
		//   if err := cli.Query(ctx, prompt); err != nil { yield(nil, err); return }
		//   for msg, err := range cli.ReceiveResponse(ctx) {
		//       if !yield(msg, err) { return }
		//       if _, isResult := any(msg).(ResultMessage); isResult { return }
		//   }
		_ = ctx
		yield(nil, errors.ErrUnsupported)
	}
}
