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
	return Request[ThreadStartResponse](ctx, c, RequestMethodThreadStart, paramsOrEmpty(params))
}

// ThreadResume calls thread/resume.
func (c *Client) ThreadResume(ctx context.Context, threadID string, params *ThreadResumeParams) (ThreadResumeResponse, error) {
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID})
	if err != nil {
		return ThreadResumeResponse{}, err
	}
	return Request[ThreadResumeResponse](ctx, c, RequestMethodThreadResume, payload)
}

// ThreadFork calls thread/fork.
func (c *Client) ThreadFork(ctx context.Context, threadID string, params *ThreadForkParams) (ThreadForkResponse, error) {
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID})
	if err != nil {
		return ThreadForkResponse{}, err
	}
	return Request[ThreadForkResponse](ctx, c, RequestMethodThreadFork, payload)
}

// ThreadList calls thread/list.
func (c *Client) ThreadList(ctx context.Context, params *ThreadListParams) (ThreadListResponse, error) {
	return Request[ThreadListResponse](ctx, c, RequestMethodThreadList, paramsOrEmpty(params))
}

// ThreadRead calls thread/read.
func (c *Client) ThreadRead(ctx context.Context, threadID string, params *ThreadReadParams) (ThreadReadResponse, error) {
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID})
	if err != nil {
		return ThreadReadResponse{}, err
	}
	return Request[ThreadReadResponse](ctx, c, RequestMethodThreadRead, payload)
}

// ThreadArchive calls thread/archive.
func (c *Client) ThreadArchive(ctx context.Context, threadID string) (ThreadArchiveResponse, error) {
	return Request[ThreadArchiveResponse](ctx, c, RequestMethodThreadArchive, ThreadArchiveParams{ThreadID: threadID})
}

// ThreadUnsubscribe calls thread/unsubscribe.
func (c *Client) ThreadUnsubscribe(ctx context.Context, threadID string, params *ThreadUnsubscribeParams) (ThreadUnsubscribeResponse, error) {
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID})
	if err != nil {
		return ThreadUnsubscribeResponse{}, err
	}
	return Request[ThreadUnsubscribeResponse](ctx, c, RequestMethodThreadUnsubscribe, payload)
}

// ThreadMetadataUpdate calls thread/metadata/update.
func (c *Client) ThreadMetadataUpdate(ctx context.Context, threadID string, params *ThreadMetadataUpdateParams) (ThreadMetadataUpdateResponse, error) {
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID})
	if err != nil {
		return ThreadMetadataUpdateResponse{}, err
	}
	return Request[ThreadMetadataUpdateResponse](ctx, c, RequestMethodThreadMetadataUpdate, payload)
}

// ThreadUnarchive calls thread/unarchive.
func (c *Client) ThreadUnarchive(ctx context.Context, threadID string) (ThreadUnarchiveResponse, error) {
	return Request[ThreadUnarchiveResponse](ctx, c, RequestMethodThreadUnarchive, ThreadUnarchiveParams{ThreadID: threadID})
}

// ThreadSetName calls thread/name/set.
func (c *Client) ThreadSetName(ctx context.Context, threadID, name string) (ThreadSetNameResponse, error) {
	return Request[ThreadSetNameResponse](ctx, c, RequestMethodThreadNameSet, ThreadSetNameParams{ThreadID: threadID, Name: name})
}

// ThreadGoalSet calls thread/goal/set.
func (c *Client) ThreadGoalSet(ctx context.Context, threadID string, params *ThreadGoalSetParams) (ThreadGoalSetResponse, error) {
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID})
	if err != nil {
		return ThreadGoalSetResponse{}, err
	}
	return Request[ThreadGoalSetResponse](ctx, c, RequestMethodThreadGoalSet, payload)
}

// ThreadGoalGet calls thread/goal/get.
func (c *Client) ThreadGoalGet(ctx context.Context, threadID string) (ThreadGoalGetResponse, error) {
	return Request[ThreadGoalGetResponse](ctx, c, RequestMethodThreadGoalGet, ThreadGoalGetParams{ThreadID: threadID})
}

