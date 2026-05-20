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

import "strings"

// sdkEntrypoint identifies this SDK to the claude CLI for telemetry, set as
// CLAUDE_CODE_ENTRYPOINT on the subprocess. Upstream's Python SDK uses
// "sdk-py"; this Go port reports "sdk-go" so the CLI attributes traffic
// accurately.
const sdkEntrypoint = "sdk-go"

// sdkVersion is the pkg/claude version reported to the CLI as
// CLAUDE_AGENT_SDK_VERSION. It is hand-maintained (not derived from build info,
// which is unreliable across go run / vendoring / replace directives) and
// bumped as part of the package's release process.
//
// TODO(release): the release process must bump this constant (or inject it via
// -ldflags "-X github.com/zchee/pandaemonium/pkg/claude.sdkVersion=<v>"); until
// then external builds report "0.0.0-dev" to CLI telemetry.
const sdkVersion = "0.0.0-dev"

// buildSubprocessEnv computes the environment for the claude CLI subprocess
// from the inherited process environment, the session options, and the
// resolved working directory. It mirrors upstream subprocess_cli.py's env
// construction (lines 428-469):
//
//   - The inherited CLAUDECODE variable is dropped so an SDK-spawned CLI does
//     not believe it is running inside an interactive Claude Code session.
//   - CLAUDE_CODE_ENTRYPOINT and CLAUDE_AGENT_SDK_VERSION are injected to
//     identify the SDK.
//   - PWD is set to cwd when cwd is non-empty.
//   - Any inherited W3C trace context (TRACEPARENT/TRACESTATE) is preserved by
//     inheritance so an ambient OTEL span propagates into the subprocess.
//     Active-span injection via an OTEL SDK is intentionally out of scope.
//   - opts.Env is applied last, so explicit user values win over every key
//     above (matching upstream's "explicit env always wins").
//
// inherited is the list of "KEY=VALUE" strings from os.Environ(); it is not
// mutated. The returned slice is a fresh "KEY=VALUE" list suitable for
// exec.Cmd.Env.
func buildSubprocessEnv(inherited []string, opts *Options, cwd string) []string {
	// Index inherited entries by key so injected/explicit values overwrite
	// rather than duplicate (exec uses the last value for a duplicate key, but
	// de-duping keeps the slice clean and the behavior obvious).
	env := make(map[string]string, len(inherited)+4)
	order := make([]string, 0, len(inherited)+4)
	add := func(k, v string) {
		if _, seen := env[k]; !seen {
			order = append(order, k)
		}
		env[k] = v
	}
	for _, kv := range inherited {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if k == "CLAUDECODE" {
			continue // dropped: see doc comment
		}
		add(k, v)
	}

	add("CLAUDE_CODE_ENTRYPOINT", sdkEntrypoint)
	add("CLAUDE_AGENT_SDK_VERSION", sdkVersion)
	if cwd != "" {
		add("PWD", cwd)
	}

	if opts != nil {
		for k, v := range opts.Env {
			add(k, v)
		}
	}

	out := make([]string, 0, len(order))
	for _, k := range order {
		out = append(out, k+"="+env[k])
	}
	return out
}
