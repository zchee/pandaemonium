### Question

Investigate Codex’s authentication login/logout logic and verify whether a **dynamic auth/account switching implementation in Go** is possible.

### Ranked synthesis

| Rank | Explanation | Confidence | Basis |
|---:|---|---|---|
| 1 | **Yes: dynamic switching is feasible from Go through the app-server JSON-RPC account API, especially using `account/login/start` with `chatgptAuthTokens` when the Go host owns/refreshes ChatGPT access tokens.** | High | Codex exposes `account/login/start`, `account/logout`, `account/read`, and server-initiated `account/chatgptAuthTokens/refresh`; the app-server implementation writes external tokens into ephemeral auth, reloads `AuthManager`, and tests prove updated tokens are used after refresh. |
| 2 | **API-key switching is also feasible from Go through `account/login/start` with `type: "apiKey"`, but switching from external ChatGPT-token mode to API-key mode requires `account/logout` first.** | High | `login_api_key_common` rejects API-key login while external ChatGPT auth is active and explicitly says to update with `chatgptAuthTokens` or clear with `account/logout`. |
| 3 | **Direct `auth.json` profile swapping from Go is possible only as a weaker, mostly out-of-process strategy; it is not reliable for live switching in a running Codex process.** | Medium-High | `AuthManager` intentionally caches auth and says external `auth.json` changes are not observed until explicit `reload()`. Unauthorized recovery only reloads if account id matches, preventing accidental cross-account live switches. Keyring/auto storage can also make file-only switching incomplete. |
| 4 | **Using Codex-managed ChatGPT login/logout as a profile switcher is risky for multi-account switching because logout and replacement login may revoke managed OAuth tokens.** | High | `logout_with_revoke` revokes current managed ChatGPT tokens best-effort; `persist_tokens_async` revokes superseded managed ChatGPT tokens after saving replacement auth. |

---

## Evidence

### 1. Codex auth state is centered on `AuthDotJson`, with file/keyring/auto/ephemeral storage

**Evidence:** `codex-rs/login/src/auth/storage.rs:31-48` defines the persisted auth payload:

- `auth_mode`
- `OPENAI_API_KEY`
- `tokens`
- `last_refresh`
- `agent_identity`

**Evidence:** `codex-rs/login/src/auth/storage.rs:84-86` stores the file at `CODEX_HOME/auth.json`.

**Evidence:** `codex-rs/login/src/auth/storage.rs:136-152` writes the file with `create`, `truncate`, and Unix mode `0600`.

**Evidence:** `codex-rs/config/src/types.rs:85-98` defines credential storage modes:

- `File`: `CODEX_HOME/auth.json`
- `Keyring`
- `Auto`
- `Ephemeral`: memory only for the current process

**Inference:** A Go implementation can reproduce the **file format** when Codex is using file storage, but it cannot assume `auth.json` is authoritative when the configured storage mode is `keyring` or `auto`.

---

### 2. Login writes a single active auth record, not a multi-account registry

**Evidence:** API-key login constructs one `AuthDotJson` with `auth_mode: ApiKey`, `OPENAI_API_KEY`, and no tokens, then saves it: `codex-rs/login/src/auth/manager.rs:528-542`.

**Evidence:** browser/device ChatGPT login persists one `AuthDotJson` with `auth_mode: Chatgpt`, OAuth tokens, and `last_refresh`: `codex-rs/login/src/server.rs:788-827`.

**Evidence:** device code login exchanges the device authorization for tokens and then calls the same persistence function: `codex-rs/login/src/device_code_auth.rs:196-220`.

**Inference:** Codex itself models **one active credential set per `CODEX_HOME`**, not multiple named profiles. A Go-side account switcher must therefore either:

1. drive the app-server API to replace the active auth, or
2. maintain its own external registry/snapshots and restore one active record at a time.

---

### 3. Logout clears auth and reloads process state

**Evidence:** `logout` deletes storage through the selected storage backend: `codex-rs/login/src/auth/manager.rs:503-511`.

**Evidence:** `AuthManager::logout` clears all stores, then reloads cached auth so callers immediately see unauthenticated state: `codex-rs/login/src/auth/manager.rs:1748-1757`.

