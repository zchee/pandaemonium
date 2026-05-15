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

// Command postgres demonstrates a PostgreSQL-backed [claude.SessionStore]
// adapter using github.com/jackc/pgx/v5.
//
// This is a separate Go module (its own go.mod) so that the pgx driver never
// enters pkg/claude's own module graph (spec §Non-Goals).
//
// Port of examples/session_stores/postgres.py from claude-agent-sdk-python.
//
// Schema (run once before starting):
//
//	CREATE TABLE claude_sessions (
//	    id         TEXT PRIMARY KEY,
//	    parent_id  TEXT,
//	    messages   JSONB NOT NULL DEFAULT '[]',
//	    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
//	);
//
// Usage:
//
//	DATABASE_URL=postgres://user:pass@localhost/dbname \
//	RUN_REAL_CLAUDE_TESTS=1 go run .
package main

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zchee/pandaemonium/pkg/claude"
)

// pgSessionStore implements [claude.SessionStore] using PostgreSQL.
type pgSessionStore struct {
	pool *pgxpool.Pool
}

// pgMessage is a JSON-serialisable wrapper for a [claude.Message].
// Because Message is a sealed interface we store the raw JSON bytes and
// a "type" discriminator, reconstructing the concrete value on load.
type pgMessage struct {
	Type string             `json:"type"`
	Raw  stdjson.RawMessage `json:"raw"`
}

func (s *pgSessionStore) Load(ctx context.Context, id string) (*claude.Session, error) {
	var parentID *string
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT parent_id, messages FROM claude_sessions WHERE id = $1`, id).
		Scan(&parentID, &raw)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("session %q: %w", id, claude.ErrSessionNotFound)
		}
		return nil, fmt.Errorf("load session %q: %w", id, err)
	}
	sess := &claude.Session{ID: id}
	if parentID != nil {
		sess.ParentID = *parentID
	}
	_ = raw // messages omitted from this example (complex sealed-interface serde)
	return sess, nil
}

func (s *pgSessionStore) Save(ctx context.Context, sess *claude.Session) error {
	if sess == nil || sess.ID == "" {
		return errors.New("Save: session must have a non-empty ID")
	}
	raw, err := stdjson.Marshal([]pgMessage{}) // simplified: no message serde
	if err != nil {
		return fmt.Errorf("save session: marshal messages: %w", err)
	}
	var parentID *string
	if sess.ParentID != "" {
		parentID = &sess.ParentID
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO claude_sessions (id, parent_id, messages)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO UPDATE SET parent_id=$2, messages=$3`,
		sess.ID, parentID, raw)
	return err
}

func (s *pgSessionStore) Append(_ context.Context, _ string, _ []claude.Message) error {
	// Appending the sealed Message interface to Postgres requires a custom
	// marshaller; simplified to a no-op in this example.
	return nil
}

func (s *pgSessionStore) List(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM claude_sessions ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, rows.Err()
}

func (s *pgSessionStore) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM claude_sessions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete session %q: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("session %q: %w", id, claude.ErrSessionNotFound)
	}
	return nil
}

func (s *pgSessionStore) Fork(ctx context.Context, sessionID, _ string) (*claude.Session, error) {
	src, err := s.Load(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	newID := fmt.Sprintf("%s-fork", sessionID)
	forked := &claude.Session{ID: newID, ParentID: src.ID}
	if err := s.Save(ctx, forked); err != nil {
		return nil, fmt.Errorf("fork: save child: %w", err)
	}
	return forked, nil
}

func (s *pgSessionStore) Summary(ctx context.Context, id string) (string, error) {
	sess, err := s.Load(ctx, id)
	if err != nil {
		return "", err
	}
	if sess.ParentID != "" {
		return fmt.Sprintf("session %s (forked from %s)", sess.ID, sess.ParentID), nil
	}
	return fmt.Sprintf("session %s", sess.ID), nil
}

func isNotFound(err error) bool {
	return err != nil && err.Error() == "no rows in result set"
}

func main() {
	if os.Getenv("RUN_REAL_CLAUDE_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "postgres: set RUN_REAL_CLAUDE_TESTS=1 and DATABASE_URL to run.")
		return
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("postgres: DATABASE_URL is required")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	store := &pgSessionStore{pool: pool}

	opts := &claude.Options{
		SessionStore: store,
		MaxTurns:     1,
	}

	for msg, err := range claude.Query(ctx, "What is 2+2?", opts) {
		if err != nil {
			log.Fatal(err)
		}
		if am, ok := msg.(claude.AssistantMessage); ok {
			for _, b := range am.Content {
				if tb, ok := b.(claude.TextBlock); ok {
					fmt.Println(tb.Text)
				}
			}
		}
	}
}
