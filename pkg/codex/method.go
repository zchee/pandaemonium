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
	"context"

	"github.com/go-json-experiment/json/jsontext"
)

// ThreadStart calls thread/start.
func (c *Client) ThreadStart(ctx context.Context, params *ThreadStartParams) (ThreadStartResponse, error) {
	return c.Request[ThreadStartResponse](ctx, RequestMethodThreadStart, paramsOrEmpty(params))
}

// ThreadResume calls thread/resume.
func (c *Client) ThreadResume(ctx context.Context, threadID string, params *ThreadResumeParams) (ThreadResumeResponse, error) {
	payload := mergeParams(params, Object{"threadId": threadID})
	return c.Request[ThreadResumeResponse](ctx, RequestMethodThreadResume, payload)
}

// ThreadFork calls thread/fork.
func (c *Client) ThreadFork(ctx context.Context, threadID string, params *ThreadForkParams) (ThreadForkResponse, error) {
	payload := mergeParams(params, Object{"threadId": threadID})
	return c.Request[ThreadForkResponse](ctx, RequestMethodThreadFork, payload)
}

// ThreadList calls thread/list.
func (c *Client) ThreadList(ctx context.Context, params *ThreadListParams) (ThreadListResponse, error) {
	return c.Request[ThreadListResponse](ctx, RequestMethodThreadList, paramsOrEmpty(params))
}

// ThreadRead calls thread/read.
func (c *Client) ThreadRead(ctx context.Context, threadID string, includeTurns bool) (ThreadReadResponse, error) {
	return c.Request[ThreadReadResponse](ctx, RequestMethodThreadRead, ThreadReadParams{ThreadID: threadID, IncludeTurns: &includeTurns})
}

// ThreadArchive calls thread/archive.
func (c *Client) ThreadArchive(ctx context.Context, threadID string) (ThreadArchiveResponse, error) {
	return c.Request[ThreadArchiveResponse](ctx, RequestMethodThreadArchive, ThreadArchiveParams{ThreadID: threadID})
}

// ThreadUnsubscribe calls thread/unsubscribe.
func (c *Client) ThreadUnsubscribe(ctx context.Context, params *ThreadUnsubscribeParams) (ThreadUnsubscribeResponse, error) {
	return c.Request[ThreadUnsubscribeResponse](ctx, RequestMethodThreadUnsubscribe, paramsOrEmpty(params))
}

// ThreadMetadataUpdate calls thread/metadata/update.
func (c *Client) ThreadMetadataUpdate(ctx context.Context, params *ThreadMetadataUpdateParams) (ThreadMetadataUpdateResponse, error) {
	return c.Request[ThreadMetadataUpdateResponse](ctx, RequestMethodThreadMetadataUpdate, paramsOrEmpty(params))
}

// ThreadUnarchive calls thread/unarchive.
func (c *Client) ThreadUnarchive(ctx context.Context, threadID string) (ThreadUnarchiveResponse, error) {
	return c.Request[ThreadUnarchiveResponse](ctx, RequestMethodThreadUnarchive, ThreadUnarchiveParams{ThreadID: threadID})
}

// ThreadSetName calls thread/name/set.
func (c *Client) ThreadSetName(ctx context.Context, threadID, name string) (ThreadSetNameResponse, error) {
	return c.Request[ThreadSetNameResponse](ctx, RequestMethodThreadNameSet, ThreadSetNameParams{ThreadID: threadID, Name: name})
}

// ThreadCompact calls thread/compact/start.
func (c *Client) ThreadCompact(ctx context.Context, threadID string) (ThreadCompactStartResponse, error) {
	return c.Request[ThreadCompactStartResponse](ctx, RequestMethodThreadCompactStart, ThreadCompactStartParams{ThreadID: threadID})
}

// ThreadShellCommand calls thread/shellCommand.
func (c *Client) ThreadShellCommand(ctx context.Context, params *ThreadShellCommandParams) (ThreadShellCommandResponse, error) {
	return c.Request[ThreadShellCommandResponse](ctx, RequestMethodThreadShellCommand, paramsOrEmpty(params))
}

