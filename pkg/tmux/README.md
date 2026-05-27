# pkg/tmux

`pkg/tmux` is a Go client for tmux control mode. It keeps one persistent
`tmux -C` subprocess alive, sends newline-framed tmux commands over stdin,
parses `%begin`/`%end`/`%error` response blocks from stdout, and exposes async
`%` notifications through a bounded event channel.

Use this package when you want to automate tmux without paying a `tmux(1)`
fork/exec cost for every command, or when you need control-mode notifications
such as pane output, format subscription changes, pause/continue events, and
client exit events.

## How to usage

### 1. Import the package

```go
import "github.com/zchee/pandaemonium/pkg/tmux"
```

The package shells out to a real `tmux` binary. By default, `tmux.New` resolves
`tmux` with `exec.LookPath("tmux")`; use `tmux.WithPath` when you need a pinned
binary path.

### 2. Start a control-mode client

`tmux.New` requires either an explicit initial command or an explicit session
name. This is intentional: the zero-value option set is rejected so a program
does not accidentally attach to a user's default tmux server.

Create or attach to a named session:

```go
ctx := context.Background()

client, err := tmux.New(
	ctx,
	tmux.WithSessionName("pandaemonium-demo"),
	tmux.WithCreateSession(true), // new-session -A -s pandaemonium-demo
)
if err != nil {
	return err
}
defer func() {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = client.Close(closeCtx)
}()
```

Attach to an existing session instead:

```go
client, err := tmux.New(
	ctx,
	tmux.WithSessionName("work"),
	tmux.WithCreateSession(false), // attach-session -t work
)
```

Start with a fully custom initial command:

```go
client, err := tmux.New(
	ctx,
	tmux.WithInitialCommand("new-session", "-A", "-s", "pandaemonium-demo"),
)
```

For isolated automation, prefer a private socket and config file:

```go
client, err := tmux.New(
	ctx,
	tmux.WithSocketPath(filepath.Join(tmpDir, "tmux.sock")),
	tmux.WithConfigFile(filepath.Join(tmpDir, "tmux.conf")),
	tmux.WithSessionName("isolated"),
	tmux.WithCreateSession(true),
)
```

### 3. Execute tmux commands

The simplest path is `Client.Exec`, which renders one safe command line and
waits for its response block.

```go
resp, err := client.Exec(
	ctx,
	tmux.DisplayMessage,
	tmux.RawArg("-p"),
	tmux.StringArg("#{session_name}"),
)
if err != nil {
	return err
}
fmt.Println(strings.Join(resp.Lines, "\n"))
```

Use these argument constructors deliberately:

- `tmux.StringArg(value)` quotes a normal argument using tmux-safe quoting.
- `tmux.RawArg(value)` passes a trusted tmux syntax fragment such as `-p`,
  `-F`, `Enter`, or another flag token.
- `tmux.Args(values...)` converts multiple values to `StringArg` arguments.

Only a small set of command constants is currently exported for common control
client operations:

- `tmux.DetachClient`
- `tmux.DisplayMessage`
- `tmux.ListPanes`
- `tmux.RefreshClient`

For other tmux commands, construct a command token explicitly:

```go
_, err := client.Exec(
	ctx,
	tmux.Command("send-keys"),
	tmux.RawArg("-t"),
	tmux.StringArg("%1"),
	tmux.StringArg("printf hello"),
	tmux.RawArg("Enter"),
)
```

When you already have a rendered command line, use `ExecLine` or `ExecRaw`:

```go
line := tmux.NewCommandLine(tmux.DisplayMessage, tmux.RawArg("-p"), tmux.StringArg("#{pane_id}"))
resp, err := client.ExecLine(ctx, line)

resp, err = client.ExecRaw(ctx, "display-message -p '#{pane_id}'")
```

`ExecRaw` still validates that the command line is non-empty, UTF-8, and has no
embedded newline.

### 4. Understand command execution semantics

Commands are serialized by the client. There is no pipelining: one command is
pending at a time, and the next command waits until the previous response block
has been matched.

If the context passed to `Exec`, `ExecLine`, or `ExecRaw` is canceled while the
client is waiting for tmux's response, the client closes itself. A late response
cannot be safely associated with a future command, so the conservative behavior
is to abort the persistent control-mode connection.

### 5. Read pane output and notifications

`Client.Events` returns a bounded channel of asynchronous tmux notifications.
The reader keeps draining tmux stdout even if your consumer falls behind: when
the event buffer is full, the oldest buffered notification that can be observed
is dropped and `DroppedNotifications` is incremented.

```go
go func() {
	for event := range client.Events() {
		if output, ok, err := event.Output(); ok {
			if err != nil {
				log.Printf("bad output notification: %v", err)
				continue
			}
			text := output.TextLossy()
			log.Printf("pane %s: %s", output.Pane, text)
		}
	}
}()
```