// ThreadGoalClear calls thread/goal/clear.
func (c *Client) ThreadGoalClear(ctx context.Context, threadID string) (ThreadGoalClearResponse, error) {
	return Request[ThreadGoalClearResponse](ctx, c, RequestMethodThreadGoalClear, ThreadGoalClearParams{ThreadID: threadID})
}

// ThreadCompact calls thread/compact/start.
func (c *Client) ThreadCompact(ctx context.Context, threadID string) (ThreadCompactStartResponse, error) {
	return Request[ThreadCompactStartResponse](ctx, c, RequestMethodThreadCompactStart, ThreadCompactStartParams{ThreadID: threadID})
}

// ThreadShellCommand calls thread/shellCommand.
func (c *Client) ThreadShellCommand(ctx context.Context, threadID string, params *ThreadShellCommandParams) (ThreadShellCommandResponse, error) {
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID})
	if err != nil {
		return ThreadShellCommandResponse{}, err
	}
	return Request[ThreadShellCommandResponse](ctx, c, RequestMethodThreadShellCommand, payload)
}

// ThreadApproveGuardianDeniedAction calls thread/approveGuardianDeniedAction.
func (c *Client) ThreadApproveGuardianDeniedAction(ctx context.Context, threadID string, params *ThreadApproveGuardianDeniedActionParams) (ThreadApproveGuardianDeniedActionResponse, error) {
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID})
	if err != nil {
		return ThreadApproveGuardianDeniedActionResponse{}, err
	}
	return Request[ThreadApproveGuardianDeniedActionResponse](ctx, c, RequestMethodThreadApproveGuardianDeniedAction, payload)
}

// ThreadRollback calls thread/rollback.
func (c *Client) ThreadRollback(ctx context.Context, threadID string, params *ThreadRollbackParams) (ThreadRollbackResponse, error) {
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID})
	if err != nil {
		return ThreadRollbackResponse{}, err
	}
	return Request[ThreadRollbackResponse](ctx, c, RequestMethodThreadRollback, payload)
}

// ThreadLoadedList calls thread/loaded/list.
func (c *Client) ThreadLoadedList(ctx context.Context, params *ThreadLoadedListParams) (ThreadLoadedListResponse, error) {
	return Request[ThreadLoadedListResponse](ctx, c, RequestMethodThreadLoadedList, paramsOrEmpty(params))
}

// ThreadInjectItems calls thread/inject_items.
func (c *Client) ThreadInjectItems(ctx context.Context, threadID string, params *ThreadInjectItemsParams) (ThreadInjectItemsResponse, error) {
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID})
	if err != nil {
		return ThreadInjectItemsResponse{}, err
	}
	return Request[ThreadInjectItemsResponse](ctx, c, RequestMethodThreadInjectItems, payload)
}

// SkillsList calls skills/list.
func (c *Client) SkillsList(ctx context.Context, params *SkillsListParams) (SkillsListResponse, error) {
	return Request[SkillsListResponse](ctx, c, RequestMethodSkillsList, paramsOrEmpty(params))
}

// SkillsExtraRootsSet calls skills/extraRoots/set.
func (c *Client) SkillsExtraRootsSet(ctx context.Context, params *SkillsExtraRootsSetParams) (SkillsExtraRootsSetResponse, error) {
	return Request[SkillsExtraRootsSetResponse](ctx, c, RequestMethodSkillsExtraRootsSet, paramsOrEmpty(params))
}

// HooksList calls hooks/list.
func (c *Client) HooksList(ctx context.Context, params *HooksListParams) (HooksListResponse, error) {
	return Request[HooksListResponse](ctx, c, RequestMethodHooksList, paramsOrEmpty(params))
}

// MarketplaceAdd calls marketplace/add.
func (c *Client) MarketplaceAdd(ctx context.Context, params *MarketplaceAddParams) (MarketplaceAddResponse, error) {
	return Request[MarketplaceAddResponse](ctx, c, RequestMethodMarketplaceAdd, paramsOrEmpty(params))
}

// MarketplaceRemove calls marketplace/remove.
func (c *Client) MarketplaceRemove(ctx context.Context, params *MarketplaceRemoveParams) (MarketplaceRemoveResponse, error) {
	return Request[MarketplaceRemoveResponse](ctx, c, RequestMethodMarketplaceRemove, paramsOrEmpty(params))
}