**Evidence:** `AuthManager::logout_with_revoke` revokes current managed tokens best-effort, clears all stores, then reloads: `codex-rs/login/src/auth/manager.rs:1759-1769`.

**Evidence:** app-server `logout_common` cancels any active login, calls `auth_manager.logout_with_revoke()`, refreshes plugin cache state, then reports current auth mode: `codex-rs/app-server/src/request_processors/account_processor.rs:670-699`.

**Evidence:** app-server `logout_v2` returns `{}` and emits `account/updated` when auth changes: `codex-rs/app-server/src/request_processors/account_processor.rs:701-721`.

**Evidence:** an integration test verifies `account/logout` removes `auth.json`, emits `account/updated` with no auth mode, and makes `account/read` return no account: `codex-rs/app-server/tests/suite/v2/account.rs:192-247`.

**Inference:** `account/logout` is a clean way for a Go app-server client to clear active auth, but it is **not a safe “temporarily switch away and preserve this token” primitive** for managed ChatGPT auth because revocation may invalidate the stored credentials.

---

### 4. Managed ChatGPT login replacement can revoke superseded tokens

**Evidence:** `persist_tokens_async` loads previous auth, saves replacement ChatGPT auth, and then revokes the previous token when `should_revoke_auth_tokens` says it should: `codex-rs/login/src/server.rs:799-837`.

**Evidence:** revocation uses a managed ChatGPT refresh token first, falling back to access token: `codex-rs/login/src/auth/revoke.rs:55-65` and `codex-rs/login/src/auth/revoke.rs:84-101`.

**Evidence:** `should_revoke_auth_tokens` returns `true` when the replacement auth does not reuse the same managed ChatGPT token: `codex-rs/login/src/auth/revoke.rs:67-82`.

**Inference:** A Go profile switcher that wants to preserve multiple ChatGPT-managed accounts should not rely on repeated Codex-managed login/logout cycles, because switching to account B may revoke account A’s refresh token. That pushes a robust dynamic implementation toward **host-managed external tokens** or carefully controlled snapshot restoration outside Codex’s login/logout path.

---

### 5. Running Codex caches auth; raw file edits are not live switching

**Evidence:** `AuthManager` documentation says it loads once and hands out cloned snapshots so the program has a consistent auth view: `codex-rs/login/src/auth/manager.rs:1244-1247`.

**Evidence:** the same doc states external `auth.json` modifications are **not observed until `reload()` is called explicitly**, by design, to avoid inconsistent mid-run auth: `codex-rs/login/src/auth/manager.rs:1249-1251`.

**Evidence:** `AuthManager::new` loads initial auth into an internal `RwLock<CachedAuth>`: `codex-rs/login/src/auth/manager.rs:1303-1335`.

**Evidence:** `AuthManager::auth()` normally returns cached auth; it only refreshes stale managed ChatGPT tokens proactively: `codex-rs/login/src/auth/manager.rs:1408-1424`.

**Evidence:** unauthorized recovery does a guarded reload only if the account id still matches the cached account id: `codex-rs/login/src/auth/manager.rs:1057-1074` and `codex-rs/login/src/auth/manager.rs:1192-1217`.

**Evidence:** `reload_if_account_id_matches` skips reload when the on-disk account id differs from the expected account id: `codex-rs/login/src/auth/manager.rs:1434-1467`.

**Inference:** A Go process that only swaps `CODEX_HOME/auth.json` should expect existing Codex processes to keep using their cached auth until restart or an explicit app-server/Rust-side reload path is triggered. The code actively avoids cross-account reload during 401 recovery.

---

### 6. App-server exposes account login/logout/read as JSON-RPC methods

**Evidence:** `AuthMode` defines `apiKey`, `chatgpt`, experimental `chatgptAuthTokens`, and `agentIdentity`: `codex-rs/app-server-protocol/src/protocol/common.rs:18-39`.

**Evidence:** app-server request registration exposes:

- `account/login/start`
- `account/login/cancel`
- `account/logout`

with global `account-auth` serialization: `codex-rs/app-server-protocol/src/protocol/common.rs:884-900`.

