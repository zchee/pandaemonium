### 質問

Codex の認証ログイン/ログアウトロジックを調査し、**Go で動的な認証/アカウント切り替え実装が可能か**を検証する。

### 優先順位付きの統合見解

| 順位 | 説明 | 確信度 | 根拠 |
|---:|---|---|---|
| 1 | **可能: Go から app-server の JSON-RPC account API 経由で動的切り替えを実現できる。特に Go ホストが ChatGPT アクセストークンを所有/更新する場合、`account/login/start` に `chatgptAuthTokens` を渡す方法が有力。** | 高 | Codex は `account/login/start`、`account/logout`、`account/read`、およびサーバー起点の `account/chatgptAuthTokens/refresh` を公開している。app-server 実装は外部トークンを ephemeral auth に書き込み、`AuthManager` を reload し、テストでも refresh 後に更新済みトークンが使われることが証明されている。 |
| 2 | **API キー切り替えも、`type: "apiKey"` の `account/login/start` により Go から実現できる。ただし外部 ChatGPT トークンモードから API キーモードへ切り替えるには、先に `account/logout` が必要。** | 高 | `login_api_key_common` は外部 ChatGPT auth が有効な間の API キーログインを拒否し、`chatgptAuthTokens` で更新するか `account/logout` でクリアするよう明示している。 |
| 3 | **Go から直接 `auth.json` プロファイルを差し替える方法は、弱めの、主にプロセス外の戦略としてのみ可能。稼働中の Codex プロセスでのライブ切り替えには信頼性がない。** | 中〜高 | `AuthManager` は意図的に auth をキャッシュし、明示的な `reload()` まで外部の `auth.json` 変更を観測しない。unauthorized recovery もアカウント ID が一致する場合にだけ reload するため、意図しないクロスアカウントのライブ切り替えを防いでいる。keyring/auto storage によって、ファイルだけの切り替えが不完全になることもある。 |
| 4 | **Codex 管理の ChatGPT login/logout をプロファイル切り替え器として使うのは、複数アカウント切り替えでは危険。logout や置換ログインが管理対象 OAuth トークンを revoke する可能性があるため。** | 高 | `logout_with_revoke` は現在の管理対象 ChatGPT トークンを best-effort で revoke する。`persist_tokens_async` は置換 auth の保存後、置き換えられた管理対象 ChatGPT トークンを revoke する。 |

---

## 証拠

### 1. Codex の auth 状態は `AuthDotJson` を中心にしており、file/keyring/auto/ephemeral storage を持つ

**証拠:** `codex-rs/login/src/auth/storage.rs:31-48` は永続化される auth ペイロードを定義している:

- `auth_mode`
- `OPENAI_API_KEY`
- `tokens`
- `last_refresh`
- `agent_identity`

**証拠:** `codex-rs/login/src/auth/storage.rs:84-86` はファイルを `CODEX_HOME/auth.json` に保存する。

**証拠:** `codex-rs/login/src/auth/storage.rs:136-152` は `create`、`truncate`、Unix mode `0600` でファイルを書き込む。

**証拠:** `codex-rs/config/src/types.rs:85-98` は credential storage modes を定義している:

- `File`: `CODEX_HOME/auth.json`
- `Keyring`
- `Auto`
- `Ephemeral`: 現在のプロセスだけのメモリ

**推論:** Codex が file storage を使っている場合、Go 実装は**ファイル形式**を再現できる。しかし、storage mode が `keyring` または `auto` に設定されている場合、`auth.json` が authoritative だとは仮定できない。

---

### 2. Login は複数アカウントのレジストリではなく、単一のアクティブ auth レコードを書き込む

**証拠:** API キーログインは `auth_mode: ApiKey`、`OPENAI_API_KEY`、tokens なしの単一 `AuthDotJson` を構築して保存する: `codex-rs/login/src/auth/manager.rs:528-542`。

**証拠:** browser/device ChatGPT ログインは `auth_mode: Chatgpt`、OAuth tokens、`last_refresh` を持つ単一 `AuthDotJson` を永続化する: `codex-rs/login/src/server.rs:788-827`。

**証拠:** device code login は device authorization を tokens に交換し、同じ persistence 関数を呼ぶ: `codex-rs/login/src/device_code_auth.rs:196-220`。

**推論:** Codex 自体は、**`CODEX_HOME` ごとに 1 つのアクティブ credential set**をモデル化しており、複数の named profiles は持たない。したがって Go 側のアカウント切り替え器は次のどちらかを行う必要がある:

