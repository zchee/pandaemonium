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

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BackendRequest describes a delegated upstream OMX invocation.
type BackendRequest struct {
	Args   []string
	Env    map[string]string
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Backend runs delegated upstream OMX commands.
type Backend interface {
	Run(context.Context, BackendRequest) error
}

// ExitError carries a process exit code without losing the wrapped error.
type ExitError struct {
	code int
	err  error
}

// Error returns the underlying error string.
func (e ExitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit status %d", e.code)
	}
	return e.err.Error()
}

// Unwrap returns the underlying error.
func (e ExitError) Unwrap() error { return e.err }

// Code returns the process exit code.
func (e ExitError) Code() int { return e.code }

// SystemBackend delegates commands to a compatible upstream omx binary.
type SystemBackend struct{}

// Run discovers and executes the upstream OMX backend.
func (SystemBackend) Run(ctx context.Context, req BackendRequest) error {
	backend, err := discoverBackend(req.Env)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, backend, req.Args...)
	command.Dir = req.Dir
	command.Env = envSlice(req.Env)
	command.Stdin = req.Stdin
	command.Stdout = req.Stdout
	command.Stderr = req.Stderr
	if err := command.Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return ExitError{code: exitErr.ExitCode(), err: err}
		}
		return err
	}
	return nil
}

func discoverBackend(env map[string]string) (string, error) {
	if configured := strings.TrimSpace(env["PAND_OMX_BINARY"]); configured != "" {
		path, err := resolveExecutable(configured)
		if err != nil {
			return "", fmt.Errorf("resolve PAND_OMX_BINARY: %w", err)
		}
		if isSelfExecutable(path) {
			return "", fmt.Errorf("PAND_OMX_BINARY resolves to pand itself: %s", path)
		}
		return path, nil
	}

	pathEnv := env["PATH"]
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "omx")
		path, err := resolveExecutable(candidate)
		if err != nil {
			continue
		}
		if isSelfExecutable(path) {
			continue
		}
		return path, nil
	}
	return "", fmt.Errorf("no compatible omx backend found; set PAND_OMX_BINARY or install oh-my-codex omx on PATH")
}

func resolveExecutable(path string) (string, error) {
	if !filepath.IsAbs(path) {
		found, err := exec.LookPath(path)
		if err != nil {
			return "", err
		}
		path = found
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("%s is not executable", path)
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func isSelfExecutable(candidate string) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	selfResolved := self
	if real, err := filepath.EvalSymlinks(selfResolved); err == nil {
		selfResolved = real
	}
	selfAbs, err := filepath.Abs(selfResolved)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	return samePath(selfAbs, candidateAbs)
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func environMap(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			out[entry] = ""
			continue
		}
		out[key] = value
	}
	return out
}

func envSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}