**Evidence:** app-server also defines server-to-client `account/chatgptAuthTokens/refresh`: `codex-rs/app-server-protocol/src/protocol/common.rs:1341-1344`.

**Evidence:** app-server docs list `account/read`, `account/login/start`, `account/login/completed`, `account/login/cancel`, `account/logout`, and `account/updated`: `codex-rs/app-server/README.md:1711-1730`.

**Evidence:** docs show API-key login request/response and account update notification shape: `codex-rs/app-server/README.md:1758-1776`.

**Evidence:** docs show ChatGPT browser/device login flows and logout shape: `codex-rs/app-server/README.md:1778-1819`.

**Inference:** A Go implementation does not need Rust FFI for dynamic switching if it talks JSON-RPC to the app-server over stdio/unix/ws transport and serializes these request/notification shapes correctly.

---

### 7. `chatgptAuthTokens` is the strongest dynamic-switching hook

**Evidence:** `LoginAccountParams::ChatgptAuthTokens` accepts:

- `access_token`
- `chatgpt_account_id`
- `chatgpt_plan_type`

and is marked experimental/internal: `codex-rs/app-server-protocol/src/protocol/v2/account.rs:64-81`.

**Evidence:** `LoginAccountResponse::ChatgptAuthTokens {}` is a first-class response variant: `codex-rs/app-server-protocol/src/protocol/v2/account.rs:112-115`.

**Evidence:** `GetAccountParams.refresh_token` docs say that in external auth mode refresh is ignored and clients should refresh tokens themselves and call `account/login/start` with `chatgptAuthTokens`: `codex-rs/app-server-protocol/src/protocol/v2/account.rs:221-228`.

**Evidence:** `ChatgptAuthTokensRefreshParams` includes a refresh reason and `previous_account_id`, explicitly for clients managing multiple accounts/workspaces: `codex-rs/app-server-protocol/src/protocol/v2/account.rs:145-167`.

**Evidence:** `ChatgptAuthTokensRefreshResponse` returns a fresh access token, account id, and plan type: `codex-rs/app-server-protocol/src/protocol/v2/account.rs:169-176`.

**Evidence:** app-server `login_chatgpt_auth_tokens_response` validates workspace restrictions, calls `login_with_chatgpt_auth_tokens`, reloads `AuthManager`, updates config residency requirements, and returns `ChatgptAuthTokens {}`: `codex-rs/app-server/src/request_processors/account_processor.rs:550-598`.

**Evidence:** `login_with_chatgpt_auth_tokens` stores external token auth in the **ephemeral** store: `codex-rs/login/src/auth/manager.rs:566-583`.

**Evidence:** `load_auth` always checks the ephemeral store first so external auth overrides persisted credentials: `codex-rs/login/src/auth/manager.rs:731-757`.

**Inference:** For a Go app that can obtain or refresh ChatGPT access tokens, the intended dynamic switch operation is:

```json
{
  "method": "account/login/start",
  "id": 42,
  "params": {
    "type": "chatgptAuthTokens",
    "accessToken": "<jwt-access-token>",
    "chatgptAccountId": "<workspace/account-id>",
    "chatgptPlanType": "pro"
  }
}
```

That updates the running Codex app-server’s active auth without rewriting managed `auth.json`.

---

### 8. Tests prove external-token dynamic refresh changes the authorization header

**Evidence:** `set_auth_token_updates_account_and_notifies` logs in with `chatgptAuthTokens`, expects `LoginAccountResponse::ChatgptAuthTokens {}`, receives `account/updated`, and `account/read` returns a ChatGPT account with email and plan: `codex-rs/app-server/tests/suite/v2/account.rs:250-323`.

**Evidence:** `account_read_refresh_token_is_noop_in_external_mode` proves `account/read { refreshToken: true }` does **not** trigger server-side refresh for external mode: `codex-rs/app-server/tests/suite/v2/account.rs:326-400`.

**Evidence:** app-server installs `ExternalAuthRefreshBridge`, which sends `account/chatgptAuthTokens/refresh` to the client on refresh requests and expects a `ChatgptAuthTokensRefreshResponse`: `codex-rs/app-server/src/message_processor.rs:92-157`.