1. app-server API を駆動してアクティブ auth を置き換える、または
2. 独自の外部レジストリ/スナップショットを維持し、1 回につき 1 つのアクティブレコードを復元する。

---

### 3. Logout は auth をクリアし、プロセス状態を reload する

**証拠:** `logout` は選択された storage backend 経由で storage を削除する: `codex-rs/login/src/auth/manager.rs:503-511`。

**証拠:** `AuthManager::logout` はすべての stores をクリアし、その後 cached auth を reload して、呼び出し元が即座に unauthenticated state を見るようにする: `codex-rs/login/src/auth/manager.rs:1748-1757`。

**証拠:** `AuthManager::logout_with_revoke` は現在の managed tokens を best-effort で revoke し、すべての stores をクリアしてから reload する: `codex-rs/login/src/auth/manager.rs:1759-1769`。

**証拠:** app-server の `logout_common` は active login があればキャンセルし、`auth_manager.logout_with_revoke()` を呼び、plugin cache state を refresh してから現在の auth mode を報告する: `codex-rs/app-server/src/request_processors/account_processor.rs:670-699`。

**証拠:** app-server の `logout_v2` は `{}` を返し、auth が変化した場合に `account/updated` を emit する: `codex-rs/app-server/src/request_processors/account_processor.rs:701-721`。

**証拠:** integration test は、`account/logout` が `auth.json` を削除し、auth mode なしの `account/updated` を emit し、`account/read` が no account を返すことを検証している: `codex-rs/app-server/tests/suite/v2/account.rs:192-247`。

**推論:** `account/logout` は、Go の app-server client が active auth をクリアするためのクリーンな方法である。ただし、managed ChatGPT auth では revoke によって保存済み credentials が無効化される可能性があるため、**「一時的に切り替えて、この token を保持する」ための安全なプリミティブではない**。

---

### 4. 管理対象 ChatGPT ログインの置換は、置き換えられた token を revoke し得る

**証拠:** `persist_tokens_async` は previous auth を load し、replacement ChatGPT auth を save し、その後 `should_revoke_auth_tokens` が revoke すべきと判断した場合に previous token を revoke する: `codex-rs/login/src/server.rs:799-837`。

**証拠:** revocation はまず managed ChatGPT refresh token を使い、なければ access token に fallback する: `codex-rs/login/src/auth/revoke.rs:55-65` および `codex-rs/login/src/auth/revoke.rs:84-101`。

**証拠:** `should_revoke_auth_tokens` は、replacement auth が同じ managed ChatGPT token を再利用していない場合に `true` を返す: `codex-rs/login/src/auth/revoke.rs:67-82`。

**推論:** 複数の ChatGPT-managed accounts を保持したい Go profile switcher は、Codex 管理の login/logout cycle の反復に依存すべきではない。account B に切り替えることで account A の refresh token が revoke され得るためである。堅牢な動的実装は、**host-managed external tokens** または Codex の login/logout path の外側で慎重に制御された snapshot restoration に寄せるべきである。

---

### 5. 稼働中の Codex は auth をキャッシュする。生のファイル編集はライブ切り替えではない

**証拠:** `AuthManager` のドキュメントは、一度 load して cloned snapshots を渡すことで、program が一貫した auth view を持つと説明している: `codex-rs/login/src/auth/manager.rs:1244-1247`。

**証拠:** 同じドキュメントは、外部の `auth.json` 変更は一貫性のない mid-run auth を避けるため、明示的に `reload()` が呼ばれるまで**観測されない**と述べている: `codex-rs/login/src/auth/manager.rs:1249-1251`。

**証拠:** `AuthManager::new` は初期 auth を internal `RwLock<CachedAuth>` に load する: `codex-rs/login/src/auth/manager.rs:1303-1335`。

**証拠:** `AuthManager::auth()` は通常 cached auth を返す。stale な managed ChatGPT tokens だけを proactive に refresh する: `codex-rs/login/src/auth/manager.rs:1408-1424`。

**証拠:** unauthorized recovery は、account id が cached account id と一致する場合にだけ guarded reload を行う: `codex-rs/login/src/auth/manager.rs:1057-1074` および `codex-rs/login/src/auth/manager.rs:1192-1217`。

**証拠:** `reload_if_account_id_matches` は、on-disk account id が expected account id と異なる場合に reload を skip する: `codex-rs/login/src/auth/manager.rs:1434-1467`。