// MarketplaceUpgrade calls marketplace/upgrade.
func (c *Client) MarketplaceUpgrade(ctx context.Context, params *MarketplaceUpgradeParams) (MarketplaceUpgradeResponse, error) {
	return Request[MarketplaceUpgradeResponse](ctx, c, RequestMethodMarketplaceUpgrade, paramsOrEmpty(params))
}

// PluginList calls plugin/list.
func (c *Client) PluginList(ctx context.Context, params *PluginListParams) (PluginListResponse, error) {
	return Request[PluginListResponse](ctx, c, RequestMethodPluginList, paramsOrEmpty(params))
}

// PluginRead calls plugin/read.
func (c *Client) PluginRead(ctx context.Context, params *PluginReadParams) (PluginReadResponse, error) {
	return Request[PluginReadResponse](ctx, c, RequestMethodPluginRead, paramsOrEmpty(params))
}

// PluginSkillRead calls plugin/skill/read.
func (c *Client) PluginSkillRead(ctx context.Context, params *PluginSkillReadParams) (PluginSkillReadResponse, error) {
	return Request[PluginSkillReadResponse](ctx, c, RequestMethodPluginSkillRead, paramsOrEmpty(params))
}

// PluginShareSave calls plugin/share/save.
func (c *Client) PluginShareSave(ctx context.Context, params *PluginShareSaveParams) (PluginShareSaveResponse, error) {
	return Request[PluginShareSaveResponse](ctx, c, RequestMethodPluginShareSave, paramsOrEmpty(params))
}

// PluginShareUpdateTargets calls plugin/share/updateTargets.
func (c *Client) PluginShareUpdateTargets(ctx context.Context, params *PluginShareUpdateTargetsParams) (PluginShareUpdateTargetsResponse, error) {
	return Request[PluginShareUpdateTargetsResponse](ctx, c, RequestMethodPluginShareUpdateTargets, paramsOrEmpty(params))
}

// PluginShareList calls plugin/share/list.
func (c *Client) PluginShareList(ctx context.Context, params *PluginShareListParams) (PluginShareListResponse, error) {
	return Request[PluginShareListResponse](ctx, c, RequestMethodPluginShareList, paramsOrEmpty(params))
}

// PluginShareCheckout calls plugin/share/checkout.
func (c *Client) PluginShareCheckout(ctx context.Context, params *PluginShareCheckoutParams) (PluginShareCheckoutResponse, error) {
	return Request[PluginShareCheckoutResponse](ctx, c, RequestMethodPluginShareCheckout, paramsOrEmpty(params))
}

// PluginShareDelete calls plugin/share/delete.
func (c *Client) PluginShareDelete(ctx context.Context, params *PluginShareDeleteParams) (PluginShareDeleteResponse, error) {
	return Request[PluginShareDeleteResponse](ctx, c, RequestMethodPluginShareDelete, paramsOrEmpty(params))
}

// AppList calls app/list.
func (c *Client) AppList(ctx context.Context, params *AppsListParams) (AppsListResponse, error) {
	return Request[AppsListResponse](ctx, c, RequestMethodAppList, paramsOrEmpty(params))
}

// FSReadFile calls fs/readFile.
func (c *Client) FSReadFile(ctx context.Context, params *FSReadFileParams) (FSReadFileResponse, error) {
	return Request[FSReadFileResponse](ctx, c, RequestMethodFSReadFile, paramsOrEmpty(params))
}

// FSRead calls fs/read.
func (c *Client) FSRead(ctx context.Context, params *FSReadFileParams) (FSReadFileResponse, error) {
	return Request[FSReadFileResponse](ctx, c, "fs/read", paramsOrEmpty(params))
}

// FSWriteFile calls fs/writeFile.
func (c *Client) FSWriteFile(ctx context.Context, params *FSWriteFileParams) (FSWriteFileResponse, error) {
	return Request[FSWriteFileResponse](ctx, c, RequestMethodFSWriteFile, paramsOrEmpty(params))
}

// FSWrite calls fs/write.
func (c *Client) FSWrite(ctx context.Context, params *FSWriteFileParams) (FSWriteFileResponse, error) {
	return Request[FSWriteFileResponse](ctx, c, "fs/write", paramsOrEmpty(params))
}

