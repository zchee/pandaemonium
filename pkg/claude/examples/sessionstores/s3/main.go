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

// Command s3 demonstrates an AWS S3-backed [claude.SessionStore] adapter
// using github.com/aws/aws-sdk-go-v2.
//
// This is a separate Go module (its own go.mod) so that the AWS SDK never
// enters pkg/claude's own module graph (spec §Non-Goals).
//
// Each session is stored as a JSON object at key "sessions/<id>.json" in the
// configured S3 bucket.
//
// Port of examples/session_stores/s3.py from claude-agent-sdk-python.
//
// Usage:
//
//	S3_BUCKET=my-claude-sessions \
//	AWS_REGION=us-east-1 \
//	RUN_REAL_CLAUDE_TESTS=1 go run .
package main

import (
	"bytes"
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/zchee/pandaemonium/pkg/claude"
)

// s3SessionStore implements [claude.SessionStore] using AWS S3.
type s3SessionStore struct {
	client *s3.Client
	bucket string
}

// sessionRecord is the JSON shape stored per S3 object.
type sessionRecord struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
}

func objectKey(id string) string { return "sessions/" + id + ".json" }

func (s *s3SessionStore) Load(ctx context.Context, id string) (*claude.Session, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    ptr(objectKey(id)),
	})
	if err != nil {
		var nf *types.NoSuchKey
		if errors.As(err, &nf) {
			return nil, fmt.Errorf("session %q: %w", id, claude.ErrSessionNotFound)
		}
		return nil, fmt.Errorf("load session %q: %w", id, err)
	}
	defer out.Body.Close()
	raw, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("read session %q: %w", id, err)
	}
	var rec sessionRecord
	if err := stdjson.Unmarshal(raw, &rec); err != nil {
		return nil, fmt.Errorf("decode session %q: %w", id, err)
	}
	return &claude.Session{ID: rec.ID, ParentID: rec.ParentID}, nil
}

func (s *s3SessionStore) Save(ctx context.Context, sess *claude.Session) error {
	if sess == nil || sess.ID == "" {
		return errors.New("Save: session must have a non-empty ID")
	}
	raw, err := stdjson.Marshal(sessionRecord{ID: sess.ID, ParentID: sess.ParentID})
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         ptr(objectKey(sess.ID)),
		Body:        bytes.NewReader(raw),
		ContentType: ptr("application/json"),
	})
	return err
}

func (s *s3SessionStore) Append(_ context.Context, _ string, _ []claude.Message) error {
	// Appending the sealed Message interface requires a custom marshaller;
	// simplified to a no-op in this example.
	return nil
}

func (s *s3SessionStore) List(ctx context.Context) ([]string, error) {
	prefix := "sessions/"
	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: &prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	var ids []string
	for _, obj := range out.Contents {
		key := *obj.Key
		id := strings.TrimSuffix(strings.TrimPrefix(key, prefix), ".json")
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *s3SessionStore) Delete(ctx context.Context, id string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    ptr(objectKey(id)),
	})
	return err
}

func (s *s3SessionStore) Fork(ctx context.Context, sessionID, _ string) (*claude.Session, error) {
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

func (s *s3SessionStore) Summary(ctx context.Context, id string) (string, error) {
	sess, err := s.Load(ctx, id)
	if err != nil {
		return "", err
	}
	if sess.ParentID != "" {
		return fmt.Sprintf("session %s (forked from %s)", sess.ID, sess.ParentID), nil
	}
	return fmt.Sprintf("session %s", sess.ID), nil
}

func ptr[T any](v T) *T { return &v }

func main() {
	if os.Getenv("RUN_REAL_CLAUDE_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "s3: set RUN_REAL_CLAUDE_TESTS=1, S3_BUCKET, and AWS_REGION to run.")
		return
	}

	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		log.Fatal("s3: S3_BUCKET is required")
	}

	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load AWS config: %v", err)
	}

	store := &s3SessionStore{
		client: s3.NewFromConfig(cfg),
		bucket: bucket,
	}

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
