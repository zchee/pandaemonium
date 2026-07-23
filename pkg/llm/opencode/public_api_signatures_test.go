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

package opencode

import (
	"context"
	"iter"
	"reflect"
	"testing"

	"github.com/zchee/pandaemonium/pkg/llm"
)

// TestPublicAPISignatures is AC2: the exported facade surface mapped from
// pkg/llm/codex (plan section 2) must exist with these exact signatures;
// drift fails this test. Method type strings include the receiver as the
// first parameter.
func TestPublicAPISignatures(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		typ    reflect.Type
		method string
		want   string
	}{
		// Opencode facade (codex Codex analog).
		"Opencode.Health":        {typ: reflect.TypeFor[*Opencode](), method: "Health", want: "func(*opencode.Opencode, context.Context) (opencode.Health, error)"},
		"Opencode.Close":         {typ: reflect.TypeFor[*Opencode](), method: "Close", want: "func(*opencode.Opencode) error"},
		"Opencode.Client":        {typ: reflect.TypeFor[*Opencode](), method: "Client", want: "func(*opencode.Opencode) *opencode.Client"},
		"Opencode.SessionStart":  {typ: reflect.TypeFor[*Opencode](), method: "SessionStart", want: "func(*opencode.Opencode, context.Context, *opencode.SessionNewParams) (*opencode.Session, error)"},
		"Opencode.SessionResume": {typ: reflect.TypeFor[*Opencode](), method: "SessionResume", want: "func(*opencode.Opencode, context.Context, string) (*opencode.Session, error)"},
		"Opencode.SessionFork":   {typ: reflect.TypeFor[*Opencode](), method: "SessionFork", want: "func(*opencode.Opencode, context.Context, string, *opencode.SessionForkParams) (*opencode.Session, error)"},
		"Opencode.SessionList":   {typ: reflect.TypeFor[*Opencode](), method: "SessionList", want: "func(*opencode.Opencode, context.Context) ([]opencode.SessionInfo, error)"},
		"Opencode.SessionDelete": {typ: reflect.TypeFor[*Opencode](), method: "SessionDelete", want: "func(*opencode.Opencode, context.Context, string) (bool, error)"},
		"Opencode.Providers":     {typ: reflect.TypeFor[*Opencode](), method: "Providers", want: "func(*opencode.Opencode, context.Context) (opencode.ProvidersResponse, error)"},

		// Session (codex Thread analog).
		"Session.ID":        {typ: reflect.TypeFor[*Session](), method: "ID", want: "func(*opencode.Session) string"},
		"Session.Run":       {typ: reflect.TypeFor[*Session](), method: "Run", want: "func(*opencode.Session, context.Context, opencode.RunInput, *opencode.PromptParams) (opencode.RunResult, error)"},
		"Session.Turn":      {typ: reflect.TypeFor[*Session](), method: "Turn", want: "func(*opencode.Session, context.Context, opencode.RunInput, *opencode.PromptParams) (*opencode.TurnHandle, error)"},
		"Session.Read":      {typ: reflect.TypeFor[*Session](), method: "Read", want: "func(*opencode.Session, context.Context) (opencode.SessionRead, error)"},
		"Session.SetTitle":  {typ: reflect.TypeFor[*Session](), method: "SetTitle", want: "func(*opencode.Session, context.Context, string) (opencode.SessionInfo, error)"},
		"Session.Summarize": {typ: reflect.TypeFor[*Session](), method: "Summarize", want: "func(*opencode.Session, context.Context, string, string) (bool, error)"},
		"Session.Revert":    {typ: reflect.TypeFor[*Session](), method: "Revert", want: "func(*opencode.Session, context.Context, string, string) (opencode.SessionInfo, error)"},
		"Session.Unrevert":  {typ: reflect.TypeFor[*Session](), method: "Unrevert", want: "func(*opencode.Session, context.Context) (opencode.SessionInfo, error)"},
		"Session.Shell":     {typ: reflect.TypeFor[*Session](), method: "Shell", want: "func(*opencode.Session, context.Context, string, string) (opencode.MessageWithParts, error)"},
		"Session.Command":   {typ: reflect.TypeFor[*Session](), method: "Command", want: "func(*opencode.Session, context.Context, *opencode.CommandParams) (opencode.PromptResponse, error)"},
		"Session.Share":     {typ: reflect.TypeFor[*Session](), method: "Share", want: "func(*opencode.Session, context.Context) (opencode.SessionInfo, error)"},
		"Session.Unshare":   {typ: reflect.TypeFor[*Session](), method: "Unshare", want: "func(*opencode.Session, context.Context) (opencode.SessionInfo, error)"},
		"Session.Fork":      {typ: reflect.TypeFor[*Session](), method: "Fork", want: "func(*opencode.Session, context.Context, string) (*opencode.Session, error)"},

		// TurnHandle (codex TurnHandle analog; no Steer by design).
		"TurnHandle.ID":        {typ: reflect.TypeFor[*TurnHandle](), method: "ID", want: "func(*opencode.TurnHandle) string"},
		"TurnHandle.SessionID": {typ: reflect.TypeFor[*TurnHandle](), method: "SessionID", want: "func(*opencode.TurnHandle) string"},
		"TurnHandle.Interrupt": {typ: reflect.TypeFor[*TurnHandle](), method: "Interrupt", want: "func(*opencode.TurnHandle, context.Context) (bool, error)"},
		"TurnHandle.Stream":    {typ: reflect.TypeFor[*TurnHandle](), method: "Stream", want: "func(*opencode.TurnHandle, context.Context) iter.Seq2[github.com/zchee/pandaemonium/pkg/llm/opencode.Event,error]"},
		"TurnHandle.Run":       {typ: reflect.TypeFor[*TurnHandle](), method: "Run", want: "func(*opencode.TurnHandle, context.Context) (opencode.RunResult, error)"},

		// Client low-level surface (wrapped REST subset).
		"Client.Start":             {typ: reflect.TypeFor[*Client](), method: "Start", want: "func(*opencode.Client, context.Context) error"},
		"Client.Close":             {typ: reflect.TypeFor[*Client](), method: "Close", want: "func(*opencode.Client) error"},
		"Client.Health":            {typ: reflect.TypeFor[*Client](), method: "Health", want: "func(*opencode.Client, context.Context) (opencode.Health, error)"},
		"Client.Prompt":            {typ: reflect.TypeFor[*Client](), method: "Prompt", want: "func(*opencode.Client, context.Context, string, *opencode.PromptParams) (opencode.PromptResponse, error)"},
		"Client.Abort":             {typ: reflect.TypeFor[*Client](), method: "Abort", want: "func(*opencode.Client, context.Context, string) (bool, error)"},
		"Client.PermissionRespond": {typ: reflect.TypeFor[*Client](), method: "PermissionRespond", want: "func(*opencode.Client, context.Context, string, string, opencode.PermissionResponse) (bool, error)"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			method, ok := tt.typ.MethodByName(tt.method)
			if !ok {
				t.Fatalf("%s has no method %s", tt.typ, tt.method)
			}
			if got := method.Func.Type().String(); got != tt.want {
				t.Errorf("signature drift:\n got  %s\n want %s", got, tt.want)
			}
		})
	}
}