// FSCreateDirectory calls fs/createDirectory.
func (c *Client) FSCreateDirectory(ctx context.Context, params *FSCreateDirectoryParams) (FSCreateDirectoryResponse, error) {
	return Request[FSCreateDirectoryResponse](ctx, c, RequestMethodFSCreateDirectory, paramsOrEmpty(params))
}

// FSGetMetadata calls fs/getMetadata.
func (c *Client) FSGetMetadata(ctx context.Context, params *FSGetMetadataParams) (FSGetMetadataResponse, error) {
	return Request[FSGetMetadataResponse](ctx, c, RequestMethodFSGetMetadata, paramsOrEmpty(params))
}

// FSStat calls fs/stat.
func (c *Client) FSStat(ctx context.Context, params *FSGetMetadataParams) (FSGetMetadataResponse, error) {
	return Request[FSGetMetadataResponse](ctx, c, "fs/stat", paramsOrEmpty(params))
}

// FSReadDirectory calls fs/readDirectory.
func (c *Client) FSReadDirectory(ctx context.Context, params *FSReadDirectoryParams) (FSReadDirectoryResponse, error) {
	return Request[FSReadDirectoryResponse](ctx, c, RequestMethodFSReadDirectory, paramsOrEmpty(params))
}

// FSList calls fs/list.
func (c *Client) FSList(ctx context.Context, params *FSReadDirectoryParams) (FSReadDirectoryResponse, error) {
	return Request[FSReadDirectoryResponse](ctx, c, "fs/list", paramsOrEmpty(params))
}

// FSRemove calls fs/remove.
func (c *Client) FSRemove(ctx context.Context, params *FSRemoveParams) (FSRemoveResponse, error) {
	return Request[FSRemoveResponse](ctx, c, RequestMethodFSRemove, paramsOrEmpty(params))
}

// FSCopy calls fs/copy.
func (c *Client) FSCopy(ctx context.Context, params *FSCopyParams) (FSCopyResponse, error) {
	return Request[FSCopyResponse](ctx, c, RequestMethodFSCopy, paramsOrEmpty(params))
}

// FSWatch calls fs/watch.
func (c *Client) FSWatch(ctx context.Context, params *FSWatchParams) (FSWatchResponse, error) {
	return Request[FSWatchResponse](ctx, c, RequestMethodFSWatch, paramsOrEmpty(params))
}

// FSUnwatch calls fs/unwatch.
func (c *Client) FSUnwatch(ctx context.Context, params *FSUnwatchParams) (FSUnwatchResponse, error) {
	return Request[FSUnwatchResponse](ctx, c, RequestMethodFSUnwatch, paramsOrEmpty(params))
}

// SkillsConfigWrite calls skills/config/write.
func (c *Client) SkillsConfigWrite(ctx context.Context, params *SkillsConfigWriteParams) (SkillsConfigWriteResponse, error) {
	return Request[SkillsConfigWriteResponse](ctx, c, RequestMethodSkillsConfigWrite, paramsOrEmpty(params))
}

// PluginInstall calls plugin/install.
func (c *Client) PluginInstall(ctx context.Context, params *PluginInstallParams) (PluginInstallResponse, error) {
	return Request[PluginInstallResponse](ctx, c, RequestMethodPluginInstall, paramsOrEmpty(params))
}

// PluginUninstall calls plugin/uninstall.
func (c *Client) PluginUninstall(ctx context.Context, params *PluginUninstallParams) (PluginUninstallResponse, error) {
	return Request[PluginUninstallResponse](ctx, c, RequestMethodPluginUninstall, paramsOrEmpty(params))
}

// TurnStart calls turn/start.
func (c *Client) TurnStart(ctx context.Context, threadID string, input RunInput, params *TurnStartParams) (TurnStartResponse, error) {
	items, err := normalizeInput(input)
	if err != nil {
		return TurnStartResponse{}, err
	}
	payload, err := mergeParamsBaseWins(params, Object{"threadId": threadID, "input": items})
	if err != nil {
		return TurnStartResponse{}, err
	}
	return Request[TurnStartResponse](ctx, c, RequestMethodTurnStart, payload)
}