Useful typed notification helpers include:

- `Notification.Output` for `%output`
- `Notification.ExtendedOutput` for `%extended-output`
- `Notification.SubscriptionChanged` for `%subscription-changed`
- `Notification.Exit` for `%exit`
- `Notification.Pause` and `Notification.Continue` for flow-control events
- `Notification.Message` for `%message`

Pane output values are tmux-escaped. Use `tmux.DecodeOutputValue`,
`OutputNotification.Bytes`, `OutputNotification.Text`, or
`OutputNotification.TextLossy` to recover terminal bytes/text.

### 6. Use control-client helpers when possible

The package includes helpers for common `refresh-client` operations:

```go
_, err = client.RefreshClientSize(ctx, 120, 40)
_, err = client.SetClientFlags(ctx, tmux.ClientFlagNoOutput)
_, err = client.SetPauseAfter(ctx, 2*time.Second)
_, err = client.PausePane(ctx, tmux.PaneID("%1"))
_, err = client.ContinuePane(ctx, tmux.PaneID("%1"))
_, err = client.DisablePaneOutput(ctx, tmux.PaneID("%1"))
_, err = client.EnablePaneOutput(ctx, tmux.PaneID("%1"))
_, err = client.SubscribeFormat(ctx, "active-pane", tmux.SubscriptionAttachedSession, "#{pane_id}")
_, err = client.UnsubscribeFormat(ctx, "active-pane")
```

These helpers validate pane IDs, subscription names, dimensions, and
newline-sensitive fragments before sending commands to tmux.

### 7. Close the client

Always close the client. `Close` sends `detach-client` when the connection is
still open, closes stdin, waits for stdout/stderr drains, and waits for the tmux
subprocess. `WithShutdownTimeout` bounds graceful shutdown before the subprocess
is killed.

```go
closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
defer cancel()
if err := client.Close(closeCtx); err != nil {
	return err
}
```

`Close` is idempotent. `StderrTail` returns the retained tmux stderr tail for
error reporting.

## Parser-only usage

If you already own a PTY or another control-mode transport, use `Parser`
directly. This is also the correct lower-level path for consumers of `tmux -CC`,
where tmux may emit extra terminal DCS/ST framing around the control-mode
stream.

```go
var parser tmux.Parser
for _, line := range linesFromTransport {
	event, err := parser.Feed(line)
	if err != nil {
		return err
	}
	if event.Response != nil {
		fmt.Printf("response lines: %v\n", event.Response.Lines)
	}
	if event.Notification != nil {
		fmt.Printf("notification: %s\n", event.Notification.Kind)
	}
}
if err := parser.Close(); err != nil {
	return err
}
```

## Examples

Runnable examples live under `pkg/tmux/examples`:

- `01_control_mode_session` creates an isolated real tmux server, sends a
  command, lists panes, prints stable facts, and cleans up the server.
- `02_existing_session` attaches to an already-running tmux session and runs
  read-only commands against it.

Run the isolated example only when you explicitly want to exercise a real tmux
binary:

```sh
RUN_REAL_TMUX_TESTS=1 go run ./pkg/tmux/examples/01_control_mode_session
```

Run against an existing session:

```sh
PANDAEMONIUM_TMUX_SESSION=my-session \
  go run ./pkg/tmux/examples/02_existing_session
```

If the target session is not on tmux's default server, set one of:

```sh
PANDAEMONIUM_TMUX_SOCKET_PATH=/path/to/tmux.sock
PANDAEMONIUM_TMUX_SOCKET_NAME=my-socket-name
```

`PANDAEMONIUM_TMUX_CONFIG_FILE` can also point the existing-session example at a
specific tmux config file.

## Testing

Normal unit tests do not require a local tmux binary:

```sh
go test ./pkg/tmux/...
```

Real tmux integration tests are intentionally opt-in so they do not mutate a
user's normal tmux server by accident:

```sh
RUN_REAL_TMUX_TESTS=1 go test ./pkg/tmux/...
```

The real tests use isolated sockets/configuration where they create tmux state,
but you should still run them only in an environment where starting and killing
a temporary tmux server is acceptable.

## Operational notes

- Prefer `tmux.WithSocketPath` or `tmux.WithSocketName` for automation that
  should not share a user's default tmux server.
- Prefer `StringArg` for user/data values and reserve `RawArg` for trusted tmux
  syntax fragments.
- Keep event consumers draining `Events` when you enable pane output or
  subscriptions; watch `DroppedNotifications` as a backpressure signal.
- Use per-command contexts, but do not expect to reuse a client after a pending
  command context times out or is canceled.
- Use `StderrTail` when surfacing tmux startup or shutdown diagnostics.
