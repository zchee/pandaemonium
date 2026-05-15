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

	"github.com/go-json-experiment/json/jsontext"
)

// Session is a persisted conversation session managed by a [SessionStore].
type Session struct {
	// ID is the unique session identifier assigned by the CLI.
	ID string `json:"session_id,omitzero"`

	// ParentID is the ID of the session this was forked from, if any.
	ParentID string `json:"parent_session_id,omitzero"`

	// Messages is the ordered list of messages in this session.
	// Serialization of the sealed Message interface is handled in Phase B.
	Messages []Message `json:"-"`

	// Raw preserves unknown top-level fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}

// SessionStore is the pluggable persistence interface for claude sessions.
//
// Implementations must be safe for concurrent use. The package ships an
// in-memory implementation ([NewInMemorySessionStore]) and a conformance
// harness at pkg/claude/testing/sessionstoreconformance.
//
// No built-in database backends (Postgres, Redis, S3) are provided; example
// adapters under pkg/claude/examples/sessionstores/ are separate Go modules.
type SessionStore interface {
	// Load retrieves the session with the given ID.
	// Returns an error satisfying errors.Is(err, ErrSessionNotFound) if absent.
	Load(ctx context.Context, sessionID string) (*Session, error)

	// Save persists a session, creating or replacing as needed.
	Save(ctx context.Context, session *Session) error

	// Append adds messages to an existing session's history.
	Append(ctx context.Context, sessionID string, messages []Message) error

	// List returns the IDs of all stored sessions.
	List(ctx context.Context) ([]string, error)

	// Delete removes the session with the given ID.
	Delete(ctx context.Context, sessionID string) error

	// Fork creates a new session branching from fromMessageID in the source
	// session. The forked session receives a new ID.
	Fork(ctx context.Context, sessionID, fromMessageID string) (*Session, error)

	// Summary returns a human-readable summary of the session.
	Summary(ctx context.Context, sessionID string) (string, error)
}

// ErrSessionNotFound is returned by [SessionStore.Load] when the requested
// session does not exist.
var ErrSessionNotFound = errors.New("session not found")

// inMemorySessionStore is the built-in, non-persistent SessionStore.
// Implementation is filled in Phase F.
type inMemorySessionStore struct{}

// NewInMemorySessionStore returns a new in-memory [SessionStore].
//
// The store holds all sessions in process memory and is lost when the process
// exits. It is suitable for testing and short-lived scripts.
//
// The conformance harness at pkg/claude/testing/sessionstoreconformance
// validates that this implementation satisfies the full SessionStore contract.
func NewInMemorySessionStore() SessionStore {
	return &inMemorySessionStore{}
}

func (*inMemorySessionStore) Load(_ context.Context, _ string) (*Session, error) {
	return nil, errors.ErrUnsupported
}

func (*inMemorySessionStore) Save(_ context.Context, _ *Session) error {
	return errors.ErrUnsupported
}

func (*inMemorySessionStore) Append(_ context.Context, _ string, _ []Message) error {
	return errors.ErrUnsupported
}

func (*inMemorySessionStore) List(_ context.Context) ([]string, error) {
	return nil, errors.ErrUnsupported
}

func (*inMemorySessionStore) Delete(_ context.Context, _ string) error {
	return errors.ErrUnsupported
}

func (*inMemorySessionStore) Fork(_ context.Context, _, _ string) (*Session, error) {
	return nil, errors.ErrUnsupported
}

func (*inMemorySessionStore) Summary(_ context.Context, _ string) (string, error) {
	return "", errors.ErrUnsupported
}
