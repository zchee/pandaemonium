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

package codex

import (
	"testing"

	"github.com/go-json-experiment/json/jsontext"
)

func TestClientRequestMethodWrappers(t *testing.T) {
	t.Parallel()
	client := newHelperClient(t, "method_wrappers")
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := t.Context()
	calls := []struct {
		name string
		call func() error
	}{
		{name: "thread/start", call: func() error { _, err := client.ThreadStart(ctx, &ThreadStartParams{}); return err }},
		{name: "thread/resume", call: func() error { _, err := client.ThreadResume(ctx, "thread", &ThreadResumeParams{}); return err }},
		{name: "thread/fork", call: func() error { _, err := client.ThreadFork(ctx, "thread", &ThreadForkParams{}); return err }},
		{name: "thread/archive", call: func() error { _, err := client.ThreadArchive(ctx, "thread"); return err }},
		{name: "thread/unsubscribe", call: func() error {
			_, err := client.ThreadUnsubscribe(ctx, "thread", &ThreadUnsubscribeParams{})
			return err
		}},
		{name: "thread/name/set", call: func() error { _, err := client.ThreadSetName(ctx, "thread", "name"); return err }},
		{name: "thread/goal/set", call: func() error {
			objective := "ship the schema bump"
			_, err := client.ThreadGoalSet(ctx, "thread", &ThreadGoalSetParams{Objective: objective})
			return err
		}},
		{name: "thread/goal/get", call: func() error {
			_, err := client.ThreadGoalGet(ctx, "thread")
			return err
		}},
		{name: "thread/goal/clear", call: func() error {
			_, err := client.ThreadGoalClear(ctx, "thread")
			return err
		}},
		{name: "thread/metadata/update", call: func() error {
			_, err := client.ThreadMetadataUpdate(ctx, "thread", &ThreadMetadataUpdateParams{})
			return err
		}},
		{name: "thread/unarchive", call: func() error { _, err := client.ThreadUnarchive(ctx, "thread"); return err }},
		{name: "thread/compact/start", call: func() error { _, err := client.ThreadCompact(ctx, "thread"); return err }},
		{name: "thread/shellCommand", call: func() error {
			_, err := client.ThreadShellCommand(ctx, "thread", &ThreadShellCommandParams{Command: "echo ok"})
			return err
		}},
		{name: "thread/approveGuardianDeniedAction", call: func() error {
			_, err := client.ThreadApproveGuardianDeniedAction(ctx, "thread", &ThreadApproveGuardianDeniedActionParams{Event: jsontext.Value(`{}`)})
			return err
		}},
		{name: "thread/rollback", call: func() error {
			_, err := client.ThreadRollback(ctx, "thread", &ThreadRollbackParams{NumTurns: 1})
			return err
		}},
		{name: "thread/list", call: func() error { _, err := client.ThreadList(ctx, &ThreadListParams{}); return err }},
		{name: "thread/loaded/list", call: func() error { _, err := client.ThreadLoadedList(ctx, &ThreadLoadedListParams{}); return err }},
		{name: "thread/read", call: func() error {
			_, err := client.ThreadRead(ctx, "thread", &ThreadReadParams{IncludeTurns: true})
			return err
		}},
		{name: "thread/inject_items", call: func() error {
			_, err := client.ThreadInjectItems(ctx, "thread", &ThreadInjectItemsParams{})
			return err
		}},
		{name: "skills/list", call: func() error { _, err := client.SkillsList(ctx, &SkillsListParams{}); return err }},
		{name: "skills/extraRoots/set", call: func() error {
			_, err := client.SkillsExtraRootsSet(ctx, &SkillsExtraRootsSetParams{ExtraRoots: []string{"/tmp/skills"}})
			return err
		}},
		{name: "hooks/list", call: func() error { _, err := client.HooksList(ctx, &HooksListParams{}); return err }},
		{name: "marketplace/add", call: func() error {
			_, err := client.MarketplaceAdd(ctx, &MarketplaceAddParams{Source: "source"})
			return err
		}},
		{name: "marketplace/remove", call: func() error {
			_, err := client.MarketplaceRemove(ctx, &MarketplaceRemoveParams{MarketplaceName: "marketplace"})
			return err
		}},
		{name: "marketplace/upgrade", call: func() error { _, err := client.MarketplaceUpgrade(ctx, &MarketplaceUpgradeParams{}); return err }},
		{name: "plugin/list", call: func() error { _, err := client.PluginList(ctx, &PluginListParams{}); return err }},
		{name: "plugin/read", call: func() error { _, err := client.PluginRead(ctx, &PluginReadParams{PluginName: "plugin"}); return err }},
		{name: "plugin/skill/read", call: func() error {
			_, err := client.PluginSkillRead(ctx, &PluginSkillReadParams{RemoteMarketplaceName: "marketplace", RemotePluginID: "plugin", SkillName: "skill"})
			return err
		}},
		{name: "plugin/share/save", call: func() error {
			_, err := client.PluginShareSave(ctx, &PluginShareSaveParams{PluginPath: "/tmp/plugin"})
			return err
		}},
		{name: "plugin/share/updateTargets", call: func() error {
			_, err := client.PluginShareUpdateTargets(ctx, &PluginShareUpdateTargetsParams{RemotePluginID: "plugin"})
			return err
		}},
		{name: "plugin/share/list", call: func() error { _, err := client.PluginShareList(ctx, &PluginShareListParams{}); return err }},
		{name: "plugin/share/checkout", call: func() error {
			_, err := client.PluginShareCheckout(ctx, &PluginShareCheckoutParams{RemotePluginID: "plugin"})
			return err
		}},
		{name: "plugin/share/delete", call: func() error {
			_, err := client.PluginShareDelete(ctx, &PluginShareDeleteParams{RemotePluginID: "plugin"})
			return err
		}},
		{name: "app/list", call: func() error { _, err := client.AppList(ctx, &AppsListParams{}); return err }},
		{name: "fs/readFile", call: func() error { _, err := client.FSReadFile(ctx, &FSReadFileParams{Path: "/tmp/file"}); return err }},
		{name: "fs/read", call: func() error { _, err := client.FSRead(ctx, &FSReadFileParams{Path: "/tmp/file"}); return err }},
		{name: "fs/writeFile", call: func() error {
			_, err := client.FSWriteFile(ctx, &FSWriteFileParams{Path: "/tmp/file", DataBase64: "b2s="})
			return err
		}},
		{name: "fs/write", call: func() error {
			_, err := client.FSWrite(ctx, &FSWriteFileParams{Path: "/tmp/file", DataBase64: "b2s="})
			return err
		}},
		{name: "fs/createDirectory", call: func() error {
			_, err := client.FSCreateDirectory(ctx, &FSCreateDirectoryParams{Path: "/tmp/dir"})
			return err
		}},
		{name: "fs/getMetadata", call: func() error { _, err := client.FSGetMetadata(ctx, &FSGetMetadataParams{Path: "/tmp/file"}); return err }},
		{name: "fs/stat", call: func() error { _, err := client.FSStat(ctx, &FSGetMetadataParams{Path: "/tmp/file"}); return err }},
		{name: "fs/readDirectory", call: func() error { _, err := client.FSReadDirectory(ctx, &FSReadDirectoryParams{Path: "/tmp"}); return err }},
		{name: "fs/list", call: func() error { _, err := client.FSList(ctx, &FSReadDirectoryParams{Path: "/tmp"}); return err }},
		{name: "fs/remove", call: func() error { _, err := client.FSRemove(ctx, &FSRemoveParams{Path: "/tmp/file"}); return err }},
		{name: "fs/copy", call: func() error {
			_, err := client.FSCopy(ctx, &FSCopyParams{SourcePath: "/tmp/a", DestinationPath: "/tmp/b"})
			return err
		}},
		{name: "fs/watch", call: func() error {
			_, err := client.FSWatch(ctx, &FSWatchParams{Path: "/tmp", WatchID: "watch"})
			return err
		}},
		{name: "fs/unwatch", call: func() error { _, err := client.FSUnwatch(ctx, &FSUnwatchParams{WatchID: "watch"}); return err }},
		{name: "skills/config/write", call: func() error {
			_, err := client.SkillsConfigWrite(ctx, &SkillsConfigWriteParams{Enabled: true})
			return err
		}},
		{name: "plugin/install", call: func() error {
			_, err := client.PluginInstall(ctx, &PluginInstallParams{PluginName: "plugin"})
			return err
		}},
		{name: "plugin/uninstall", call: func() error {
			_, err := client.PluginUninstall(ctx, &PluginUninstallParams{PluginID: "plugin"})
			return err
		}},
		{name: "turn/start", call: func() error { _, err := client.TurnStart(ctx, "thread", "hello", &TurnStartParams{}); return err }},
		{name: "turn/steer", call: func() error { _, err := client.TurnSteer(ctx, "thread", "turn", "hello"); return err }},
		{name: "turn/interrupt", call: func() error { _, err := client.TurnInterrupt(ctx, "thread", "turn"); return err }},
		{name: "review/start", call: func() error { _, err := client.ReviewStart(ctx, &ReviewStartParams{ThreadID: "thread"}); return err }},
		{name: "model/list", call: func() error {
			_, err := client.ModelList(ctx, &ModelListParams{IncludeHidden: true})
			return err
		}},
		{name: "modelProvider/capabilities/read", call: func() error {
			_, err := client.ModelProviderCapabilitiesRead(ctx, &ModelProviderCapabilitiesReadParams{})
			return err
		}},
		{name: "experimentalFeature/list", call: func() error {
			_, err := client.ExperimentalFeatureList(ctx, &ExperimentalFeatureListParams{})
			return err
		}},
		{name: "permissionProfile/list", call: func() error {
			_, err := client.PermissionProfileList(ctx, &PermissionProfileListParams{})
			return err
		}},
		{name: "experimentalFeature/enablement/set", call: func() error {
			_, err := client.ExperimentalFeatureEnablementSet(ctx, &ExperimentalFeatureEnablementSetParams{Enablement: map[string]bool{"feature": true}})
			return err
		}},
		{name: "remoteControl/enable", call: func() error {
			got, err := client.RemoteControlEnable(ctx)
			if err != nil {
				return err
			}
			if got.InstallationID != "install-method" || got.ServerName != "server-method" || got.Status != RemoteControlConnectionStatusConnected || got.EnvironmentID != "env-method" {
				t.Fatalf("RemoteControlEnable() = %#v, want connected env-method", got)
			}
			return nil
		}},
		{name: "remoteControl/disable", call: func() error {
			got, err := client.RemoteControlDisable(ctx)
			if err != nil {
				return err
			}
			if got.Status != RemoteControlConnectionStatusDisabled || got.InstallationID != "install-method" || got.ServerName != "server-method" {
				t.Fatalf("RemoteControlDisable() = %#v, want disabled install-method", got)
			}
			return nil
		}},
		{name: "remoteControl/status/read", call: func() error {
			got, err := client.RemoteControlStatusRead(ctx)
			if err != nil {
				return err
			}
			if got.Status != RemoteControlConnectionStatusConnected || got.InstallationID != "install-method" || got.ServerName != "server-method" || got.EnvironmentID != "env-method" {
				t.Fatalf("RemoteControlStatusRead() = %#v, want connected server-method", got)
			}
			return nil
		}},
		{name: "remoteControl/pairing/start", call: func() error {
			got, err := client.RemoteControlPairingStart(ctx, &RemoteControlPairingStartParams{ManualCode: true})
			if err != nil {
				return err
			}
			if got.EnvironmentID != "env-method" || got.PairingCode != "pair-method" || got.ManualPairingCode != "manual-method" {
				t.Fatalf("RemoteControlPairingStart() = %#v, want env and pairing codes", got)
			}
			return nil
		}},
		{name: "remoteControl/pairing/status", call: func() error {
			got, err := client.RemoteControlPairingStatus(ctx, &RemoteControlPairingStatusParams{
				PairingCode:       "pair-method",
				ManualPairingCode: "manual-method",
			})
			if err != nil {
				return err
			}
			if !got.Claimed {
				t.Fatalf("RemoteControlPairingStatus() = %#v, want claimed", got)
			}
			return nil
		}},
		{name: "remoteControl/client/list", call: func() error {
			limit := int32(10)
			got, err := client.RemoteControlClientList(ctx, &RemoteControlClientsListParams{
				EnvironmentID: "env-method",
				Limit:         limit,
				Order:         RemoteControlClientsListOrderAsc,
			})
			if err != nil {
				return err
			}
			if len(got.Data) != 1 || got.Data[0].ClientID != "client-method" || got.NextCursor != "cursor-method" {
				t.Fatalf("RemoteControlClientList() = %#v, want one client and cursor", got)
			}
			return nil
		}},
		{name: "remoteControl/client/revoke", call: func() error {
			_, err := client.RemoteControlClientRevoke(ctx, &RemoteControlClientsRevokeParams{EnvironmentID: "env-method", ClientID: "client-method"})
			return err
		}},
		{name: "environment/add", call: func() error {
			_, err := client.EnvironmentAdd(ctx, &EnvironmentAddParams{EnvironmentID: "env-method", ExecServerURL: "ws://127.0.0.1:8765"})
			return err
		}},
		{name: "mcpServer/oauth/login", call: func() error {
			_, err := client.MCPServerOAuthLogin(ctx, &MCPServerOAuthLoginParams{Name: "server"})
			return err
		}},
		{name: "config/mcpServer/reload", call: func() error { _, err := client.ConfigMCPServerReload(ctx); return err }},
		{name: "mcpServerStatus/list", call: func() error { _, err := client.MCPServerStatusList(ctx, &ListMCPServerStatusParams{}); return err }},
		{name: "mcpServer/resource/read", call: func() error {
			_, err := client.MCPServerResourceRead(ctx, &MCPResourceReadParams{Server: "server", URI: "file://resource"})
			return err
		}},
		{name: "mcpServer/tool/call", call: func() error {
			_, err := client.MCPServerToolCall(ctx, &MCPServerToolCallParams{Server: "server", ThreadID: "thread", Tool: "tool"})
			return err
		}},
		{name: "windowsSandbox/setupStart", call: func() error {
			_, err := client.WindowsSandboxSetupStart(ctx, &WindowsSandboxSetupStartParams{})
			return err
		}},
		{name: "windowsSandbox/readiness", call: func() error { _, err := client.WindowsSandboxReadiness(ctx); return err }},
		{name: "account/login/start", call: func() error {
			got, err := client.AccountLoginStart(ctx, NewLoginAccountParamsAPIKey("key"))
			if err != nil {
				return err
			}
			if _, ok := got.(APIKeyv2LoginAccountResponse); !ok {
				t.Fatalf("AccountLoginStart() = %#v (%T), want APIKeyv2LoginAccountResponse", got, got)
			}
			return nil
		}},
		{name: "account/login/cancel", call: func() error { _, err := client.AccountLoginCancel(ctx, &CancelLoginAccountParams{}); return err }},
		{name: "account/logout", call: func() error { _, err := client.AccountLogout(ctx); return err }},
		{name: "account/rateLimits/read", call: func() error { _, err := client.AccountRateLimitsRead(ctx); return err }},
		{name: "account/usage/read", call: func() error {
			got, err := client.AccountUsageRead(ctx)
			if err != nil {
				return err
			}
			if got.Summary.LifetimeTokens != 12345 || len(got.DailyUsageBuckets) != 1 || got.DailyUsageBuckets[0].Tokens != 2345 {
				t.Fatalf("AccountUsageRead() = %#v, want summary and daily bucket", got)
			}
			return nil
		}},
		{name: "account/sendAddCreditsNudgeEmail", call: func() error {
			_, err := client.AccountSendAddCreditsNudgeEmail(ctx, &SendAddCreditsNudgeEmailParams{})
			return err
		}},
		{name: "feedback/upload", call: func() error { _, err := client.FeedbackUpload(ctx, &FeedbackUploadParams{}); return err }},
		{name: "command/exec", call: func() error {
			_, err := client.CommandExec(ctx, &CommandExecParams{Command: []string{"echo", "ok"}})
			return err
		}},
		{name: "command/exec/write", call: func() error {
			_, err := client.CommandExecWrite(ctx, &CommandExecWriteParams{ProcessID: "process"})
			return err
		}},
		{name: "command/exec/terminate", call: func() error {
			_, err := client.CommandExecTerminate(ctx, &CommandExecTerminateParams{ProcessID: "process"})
			return err
		}},
		{name: "command/exec/resize", call: func() error {
			_, err := client.CommandExecResize(ctx, &CommandExecResizeParams{ProcessID: "process"})
			return err
		}},
		{name: "process/spawn", call: func() error {
			_, err := client.ProcessSpawn(ctx, &ProcessSpawnParams{
				Command:       []string{"echo", "ok"},
				Cwd:           "/tmp",
				ProcessHandle: "process-handle",
			})
			return err
		}},
		{name: "process/writeStdin", call: func() error {
			delta := "b2s="
			_, err := client.ProcessWriteStdin(ctx, &ProcessWriteStdinParams{ProcessHandle: "process-handle", DeltaBase64: delta})
			return err
		}},
		{name: "process/kill", call: func() error {
			_, err := client.ProcessKill(ctx, &ProcessKillParams{ProcessHandle: "process-handle"})
			return err
		}},
		{name: "process/resizePty", call: func() error {
			_, err := client.ProcessResizePty(ctx, &ProcessResizePtyParams{
				ProcessHandle: "process-handle",
				Size:          ProcessTerminalSize{Cols: 80, Rows: 24},
			})
			return err
		}},
		{name: "http/request", call: func() error {
			_, err := client.HTTPRequest(ctx, Object{"url": "https://example.com", "method": "GET"})
			return err
		}},
		{name: "http/request/bodyDelta", call: func() error {
			_, err := client.HTTPRequestBodyDelta(ctx, Object{"url": "https://example.com", "method": "GET"})
			return err
		}},
		{name: "config/read", call: func() error { _, err := client.ConfigRead(ctx, &ConfigReadParams{}); return err }},
		{name: "externalAgentConfig/detect", call: func() error {
			_, err := client.ExternalAgentConfigDetect(ctx, &ExternalAgentConfigDetectParams{})
			return err
		}},
		{name: "externalAgentConfig/import", call: func() error {
			_, err := client.ExternalAgentConfigImport(ctx, &ExternalAgentConfigImportParams{})
			return err
		}},
		{name: "config/value/write", call: func() error {
			_, err := client.ConfigValueWrite(ctx, &ConfigValueWriteParams{KeyPath: "model"})
			return err
		}},
		{name: "config/batchWrite", call: func() error { _, err := client.ConfigBatchWrite(ctx, &ConfigBatchWriteParams{}); return err }},
		{name: "configRequirements/read", call: func() error { _, err := client.ConfigRequirementsRead(ctx); return err }},
		{name: "account/read", call: func() error { _, err := client.AccountRead(ctx, &GetAccountParams{}); return err }},
		{name: "fuzzyFileSearch", call: func() error {
			got, err := client.FuzzyFileSearch(ctx, &FuzzyFileSearchParams{Query: "main", Roots: []string{"."}})
			if err != nil {
				return err
			}
			if string(got) != `{"sessionId":"fuzzy-session"}` {
				t.Fatalf("FuzzyFileSearch() raw = %s, want session id object", got)
			}
			return nil
		}},
	}

	for _, tt := range calls {
		t.Run("success: "+tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("%s wrapper error = %v", tt.name, err)
			}
		})
	}
}

