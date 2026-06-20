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

package tmux

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	defaultEventBuffer     = 128
	defaultStderrLineLimit = 100
	defaultShutdownTimeout = 5 * time.Second
)

// Options configures a tmux control-mode client.
//
// The zero value is not startable by itself because starting tmux without an
// explicit session or command could attach to a user's default server. Provide
// InitialCommand, or provide SessionName with CreateSession as appropriate.
type Options struct {
	// Path is the tmux executable path. Empty uses exec.LookPath("tmux").
	Path string

	// SocketName is passed as `-L <name>` to select an isolated tmux socket name.
	SocketName string

	// SocketPath is passed as `-S <path>` to select an isolated tmux socket path.
	SocketPath string

	// ConfigFile is passed as `-f <path>` to select an explicit tmux config file.
	ConfigFile string

	// Dir is the subprocess working directory. Empty inherits the current process
	// directory.
	Dir string

	// Env entries are appended to the inherited subprocess environment.
	Env []string

	// SessionName is the explicit session name used by the default initial
	// attach/create command when InitialCommand is empty.
	SessionName string

	// CreateSession selects `new-session -A -s <SessionName>` instead of
	// `attach-session -t <SessionName>` when InitialCommand is empty.
	CreateSession bool

	// InitialCommand is the tmux command placed after `-C`. When set, it is used
	// verbatim as argv elements and overrides SessionName/CreateSession.
	InitialCommand []string

	// EventBuffer is the size of the asynchronous notification buffer.
	EventBuffer int

	// StderrLineLimit is the maximum number of stderr lines retained for errors.
	StderrLineLimit int

	// ShutdownTimeout bounds graceful Close before the subprocess is killed.
	ShutdownTimeout time.Duration
}

// Option mutates [Options] before a client starts.
type Option func(*Options)

// WithPath sets the tmux executable path.
func WithPath(path string) Option { return func(o *Options) { o.Path = path } }

// WithSocketName sets the tmux `-L` socket name.
func WithSocketName(name string) Option { return func(o *Options) { o.SocketName = name } }

// WithSocketPath sets the tmux `-S` socket path.
func WithSocketPath(path string) Option { return func(o *Options) { o.SocketPath = path } }

// WithConfigFile sets the tmux `-f` config file path.
func WithConfigFile(path string) Option { return func(o *Options) { o.ConfigFile = path } }

// WithDir sets the subprocess working directory.
func WithDir(dir string) Option { return func(o *Options) { o.Dir = dir } }

// WithEnv appends KEY=VALUE environment entries to the subprocess environment.
func WithEnv(env ...string) Option {
	return func(o *Options) { o.Env = append(o.Env, env...) }
}

// WithSessionName sets the session targeted by the default attach/create
// initial command.
func WithSessionName(name string) Option { return func(o *Options) { o.SessionName = name } }

// WithCreateSession makes the default initial command create or attach the
// configured session with `new-session -A -s`.
func WithCreateSession(create bool) Option { return func(o *Options) { o.CreateSession = create } }

// WithInitialCommand sets the argv elements placed after `tmux -C`.
func WithInitialCommand(args ...string) Option {
	return func(o *Options) { o.InitialCommand = slices.Clone(args) }
}

// WithEventBuffer sets the asynchronous notification buffer size.
func WithEventBuffer(n int) Option { return func(o *Options) { o.EventBuffer = n } }

// WithStderrLineLimit sets the retained stderr line count.
func WithStderrLineLimit(n int) Option { return func(o *Options) { o.StderrLineLimit = n } }

// WithShutdownTimeout sets the graceful close timeout.
func WithShutdownTimeout(d time.Duration) Option { return func(o *Options) { o.ShutdownTimeout = d } }

func applyOptions(opts []Option) (Options, error) {
	cfg := Options{
		EventBuffer:     defaultEventBuffer,
		StderrLineLimit: defaultStderrLineLimit,
		ShutdownTimeout: defaultShutdownTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg, cfg.validate()
}

//nolint:cyclop // series of independent validation guards.
func (o Options) validate() error {
	if o.SocketName != "" && o.SocketPath != "" {
		return fmt.Errorf("tmux: SocketName and SocketPath are mutually exclusive")
	}
	if o.EventBuffer <= 0 {
		return fmt.Errorf("tmux: EventBuffer must be > 0")
	}
	if o.StderrLineLimit < 0 {
		return fmt.Errorf("tmux: StderrLineLimit must be >= 0")
	}
	if o.ShutdownTimeout <= 0 {
		return fmt.Errorf("tmux: ShutdownTimeout must be > 0")
	}
	for _, entry := range o.Env {
		if entry == "" || !strings.Contains(entry, "=") {
			return fmt.Errorf("tmux: Env entry %q must be KEY=VALUE", entry)
		}
	}
	if len(o.InitialCommand) == 0 && o.SessionName == "" {
		return fmt.Errorf("tmux: InitialCommand or SessionName is required to avoid implicit default-server attach")
	}
	if len(o.InitialCommand) > 0 && o.SessionName != "" {
		return fmt.Errorf("tmux: InitialCommand and SessionName are mutually exclusive")
	}
	for _, arg := range o.InitialCommand {
		if strings.ContainsAny(arg, "\r\n") {
			return fmt.Errorf("tmux: InitialCommand argument %q contains a newline", arg)
		}
	}
	if strings.ContainsAny(o.SessionName, "\r\n") {
		return fmt.Errorf("tmux: SessionName contains a newline")
	}
	return nil
}

func (o Options) launchArgs() []string {
	args := make([]string, 0, 10+len(o.InitialCommand))
	if o.SocketName != "" {
		args = append(args, "-L", o.SocketName)
	}
	if o.SocketPath != "" {
		args = append(args, "-S", o.SocketPath)
	}
	if o.ConfigFile != "" {
		args = append(args, "-f", o.ConfigFile)
	}
	args = append(args, "-C")
	if len(o.InitialCommand) > 0 {
		return append(args, o.InitialCommand...)
	}
	if o.CreateSession {
		return append(args, "new-session", "-A", "-s", o.SessionName)
	}
	return append(args, "attach-session", "-t", o.SessionName)
}

func (o Options) initialCommandLine() string {
	if len(o.InitialCommand) > 0 {
		return strings.Join(o.InitialCommand, " ")
	}
	if o.CreateSession {
		return "new-session -A -s " + o.SessionName
	}
	return "attach-session -t " + o.SessionName
}

func (o Options) cloneEnv() []string { return slices.Clone(o.Env) }
