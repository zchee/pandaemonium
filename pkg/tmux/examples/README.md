# pkg/tmux examples

These examples use the public `github.com/zchee/pandaemonium/pkg/tmux`
package from standalone Go programs.

## 01_control_mode_session

`01_control_mode_session` starts an isolated real tmux control-mode client,
executes a command, lists panes, prints stable facts, and cleans up the tmux
server it created. The example is opt-in so normal `go test ./pkg/tmux/...`
runs do not require a local tmux binary or mutate a user's tmux server.

```sh
RUN_REAL_TMUX_TESTS=1 go run ./pkg/tmux/examples/01_control_mode_session
```

## 02_existing_session

`02_existing_session` attaches to an already-running tmux session and runs
read-only control-mode commands against it. Set `PANDAEMONIUM_TMUX_SESSION` to
the existing session name. If the target is not on tmux's default server, also
set either `PANDAEMONIUM_TMUX_SOCKET_PATH` or `PANDAEMONIUM_TMUX_SOCKET_NAME`.

```sh
PANDAEMONIUM_TMUX_SESSION=my-session \
  go run ./pkg/tmux/examples/02_existing_session
```