**推論:** Go プロセスが `CODEX_HOME/auth.json` だけを差し替える場合、既存の Codex プロセスは restart または明示的な app-server/Rust 側 reload path が発火するまで cached auth を使い続けると想定すべきである。コードは 401 recovery 中の cross-account reload を積極的に避けている。

---

### 6. App-server は account login/logout/read を JSON-RPC methods として公開している

**証拠:** `AuthMode` は `apiKey`、`chatgpt`、experimental な `chatgptAuthTokens`、および `agentIdentity` を定義している: `codex-rs/app-server-protocol/src/protocol/common.rs:18-39`。

**証拠:** app-server request registration は次を公開している:

- `account/login/start`
- `account/login/cancel`
- `account/logout`

これらは global `account-auth` serialization を伴う: `codex-rs/app-server-protocol/src/protocol/common.rs:884-900`。

**証拠:** app-server は server-to-client の `account/chatgptAuthTokens/refresh` も定義している: `codex-rs/app-server-protocol/src/protocol/common.rs:1341-1344`。

**証拠:** app-server docs は `account/read`、`account/login/start`、`account/login/completed`、`account/login/cancel`、`account/logout`、`account/updated` を列挙している: `codex-rs/app-server/README.md:1711-1730`。

**証拠:** docs は API-key login request/response と account update notification の形を示している: `codex-rs/app-server/README.md:1758-1776`。

**証拠:** docs は ChatGPT browser/device login flows と logout の形を示している: `codex-rs/app-server/README.md:1778-1819`。

**推論:** Go 実装は、stdio/unix/ws transport 経由で app-server に JSON-RPC で話し、これらの request/notification shape を正しく serialize できれば、動的切り替えのために Rust FFI を必要としない。

---

### 7. `chatgptAuthTokens` は最も強い動的切り替え hook である

**証拠:** `LoginAccountParams::ChatgptAuthTokens` は次を受け取る:

- `access_token`
- `chatgpt_account_id`
- `chatgpt_plan_type`

そして experimental/internal と記されている: `codex-rs/app-server-protocol/src/protocol/v2/account.rs:64-81`。

**証拠:** `LoginAccountResponse::ChatgptAuthTokens {}` は first-class response variant である: `codex-rs/app-server-protocol/src/protocol/v2/account.rs:112-115`。

**証拠:** `GetAccountParams.refresh_token` docs は、external auth mode では refresh は ignored であり、clients が自分で tokens を refresh し、`chatgptAuthTokens` 付きの `account/login/start` を呼ぶべきだと述べている: `codex-rs/app-server-protocol/src/protocol/v2/account.rs:221-228`。

**証拠:** `ChatgptAuthTokensRefreshParams` は refresh reason と `previous_account_id` を含み、複数 accounts/workspaces を管理する clients のためのものだと明示している: `codex-rs/app-server-protocol/src/protocol/v2/account.rs:145-167`。

**証拠:** `ChatgptAuthTokensRefreshResponse` は fresh access token、account id、plan type を返す: `codex-rs/app-server-protocol/src/protocol/v2/account.rs:169-176`。

**証拠:** app-server の `login_chatgpt_auth_tokens_response` は workspace restrictions を検証し、`login_with_chatgpt_auth_tokens` を呼び、`AuthManager` を reload し、config residency requirements を更新して、`ChatgptAuthTokens {}` を返す: `codex-rs/app-server/src/request_processors/account_processor.rs:550-598`。

**証拠:** `login_with_chatgpt_auth_tokens` は external token auth を **ephemeral** store に保存する: `codex-rs/login/src/auth/manager.rs:566-583`。

**証拠:** `load_auth` は常に最初に ephemeral store を確認するため、external auth は persisted credentials を override する: `codex-rs/login/src/auth/manager.rs:731-757`。

**推論:** ChatGPT access tokens を取得または refresh できる Go app にとって、意図された dynamic switch operation は次である:

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

これは管理対象 `auth.json` を書き換えずに、稼働中の Codex app-server の active auth を更新する。

---

### 8. テストは external-token の dynamic refresh が authorization header を変更することを証明している

**証拠:** `set_auth_token_updates_account_and_notifies` は `chatgptAuthTokens` でログインし、`LoginAccountResponse::ChatgptAuthTokens {}` を期待し、`account/updated` を受け取り、`account/read` が email と plan を持つ ChatGPT account を返すことを検証している: `codex-rs/app-server/tests/suite/v2/account.rs:250-323`。

**証拠:** `account_read_refresh_token_is_noop_in_external_mode` は、`account/read { refreshToken: true }` が external mode において server-side refresh を trigger **しない**ことを証明している: `codex-rs/app-server/tests/suite/v2/account.rs:326-400`。