// ThreadApproveGuardianDeniedAction calls thread/approveGuardianDeniedAction.
func (c *Client) ThreadApproveGuardianDeniedAction(ctx context.Context, params *ThreadApproveGuardianDeniedActionParams) (ThreadApproveGuardianDeniedActionResponse, error) {
	return c.Request[ThreadApproveGuardianDeniedActionResponse](ctx, RequestMethodThreadApproveGuardianDeniedAction, paramsOrEmpty(params))
}

// ThreadRollback calls thread/rollback.
func (c *Client) ThreadRollback(ctx context.Context, params *ThreadRollbackParams) (ThreadRollbackResponse, error) {
	return c.Request[ThreadRollbackResponse](ctx, RequestMethodThreadRollback, paramsOrEmpty(params))
}

// ThreadLoadedList calls thread/loaded/list.
func (c *Client) ThreadLoadedList(ctx context.Context, params *ThreadLoadedListParams) (ThreadLoadedListResponse, error) {
	return c.Request[ThreadLoadedListResponse](ctx, RequestMethodThreadLoadedList, paramsOrEmpty(params))
}

// ThreadInjectItems calls thread/inject_items.
func (c *Client) ThreadInjectItems(ctx context.Context, params *ThreadInjectItemsParams) (ThreadInjectItemsResponse, error) {
	return c.Request[ThreadInjectItemsResponse](ctx, RequestMethodThreadInjectItems, paramsOrEmpty(params))
}

// SkillsList calls skills/list.
func (c *Client) SkillsList(ctx context.Context, params *SkillsListParams) (SkillsListResponse, error) {
	return c.Request[SkillsListResponse](ctx, RequestMethodSkillsList, paramsOrEmpty(params))
}

// HooksList calls hooks/list.
func (c *Client) HooksList(ctx context.Context, params *HooksListParams) (HooksListResponse, error) {
	return c.Request[HooksListResponse](ctx, RequestMethodHooksList, paramsOrEmpty(params))
}

// MarketplaceAdd calls marketplace/add.
func (c *Client) MarketplaceAdd(ctx context.Context, params *MarketplaceAddParams) (MarketplaceAddResponse, error) {
	return c.Request[MarketplaceAddResponse](ctx, RequestMethodMarketplaceAdd, paramsOrEmpty(params))
}

// MarketplaceRemove calls marketplace/remove.
func (c *Client) MarketplaceRemove(ctx context.Context, params *MarketplaceRemoveParams) (MarketplaceRemoveResponse, error) {
	return c.Request[MarketplaceRemoveResponse](ctx, RequestMethodMarketplaceRemove, paramsOrEmpty(params))
}

// MarketplaceUpgrade calls marketplace/upgrade.
func (c *Client) MarketplaceUpgrade(ctx context.Context, params *MarketplaceUpgradeParams) (MarketplaceUpgradeResponse, error) {
	return c.Request[MarketplaceUpgradeResponse](ctx, RequestMethodMarketplaceUpgrade, paramsOrEmpty(params))
}

// PluginList calls plugin/list.
func (c *Client) PluginList(ctx context.Context, params *PluginListParams) (PluginListResponse, error) {
	return c.Request[PluginListResponse](ctx, RequestMethodPluginList, paramsOrEmpty(params))
}

// PluginRead calls plugin/read.
func (c *Client) PluginRead(ctx context.Context, params *PluginReadParams) (PluginReadResponse, error) {
	return c.Request[PluginReadResponse](ctx, RequestMethodPluginRead, paramsOrEmpty(params))
}

// PluginSkillRead calls plugin/skill/read.
func (c *Client) PluginSkillRead(ctx context.Context, params *PluginSkillReadParams) (PluginSkillReadResponse, error) {
	return c.Request[PluginSkillReadResponse](ctx, RequestMethodPluginSkillRead, paramsOrEmpty(params))
}

// PluginShareSave calls plugin/share/save.
func (c *Client) PluginShareSave(ctx context.Context, params *PluginShareSaveParams) (PluginShareSaveResponse, error) {
	return c.Request[PluginShareSaveResponse](ctx, RequestMethodPluginShareSave, paramsOrEmpty(params))
}

