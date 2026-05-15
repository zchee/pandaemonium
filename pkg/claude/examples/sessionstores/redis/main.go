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

// Command redis demonstrates a Redis-backed [claude.SessionStore] adapter
// using github.com/redis/go-redis/v9.
//
// This is a separate Go module (its own go.mod) so that the go-redis driver
// never enters pkg/claude's own module graph (spec §Non-Goals).
//
// Each session is stored as a Redis hash keyed by "claude:session:<id>":
//   - "parent_id" → parent session ID (or "")
//   - "messages"  → JSON-encoded message list (simplified)
//
// Port of examples/session_stores/redis.py from claude-agent-sdk-python.
//
// Usage:
//
//	REDIS_URL=redis://localhost:6379 \
//	RUN_REAL_CLAUDE_TESTS=1 go run .
package main

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"log"
	"os"

	goredis "github.com/redis/go-redis/v9"
	"github.com/zchee/pandaemonium/pkg/claude"
)

const keyPrefix = "claude:session:"

// redisSessionStore implements [claude.SessionStore] using Redis hashes.
type redisSessionStore struct {
	client *goredis.Client
}

func sessionKey(id string) string { return keyPrefix + id }

func (s *redisSessionStore) Load(ctx context.Context, id string) (*claude.Session, error) {
	vals, err := s.client.HGetAll(ctx, sessionKey(id)).Result()
	if err != nil {
		return nil, fmt.Errorf("load session %q: %w", id, err)
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("session %q: %w", id, claude.ErrSessionNotFound)
	}
	sess := &claude.Session{
		ID:       id,
		ParentID: vals["parent_id"],
	}
	return sess, nil
}

func (s *redisSessionStore) Save(ctx context.Context, sess *claude.Session) error {
	if sess == nil || sess.ID == "" {
		return errors.New("Save: session must have a non-empty ID")
	}
	raw, _ := stdjson.Marshal([]any{}) // simplified: no message serde
	return s.client.HSet(ctx, sessionKey(sess.ID),
		"parent_id", sess.ParentID,
		"messages", string(raw),
	).Err()
}

func (s *redisSessionStore) Append(_ context.Context, _ string, _ []claude.Message) error {
	// Appending the sealed Message interface requires a custom marshaller;
	// simplified to a no-op in this example.
	return nil
}

func (s *redisSessionStore) List(ctx context.Context) ([]string, error) {
	keys, err := s.client.Keys(ctx, keyPrefix+"*").Result()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = k[len(keyPrefix):]
	}
	return ids, nil
}

func (s *redisSessionStore) Delete(ctx context.Context, id string) error {
	n, err := s.client.Del(ctx, sessionKey(id)).Result()
	if err != nil {
		return fmt.Errorf("delete session %q: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("session %q: %w", id, claude.ErrSessionNotFound)
	}
	return nil
}

func (s *redisSessionStore) Fork(ctx context.Context, sessionID, _ string) (*claude.Session, error) {
	src, err := s.Load(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	newID := sessionID + "-fork"
	forked := &claude.Session{ID: newID, ParentID: src.ID}
	if err := s.Save(ctx, forked); err != nil {
		return nil, fmt.Errorf("fork: save child: %w", err)
	}
	return forked, nil
}

func (s *redisSessionStore) Summary(ctx context.Context, id string) (string, error) {
	sess, err := s.Load(ctx, id)
	if err != nil {
		return "", err
	}
	if sess.ParentID != "" {
		return fmt.Sprintf("session %s (forked from %s)", sess.ID, sess.ParentID), nil
	}
	return fmt.Sprintf("session %s", sess.ID), nil
}

func main() {
	if os.Getenv("RUN_REAL_CLAUDE_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "redis: set RUN_REAL_CLAUDE_TESTS=1 and REDIS_URL to run.")
		return
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := goredis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("parse REDIS_URL: %v", err)
	}

	ctx := context.Background()

	store := &redisSessionStore{
		client: goredis.NewClient(opt),
	}
	defer store.client.Close()

	opts := &claude.Options{
		SessionStore: store,
		MaxTurns:     1,
	}

	for msg, qerr := range claude.Query(ctx, "What is 2+2?", opts) {
		if qerr != nil {
			log.Fatal(qerr)
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