// TurnInterrupt calls turn/interrupt.
func (c *Client) TurnInterrupt(ctx context.Context, threadID, turnID string) (TurnInterruptResponse, error) {
	return Request[TurnInterruptResponse](ctx, c, RequestMethodTurnInterrupt, Object{"threadId": threadID, "turnId": turnID})
}

// TurnSteer calls turn/steer.
func (c *Client) TurnSteer(ctx context.Context, threadID, expectedTurnID string, input RunInput) (TurnSteerResponse, error) {
	items, err := normalizeInput(input)
	if err != nil {
		return TurnSteerResponse{}, err
	}
	return Request[TurnSteerResponse](ctx, c, RequestMethodTurnSteer, Object{"threadId": threadID, "expectedTurnId": expectedTurnID, "input": items})
}

// ReviewStart calls review/start.
func (c *Client) ReviewStart(ctx context.Context, params *ReviewStartParams) (ReviewStartResponse, error) {
	return Request[ReviewStartResponse](ctx, c, RequestMethodReviewStart, paramsOrEmpty(params))
}

// ModelList calls model/list.
func (c *Client) ModelList(ctx context.Context, params *ModelListParams) (ModelListResponse, error) {
	return Request[ModelListResponse](ctx, c, RequestMethodModelList, paramsOrEmpty(params))
}

// ModelProviderCapabilitiesRead calls modelProvider/capabilities/read.
func (c *Client) ModelProviderCapabilitiesRead(ctx context.Context, params *ModelProviderCapabilitiesReadParams) (ModelProviderCapabilitiesReadResponse, error) {
	return Request[ModelProviderCapabilitiesReadResponse](ctx, c, RequestMethodModelProviderCapabilitiesRead, paramsOrEmpty(params))
}

// ExperimentalFeatureList calls experimentalFeature/list.
func (c *Client) ExperimentalFeatureList(ctx context.Context, params *ExperimentalFeatureListParams) (ExperimentalFeatureListResponse, error) {
	return Request[ExperimentalFeatureListResponse](ctx, c, RequestMethodExperimentalFeatureList, paramsOrEmpty(params))
}

// PermissionProfileList calls permissionProfile/list.
func (c *Client) PermissionProfileList(ctx context.Context, params *PermissionProfileListParams) (PermissionProfileListResponse, error) {
	return Request[PermissionProfileListResponse](ctx, c, RequestMethodPermissionProfileList, paramsOrEmpty(params))
}

// ExperimentalFeatureEnablementSet calls experimentalFeature/enablement/set.
func (c *Client) ExperimentalFeatureEnablementSet(ctx context.Context, params *ExperimentalFeatureEnablementSetParams) (ExperimentalFeatureEnablementSetResponse, error) {
	return Request[ExperimentalFeatureEnablementSetResponse](ctx, c, RequestMethodExperimentalFeatureEnablementSet, paramsOrEmpty(params))
}

// RemoteControlEnable calls remoteControl/enable.
func (c *Client) RemoteControlEnable(ctx context.Context) (RemoteControlEnableResponse, error) {
	return Request[RemoteControlEnableResponse](ctx, c, RequestMethodRemoteControlEnable, nil)
}

// RemoteControlDisable calls remoteControl/disable.
func (c *Client) RemoteControlDisable(ctx context.Context) (RemoteControlDisableResponse, error) {
	return Request[RemoteControlDisableResponse](ctx, c, RequestMethodRemoteControlDisable, nil)
}

// RemoteControlStatusRead calls remoteControl/status/read.
func (c *Client) RemoteControlStatusRead(ctx context.Context) (RemoteControlStatusReadResponse, error) {
	return Request[RemoteControlStatusReadResponse](ctx, c, RequestMethodRemoteControlStatusRead, nil)
}

// RemoteControlPairingStart calls remoteControl/pairing/start.
func (c *Client) RemoteControlPairingStart(ctx context.Context, params *RemoteControlPairingStartParams) (RemoteControlPairingStartResponse, error) {
	return Request[RemoteControlPairingStartResponse](ctx, c, RequestMethodRemoteControlPairingStart, paramsOrEmpty(params))
}