// PluginShareUpdateTargets calls plugin/share/updateTargets.
func (c *Client) PluginShareUpdateTargets(ctx context.Context, params *PluginShareUpdateTargetsParams) (PluginShareUpdateTargetsResponse, error) {
	return c.Request[PluginShareUpdateTargetsResponse](ctx, RequestMethodPluginShareUpdateTargets, paramsOrEmpty(params))
}

// PluginShareList calls plugin/share/list.
func (c *Client) PluginShareList(ctx context.Context, params *PluginShareListParams) (PluginShareListResponse, error) {
	return c.Request[PluginShareListResponse](ctx, RequestMethodPluginShareList, paramsOrEmpty(params))
}

// PluginShareDelete calls plugin/share/delete.
func (c *Client) PluginShareDelete(ctx context.Context, params *PluginShareDeleteParams) (PluginShareDeleteResponse, error) {
	return c.Request[PluginShareDeleteResponse](ctx, RequestMethodPluginShareDelete, paramsOrEmpty(params))
}

// AppList calls app/list.
func (c *Client) AppList(ctx context.Context, params *AppsListParams) (AppsListResponse, error) {
	return c.Request[AppsListResponse](ctx, RequestMethodAppList, paramsOrEmpty(params))
}

// FsReadFile calls fs/readFile.
func (c *Client) FsReadFile(ctx context.Context, params *FsReadFileParams) (FsReadFileResponse, error) {
	return c.Request[FsReadFileResponse](ctx, RequestMethodFsReadFile, paramsOrEmpty(params))
}

// FsWriteFile calls fs/writeFile.
func (c *Client) FsWriteFile(ctx context.Context, params *FsWriteFileParams) (FsWriteFileResponse, error) {
	return c.Request[FsWriteFileResponse](ctx, RequestMethodFsWriteFile, paramsOrEmpty(params))
}

// FsCreateDirectory calls fs/createDirectory.
func (c *Client) FsCreateDirectory(ctx context.Context, params *FsCreateDirectoryParams) (FsCreateDirectoryResponse, error) {
	return c.Request[FsCreateDirectoryResponse](ctx, RequestMethodFsCreateDirectory, paramsOrEmpty(params))
}

// FsGetMetadata calls fs/getMetadata.
func (c *Client) FsGetMetadata(ctx context.Context, params *FsGetMetadataParams) (FsGetMetadataResponse, error) {
	return c.Request[FsGetMetadataResponse](ctx, RequestMethodFsGetMetadata, paramsOrEmpty(params))
}

// FsReadDirectory calls fs/readDirectory.
func (c *Client) FsReadDirectory(ctx context.Context, params *FsReadDirectoryParams) (FsReadDirectoryResponse, error) {
	return c.Request[FsReadDirectoryResponse](ctx, RequestMethodFsReadDirectory, paramsOrEmpty(params))
}

// FsRemove calls fs/remove.
func (c *Client) FsRemove(ctx context.Context, params *FsRemoveParams) (FsRemoveResponse, error) {
	return c.Request[FsRemoveResponse](ctx, RequestMethodFsRemove, paramsOrEmpty(params))
}

// FsCopy calls fs/copy.
func (c *Client) FsCopy(ctx context.Context, params *FsCopyParams) (FsCopyResponse, error) {
	return c.Request[FsCopyResponse](ctx, RequestMethodFsCopy, paramsOrEmpty(params))
}

// FsWatch calls fs/watch.
func (c *Client) FsWatch(ctx context.Context, params *FsWatchParams) (FsWatchResponse, error) {
	return c.Request[FsWatchResponse](ctx, RequestMethodFsWatch, paramsOrEmpty(params))
}

// FsUnwatch calls fs/unwatch.
func (c *Client) FsUnwatch(ctx context.Context, params *FsUnwatchParams) (FsUnwatchResponse, error) {
	return c.Request[FsUnwatchResponse](ctx, RequestMethodFsUnwatch, paramsOrEmpty(params))
}

