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

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type schemaInput struct {
	data    []byte
	label   string
	details []string
}

func loadSchemaInput(schemaPath, codexBin string) (schemaInput, error) {
	if strings.TrimSpace(schemaPath) != "" {
		data, err := readSchemaSource(schemaPath)
		if err != nil {
			return schemaInput{}, err
		}
		return schemaInput{data: data, label: schemaSourceLabel(schemaPath)}, nil
	}
	return generateCodexAppServerSchema(codexBin)
}

func readCodexVersion(codexBin string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, codexBin, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		command := codexBin + " --version"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("run %s: timed out after 30s%s", command, commandOutputSuffix(output))
		}
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return "", fmt.Errorf("start %s: %w", command, err)
		}
		return "", fmt.Errorf("run %s: %w%s", command, err, commandOutputSuffix(output))
	}
	version := firstOutputLine(string(output))
	if version == "" {
		return "", fmt.Errorf("run %s --version: empty output", codexBin)
	}
	return version, nil
}

func generateCodexAppServerSchema(codexBin string) (schemaInput, error) {
	codexBin = strings.TrimSpace(codexBin)
	if codexBin == "" {
		codexBin = "codex"
	}
	version, err := readCodexVersion(codexBin)
	if err != nil {
		return schemaInput{}, fmt.Errorf("read codex version: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "pandaemonium-codex-schema-*")
	if err != nil {
		return schemaInput{}, fmt.Errorf("create temporary schema directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"app-server", "generate-json-schema", "--experimental", "--out", tmpDir}
	cmd := exec.CommandContext(ctx, codexBin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		command := codexBin + " " + strings.Join(args[:3], " ")
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return schemaInput{}, fmt.Errorf("run %s: timed out after 30s%s", command, commandOutputSuffix(output))
		}
		var execErr *exec.Error
		if errors.As(err, &execErr) || errors.Is(err, os.ErrNotExist) {
			return schemaInput{}, fmt.Errorf("start %s: %w", command, err)
		}
		return schemaInput{}, fmt.Errorf("run %s: %w%s", command, err, commandOutputSuffix(output))
	}

	schemaPath := filepath.Join(tmpDir, generatedSchemaFilename)
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return schemaInput{}, fmt.Errorf("read generated schema %s: %w", generatedSchemaFilename, err)
	}
	return schemaInput{data: data, label: generatedSchemaSourceLabel, details: []string{"Source binary: " + version}}, nil
}

func commandOutputSuffix(output []byte) string {
	if len(output) == 0 {
		return ""
	}
	return ": " + boundedCommandOutput(string(output), 4096)
}

func firstOutputLine(output string) string {
	output = strings.TrimSpace(output)
	line, _, _ := strings.Cut(output, "\n")
	return strings.TrimSpace(line)
}

func boundedCommandOutput(output string, maxBytes int) string {
	output = strings.TrimSpace(output)
	if len(output) <= maxBytes {
		return output
	}
	truncated := output[:maxBytes]
	if cut := strings.LastIndexByte(truncated, '\n'); cut > 0 {
		truncated = truncated[:cut]
	}
	return truncated + "\n... command output truncated ..."
}

func readSchemaSource(source string) ([]byte, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		if !hasHTTPScheme(source) {
			return os.ReadFile(source)
		}
		return nil, fmt.Errorf("parse source: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
		if parsed.Host == "" {
			return nil, fmt.Errorf("invalid %s URL: missing host", parsed.Scheme)
		}
		return readHTTPSchemaSource(source)
	case "":
		return os.ReadFile(source)
	default:
		if parsed.Host != "" {
			return nil, fmt.Errorf("unsupported schema URL scheme %q", parsed.Scheme)
		}
		return os.ReadFile(source)
	}
}

func hasHTTPScheme(source string) bool {
	scheme, _, ok := strings.Cut(source, ":")
	if !ok {
		return false
	}
	return strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https")
}

func readHTTPSchemaSource(source string) ([]byte, error) {
	return readHTTPSchemaSourceWithLimit(source, maxHTTPSchemaBytes)
}

func readHTTPSchemaSourceWithLimit(source string, maxBytes int64) ([]byte, error) {
	sourceLabel := schemaSourceLabel(source)
	request, err := http.NewRequest(http.MethodGet, source, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		if urlError, ok := errors.AsType[*url.Error](err); ok {
			err = urlError.Err
		}
		return nil, fmt.Errorf("fetch %s: %w", sourceLabel, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("fetch %s: unexpected HTTP status %s", sourceLabel, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", sourceLabel, err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("read %s response: schema exceeds %d bytes", sourceLabel, maxBytes)
	}
	return body, nil
}

func schemaSourceLabel(source string) string {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return filepathSlash(source)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return filepathSlash(source)
	}
	redacted := *parsed
	redacted.User = nil
	redacted.RawQuery = ""
	redacted.Fragment = ""
	return redacted.String()
}

func validPackageName(name string) bool {
	if name == "" || isGoKeyword(name) {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func filepathSlash(path string) string { return strings.ReplaceAll(path, "\\", "/") }