**証拠:** app-server は `ExternalAuthRefreshBridge` を install し、refresh request 時に `account/chatgptAuthTokens/refresh` を client に送り、`ChatgptAuthTokensRefreshResponse` を期待する: `codex-rs/app-server/src/message_processor.rs:92-157`。

**証拠:** `MessageProcessor::new` はその bridge を shared `AuthManager` に登録する: `codex-rs/app-server/src/message_processor.rs:274-299`。

**証拠:** `external_auth_refreshes_on_unauthorized` は initial token で開始し、refresh request を別の token/account id で処理し、最初の outbound API request が initial bearer token を使い、retry が refreshed bearer token を使ったことを検証している: `codex-rs/app-server/tests/suite/v2/account.rs:430-545`。

**推論:** Go 実装は、次の両側を実装することで dynamic switching/refresh をサポートできる:

1. client-to-server: `chatgptAuthTokens` 付きの `account/login/start`
2. server-to-client: `account/chatgptAuthTokens/refresh` への応答
3. client-to-server: active auth を切り替えるため、必要なら別アカウントの token を持つ別の `account/login/start` を送る

---

### 9. API-key switching は安定しているが external-auth guard がある

**証拠:** API-key app-server login は `login_with_api_key` を呼んだ後、`auth_manager.reload()` を行う: `codex-rs/app-server/src/request_processors/account_processor.rs:251-287`。

**証拠:** API-key login は result に加えて `account/login/completed` と `account/updated` を emit する: `codex-rs/app-server/src/request_processors/account_processor.rs:289-300`。

**証拠:** test `login_account_api_key_succeeds_and_notifies` は response、notifications、そして `auth.json` が存在することを検証している: `codex-rs/app-server/tests/suite/v2/account.rs:900-945`。

**証拠:** external ChatGPT auth が active な場合、`login_api_key_common` は API-key login を拒否し、次を返す: “External auth is active. Use account/login/start (chatgptAuthTokens) to update it or account/logout to clear it.” `codex-rs/app-server/src/request_processors/account_processor.rs:245-257`。

**推論:** Go は別の `apiKey` login を送ることで API keys を動的に切り替えられる。ただし running session が external ChatGPT-token mode の場合、Go は API-key mode に切り替える前にまず `account/logout` を呼んで external mode をクリアしなければならない。

---

## Go における実現可能性の判定

### 判定: **可能。ただし正しい実装は、「dynamic switching」が何を意味するかに依存する。**

#### Option A — Go からの app-server external-token switching: **ライブ動的切り替えに最適**

これは最も強い経路である。

Codex app-server に接続した Go client は次を送信できる:

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

次を handle する:

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

そして次で応答する:

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

**証拠に基づく制約:**

- `chatgptAuthTokens` は protocol comments で experimental/internal とされており、public recommended mode として文書化されていない。
- Go host は token acquisition と refresh を所有しなければならない。
- `account/read { refreshToken: true }` は external tokens を代わりに refresh してくれない。
- `forced_chatgpt_workspace_id` が設定されている場合、workspace/account mismatch で失敗する可能性がある。
- auth は ephemeral、process-local であり、有効な間は persisted credentials を override する。

**これを使うべき場合:** Go 実装が app-server host/client であり、ChatGPT tokens を自分で管理できる場合。

---

#### Option B — Go からの app-server API-key switching: **安定していて単純**

Go client は次を送信できる:

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

**証拠に基づく制約:**

- configured auth store に永続化される。
- `AuthManager` が即座に reload される。
- `account/login/completed` と `account/updated` を emit する。
- external ChatGPT auth が active な場合、`account/logout` が external auth をクリアするまで API-key login は拒否される。

**これを使うべき場合:** dynamic switching が API keys 間の選択、または API-key mode と unauthenticated/logged-out state の切り替えを意味する場合。

---

#### Option C — Go 管理の `auth.json` snapshot switching: **可能だが、それ単体では live/reliable ではない**

Go プロセスは `CODEX_HOME/auth.json` の profile snapshots を維持し、その 1 つを `0600` permissions で `auth.json` に復元できる。

しかし、これは次の理由で弱い:

- 稼働中の Codex processes は auth を cache する。
- Codex は `AuthManager::reload()` まで外部ファイル変更を観測しないと明示している。
- unauthorized recovery は異なる account id の reload を拒否する。
- keyring/auto storage は `auth.json` を bypass する可能性がある。
- logout/replacement login は managed ChatGPT tokens を revoke し得る。