func TestAccountLoginStartConstructors(t *testing.T) {
	t.Parallel()

	t.Run("api_key", func(t *testing.T) {
		t.Parallel()
		got := NewLoginAccountParamsAPIKey("sk-test")
		typed, ok := got.(APIKeyv2LoginAccountParams)
		if !ok {
			t.Fatalf("NewLoginAccountParamsAPIKey() type = %T, want APIKeyv2LoginAccountParams", got)
		}
		if typed.APIKey != "sk-test" {
			t.Fatalf("NewLoginAccountParamsAPIKey().APIKey = %q, want sk-test", typed.APIKey)
		}
		if typed.Type != "apiKey" {
			t.Fatalf("NewLoginAccountParamsAPIKey().Type = %q, want apiKey", typed.Type)
		}
	})

	t.Run("chatgpt", func(t *testing.T) {
		t.Parallel()
		got := NewLoginAccountParamsChatGPT()
		typed, ok := got.(ChatGPTv2LoginAccountParams)
		if !ok {
			t.Fatalf("NewLoginAccountParamsChatGPT() type = %T, want ChatGPTv2LoginAccountParams", got)
		}
		if typed.Type != "chatgpt" {
			t.Fatalf("NewLoginAccountParamsChatGPT().Type = %q, want chatgpt", typed.Type)
		}
	})

	t.Run("headless", func(t *testing.T) {
		t.Parallel()
		got := NewLoginAccountParamsHeadless()
		typed, ok := got.(ChatGPTDeviceCodev2LoginAccountParams)
		if !ok {
			t.Fatalf("NewLoginAccountParamsHeadless() type = %T, want ChatGPTDeviceCodev2LoginAccountParams", got)
		}
		if typed.Type != "chatgptDeviceCode" {
			t.Fatalf("NewLoginAccountParamsHeadless().Type = %q, want chatgptDeviceCode", typed.Type)
		}
	})

	t.Run("implements_interface", func(t *testing.T) {
		t.Parallel()
		// All constructors must satisfy the LoginAccountParams interface.
		var _ LoginAccountParams = NewLoginAccountParamsAPIKey("")
		var _ LoginAccountParams = NewLoginAccountParamsChatGPT()
		var _ LoginAccountParams = NewLoginAccountParamsHeadless()
	})
}
