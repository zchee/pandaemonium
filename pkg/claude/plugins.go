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
	"github.com/go-json-experiment/json/jsontext"
)

// Plugin describes a claude CLI plugin to load at session start.
//
// Plugins are registered via [Options].Plugins and round-trip into CLI launch
// arguments. The exact CLI flag mapping is implemented in Phase G.
type Plugin struct {
	// Name is the human-readable name of the plugin.
	Name string `json:"name,omitzero"`

	// Path is the filesystem path or URL of the plugin to load.
	Path string `json:"path,omitzero"`

	// Raw preserves unknown fields for forward compatibility.
	Raw jsontext.Value `json:",inline"`
}