// RemoteControlPairingStatus calls remoteControl/pairing/status.
func (c *Client) RemoteControlPairingStatus(ctx context.Context, params *RemoteControlPairingStatusParams) (RemoteControlPairingStatusResponse, error) {
	return Request[RemoteControlPairingStatusResponse](ctx, c, RequestMethodRemoteControlPairingStatus, paramsOrEmpty(params))
}

// RemoteControlClientList calls remoteControl/client/list.
func (c *Client) RemoteControlClientList(ctx context.Context, params *RemoteControlClientsListParams) (RemoteControlClientsListResponse, error) {
	return Request[RemoteControlClientsListResponse](ctx, c, RequestMethodRemoteControlClientList, paramsOrEmpty(params))
}

// RemoteControlClientRevoke calls remoteControl/client/revoke.
func (c *Client) RemoteControlClientRevoke(ctx context.Context, params *RemoteControlClientsRevokeParams) (RemoteControlClientsRevokeResponse, error) {
	return Request[RemoteControlClientsRevokeResponse](ctx, c, RequestMethodRemoteControlClientRevoke, paramsOrEmpty(params))
}

// EnvironmentAdd calls environment/add.
func (c *Client) EnvironmentAdd(ctx context.Context, params *EnvironmentAddParams) (EnvironmentAddResponse, error) {
	return Request[EnvironmentAddResponse](ctx, c, RequestMethodEnvironmentAdd, paramsOrEmpty(params))
}

// MCPServerOAuthLogin calls mcpServer/oauth/login.
func (c *Client) MCPServerOAuthLogin(ctx context.Context, params *MCPServerOAuthLoginParams) (MCPServerOAuthLoginResponse, error) {
	return Request[MCPServerOAuthLoginResponse](ctx, c, RequestMethodMCPServerOAuthLogin, paramsOrEmpty(params))
}

// ConfigMCPServerReload calls config/mcpServer/reload.
func (c *Client) ConfigMCPServerReload(ctx context.Context) (MCPServerRefreshResponse, error) {
	return Request[MCPServerRefreshResponse](ctx, c, RequestMethodConfigMCPServerReload, nil)
}

// MCPServerStatusList calls mcpServerStatus/list.
func (c *Client) MCPServerStatusList(ctx context.Context, params *ListMCPServerStatusParams) (ListMCPServerStatusResponse, error) {
	return Request[ListMCPServerStatusResponse](ctx, c, RequestMethodMCPServerStatusList, paramsOrEmpty(params))
}

// MCPServerResourceRead calls mcpServer/resource/read.
func (c *Client) MCPServerResourceRead(ctx context.Context, params *MCPResourceReadParams) (MCPResourceReadResponse, error) {
	return Request[MCPResourceReadResponse](ctx, c, RequestMethodMCPServerResourceRead, paramsOrEmpty(params))
}

// MCPServerToolCall calls mcpServer/tool/call.
func (c *Client) MCPServerToolCall(ctx context.Context, params *MCPServerToolCallParams) (MCPServerToolCallResponse, error) {
	return Request[MCPServerToolCallResponse](ctx, c, RequestMethodMCPServerToolCall, paramsOrEmpty(params))
}

// WindowsSandboxSetupStart calls windowsSandbox/setupStart.
func (c *Client) WindowsSandboxSetupStart(ctx context.Context, params *WindowsSandboxSetupStartParams) (WindowsSandboxSetupStartResponse, error) {
	return Request[WindowsSandboxSetupStartResponse](ctx, c, RequestMethodWindowsSandboxSetupStart, paramsOrEmpty(params))
}

// WindowsSandboxReadiness calls windowsSandbox/readiness.
func (c *Client) WindowsSandboxReadiness(ctx context.Context) (WindowsSandboxReadinessResponse, error) {
	return Request[WindowsSandboxReadinessResponse](ctx, c, RequestMethodWindowsSandboxReadiness, nil)
}

// AccountLoginStart calls account/login/start.
// Use NewLoginAccountParamsAPIKey, NewLoginAccountParamsChatGPT, or
// NewLoginAccountParamsHeadless to construct the params argument.
func (c *Client) AccountLoginStart(ctx context.Context, params LoginAccountParams) (LoginAccountResponse, error) {
	return Request[LoginAccountResponse](ctx, c, RequestMethodAccountLoginStart, paramsOrEmpty(params))
}

