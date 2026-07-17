# pkg/llm/opencode — knowledge base

Control wrapper for an OpenCode (`sst/opencode`, opencode.ai) server, presenting
the same API shape and ergonomics as `pkg/llm/codex`. The package is fully
self-contained (plan v4, user directive 2026-07-17): hand-written typed HTTP
client, hand-built SSE event stream, server-process launcher, and
wrapper-synthesized async turns. **`github.com/sst/opencode-sdk-go` is never
imported** — the SDK source in the module cache
(`$GOMODCACHE/github.com/sst/opencode-sdk-go@v0.19.2`) is a read-only shape
reference only, cross-checked against `testdata/openapi.json` and real-server
probes.

Plan of record: `.omc/plans/pkg-llm-opencode-control-wrapper.md` (v4).

## Shape ground truth — three sources, in priority order

1. **Live server** (`opencode serve`, currently 1.18.3): `GET /doc` returns the
   OpenAPI 3.1 document. Snapshot committed at `testdata/openapi.json`
   (captured 2026-07-17 from 1.18.3; 162 paths). The AC9 drift canary compares
   the wrapped subset against this snapshot on every gated real run.
2. **`testdata/openapi.json`** — the in-repo contract. Re-capture per opencode
   release: `opencode serve --hostname 127.0.0.1 --port 0`, parse the announce
   line, `curl $BASE/doc > testdata/openapi.json`, re-run the drift canary.
3. **SDK module cache** (v0.19.2) — historical cross-check only. It lags the
   server: see "Step 0b corrections" below for where 1.18.3 diverges from it.

## Step 0b corrections (live 1.18.3 vs the older SDK-derived notes)

Verified 2026-07-17 against a real 1.18.3 server; the live server wins:

- **`POST /session/{id}/fork` exists** (body `{messageID?}` → `Session`).
  Earlier SDK-derived note ("no fork endpoint; use POST /session with
  parentID") is obsolete. `SessionFork` uses the real endpoint.
- **`POST /session/{id}/prompt_async` exists** (same body as
  `/session/{id}/message`; 200 response body is undocumented in the OpenAPI
  doc). v1 still synthesizes async turns from the blocking prompt (see
  concurrency contract) because the dual-source terminal signal depends on the
  blocking response; revisit as follow-up F7.
- **`/session/{id}/shell` returns `{info: Message, parts: []Part}`**, not a
  bare `AssistantMessage` (the SDK wrapped an older shape).
- **`/session/{id}/command`**: `model` is a **string** (not
  `{providerID,modelID}`), `arguments` + `command` required; returns
  `{info: AssistantMessage, parts}`.
- **`PATCH /session/{id}` supports `time.archived`** — archive genuinely
  exists in 1.18.3. v1 scope keeps Share/Unshare only (plan divergence 4);
  adding `SetArchived` is an easy additive follow-up.
- **Event union is 89 types** (SDK-era notes said 19). Load-bearing types all
  exist: `server.connected` (props `{}`), `session.idle` (`{sessionID}`),
  `session.error` (`{sessionID?, error?}` — **both optional**, hence the
  `unattributed_session_error` counter), `message.updated` (`{sessionID,
  info}`), `message.part.updated` (`{sessionID, part, time}`),
  `message.part.delta` (`{sessionID, messageID, partID, field, delta}`).
- **`permission.updated` no longer exists.** 1.18.3 emits `permission.asked`
  with properties `{id ^per, sessionID, permission, patterns, metadata,
  always, tool{messageID, callID}}`, plus a parallel `permission.v2.asked`
  (`{id, sessionID, action, resources, save?, metadata?, source?}`). Replies:
  legacy `POST /session/{id}/permissions/{permissionID}`
  `{response: once|always|reject}` → `bool` (verified working), or v2
  `POST /permission/{requestID}/reply` `{reply, message?}`.
- **`server.heartbeat` is emitted on `/event` but is NOT in the OpenAPI Event
  union** — raw passthrough for unknown event types is mandatory, not
  optional.
- `AssistantMessage.error` union has **8 variants** in 1.18.3:
  `ProviderAuthError | UnknownError | MessageOutputLengthError |
  MessageAbortedError | StructuredOutputError | ContextOverflowError |
  ContentFilterError | APIError`.
- `Part` union has 12 types: text, subtask, reasoning, file, tool, step-start,
  step-finish, snapshot, patch, agent, retry, compaction. Part inputs:
  `TextPartInput{type,text}`, `FilePartInput{type,mime,url[,filename,source]}`,
  `AgentPartInput{type,name}`, `SubtaskPartInput{type,prompt,description,agent}`.
- **HTTP error envelope**: `{"name": string, "data": {"message": string,
  ...}}`. Observed: 404 → `{"name":"NotFoundError"}`, 400 →
  `{"name":"BadRequest","data":{...,"kind":"Payload"}}`, 500 →
  `{"name":"UnknownError","data":{"message":...,"ref":"err_..."}}`. A bogus
  provider/model in a prompt surfaces as HTTP 500 UnknownError, not a turn
  error.
- Every session endpoint takes optional `directory` / `workspace` query
  params (multi-project servers; deferred, follow-up F6).
- Prompt body (`POST /session/{id}/message`): `parts` required;
  optional `messageID ^msg`, `model{providerID,modelID}`, `agent`, `noReply`,
  `tools map[string]bool`, `format` (OutputFormat), `system`, `variant`.
- `GET /session/{id}/message` returns `[]{info: Message, parts: []Part}`;
  `GET /config/providers` → `{providers: []Provider, default:
  map[string]string}`; `GET /global/health` → `{healthy: true, version}`
  (all confirmed live).

## Step 0c probe answers (real 1.18.3, 2026-07-17)

- **(a) Permissions:** with no user config (`OPENCODE_CONFIG=/dev/null`), the
  default server config **auto-allows** tools — no permission events fire. A
  per-session ruleset (`POST /session` body
  `{"permission":[{"permission":"bash","pattern":"*","action":"ask"}]}`)
  makes the server emit `permission.asked` and **pause the tool call**; the
  blocking sync prompt POST hangs until
  `POST /session/{id}/permissions/{permID}` `{"response":"once"}` (returns
  `true`), after which the turn resumes and completes. This empirically
  confirms the sync-path deadlock the client-lifetime permission consumer
  prevents (plan locked decision 6). `PermissionAction` = `allow|deny|ask`;
  reply responses = `once|always|reject`.
- **(b) Steer via `noReply`:** a `noReply` prompt POSTed mid-turn returns
  immediately (HTTP 200 in ~1s with `{info: UserMessage, parts}`) but does
  **not** influence the active generation — it becomes visible only to the
  next turn. Mid-turn steering is NOT viable via `noReply`; `Steer` stays out
  of v1 (locked decision 5; F5 answered: no).
- **(c) `/event` subscribers:** multiple concurrent subscribers are served
  without issue; each connection gets its own `server.connected` first event.
  Frames observed are single-line `data: {json}` records (the event id rides
  inside the JSON as `id ^evt_`; no SSE `id:`/`event:` fields are used).
  Periodic `server.heartbeat` data events serve as keepalives.

## Real server operational facts (1.18.3)

- `opencode serve --hostname 127.0.0.1 --port 0` announces
  `opencode server listening on http://127.0.0.1:PORT` on stdout. **A config
  file's `server.port` overrides the `--port 0` flag default** (a user config
  with `server.port: 4096` makes `--port 0` bind 4096) — the announce line is
  the ONLY reliable port source. Hermetic tests must isolate config
  (`OPENCODE_CONFIG=/dev/null` works).
