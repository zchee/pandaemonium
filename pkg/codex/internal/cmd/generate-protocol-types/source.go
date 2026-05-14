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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"
)

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
	defer response.Body.Close()
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
