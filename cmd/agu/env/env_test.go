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

package env

import (
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	envconfig "github.com/sethvargo/go-envconfig"
)

func TestProcessConfig(t *testing.T) {
	tests := map[string]struct {
		env  map[string]string
		want *XDG
	}{
		"success: xdg homes are read from the process environment": {
			env: map[string]string{
				"XDG_CONFIG_HOME": "/tmp/agu/config",
				"XDG_STATE_HOME":  "/tmp/agu/state",
			},
			want: &XDG{
				ConfigHome: "/tmp/agu/config",
				StateHome:  "/tmp/agu/state",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg := ProcessConfig(t.Context())
			if cfg == nil {
				t.Fatal("ProcessConfig() returned nil config")
			}

			if diff := gocmp.Diff(tt.want, cfg.XDG); diff != "" {
				t.Errorf("ProcessConfig() XDG mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestProcessConfigWithLookuper(t *testing.T) {
	tests := map[string]struct {
		env  map[string]string
		want *XDG
	}{
		"success: missing xdg homes use zero values": {
			env: map[string]string{},
			want: &XDG{
				ConfigHome: "",
				StateHome:  "",
			},
		},
		"success: xdg homes are populated": {
			env: map[string]string{
				"XDG_CONFIG_HOME": "/tmp/agu/config",
				"XDG_STATE_HOME":  "/tmp/agu/state",
			},
			want: &XDG{
				ConfigHome: "/tmp/agu/config",
				StateHome:  "/tmp/agu/state",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := processConfig(t.Context(), envconfig.MapLookuper(tt.env))
			if cfg == nil {
				t.Fatal("processConfig() returned nil config")
			}

			if diff := gocmp.Diff(tt.want, cfg.XDG); diff != "" {
				t.Errorf("processConfig() XDG mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