- When `OPENCODE_SERVER_PASSWORD` is unset the server prints
  `Warning: OPENCODE_SERVER_PASSWORD is not set; server is unsecured.` before
  the announce line — the announce parser must scan lines, not read one line.
- There is no password flag: `OPENCODE_SERVER_PASSWORD` env only (never argv —
  AC8). With a password set, HTTP basic auth is enforced on every route
  including `/event`; 401 responses carry
  `www-authenticate: Basic realm="Secure Area"` and an empty body.
- **The basic-auth username IS verified: it must be `opencode`.**
  (`opencode:pass` → 200; `otheruser:pass` → 401. Earlier SDK-era note
  claiming the username is not verified is wrong for 1.18.3.)
- `GET /global/health` requires auth when a password is set; returns
  `{"healthy":true,"version":"1.18.3"}`.

## Concurrency contract (mirrors pkg/llm/codex/doc.go)

- `Client`, `Opencode`, and `Session` are safe for concurrent use.
- **One shared client-lifetime SSE bus** (locked decision 4): exactly one
  `GET /event` connection, owned by `Client`, fanned out by the router. The
  facade constructors dial eagerly and fail fast; per-turn subscriptions are
  router registrations, never new connections. The bus owns bounded reconnect;
  on reconnect every registered consumer receives a gap notification
  (`/event` has no resume cursor).
- Each `Session` supports at most one active turn at a time (`Session.Turn`
  returns an error while another turn is in flight). A second prompt POSTed to
  the same session mid-turn does not steer or join the active turn (probe (b))
  — the wrapper makes the one-active-turn rule explicit instead of letting
  queued prompts cross-contaminate the session-scoped event filter.
- Each `TurnHandle` may have at most one active stream consumer
  (`Stream`/`Run`); a second concurrent consumer errors immediately.
- Turn consumers are registered on the live bus **before** the prompt POST is
  issued, so a stream observes every event of its own turn.
- Terminal signal is dual-source: the blocking prompt goroutine's return is
  necessary and authoritative; `session.idle`/`session.error` events enrich
  it within a bounded drain window (see the truth table in plan §6 Step 6).
- There is no `Steer` (probe (b)); there is no server-side turn resource —
  `TurnHandle` is a wrapper construct over the blocking prompt.

## Anti-patterns (hard rules)

- **Never** import `github.com/sst/opencode-sdk-go`. Shape questions are
  answered by `testdata/openapi.json` and real-server probes; the module-cache
  SDK source is tertiary.
- **Never** place `Config.Password`/`RemoteConfig.Password` in argv, URLs,
  error strings, or logs. Auth is header-only (`opencode:<password>`); the
  spawned server receives the password via `OPENCODE_SERVER_PASSWORD` env.
- Never claim turn completion from SSE events alone, and never let HTTP
  success outrank an explicit `session.error` for our session: the terminal
  signal is the prompt goroutine outcome joined with session-scoped
  idle/error events under a bounded drain (pre-mortem #1).
- Always drain child stdout/stderr from the moment of spawn (os/exec pipe
  deadlock, pre-mortem #2); the announce parser is a consumer of the drain,
  not a one-shot read.
- Do not emulate codex semantics OpenCode lacks (pairing, approval callback,
  mid-turn steer). Name methods for real behavior; divergences live in
  doc.go.
- Decode tolerantly: unknown JSON fields are ignored, unknown event types pass
  through raw (`server.heartbeat` proves the union is open).