// SkillsConfigWrite calls skills/config/write.
func (c *Client) SkillsConfigWrite(ctx context.Context, params *SkillsConfigWriteParams) (SkillsConfigWriteResponse, error) {
	return c.Request[SkillsConfigWriteResponse](ctx, RequestMethodSkillsConfigWrite, paramsOrEmpty(params))
}

// PluginInstall calls plugin/install.
func (c *Client) PluginInstall(ctx context.Context, params *PluginInstallParams) (PluginInstallResponse, error) {
	return c.Request[PluginInstallResponse](ctx, RequestMethodPluginInstall, paramsOrEmpty(params))
}

// PluginUninstall calls plugin/uninstall.
func (c *Client) PluginUninstall(ctx context.Context, params *PluginUninstallParams) (PluginUninstallResponse, error) {
	return c.Request[PluginUninstallResponse](ctx, RequestMethodPluginUninstall, paramsOrEmpty(params))
}

// TurnStart calls turn/start.
func (c *Client) TurnStart(ctx context.Context, threadID string, input any, params *TurnStartParams) (TurnStartResponse, error) {
	items, err := normalizeInput(input)
	if err != nil {
		return TurnStartResponse{}, err
	}
	payload := mergeParams(params, Object{"threadId": threadID, "input": items})
	return c.Request[TurnStartResponse](ctx, RequestMethodTurnStart, payload)
}

// TurnInterrupt calls turn/interrupt.
func (c *Client) TurnInterrupt(ctx context.Context, threadID, turnID string) (TurnInterruptResponse, error) {
	return c.Request[TurnInterruptResponse](ctx, RequestMethodTurnInterrupt, Object{"threadId": threadID, "turnId": turnID})
}

// TurnSteer calls turn/steer.
func (c *Client) TurnSteer(ctx context.Context, threadID, expectedTurnID string, input any) (TurnSteerResponse, error) {
	items, err := normalizeInput(input)
	if err != nil {
		return TurnSteerResponse{}, err
	}
	return c.Request[TurnSteerResponse](ctx, RequestMethodTurnSteer, Object{"threadId": threadID, "expectedTurnId": expectedTurnID, "input": items})
}

// ReviewStart calls review/start.
func (c *Client) ReviewStart(ctx context.Context, params *ReviewStartParams) (ReviewStartResponse, error) {
	return c.Request[ReviewStartResponse](ctx, RequestMethodReviewStart, paramsOrEmpty(params))
}

// ModelList calls model/list.
func (c *Client) ModelList(ctx context.Context, includeHidden bool) (ModelListResponse, error) {
	return c.Request[ModelListResponse](ctx, RequestMethodModelList, ModelListParams{IncludeHidden: &includeHidden})
}

// ModelProviderCapabilitiesRead calls modelProvider/capabilities/read.
func (c *Client) ModelProviderCapabilitiesRead(ctx context.Context, params *ModelProviderCapabilitiesReadParams) (ModelProviderCapabilitiesReadResponse, error) {
	return c.Request[ModelProviderCapabilitiesReadResponse](ctx, RequestMethodModelProviderCapabilitiesRead, paramsOrEmpty(params))
}

// ExperimentalFeatureList calls experimentalFeature/list.
func (c *Client) ExperimentalFeatureList(ctx context.Context, params *ExperimentalFeatureListParams) (ExperimentalFeatureListResponse, error) {
	return c.Request[ExperimentalFeatureListResponse](ctx, RequestMethodExperimentalFeatureList, paramsOrEmpty(params))
}

// ExperimentalFeatureEnablementSet calls experimentalFeature/enablement/set.
func (c *Client) ExperimentalFeatureEnablementSet(ctx context.Context, params *ExperimentalFeatureEnablementSetParams) (ExperimentalFeatureEnablementSetResponse, error) {
	return c.Request[ExperimentalFeatureEnablementSetResponse](ctx, RequestMethodExperimentalFeatureEnablementSet, paramsOrEmpty(params))
}

