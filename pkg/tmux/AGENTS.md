# pkg/tmux KNOWLEDGE BASE

## OVERVIEW

Persistent tmux control-mode client. It owns one `tmux -C` subprocess, sends
newline-framed commands, parses `%begin` / `%end` / `%error` response blocks,
and exposes asynchronous `%` notifications.

## WHERE TO LOOK

| Task | Location | Notes |
| --- | --- | --- |
| Client lifecycle | `client.go`, `options.go` | subprocess start, close, stderr tail. |
| Command rendering | `command.go`, `flow.go` | safe `Command`, `RawArg`, `StringArg`. |
| Protocol parser | `parser.go`, `protocol.go` | `%begin`, `%end`, `%error`, `%output`. |
| Output decoding | `output.go`, `output_test.go` | tmux escapes and lossy text helpers. |
| Notifications | `notification.go`, `client_test.go` | bounded event channel and drops. |
| Examples | `examples/` | opt-in real tmux demos. |

## CONVENTIONS

- Use single `-C`, not double `-CC`; this package communicates over pipes, not
  a controlling terminal.
- A client must be given an explicit initial command or explicit session target.
  The zero option set is rejected to avoid attaching to a user's default tmux.
- Commands are serialized; there is no pipelining.
- If a pending command context is canceled, close the client. Late responses
  cannot be safely matched to future commands.
- `Client.Events` is bounded. When full, it drops the oldest observable event
  and increments `DroppedNotifications`.
- Pane output is byte-oriented and tmux-escaped; decode with typed helpers.
- Real tmux coverage stays behind `RUN_REAL_TMUX_TESTS=1` and isolated sockets.

## ANTI-PATTERNS

- Do not pass untrusted user data through `RawArg`.
- Do not reuse a client after a pending command times out or is canceled.
- Do not make ordinary tests attach to, kill, or mutate a user's tmux server.
- Do not treat `DroppedNotifications` as an exact audit log under concurrency.

## COMMANDS

```bash
go test -count=1 ./pkg/tmux/...
go test -race -count=1 -shuffle=on ./pkg/tmux/...
RUN_REAL_TMUX_TESTS=1 go test -v -race -count=1 -shuffle=on ./pkg/tmux/...
```