**これを使うべき場合:** 切り替えが Codex 起動前に起きる、または profile 復元後に Codex process を restart できる場合。

**こう扱ってはいけない:** 稼働中の app-server session の安全な live account switch。

---

#### Option D — Go から `codex login` / `codex logout` を shell out する: **可能だが、複数アカウントの動的切り替えには不向き**

CLI paths は動作するが、1 つの active login を中心に設計されている:

- `codex login --with-api-key` は API-key auth を書き込む。
- ChatGPT browser/device login は managed OAuth tokens を永続化する。
- `codex logout` は auth を revoke/clear する。

これは、logout と replacement login が保持したかった tokens を revoke し得るため、複数 ChatGPT profiles の dynamic account switcher としては適していない。

---

## Go での実装境界

### Go が安全に実装できること

**証拠に基づく yes:**

1. 次を送信する JSON-RPC app-server client:
   - `account/read`
   - `account/login/start`
   - `account/logout`
   - `account/login/cancel`

2. 次を使う host-managed ChatGPT token switcher:
   - `type: "chatgptAuthTokens"`
   - `account/chatgptAuthTokens/refresh`

3. 次による API-key switching:
   - `type: "apiKey"`

4. Codex が file storage を使う場合に complete な `auth.json` snapshot を復元する pre-launch profile switching。

### Go が仮定すべきでないこと

**不明 / 現在の証拠では未サポート:**

1. この repo に、これらの auth methods 向けの安定した public Go SDK があることを示す証拠はない。
2. login/logout なしに「今すぐ disk から auth を reload する」ことを意味する public app-server method は見当たらない。
3. Codex 自体に multi-profile auth registry は存在しない。
4. `chatgptAuthTokens` が安定した public API であることを示す証拠はない。protocol は experimental/internal と記している。
5. Go から keyring を直接操作することが supported または portable であるという証拠はない。

---

## 直接回答

**はい、Go で dynamic switching を実装することは可能である。ただし、堅牢な live-switching path は「Codex の実行中に `auth.json` を編集する」ことではない。**

最もよくサポートされた Go design は次の通り:

1. JSON-RPC transport 経由で Codex app-server に接続する。
2. API-key switching には `type: "apiKey"` の `account/login/start` を使う。
3. host-managed ChatGPT account switching には `type: "chatgptAuthTokens"` の `account/login/start` を使う。
4. `account/chatgptAuthTokens/refresh` の server-request handling を実装する。
5. `account/logout` は active auth をクリアするためだけに使い、profile-preserving switch primitive としては使わない。

file-snapshot-based な Go tool の安全な境界は次の通り:

- snapshots は**プロセス起動前**、または Codex を停止/再起動した後に切り替える。
- `0600` permissions を保持する。
- keyring/auto storage を考慮する。
- ChatGPT OAuth tokens を保持する意図がある場合は `codex logout` を避ける。

---

## 不明点 / 限界

- このターンでは外部の Go repository は調査していない。Go feasibility conclusion は Codex の Rust app-server protocol と auth implementation に基づく。
- 現在の repository evidence では、OpenAI が `chatgptAuthTokens` を external consumers 向けに stable と見なしているかは確定できない。protocol comment は明示的に experimental/internal と記している。
- repository は public な「disk から auth を reload する」JSON-RPC method を公開していないため、direct file-based live switching は証拠上 unsupported のままである。
- ChatGPT access tokens の token acquisition は、ここで証明された Go feasibility の範囲外である。Codex は host-provided tokens を consume できることを証明しているのであって、Go が別の auth integration なしにそれらを取得できることを証明しているわけではない。

---

## 確信度スコア

- **Performance:** 0.95
  app-server 経由の dynamic switching は安価である。JSON-RPC request、in-memory/ephemeral auth update、`AuthManager` reload で済む。主なコストは Codex 外部での token acquisition/refresh である。

- **Scalability:** 0.93
  protocol は account-auth operations を global に serialize しており、単一 active auth state には適切である。1 つの Codex process 内で複数 simultaneous active accounts には scale しないが、単一 active account の switching には scale する。

- **Reliability:** 0.91
  app-server-mediated switching は Codex 自身の reload と notification paths を使うため信頼性が高い。file-based switching は信頼性が低いため、live switching には使わない。

- **Cost-effectiveness:** 0.96
  Go JSON-RPC implementation は、Codex Rust auth internals を変更したり、別の keyring/file mutation layer を構築したりするより安価で侵襲が少ない。