// MCPServerOAuthLogin calls mcpServer/oauth/login.
func (c *Client) MCPServerOAuthLogin(ctx context.Context, params *MCPServerOAuthLoginParams) (MCPServerOAuthLoginResponse, error) {
	return c.Request[MCPServerOAuthLoginResponse](ctx, RequestMethodMCPServerOAuthLogin, paramsOrEmpty(params))
}

// ConfigMCPServerReload calls config/mcpServer/reload.
func (c *Client) ConfigMCPServerReload(ctx context.Context) (MCPServerRefreshResponse, error) {
	return c.Request[MCPServerRefreshResponse](ctx, RequestMethodConfigMCPServerReload, nil)
}

// MCPServerStatusList calls mcpServerStatus/list.
func (c *Client) MCPServerStatusList(ctx context.Context, params *ListMCPServerStatusParams) (ListMCPServerStatusResponse, error) {
	return c.Request[ListMCPServerStatusResponse](ctx, RequestMethodMCPServerStatusList, paramsOrEmpty(params))
}

// MCPServerResourceRead calls mcpServer/resource/read.
func (c *Client) MCPServerResourceRead(ctx context.Context, params *MCPResourceReadParams) (MCPResourceReadResponse, error) {
	return c.Request[MCPResourceReadResponse](ctx, RequestMethodMCPServerResourceRead, paramsOrEmpty(params))
}

// MCPServerToolCall calls mcpServer/tool/call.
func (c *Client) MCPServerToolCall(ctx context.Context, params *MCPServerToolCallParams) (MCPServerToolCallResponse, error) {
	return c.Request[MCPServerToolCallResponse](ctx, RequestMethodMCPServerToolCall, paramsOrEmpty(params))
}

// WindowsSandboxSetupStart calls windowsSandbox/setupStart.
func (c *Client) WindowsSandboxSetupStart(ctx context.Context, params *WindowsSandboxSetupStartParams) (WindowsSandboxSetupStartResponse, error) {
	return c.Request[WindowsSandboxSetupStartResponse](ctx, RequestMethodWindowsSandboxSetupStart, paramsOrEmpty(params))
}

// WindowsSandboxReadiness calls windowsSandbox/readiness.
func (c *Client) WindowsSandboxReadiness(ctx context.Context) (WindowsSandboxReadinessResponse, error) {
	return c.Request[WindowsSandboxReadinessResponse](ctx, RequestMethodWindowsSandboxReadiness, nil)
}

// AccountLoginStart calls account/login/start.
func (c *Client) AccountLoginStart(ctx context.Context, params LoginAccountParams) (LoginAccountResponse, error) {
	return c.Request[LoginAccountResponse](ctx, RequestMethodAccountLoginStart, paramsOrEmpty(params))
}

// AccountLoginCancel calls account/login/cancel.
func (c *Client) AccountLoginCancel(ctx context.Context, params *CancelLoginAccountParams) (CancelLoginAccountResponse, error) {
	return c.Request[CancelLoginAccountResponse](ctx, RequestMethodAccountLoginCancel, paramsOrEmpty(params))
}

// AccountLogout calls account/logout.
func (c *Client) AccountLogout(ctx context.Context) (LogoutAccountResponse, error) {
	return c.Request[LogoutAccountResponse](ctx, RequestMethodAccountLogout, nil)
}

// AccountRateLimitsRead calls account/rateLimits/read.
func (c *Client) AccountRateLimitsRead(ctx context.Context) (GetAccountRateLimitsResponse, error) {
	return c.Request[GetAccountRateLimitsResponse](ctx, RequestMethodAccountRateLimitsRead, nil)
}

// AccountSendAddCreditsNudgeEmail calls account/sendAddCreditsNudgeEmail.
func (c *Client) AccountSendAddCreditsNudgeEmail(ctx context.Context, params *SendAddCreditsNudgeEmailParams) (SendAddCreditsNudgeEmailResponse, error) {
	return c.Request[SendAddCreditsNudgeEmailResponse](ctx, RequestMethodAccountSendAddCreditsNudgeEmail, paramsOrEmpty(params))
}

