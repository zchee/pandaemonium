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
	"crypto/rand"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"

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

	// List returns the IDs of all stored sessions, sorted lexicographically.
	List(ctx context.Context) ([]string, error)

	// Delete removes the session with the given ID.
	Delete(ctx context.Context, sessionID string) error

	// Fork creates a new session branching from fromMessageID in the source
	// session. The forked session receives a new ID and includes only the
	// messages up to and including the one whose raw "id" field matches
	// fromMessageID. If fromMessageID is empty, all messages are copied.
	Fork(ctx context.Context, sessionID, fromMessageID string) (*Session, error)

	// Summary returns a human-readable summary of the session.
	Summary(ctx context.Context, sessionID string) (string, error)
}

// ErrSessionNotFound is returned by [SessionStore.Load] when the requested
// session does not exist.
var ErrSessionNotFound = errors.New("session not found")

// ── in-memory implementation ─────────────────────────────────────────────────

// inMemorySessionStore is the built-in, non-persistent [SessionStore].
// It holds all sessions in process memory; data is lost when the process exits.
type inMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewInMemorySessionStore returns a new in-memory [SessionStore].
//
// The store holds all sessions in process memory and is lost when the process
// exits. It is suitable for testing and short-lived scripts.
//
// The conformance harness at pkg/claude/testing/sessionstoreconformance
// validates that this implementation satisfies the full SessionStore contract.
func NewInMemorySessionStore() SessionStore {
	return &inMemorySessionStore{
		sessions: make(map[string]*Session),
	}
}

// Load retrieves the session with the given ID.
func (s *inMemorySessionStore) Load(_ context.Context, id string) (*Session, error) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %q: %w", id, ErrSessionNotFound)
	}
	return shallowCopySession(sess), nil
}

// Save persists a session, creating or replacing as needed.
func (s *inMemorySessionStore) Save(_ context.Context, sess *Session) error {
	if sess == nil {
		return &CLIConnectionError{Message: "Save: session must not be nil"}
	}
	if sess.ID == "" {
		return &CLIConnectionError{Message: "Save: session ID must not be empty"}
	}
	cp := shallowCopySession(sess)
	s.mu.Lock()
	s.sessions[cp.ID] = cp
	s.mu.Unlock()
	return nil
}

// Append adds messages to an existing session's history.
func (s *inMemorySessionStore) Append(_ context.Context, id string, messages []Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("session %q: %w", id, ErrSessionNotFound)
	}
	sess.Messages = append(sess.Messages, messages...)
	return nil
}

// List returns the IDs of all stored sessions, sorted lexicographically.
func (s *inMemorySessionStore) List(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// Delete removes the session with the given ID.
func (s *inMemorySessionStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("session %q: %w", id, ErrSessionNotFound)
	}
	delete(s.sessions, id)
	return nil
}

// Fork creates a new session branching from fromMessageID in the source session.
func (s *inMemorySessionStore) Fork(_ context.Context, sessionID, fromMessageID string) (*Session, error) {
	s.mu.RLock()
	src, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %q: %w", sessionID, ErrSessionNotFound)
	}

	// Determine how many messages to include in the fork.
	cutoff := len(src.Messages)
	if fromMessageID != "" {
		cutoff = 0 // default to empty if ID is not found
		for i, msg := range src.Messages {
			if extractMessageID(msg) == fromMessageID {
				cutoff = i + 1
				break
			}
		}
	}

	newID, err := newSessionID()
	if err != nil {
		return nil, fmt.Errorf("fork: generate session ID: %w", err)
	}

	forked := &Session{
		ID:       newID,
		ParentID: sessionID,
		Messages: append([]Message(nil), src.Messages[:cutoff]...),
	}

	s.mu.Lock()
	s.sessions[newID] = forked
	s.mu.Unlock()

	return shallowCopySession(forked), nil
}

// Summary returns a human-readable summary of the session.
func (s *inMemorySessionStore) Summary(_ context.Context, id string) (string, error) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("session %q: %w", id, ErrSessionNotFound)
	}
	if sess.ParentID != "" {
		return fmt.Sprintf("session %s: %d messages (forked from %s)",
			sess.ID, len(sess.Messages), sess.ParentID), nil
	}
	return fmt.Sprintf("session %s: %d messages", sess.ID, len(sess.Messages)), nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// newSessionID generates a cryptographically random 16-hex-character session ID.
func newSessionID() (string, error) {
	var buf [8]byte
	if _, err := io.ReadFull(rand.Reader, buf[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}

// shallowCopySession returns a shallow copy of sess with an independent
// Messages slice. The Message values themselves are not deep-copied because
// they are immutable struct values.
func shallowCopySession(sess *Session) *Session {
	cp := *sess
	if sess.Messages != nil {
		cp.Messages = append([]Message(nil), sess.Messages...)
	}
	return &cp
}

// extractMessageID extracts the top-level "id" string field from a message's
// Raw inline JSON, returning "" if absent or unparseable.
func extractMessageID(msg Message) string {
	raw := msg.jsonRaw()
	if len(raw) == 0 {
		return ""
	}
	var fields map[string]stdjson.RawMessage
	if err := stdjson.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	idRaw, ok := fields["id"]
	if !ok {
		return ""
	}
	var id string
	if err := stdjson.Unmarshal(idRaw, &id); err != nil {
		return ""
	}
	return id
}