// NewLoginAccountParamsAPIKey constructs LoginAccountParams for API key authentication.
func NewLoginAccountParamsAPIKey(apiKey string) LoginAccountParams {
	return APIKeyv2LoginAccountParams{Type: "apiKey", APIKey: apiKey}
}

// NewLoginAccountParamsChatGPT constructs LoginAccountParams for ChatGPT OAuth authentication.
func NewLoginAccountParamsChatGPT() LoginAccountParams {
	return ChatGPTv2LoginAccountParams{Type: "chatgpt"}
}

// NewLoginAccountParamsHeadless constructs LoginAccountParams for headless device-code authentication.
func NewLoginAccountParamsHeadless() LoginAccountParams {
	return ChatGPTDeviceCodev2LoginAccountParams{Type: "chatgptDeviceCode"}
}

// AccountLoginCancel calls account/login/cancel.
func (c *Client) AccountLoginCancel(ctx context.Context, params *CancelLoginAccountParams) (CancelLoginAccountResponse, error) {
	return Request[CancelLoginAccountResponse](ctx, c, RequestMethodAccountLoginCancel, paramsOrEmpty(params))
}

// AccountLogout calls account/logout.
func (c *Client) AccountLogout(ctx context.Context) (LogoutAccountResponse, error) {
	return Request[LogoutAccountResponse](ctx, c, RequestMethodAccountLogout, nil)
}

// AccountRateLimitsRead calls account/rateLimits/read.
func (c *Client) AccountRateLimitsRead(ctx context.Context) (GetAccountRateLimitsResponse, error) {
	return Request[GetAccountRateLimitsResponse](ctx, c, RequestMethodAccountRateLimitsRead, nil)
}

// AccountUsageRead calls account/usage/read.
func (c *Client) AccountUsageRead(ctx context.Context) (GetAccountTokenUsageResponse, error) {
	return Request[GetAccountTokenUsageResponse](ctx, c, RequestMethodAccountUsageRead, nil)
}

// AccountSendAddCreditsNudgeEmail calls account/sendAddCreditsNudgeEmail.
func (c *Client) AccountSendAddCreditsNudgeEmail(ctx context.Context, params *SendAddCreditsNudgeEmailParams) (SendAddCreditsNudgeEmailResponse, error) {
	return Request[SendAddCreditsNudgeEmailResponse](ctx, c, RequestMethodAccountSendAddCreditsNudgeEmail, paramsOrEmpty(params))
}

// FeedbackUpload calls feedback/upload.
func (c *Client) FeedbackUpload(ctx context.Context, params *FeedbackUploadParams) (FeedbackUploadResponse, error) {
	return Request[FeedbackUploadResponse](ctx, c, RequestMethodFeedbackUpload, paramsOrEmpty(params))
}

// CommandExec calls command/exec.
func (c *Client) CommandExec(ctx context.Context, params *CommandExecParams) (CommandExecResponse, error) {
	return Request[CommandExecResponse](ctx, c, RequestMethodCommandExec, paramsOrEmpty(params))
}

// CommandExecWrite calls command/exec/write.
func (c *Client) CommandExecWrite(ctx context.Context, params *CommandExecWriteParams) (CommandExecWriteResponse, error) {
	return Request[CommandExecWriteResponse](ctx, c, RequestMethodCommandExecWrite, paramsOrEmpty(params))
}

// CommandExecTerminate calls command/exec/terminate.
func (c *Client) CommandExecTerminate(ctx context.Context, params *CommandExecTerminateParams) (CommandExecTerminateResponse, error) {
	return Request[CommandExecTerminateResponse](ctx, c, RequestMethodCommandExecTerminate, paramsOrEmpty(params))
}

// HTTPRequest calls http/request.
func (c *Client) HTTPRequest(ctx context.Context, params any) (jsontext.Value, error) {
	return c.RequestRaw(ctx, "http/request", paramsOrEmpty(params))
}

// HTTPRequestBodyDelta calls http/request/bodyDelta.
func (c *Client) HTTPRequestBodyDelta(ctx context.Context, params any) (jsontext.Value, error) {
	return c.RequestRaw(ctx, "http/request/bodyDelta", paramsOrEmpty(params))
}

