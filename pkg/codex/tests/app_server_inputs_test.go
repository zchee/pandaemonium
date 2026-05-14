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

package codex_test

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zchee/pandaemonium/pkg/codex"
)

func TestAppServerInputsPort(t *testing.T) {
	t.Run("remote image input reaches app-server boundary", func(t *testing.T) {
		sdk, ctx := newInputCaptureCodex(t)
		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		result, err := thread.Run(ctx, []codex.InputItem{
			codex.TextInput{Text: "Describe the remote image."},
			codex.ImageInput{URL: "https://example.com/codex.png"},
		}, nil)
		if err != nil {
			t.Fatalf("Thread.Run(remote image input) error = %v", err)
		}
		if result.FinalResponse != "remote image received" {
			t.Fatalf("FinalResponse = %q, want remote image received", result.FinalResponse)
		}
	})

	t.Run("local image input reaches app-server boundary", func(t *testing.T) {
		sdk, ctx := newInputCaptureCodex(t)
		localImage := filepath.Join(t.TempDir(), "local.png")
		if err := os.WriteFile(localImage, tinyPNGBytes(t), 0o644); err != nil {
			t.Fatalf("os.WriteFile(local image) error = %v", err)
		}
		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		result, err := thread.Run(ctx, []codex.InputItem{
			codex.TextInput{Text: "Describe the local image."},
			codex.LocalImageInput{Path: localImage},
		}, nil)
		if err != nil {
			t.Fatalf("Thread.Run(local image input) error = %v", err)
		}
		if result.FinalResponse != "local image received" {
			t.Fatalf("FinalResponse = %q, want local image received", result.FinalResponse)
		}
	})

	t.Run("skill input reaches app-server boundary", func(t *testing.T) {
		sdk, ctx := newInputCaptureCodex(t)
		skillPath := filepath.Join(t.TempDir(), ".agents", "skills", "demo", "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
			t.Fatalf("os.MkdirAll(skill dir) error = %v", err)
		}
		if err := os.WriteFile(skillPath, []byte("---\nname: demo\ndescription: demo skill\n---\n\nUse the word cobalt.\n"), 0o644); err != nil {
			t.Fatalf("os.WriteFile(skill file) error = %v", err)
		}
		thread, err := sdk.ThreadStart(ctx, nil)
		if err != nil {
			t.Fatalf("ThreadStart() error = %v", err)
		}
		result, err := thread.Run(ctx, []codex.InputItem{
			codex.TextInput{Text: "Use the selected skill."},
			codex.SkillInput{Name: "demo", Path: skillPath},
		}, nil)
		if err != nil {
			t.Fatalf("Thread.Run(skill input) error = %v", err)
		}
		if result.FinalResponse != "skill received" {
			t.Fatalf("FinalResponse = %q, want skill received", result.FinalResponse)
		}
	})
}

func newInputCaptureCodex(t *testing.T) (*codex.Codex, context.Context) {
	t.Helper()
	sdk := newHelperCodex(t, "input_capture")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return sdk, ctx
}

func tinyPNGBytes(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode tiny PNG fixture: %v", err)
	}
	return data
}
