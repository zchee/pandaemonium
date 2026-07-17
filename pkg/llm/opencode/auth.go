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
	"net/http"
)

// basicAuthUsername is the username OpenCode's basic auth requires when
// OPENCODE_SERVER_PASSWORD is set. The server verifies it (probed on 1.18.3:
// any other username is rejected with 401).
const basicAuthUsername = "opencode"

// basicAuthTransport injects the OpenCode basic-auth header on every request,
// including the SSE dial. The password never appears in URLs, argv, error
// strings, or logs (AC8); it lives only in this header.
type basicAuthTransport struct {
	password string
	next     http.RoundTripper
}

// RoundTrip implements http.RoundTripper. The request is cloned before the
// header is set, per the RoundTripper contract that the request must not be
// mutated.
func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.password != "" {
		req = req.Clone(req.Context())
		req.SetBasicAuth(basicAuthUsername, t.password)
	}
	return t.next.RoundTrip(req)
}

// newAuthHTTPClient wraps base (or http.DefaultTransport) with basic auth for
// password. Timeouts are managed per-request via contexts, not on the client,
// because the same client serves both bounded REST calls and the long-lived
// SSE stream.
func newAuthHTTPClient(base *http.Client, password string) *http.Client {
	client := &http.Client{}
	next := http.DefaultTransport
	if base != nil {
		clone := *base
		client = &clone
		client.Timeout = 0
		if base.Transport != nil {
			next = base.Transport
		}
	}
	client.Transport = &basicAuthTransport{password: password, next: next}
	return client
}