**Evidence:** `MessageProcessor::new` registers that bridge on the shared `AuthManager`: `codex-rs/app-server/src/message_processor.rs:274-299`.

**Evidence:** `external_auth_refreshes_on_unauthorized` starts with an initial token, handles the refresh request with a different token/account id, and verifies the first outbound API request used the initial bearer token while the retry used the refreshed bearer token: `codex-rs/app-server/tests/suite/v2/account.rs:430-545`.

**Inference:** A Go implementation can support dynamic switching/refresh by implementing both sides:

1. client-to-server: `account/login/start` with `chatgptAuthTokens`
2. server-to-client: respond to `account/chatgptAuthTokens/refresh`
3. client-to-server: optionally send another `account/login/start` with a different account’s token to switch active auth

---

### 9. API-key switching is stable but has an external-auth guard

**Evidence:** API-key app-server login calls `login_with_api_key`, then `auth_manager.reload()`: `codex-rs/app-server/src/request_processors/account_processor.rs:251-287`.

**Evidence:** API-key login emits result plus `account/login/completed` and `account/updated`: `codex-rs/app-server/src/request_processors/account_processor.rs:289-300`.

**Evidence:** the test `login_account_api_key_succeeds_and_notifies` verifies the response, notifications, and that `auth.json` exists: `codex-rs/app-server/tests/suite/v2/account.rs:900-945`.

**Evidence:** when external ChatGPT auth is active, `login_api_key_common` rejects API-key login and returns: “External auth is active. Use account/login/start (chatgptAuthTokens) to update it or account/logout to clear it.” `codex-rs/app-server/src/request_processors/account_processor.rs:245-257`.

**Inference:** Go can dynamically switch API keys by sending another `apiKey` login, but if the running session is in external ChatGPT-token mode, Go must first call `account/logout` to clear external mode before switching to API-key mode.

---

## Feasibility verdict for Go

### Verdict: **Yes, feasible — but the correct implementation depends on which “dynamic switching” you mean.**

#### Option A — App-server external-token switching from Go: **best fit for live dynamic switching**

This is the strongest path.

A Go client connected to Codex app-server can send:

```json
{
  "method": "account/login/start",
  "id": 1,
  "params": {
    "type": "chatgptAuthTokens",
    "accessToken": "<jwt>",
    "chatgptAccountId": "<account-or-workspace-id>",
    "chatgptPlanType": "pro"
  }
}
```

Then handle:

```json
{
  "method": "account/chatgptAuthTokens/refresh",
  "id": 99,
  "params": {
    "reason": "unauthorized",
    "previousAccountId": "<previous-account-id-or-null>"
  }
}
```

and respond:

```json
{
  "id": 99,
  "result": {
    "accessToken": "<new-jwt>",
    "chatgptAccountId": "<account-or-workspace-id>",
    "chatgptPlanType": "pro"
  }
}
```

**Evidence-backed constraints:**

- `chatgptAuthTokens` is experimental/internal in protocol comments, not documented as the public recommended mode.
- The Go host must own token acquisition and refresh.
- `account/read { refreshToken: true }` will not refresh external tokens for you.
- Workspace/account mismatches can fail if `forced_chatgpt_workspace_id` is configured.
- The auth is ephemeral, process-local, and overrides persisted credentials while active.

**Use this if:** the Go implementation is an app-server host/client and can manage ChatGPT tokens itself.

---

#### Option B — App-server API-key switching from Go: **stable and simple**

A Go client can send:

```json
{
  "method": "account/login/start",
  "id": 2,
  "params": {
    "type": "apiKey",
    "apiKey": "sk-..."
  }
}
```

**Evidence-backed constraints:**

- It persists to the configured auth store.
- It reloads `AuthManager` immediately.
- It emits `account/login/completed` and `account/updated`.
- If external ChatGPT auth is active, API-key login is rejected until `account/logout` clears external auth.

**Use this if:** dynamic switching means selecting between API keys or between API-key mode and unauthenticated/logged-out state.

---

#### Option C — Go-managed `auth.json` snapshot switching: **possible, but not live/reliable by itself**

A Go process can maintain profile snapshots of `CODEX_HOME/auth.json` and restore one to `auth.json` with `0600` permissions.

