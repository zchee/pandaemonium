# pkg/claude

Go port of [anthropics/claude-agent-sdk-python](https://github.com/anthropics/claude-sdk-python) for the pandaemonium module.

Requires the [claude CLI](https://docs.anthropic.com/en/docs/claude-code/cli-reference) on `PATH`.

## Quick start

### One-shot query

```go
import "github.com/zchee/pandaemonium/pkg/claude"

for msg, err := range claude.Query(ctx, "Say hello", nil) {
    if err != nil {
        log.Fatal(err)
    }
    if am, ok := msg.(claude.AssistantMessage); ok {
        for _, block := range am.Content {
            if tb, ok := block.(claude.TextBlock); ok {
                fmt.Println(tb.Text)
            }
        }
    }
    if _, ok := msg.(claude.ResultMessage); ok {
        break
    }
}
```

### Multi-turn conversation

```go
cli, err := claude.NewClient(ctx, &claude.Options{Model: "claude-opus-4-5"})
if err != nil {
    log.Fatal(err)
}
defer cli.Close()

for _, prompt := range []string{"Hello!", "Tell me a joke."} {
    if err := cli.Query(ctx, prompt); err != nil {
        log.Fatal(err)
    }
    for msg, err := range cli.ReceiveResponse(ctx) {
        if err != nil {
            log.Fatal(err)
        }
        fmt.Println(msg)
        if _, ok := msg.(claude.ResultMessage); ok {
            break
        }
    }
}
```

### Options

```go
opts := &claude.Options{
    Model:        "claude-opus-4-5",
    SystemPrompt: "You are a helpful assistant.",
    MaxTurns:     5,
    MaxBudgetUSD: 0.10,
    // In-process MCP server
    MCPServers: []claude.MCPServer{
        claude.NewSDKMCPServer("my-tools", "1.0.0", myTool),
    },
    // Permission hook
    CanUseTool: func(ctx context.Context, name string, _ jsontext.Value) (claude.PermissionDecision, error) {
        if name == "Bash" {
            return claude.PermissionDeny, nil
        }
        return claude.PermissionAllow, nil
    },
}
```

### Tool definition

```go
type GreetInput struct {
    Name string `json:"name"`
}

greetTool := claude.Tool("greet", "Greet someone by name", nil,
    func(ctx context.Context, in GreetInput) (claude.ToolResult, error) {
        return claude.ToolResult{Content: "Hello, " + in.Name + "!"}, nil
    },
)
```

### Session persistence

```go
store := claude.NewInMemorySessionStore()
opts := &claude.Options{SessionStore: store}
```

## Directory layout

```
pkg/claude/
├── api.go                  # NewClient constructor
├── client.go               # ClaudeSDKClient: Query, ReceiveResponse, Fork, Close
├── doc.go                  # Package-level documentation
├── errors.go               # CLINotFoundError, CLIConnectionError, ProcessError
├── hooks.go                # dispatchHooks, applyCanUseTool, applyPermissions
├── mcp.go                  # Tool, NewSDKMCPServer, inProcessMCPServer
├── message_parser.go       # parseMessage — two-pass JSON discriminator
├── options.go              # Options, validate
├── plugins.go              # Plugin, PluginType
├── public_types.go         # Message, ContentBlock, HookEvent, PermissionDecision
├── query.go                # Package-level Query helper
├── session_store.go        # SessionStore interface + in-memory impl
├── setting_sources.go      # SettingSource, SettingSourceType
├── examples/               # Runnable examples (require real claude CLI)
├── internal/fakecli/       # Hermetic FakeCLI test transport
├── internal/version/       # Pinned dependency versions
└── testing/sessionstoreconformance/  # Reusable conformance harness
```

## Testing

```sh
# Unit tests (no real CLI required)
go test -race ./pkg/claude/...

# Integration tests (real claude binary required)
RUN_REAL_CLAUDE_TESTS=1 go test -race -timeout 120s ./pkg/claude/...
```

## License

Apache 2.0 — see [LICENSE](../../LICENSE).
