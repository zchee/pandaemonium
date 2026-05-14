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
			_, err := client.ThreadUnsubscribe(ctx, &ThreadUnsubscribeParams{ThreadID: "thread"})
			return err
		}},
		{name: "thread/name/set", call: func() error { _, err := client.ThreadSetName(ctx, "thread", "name"); return err }},
		{name: "thread/metadata/update", call: func() error {
			_, err := client.ThreadMetadataUpdate(ctx, &ThreadMetadataUpdateParams{ThreadID: "thread"})
			return err
		}},
		{name: "thread/unarchive", call: func() error { _, err := client.ThreadUnarchive(ctx, "thread"); return err }},
		{name: "thread/compact/start", call: func() error { _, err := client.ThreadCompact(ctx, "thread"); return err }},
		{name: "thread/shellCommand", call: func() error {
			_, err := client.ThreadShellCommand(ctx, &ThreadShellCommandParams{ThreadID: "thread", Command: "echo ok"})
			return err
		}},
		{name: "thread/approveGuardianDeniedAction", call: func() error {
			_, err := client.ThreadApproveGuardianDeniedAction(ctx, &ThreadApproveGuardianDeniedActionParams{ThreadID: "thread", Event: jsontext.Value(`{}`)})
			return err
		}},
		{name: "thread/rollback", call: func() error {
			_, err := client.ThreadRollback(ctx, &ThreadRollbackParams{ThreadID: "thread", NumTurns: 1})
			return err
		}},
		{name: "thread/list", call: func() error { _, err := client.ThreadList(ctx, &ThreadListParams{}); return err }},
		{name: "thread/loaded/list", call: func() error { _, err := client.ThreadLoadedList(ctx, &ThreadLoadedListParams{}); return err }},
		{name: "thread/read", call: func() error { _, err := client.ThreadRead(ctx, "thread", true); return err }},
		{name: "thread/inject_items", call: func() error {
			_, err := client.ThreadInjectItems(ctx, &ThreadInjectItemsParams{ThreadID: "thread"})
			return err
		}},
		{name: "skills/list", call: func() error { _, err := client.SkillsList(ctx, &SkillsListParams{}); return err }},
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
		{name: "plugin/share/delete", call: func() error {
			_, err := client.PluginShareDelete(ctx, &PluginShareDeleteParams{RemotePluginID: "plugin"})
			return err
		}},
		{name: "app/list", call: func() error { _, err := client.AppList(ctx, &AppsListParams{}); return err }},
		{name: "fs/readFile", call: func() error { _, err := client.FsReadFile(ctx, &FsReadFileParams{Path: "/tmp/file"}); return err }},
		{name: "fs/writeFile", call: func() error {
			_, err := client.FsWriteFile(ctx, &FsWriteFileParams{Path: "/tmp/file", DataBase64: "b2s="})
			return err
		}},
		{name: "fs/createDirectory", call: func() error {
			_, err := client.FsCreateDirectory(ctx, &FsCreateDirectoryParams{Path: "/tmp/dir"})
			return err
		}},
		{name: "fs/getMetadata", call: func() error { _, err := client.FsGetMetadata(ctx, &FsGetMetadataParams{Path: "/tmp/file"}); return err }},
		{name: "fs/readDirectory", call: func() error { _, err := client.FsReadDirectory(ctx, &FsReadDirectoryParams{Path: "/tmp"}); return err }},
		{name: "fs/remove", call: func() error { _, err := client.FsRemove(ctx, &FsRemoveParams{Path: "/tmp/file"}); return err }},
		{name: "fs/copy", call: func() error {
			_, err := client.FsCopy(ctx, &FsCopyParams{SourcePath: "/tmp/a", DestinationPath: "/tmp/b"})
			return err
		}},
		{name: "fs/watch", call: func() error {
			_, err := client.FsWatch(ctx, &FsWatchParams{Path: "/tmp", WatchID: "watch"})
			return err
		}},
		{name: "fs/unwatch", call: func() error { _, err := client.FsUnwatch(ctx, &FsUnwatchParams{WatchID: "watch"}); return err }},
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
		{name: "model/list", call: func() error { _, err := client.ModelList(ctx, true); return err }},
		{name: "modelProvider/capabilities/read", call: func() error {
			_, err := client.ModelProviderCapabilitiesRead(ctx, &ModelProviderCapabilitiesReadParams{})
			return err
		}},
		{name: "experimentalFeature/list", call: func() error {
			_, err := client.ExperimentalFeatureList(ctx, &ExperimentalFeatureListParams{})
			return err
		}},
		{name: "experimentalFeature/enablement/set", call: func() error {
			_, err := client.ExperimentalFeatureEnablementSet(ctx, &ExperimentalFeatureEnablementSetParams{Enablement: map[string]bool{"feature": true}})
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
			got, err := client.AccountLoginStart(ctx, APIKeyv2LoginAccountParams{Type: "apiKey", APIKey: "key"})
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