// TestConstructorSignatures pins the package-level constructors.
func TestConstructorSignatures(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		fn   any
		want string
	}{
		"NewOpencode":       {fn: NewOpencode, want: "func(context.Context, *opencode.Config) (*opencode.Opencode, error)"},
		"NewClient":         {fn: NewClient, want: "func(*opencode.Config) *opencode.Client"},
		"NewRemoteClient":   {fn: NewRemoteClient, want: "func(*opencode.RemoteConfig) (*opencode.Client, error)"},
		"NewRemoteOpencode": {fn: NewRemoteOpencode, want: "func(context.Context, *opencode.RemoteConfig) (*opencode.Opencode, error)"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := reflect.TypeOf(tt.fn).String(); got != tt.want {
				t.Errorf("signature drift:\n got  %s\n want %s", got, tt.want)
			}
		})
	}
}

// TestNoSteerMethod pins locked decision 5: OpenCode cannot steer an active
// turn, so TurnHandle must not grow a Steer method without a design change.
func TestNoSteerMethod(t *testing.T) {
	t.Parallel()

	if _, ok := reflect.TypeFor[*TurnHandle]().MethodByName("Steer"); ok {
		t.Fatal("TurnHandle.Steer exists; OpenCode has no mid-turn steering (plan divergence 7, locked decision 5)")
	}
}

// Compile-time shape checks that reflect cannot express for generic code.
var (
	_ func(context.Context, llm.RetryConfig, func() (int, error)) (int, error) = RetryOnOverload[int]
	_ func(context.Context) iter.Seq2[Event, error]                            = (&TurnHandle{}).Stream
)
