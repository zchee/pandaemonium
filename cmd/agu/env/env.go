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

// Package env loads agu configuration from environment variables.
package env

import (
	"context"
	"fmt"

	envconfig "github.com/sethvargo/go-envconfig"
)

// Config holds environment-derived configuration for agu.
type Config struct {
	*XDG
}

// XDG contains XDG base directory environment values used by agu.
type XDG struct {
	ConfigHome string `env:"XDG_CONFIG_HOME"`
	StateHome  string `env:"XDG_STATE_HOME"`
}

// ProcessConfig reads environment values into a Config.
func ProcessConfig(ctx context.Context) *Config {
	return processConfig(ctx, envconfig.OsLookuper())
}

func processConfig(ctx context.Context, lookuper envconfig.Lookuper) *Config {
	cfg := new(Config)
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   cfg,
		Lookuper: lookuper,
	}); err != nil {
		panic(fmt.Errorf("process envconfig: %w", err))
	}

	return cfg
}
