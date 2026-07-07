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
	"os"
	"strings"

	"github.com/go-json-experiment/json"
)

// buildSettingsValue resolves the value passed to --settings, merging
// [Options.Sandbox] into [Options.Settings] when both are set. Mirrors upstream
// _build_settings_value (subprocess_cli.py:129-181):
//
//   - Neither Settings nor Sandbox set → "" (caller omits the flag).
//   - Settings set, Sandbox nil → Settings is passed through verbatim.
//   - Settings set as a JSON object literal + Sandbox → the literal is parsed
//     and Sandbox is added under the "sandbox" key.
//   - Settings set as a file path + Sandbox → the file is read and parsed if
//     it exists, then Sandbox is added; if the file is missing or unreadable,
//     parity with upstream is to continue with an empty base object (upstream
//     logs a warning; this SDK has no logger and silently proceeds).
//   - Sandbox alone → a fresh {"sandbox": ...} object.
//
// The JSON-vs-path heuristic matches upstream: the trimmed value must both
// start with "{" and end with "}" to be parsed as JSON. Using json.Valid
// would diverge on edge cases.
//
// An error is returned only when Settings is a JSON literal that fails to
// parse AND Sandbox is set (the parser cannot proceed); upstream falls back
// to treating it as a file path here, but in Go a malformed JSON literal
// almost certainly indicates a programmer mistake rather than a path, and
// surfacing it as CLIConnectionError is more useful than silently writing
// only the sandbox key. When Sandbox is nil the original literal is passed
// through to the CLI (parity with upstream's "if has_settings and not
// has_sandbox: return self._options.settings").
func buildSettingsValue(opts *Options) (string, error) {
	hasSettings := opts.Settings != ""
	hasSandbox := opts.Sandbox != nil

	switch {
	case !hasSettings && !hasSandbox:
		return "", nil
	case hasSettings && !hasSandbox:
		return opts.Settings, nil
	}

	settingsObj := map[string]any{}

	if hasSettings {
		if err := parseSettingsBase(opts.Settings, settingsObj); err != nil {
			return "", err
		}
	}

	settingsObj["sandbox"] = opts.Sandbox

	out, err := json.Marshal(settingsObj)
	if err != nil {
		return "", &CLIConnectionError{Message: "marshal merged --settings: " + err.Error()}
	}
	return string(out), nil
}

// parseSettingsBase parses settings — either a JSON object literal or a path to
// a settings file — into dst. The JSON-vs-path heuristic matches upstream: the
// trimmed value must both start with "{" and end with "}" to be treated as
// JSON. A JSON literal that fails to parse is an error; a missing or unreadable
// file is tolerated (dst is left unchanged), matching upstream's
// warn-and-continue behavior so Sandbox is still merged into an empty base.
func parseSettingsBase(settings string, dst map[string]any) error {
	trimmed := strings.TrimSpace(settings)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		if err := json.Unmarshal([]byte(trimmed), &dst); err != nil {
			return &CLIConnectionError{Message: "parse Options.Settings as JSON: " + err.Error()}
		}
		return nil
	}
	// A missing or unreadable file is tolerated (dst left unchanged), matching
	// upstream's warn-and-continue behavior; only a present-but-unparsable file
	// is an error.
	data, err := os.ReadFile(trimmed)
	if err == nil {
		if err := json.Unmarshal(data, &dst); err != nil {
			return &CLIConnectionError{Message: "parse Options.Settings file " + trimmed + " as JSON: " + err.Error()}
		}
	}
	return nil
}

// ─── Sandbox configuration ───────────────────────────────────────────────────

