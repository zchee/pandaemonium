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

package codexappserver

import (
	"github.com/go-json-experiment/json/jsontext"
)

// Object is a JSON object exchanged with the Codex app-server.
type Object = map[string]any

// ServerInfo describes the app-server process returned by initialize.
type ServerInfo struct {
	Name    string `json:"name,omitzero"`
	Version string `json:"version,omitzero"`
}

// InitializeResponse is the metadata returned by the app-server initialize method.
type InitializeResponse struct {
	UserAgent  string         `json:"userAgent,omitzero"`
	ServerInfo *ServerInfo    `json:"serverInfo,omitzero"`
	Raw        jsontext.Value `json:",inline"`
}