But this is weaker because:

- running Codex processes cache auth;
- Codex explicitly does not observe external file changes until `AuthManager::reload()`;
- unauthorized recovery refuses to reload a different account id;
- keyring/auto storage may bypass `auth.json`;
- logout/replacement login can revoke managed ChatGPT tokens.

**Use this if:** switching happens before launching Codex, or you can restart the Codex process after restoring a profile.

**Do not treat this as:** a safe live account switch for a running app-server session.

---

#### Option D — Shelling out to `codex login` / `codex logout` from Go: **possible but poor for multi-account dynamic switching**

The CLI paths work, but they are designed around one active login:

- `codex login --with-api-key` writes API-key auth.
- ChatGPT browser/device login persists managed OAuth tokens.
- `codex logout` revokes/clears auth.

This is not a good dynamic account switcher for multiple ChatGPT profiles because logout and replacement login can revoke tokens you intended to keep.

---

## Implementation boundary in Go

### What Go can safely implement

**Evidence-backed yes:**

1. A JSON-RPC app-server client that sends:
   - `account/read`
   - `account/login/start`
   - `account/logout`
   - `account/login/cancel`

2. A host-managed ChatGPT token switcher using:
   - `type: "chatgptAuthTokens"`
   - `account/chatgptAuthTokens/refresh`

3. API-key switching with:
   - `type: "apiKey"`

4. Pre-launch profile switching by restoring a complete `auth.json` snapshot when Codex uses file storage.

### What Go should not assume

**Unknown / not supported by current evidence:**

1. No repository evidence shows a stable public Go SDK for these auth methods in this repo.
2. No public app-server method appears to mean “reload auth from disk now” without login/logout.
3. No multi-profile auth registry exists in Codex itself.
4. No evidence says `chatgptAuthTokens` is a stable public API; the protocol marks it experimental/internal.
5. No evidence says direct keyring manipulation from Go is supported or portable.

---

## Direct answer

**Yes, it is possible to implement dynamic switching in Go, but the robust live-switching path is not “edit `auth.json` while Codex is running.”**

The best-supported Go design is:

1. connect to Codex app-server over its JSON-RPC transport;
2. use `account/login/start` with `type: "apiKey"` for API-key switching;
3. use `account/login/start` with `type: "chatgptAuthTokens"` for host-managed ChatGPT account switching;
4. implement server-request handling for `account/chatgptAuthTokens/refresh`;
5. use `account/logout` only to clear active auth, not as a profile-preserving switch primitive.

For a file-snapshot-based Go tool, the safe boundary is:

- switch snapshots **before process start** or after stopping/restarting Codex;
- preserve `0600` permissions;
- account for keyring/auto storage;
- avoid `codex logout` if the intention is to preserve ChatGPT OAuth tokens.

---

## Unknowns / limits

- I did not inspect any external Go repository in this turn; the Go feasibility conclusion is based on Codex’s Rust app-server protocol and auth implementation.
- The current repository evidence does not establish whether OpenAI considers `chatgptAuthTokens` stable for external consumers; the protocol comment explicitly marks it experimental/internal.
- The repository does not expose a public “reload auth from disk” JSON-RPC method, so direct file-based live switching remains unsupported by evidence.
- Token acquisition for ChatGPT access tokens is outside the Go feasibility proven here; Codex proves it can consume host-provided tokens, not that Go can obtain them without a separate auth integration.

---

## Confidence scores

- **Performance:** 0.95
  Dynamic switching via app-server is cheap: a JSON-RPC request, in-memory/ephemeral auth update, and `AuthManager` reload. The main cost is token acquisition/refresh outside Codex.

- **Scalability:** 0.93
  The protocol serializes account-auth operations globally, which is appropriate for a single active auth state. It does not scale to multiple simultaneous active accounts in one Codex process, but it scales for switching the single active account.

- **Reliability:** 0.91
  App-server-mediated switching is reliable because it uses Codex’s own reload and notification paths. File-based switching is less reliable, so I would not use it for live switching.

- **Cost-effectiveness:** 0.96
  A Go JSON-RPC implementation is cheaper and less invasive than modifying Codex Rust auth internals or building a separate keyring/file mutation layer.