// SandboxNetworkConfig configures network access for sandboxed commands.
// Mirrors upstream SandboxNetworkConfig (types.py:835) — a total=False
// TypedDict where any unset key is omitted from the wire JSON. All bool
// defaults here are false, so plain bool with omitzero is correct.
type SandboxNetworkConfig struct {
	// AllowedDomains lists domain names that sandboxed processes can access.
	AllowedDomains []string `json:"allowedDomains,omitzero"`

	// DeniedDomains lists domains that are always blocked, even if matched by
	// AllowedDomains.
	DeniedDomains []string `json:"deniedDomains,omitzero"`

	// AllowManagedDomainsOnly, when true in managed settings, restricts
	// network to only the managed-settings AllowedDomains.
	AllowManagedDomainsOnly bool `json:"allowManagedDomainsOnly,omitzero"`

	// AllowUnixSockets lists Unix socket paths accessible inside the sandbox
	// (for example, an SSH agent socket).
	AllowUnixSockets []string `json:"allowUnixSockets,omitzero"`

	// AllowAllUnixSockets allows every Unix socket (less secure).
	AllowAllUnixSockets bool `json:"allowAllUnixSockets,omitzero"`

	// AllowLocalBinding allows binding to localhost ports (macOS only).
	AllowLocalBinding bool `json:"allowLocalBinding,omitzero"`

	// AllowMachLookup lists macOS XPC/Mach service names to allow (trailing
	// wildcard supported).
	AllowMachLookup []string `json:"allowMachLookup,omitzero"`

	// HTTPProxyPort is an HTTP proxy port if bringing your own proxy.
	HTTPProxyPort int `json:"httpProxyPort,omitzero"`

	// SocksProxyPort is a SOCKS5 proxy port if bringing your own proxy.
	SocksProxyPort int `json:"socksProxyPort,omitzero"`
}

// SandboxIgnoreViolations lists sandbox violations to ignore.
// Mirrors upstream SandboxIgnoreViolations (types.py:861).
type SandboxIgnoreViolations struct {
	// File lists file paths whose sandbox violations should be ignored.
	File []string `json:"file,omitzero"`

	// Network lists network hosts whose sandbox violations should be ignored.
	Network []string `json:"network,omitzero"`
}

// SandboxSettings configures how the CLI sandboxes bash commands. It is merged
// into the --settings JSON at launch by [Options.Sandbox] (mirrors upstream
// _build_settings_value, subprocess_cli.py:129-181).
//
// Filesystem and network restrictions are configured via permission rules, not
// here: Read deny rules for read restrictions, Edit allow/deny for writes,
// WebFetch allow/deny for network. SandboxSettings controls only the sandbox
// behavior (whether sandboxing is on, which commands run sandboxed, etc.).
//
// Wire-default trap: AutoAllowBashIfSandboxed and AllowUnsandboxedCommands
// default to true upstream (types.py:911,913) — to send false the field must
// be transmitted, so they are *bool, not bool. The other booleans default to
// false upstream, so a missing key and an explicit false coincide and plain
// bool with omitzero is fine.
type SandboxSettings struct {
	// Enabled turns on bash sandboxing (macOS/Linux only). Upstream default
	// is false.
	Enabled bool `json:"enabled,omitzero"`

	// AutoAllowBashIfSandboxed auto-approves bash commands when sandboxed.
	// Upstream default is true, so a nil pointer omits the field and lets the
	// CLI default apply; pass new(false) or new(true) to send an explicit value.
	AutoAllowBashIfSandboxed *bool `json:"autoAllowBashIfSandboxed,omitzero"`

	// ExcludedCommands lists commands that should run outside the sandbox
	// (e.g. {"git", "docker"}).
	ExcludedCommands []string `json:"excludedCommands,omitzero"`

	// AllowUnsandboxedCommands controls whether commands may bypass the
	// sandbox via dangerouslyDisableSandbox. When false, all commands must
	// run sandboxed (or be in ExcludedCommands). Upstream default is true;
	// nil omits the field. Pass new(false) or new(true) to send an explicit value.
	AllowUnsandboxedCommands *bool `json:"allowUnsandboxedCommands,omitzero"`

	// Network configures sandbox network access.
	Network SandboxNetworkConfig `json:"network,omitzero"`

	// IgnoreViolations lists sandbox violations to ignore.
	IgnoreViolations SandboxIgnoreViolations `json:"ignoreViolations,omitzero"`

	// EnableWeakerNestedSandbox enables a weaker sandbox for unprivileged
	// Docker environments (Linux only). Reduces security. Upstream default
	// is false.
	EnableWeakerNestedSandbox bool `json:"enableWeakerNestedSandbox,omitzero"`
}
