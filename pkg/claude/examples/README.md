# pkg/claude examples

Go ports of the [claude-agent-sdk-python](https://github.com/anthropics/claude-agent-sdk-python) example programs.

## Running an example

All examples require the real `claude` CLI and the environment variable
`RUN_REAL_CLAUDE_TESTS=1`. Without it, each program prints a short notice and
exits 0 so the hermetic `go test ./...` suite is unaffected.

```sh
RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/quick_start
```

## Hermetic parity tests

Every example has a corresponding test in `pkg/claude/examples_parity_test.go`
that exercises the same API surface against the in-process `FakeCLI` transport.
Run them without the real CLI:

```sh
go test -v -race -count=1 -shuffle=on -run TestExampleParity ./pkg/claude
```

## Examples index

| Example              | Python source              | Description                                         |
|----------------------|----------------------------|-----------------------------------------------------|
| quick_start          | quick_start.py             | One-shot `Query` with a single question             |
| streaming_mode       | streaming_mode.py          | Stream messages and print in real-time              |
| hooks                | hooks.py                   | `PreToolUse` hook blocks dangerous Bash commands    |
| mcp_calculator       | mcp_calculator.py          | In-process MCP server with arithmetic tools         |
| tool_permission_callback | tool_permission_callback.py | `CanUseTool` permission callback              |
| system_prompt        | system_prompt.py           | Custom system prompt via `Options.SystemPrompt`     |
| tools_option         | tools_option.py            | Restrict tool use via `Options.AllowedTools`        |
| setting_sources      | setting_sources.py         | External settings via `Options.SettingSources`      |
| agents               | agents.py                  | Programmatic subagents via `Options.Agents`         |
| filesystem_agents    | filesystem_agents.py       | Agents that operate on the filesystem               |
| plugin_example       | plugin_example.py          | Load a CLI plugin via `Options.Plugins`             |
| stderr_callback      | stderr_callback_example.py | Surface subprocess stderr via `ProcessError`        |
| max_budget_usd       | max_budget_usd.py          | Budget limit via `Options.MaxBudgetUSD`             |
| include_partial_messages | include_partial_messages.py | Stream partial messages as they arrive         |
| sessionstores/postgres | session_stores/postgres.py | SessionStore adapter for PostgreSQL (separate module) |
| sessionstores/redis  | session_stores/redis.py    | SessionStore adapter for Redis (separate module)    |
| sessionstores/s3     | session_stores/s3.py       | SessionStore adapter for AWS S3 (separate module)   |

## Skipped examples

The following upstream Python examples are **not ported** to Go:

- **streaming_mode_trio.py** — uses the `trio` async runtime, which has no Go
  equivalent; `context.Context` cancellation covers the same use-case.
- Any IPython-specific notebooks or REPL-only examples — these have no
  command-line equivalent to port.
