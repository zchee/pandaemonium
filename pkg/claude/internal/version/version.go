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

// Package version records pinned dependency versions for pkg/claude.
//
// Version constants are bumped in dedicated commits with vendor refresh,
// mirroring the schema-pin discipline of pkg/codex/generate.go:17.
package version

const (
	// SDKVersion is the pkg/claude SDK version string.
	SDKVersion = "0.1.0-dev"

	// MCPGoSDKVersion is the pinned tag of github.com/modelcontextprotocol/go-sdk.
	// Bump this constant in the same commit that updates go.mod + vendor/.
	MCPGoSDKVersion = "v1.6.0"
)