// CommandExecResize calls command/exec/resize.
func (c *Client) CommandExecResize(ctx context.Context, params *CommandExecResizeParams) (CommandExecResizeResponse, error) {
	return Request[CommandExecResizeResponse](ctx, c, RequestMethodCommandExecResize, paramsOrEmpty(params))
}

// ProcessSpawn calls process/spawn.
func (c *Client) ProcessSpawn(ctx context.Context, params *ProcessSpawnParams) (ProcessSpawnResponse, error) {
	return Request[ProcessSpawnResponse](ctx, c, RequestMethodProcessSpawn, paramsOrEmpty(params))
}

// ProcessWriteStdin calls process/writeStdin.
func (c *Client) ProcessWriteStdin(ctx context.Context, params *ProcessWriteStdinParams) (ProcessWriteStdinResponse, error) {
	return Request[ProcessWriteStdinResponse](ctx, c, RequestMethodProcessWriteStdin, paramsOrEmpty(params))
}

// ProcessKill calls process/kill.
func (c *Client) ProcessKill(ctx context.Context, params *ProcessKillParams) (ProcessKillResponse, error) {
	return Request[ProcessKillResponse](ctx, c, RequestMethodProcessKill, paramsOrEmpty(params))
}

// ProcessResizePty calls process/resizePty.
func (c *Client) ProcessResizePty(ctx context.Context, params *ProcessResizePtyParams) (ProcessResizePtyResponse, error) {
	return Request[ProcessResizePtyResponse](ctx, c, RequestMethodProcessResizePty, paramsOrEmpty(params))
}

// ConfigRead calls config/read.
func (c *Client) ConfigRead(ctx context.Context, params *ConfigReadParams) (ConfigReadResponse, error) {
	return Request[ConfigReadResponse](ctx, c, RequestMethodConfigRead, paramsOrEmpty(params))
}

// ExternalAgentConfigDetect calls externalAgentConfig/detect.
func (c *Client) ExternalAgentConfigDetect(ctx context.Context, params *ExternalAgentConfigDetectParams) (ExternalAgentConfigDetectResponse, error) {
	return Request[ExternalAgentConfigDetectResponse](ctx, c, RequestMethodExternalAgentConfigDetect, paramsOrEmpty(params))
}

// ExternalAgentConfigImport calls externalAgentConfig/import.
func (c *Client) ExternalAgentConfigImport(ctx context.Context, params *ExternalAgentConfigImportParams) (ExternalAgentConfigImportResponse, error) {
	return Request[ExternalAgentConfigImportResponse](ctx, c, RequestMethodExternalAgentConfigImport, paramsOrEmpty(params))
}

// ConfigValueWrite calls config/value/write.
func (c *Client) ConfigValueWrite(ctx context.Context, params *ConfigValueWriteParams) (ConfigWriteResponse, error) {
	return Request[ConfigWriteResponse](ctx, c, RequestMethodConfigValueWrite, paramsOrEmpty(params))
}

// ConfigBatchWrite calls config/batchWrite.
func (c *Client) ConfigBatchWrite(ctx context.Context, params *ConfigBatchWriteParams) (ConfigWriteResponse, error) {
	return Request[ConfigWriteResponse](ctx, c, RequestMethodConfigBatchWrite, paramsOrEmpty(params))
}

// ConfigRequirementsRead calls configRequirements/read.
func (c *Client) ConfigRequirementsRead(ctx context.Context) (ConfigRequirementsReadResponse, error) {
	return Request[ConfigRequirementsReadResponse](ctx, c, RequestMethodConfigRequirementsRead, nil)
}

// AccountRead calls account/read.
func (c *Client) AccountRead(ctx context.Context, params *GetAccountParams) (GetAccountResponse, error) {
	return Request[GetAccountResponse](ctx, c, RequestMethodAccountRead, paramsOrEmpty(params))
}

// FuzzyFileSearch calls fuzzyFileSearch and returns the raw JSON result.
func (c *Client) FuzzyFileSearch(ctx context.Context, params *FuzzyFileSearchParams) (jsontext.Value, error) {
	return c.RequestRaw(ctx, RequestMethodFuzzyFileSearch, paramsOrEmpty(params))
}
