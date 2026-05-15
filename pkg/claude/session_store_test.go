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

package claude_test

import (
	"testing"

	"github.com/zchee/pandaemonium/pkg/claude"
	"github.com/zchee/pandaemonium/pkg/claude/testing/sessionstoreconformance"
)

// TestInMemorySessionStore_Conformance runs the full SessionStore conformance
// harness against NewInMemorySessionStore.
func TestInMemorySessionStore_Conformance(t *testing.T) {
	t.Parallel()
	sessionstoreconformance.Run(t, func() claude.SessionStore {
		return claude.NewInMemorySessionStore()
	})
}
