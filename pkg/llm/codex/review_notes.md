# pkg/codex Unix WebSocket review notes

These notes are the review/documentation guardrails for implementing
`.omx/plans/plan-pkg-codex-websocket-over-unixstream-20260524.md`. They are
intended to stay useful after the transport, hermetic tests, real integration
lane, and verification commits are merged.

## Scope guardrails

- Treat `unix://` and `unix:///absolute/path` as websocket-over-Unix-stream
  endpoints, matching the app-server control socket contract. Do not implement
  raw newline-delimited JSON over Unix sockets.
- Keep `pkg/codex` request, notification, turn, and stream routing unchanged.
  Unix support should feed the existing websocket transport abstraction after
  the socket is upgraded.
- Keep the TCP websocket remote-host guard intact for `ws://` endpoints. Unix
  sockets are local transport endpoints and must not relax TCP safety checks.
- Keep launch identity, dial identity, and websocket handshake identity separate:
  pass the configured listen URL to `codex app-server --listen`, dial a resolved
  filesystem socket path, and use a synthetic websocket HTTP URL only for the
  Upgrade request.
- Do not log or return configured bearer token values in dial, validation, or
  readiness errors. Error messages may include the transport mode and resolved
  socket path.

## Review checklist

1. `ListenConfig.URL` classification recognizes stdio, TCP websocket, and Unix
   websocket transports without changing the stdio default.
2. `buildAppServerArgs` preserves the exact configured `--listen` value for
   Unix endpoints and rejects websocket auth fields there; Unix socket access is
   controlled by filesystem permissions, not bearer authentication.
3. Unix socket path resolution handles the default `unix://` control socket,
   absolute custom paths, and relative suffixes using one shared contract that
   launch and dial tests cover.
   - For the default `unix://` case, the hermetic helper process must resolve
     the same effective child `CODEX_HOME` as the launched app-server so launch
     and dial stay in sync.
4. The Unix dialer uses the standard library `net.Dialer` and an
   `http.Transport` supplied through `github.com/coder/websocket.DialOptions`;
   no additional dependency is needed.
5. Readiness failures for a missing socket or an app-server process that exits
   before listening return promptly with actionable context.
6. Hermetic coverage exercises initialize, request/response, server-initiated
   requests, and global notifications over a Unix socket.
7. Real integration coverage remains opt-in behind the existing real-test gate
   and skips cleanly when the installed `codex` binary lacks Unix listen support.
8. `Client.RequestRaw`, `Notify`, `NextNotification`, `TurnHandle.Stream`,
   `Thread.Run`, and `StreamThread.RunStream` keep their existing behavior and
   should not gain Unix-specific routing branches.

## Remote launch guardrails (LaunchRemoteAppServer)

- `--remote-control` is a hidden clap flag on codex-cli 0.140.0-alpha.13: absent
  from `--help` but accepted by both `codex-app-server` and `codex app-server`.
  Help enumeration is not an existence test; verify hidden flags by probing the
  standalone binary against a bogus-flag control, and probe the subcommand with
  `codex app-server --remote-control --listen stdio://` because the subcommand
  rejects `--version`.
- Never place the websocket bearer token in argv, error strings, logs, or the
  stderr tail. The attached codex child receives it only through the
  `CODEX_REMOTE_AUTH_TOKEN` environment variable paired with
  `--remote-auth-token-env CODEX_REMOTE_AUTH_TOKEN`.
- Never remove the default control socket
  (`$CODEX_HOME/app-server-control/app-server-control.sock`); it is shared with
  daemon tooling. Close() removes only explicit custom `unix://PATH` sockets,
  and only after the server child has exited.
- Keep ws:// listeners loopback-only unless
  `ListenConfig.AllowInsecureRemoteWebSocket` is set; unix sockets rely on
  filesystem permissions, not bearer auth.
- Upstream rejects websocket listen port 0, so `ReserveLoopbackPort` uses a
  bind-then-release pattern with an inherent TOCTOU window; a failed launch
  surfaces the stderr tail and retrying with a fresh port is the caller's loop.
- Reject unix socket filesystem paths longer than 103 bytes before spawn
  (darwin `sun_path` holds 104 bytes including the NUL terminator).
- Close() ordering is attached codex children first (interrupt, grace, kill),
  then the server child, then custom-socket cleanup. CodexCommand must inject
  `--remote=<endpoint>` exactly once and reject caller args that already carry
  a `--remote` flag.

## Verification bundle

Run the smallest focused checks first, then the package bundle:

```bash
go test -run 'WebSocket|Unix|LaunchArgs' ./pkg/codex ./pkg/codex/tests
go vet ./pkg/codex ./pkg/codex/tests
go test -v -race -count=1 -shuffle=on ./pkg/codex/...
git diff --check
```

When a compatible `codex` binary is available, run the opt-in real lane:

```bash
RUN_REAL_CODEX_TESTS=1 go test -v -race -count=1 -shuffle=on -run 'Unix|WebSocket' ./pkg/codex/...
```