// FeedbackUpload calls feedback/upload.
func (c *Client) FeedbackUpload(ctx context.Context, params *FeedbackUploadParams) (FeedbackUploadResponse, error) {
	return c.Request[FeedbackUploadResponse](ctx, RequestMethodFeedbackUpload, paramsOrEmpty(params))
}

// CommandExec calls command/exec.
func (c *Client) CommandExec(ctx context.Context, params *CommandExecParams) (CommandExecResponse, error) {
	return c.Request[CommandExecResponse](ctx, RequestMethodCommandExec, paramsOrEmpty(params))
}

// CommandExecWrite calls command/exec/write.
func (c *Client) CommandExecWrite(ctx context.Context, params *CommandExecWriteParams) (CommandExecWriteResponse, error) {
	return c.Request[CommandExecWriteResponse](ctx, RequestMethodCommandExecWrite, paramsOrEmpty(params))
}

// CommandExecTerminate calls command/exec/terminate.
func (c *Client) CommandExecTerminate(ctx context.Context, params *CommandExecTerminateParams) (CommandExecTerminateResponse, error) {
	return c.Request[CommandExecTerminateResponse](ctx, RequestMethodCommandExecTerminate, paramsOrEmpty(params))
}

// CommandExecResize calls command/exec/resize.
func (c *Client) CommandExecResize(ctx context.Context, params *CommandExecResizeParams) (CommandExecResizeResponse, error) {
	return c.Request[CommandExecResizeResponse](ctx, RequestMethodCommandExecResize, paramsOrEmpty(params))
}

// ConfigRead calls config/read.
func (c *Client) ConfigRead(ctx context.Context, params *ConfigReadParams) (ConfigReadResponse, error) {
	return c.Request[ConfigReadResponse](ctx, RequestMethodConfigRead, paramsOrEmpty(params))
}

// ExternalAgentConfigDetect calls externalAgentConfig/detect.
func (c *Client) ExternalAgentConfigDetect(ctx context.Context, params *ExternalAgentConfigDetectParams) (ExternalAgentConfigDetectResponse, error) {
	return c.Request[ExternalAgentConfigDetectResponse](ctx, RequestMethodExternalAgentConfigDetect, paramsOrEmpty(params))
}

// ExternalAgentConfigImport calls externalAgentConfig/import.
func (c *Client) ExternalAgentConfigImport(ctx context.Context, params *ExternalAgentConfigImportParams) (ExternalAgentConfigImportResponse, error) {
	return c.Request[ExternalAgentConfigImportResponse](ctx, RequestMethodExternalAgentConfigImport, paramsOrEmpty(params))
}

// ConfigValueWrite calls config/value/write.
func (c *Client) ConfigValueWrite(ctx context.Context, params *ConfigValueWriteParams) (ConfigWriteResponse, error) {
	return c.Request[ConfigWriteResponse](ctx, RequestMethodConfigValueWrite, paramsOrEmpty(params))
}

// ConfigBatchWrite calls config/batchWrite.
func (c *Client) ConfigBatchWrite(ctx context.Context, params *ConfigBatchWriteParams) (ConfigWriteResponse, error) {
	return c.Request[ConfigWriteResponse](ctx, RequestMethodConfigBatchWrite, paramsOrEmpty(params))
}

// ConfigRequirementsRead calls configRequirements/read.
func (c *Client) ConfigRequirementsRead(ctx context.Context) (ConfigRequirementsReadResponse, error) {
	return c.Request[ConfigRequirementsReadResponse](ctx, RequestMethodConfigRequirementsRead, nil)
}

// AccountRead calls account/read.
func (c *Client) AccountRead(ctx context.Context, params *GetAccountParams) (GetAccountResponse, error) {
	return c.Request[GetAccountResponse](ctx, RequestMethodAccountRead, paramsOrEmpty(params))
}

// FuzzyFileSearch calls fuzzyFileSearch and returns the raw JSON result.
func (c *Client) FuzzyFileSearch(ctx context.Context, params *FuzzyFileSearchParams) (jsontext.Value, error) {
	return c.RequestRaw(ctx, RequestMethodFuzzyFileSearch, paramsOrEmpty(params))
}
