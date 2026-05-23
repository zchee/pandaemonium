# pkg/claude

Go port of [anthropics/claude-agent-sdk-python](https://github.com/anthropics/claude-agent-sdk-python) for the pandaemonium module.

The package wraps the [claude CLI](https://docs.anthropic.com/en/docs/claude-code/cli-reference) with typed Go APIs for one-shot queries, interactive sessions, stream messages, control requests, hooks, in-process and external MCP servers, subagents, plugins, settings, sandboxing, skills, and session forking.

Requires the `claude` CLI on `PATH`, or set `Options.CLIPath`.

## Quick start

### One-shot query

`Query` returns an `iter.Seq2[Message, error]`. Breaking out of the range loop early closes the private client and reaps the subprocess.

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

`NewClient` validates options but starts the subprocess lazily on the first `Query` call. `ReceiveResponse` also returns an `iter.Seq2[Message, error]` and stops cleanly after a `ResultMessage`.

```go
cli, err := claude.NewClient(ctx, &claude.Options{Model: "claude-opus-4-7"})
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

`Options` is the main parity surface with the Python SDK and claude CLI. The zero value is valid.

```go
opts := &claude.Options{
    Model:        "claude-opus-4-7",
    SystemPrompt: claude.SystemPromptText("You are a helpful assistant."),
    MaxTurns:     5,
    MaxBudgetUSD: 0.10,

    // Base loaded tool set, distinct from permission gates.
    Tools:        []string{"Read", "Grep", "Bash"},
    ToolsPreset:  "", // when set, overrides Tools and emits --tools <preset>
    AllowedTools: []string{"Read", "Grep", "Skill(docs)"},
    Skills:       []string{"docs"}, // or claude.AllSkills()

    // In-process MCP server.
    MCPServers: []claude.MCPServer{
        claude.NewSDKMCPServer("my-tools", "1.0.0", myTool),
    },
    StrictMCPConfig: true,

    // Permission callback. Hooks run first; a hook deny skips CanUseTool.
    CanUseTool: func(ctx context.Context, name string, input jsontext.Value, info claude.ToolPermissionContext) (claude.PermissionResult, error) {
        if name == "Bash" {
            return claude.PermissionResultDeny{Message: "Bash is restricted"}, nil
        }
        return claude.PermissionResultAllow{}, nil
    },
}
```

Common option groups:

| Area | API |
|------|-----|
| CLI discovery and process | `CLIPath`, `Cwd`, `Env`, `Verbose`, `APIKeyHelper`, `ExtraArgs` |
| Streaming formats | `OutputFormat`, `InputFormat`, `IncludePartialMessages`, `IncludeHookEvents` |
| Models and budgets | `Model`, `FallbackModel`, `MaxBudgetUSD`, `TaskBudget`, `MaxTurns` |
| Structured output | `JSONSchema *jsonschema.Schema` emits `--json-schema <schema>` |
| System prompts | `SystemPromptText`, `SystemPromptFile`, `SystemPromptPreset` |
| Thinking | `ThinkingConfigAdaptive`, `ThinkingConfigEnabled`, `ThinkingConfigDisabled`, `Effort` |
| Permissions | `PermissionMode`, `AllowedTools`, `DisallowedTools`, `PermissionPromptToolName`, `CanUseTool`, `PermissionUpdate` |
| Tools and skills | `Tools`, `ToolsPreset`, `Skills`, `AllSkills()` |
| MCP | `NewSDKMCPServer`, `MCPStdioServerConfig`, `MCPSSEServerConfig`, `MCPHTTPServerConfig`, `StrictMCPConfig` |
| Agents and plugins | `AgentDefinition`, `Plugin` |
| Settings and sandbox | `SettingSources`, `Settings`, `SandboxSettings` |
| Sessions | `SessionID`, `Resume`, `ContinueConversation`, `ForkSession`, `SessionStore` |

## Tool definition

`Tool` adapts a typed Go handler into an in-process MCP tool. Pass a JSON schema when you want the CLI to validate tool inputs; pass `nil` to omit schema validation.

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

Use `ToolWithAnnotations` when MCP tool registration hints such as `ReadOnlyHint` or `DestructiveHint` matter. `ToolResult.RawContent` can return typed MCP content blocks (text, image, audio, resource links, embedded resources); when it is nil, `ToolResult.Content` is returned as a single text block.

## External MCP servers

External MCP servers satisfy the same `MCPServer` interface as in-process servers and are serialized into the launch `--mcp-config` payload.

```go
opts := &claude.Options{
    MCPServers: []claude.MCPServer{
        &claude.MCPStdioServerConfig{
            MCPName: "fs",
            Command: "mcp-fs",
            Args:    []string{"--root", "/srv"},
        },
        &claude.MCPSSEServerConfig{
            MCPName: "events",
            URL:     "https://example.com/sse",
        },
        &claude.MCPHTTPServerConfig{
            MCPName: "tools",
            URL:     "https://api.example.com/mcp",
        },
        claude.NewSDKMCPServer("local-tools", "1.0.0", greetTool),
    },
}
```

## Subagents, plugins, settings, and sandbox

Subagents are sent in the streaming initialize request, not as CLI flags. They can reference parent MCP servers by name or inline per-agent MCP server configs.

```go
opts := &claude.Options{
    Agents: []claude.AgentDefinition{
        {
            Name:           "reviewer",
            Description:    "Review code and docs changes.",
            SystemPrompt:   "Be precise and cite files.",
            AllowedTools:   []string{"Read", "Grep"},
            Skills:         []string{"code-review"},
            MCPServers:     []string{"local-tools"},
            Model:          "inherit",
            Effort:         claude.EffortLevelHigh,
            Memory:         claude.MemoryScopeProject,
            PermissionMode: claude.PermissionModePlan,
        },
    },
    Plugins: []claude.Plugin{
        {Name: "local-plugin", Path: "./.claude/plugins/local-plugin"},
    },
    SettingSources: []claude.SettingSource{
        claude.SettingSourceUser,
        claude.SettingSourceProject,
    },
    Sandbox: &claude.SandboxSettings{
        Enabled: true,
        Network: claude.SandboxNetworkConfig{
            AllowedDomains: []string{"docs.anthropic.com"},
        },
    },
}
```

## Session persistence and forking

CLI-side session control uses `SessionID`, `Resume`, `ContinueConversation`, and `ForkSession`. Go-side `SessionStore` is separate: it is used by `ClaudeSDKClient.Fork` to snapshot message history into a new branch and is not sent to the CLI as a `--session-store` flag.

```go
store := claude.NewInMemorySessionStore()
cli, err := claude.NewClient(ctx, &claude.Options{SessionStore: store})
if err != nil {
    log.Fatal(err)
}
// After the parent has an active session and stored messages:
child, err := cli.Fork(ctx, assistantMessageID)
if err != nil {
    log.Fatal(err)
}
defer child.Close()
```

Reusable conformance tests for custom stores live in `pkg/claude/testing/sessionstoreconformance`.

## Control requests

A running `ClaudeSDKClient` also exposes control-protocol helpers:

- `Interrupt`
- `SetModel`
- `SetPermissionMode`
- `GetServerInfo`
- `GetMCPStatus` / `GetMCPStatusTyped`
- `GetContextUsage`

These require a live subprocess; call them after the first `Query` starts the session.

## Stream message and content types

`Message` is a sealed interface. Known stream messages include `AssistantMessage`, `UserMessage`, `SystemMessage`, `ResultMessage`, task messages (`TaskStartedMessage`, `TaskProgressMessage`, `TaskNotificationMessage`), `HookEventMessage`, `StreamEvent` for partial Anthropic stream events, and `RateLimitEvent`. Unknown future CLI message types are preserved as raw JSON where possible instead of being dropped.

Content blocks are also typed: `TextBlock`, `ToolUseBlock`, `ToolResultBlock`, `ThinkingBlock`, `ServerToolUseBlock`, and `ServerToolResultBlock`. Use the `Raw` fields on public message/content structs when the CLI adds fields before the Go SDK promotes them.

## Directory layout

```text
pkg/claude/
├── api.go                       # NewClient constructor
├── cli_discovery.go             # claude binary discovery
├── client.go                    # ClaudeSDKClient: Query, ReceiveResponse, Fork, Close
├── client_control.go            # typed control-request helpers
├── client_env.go                # subprocess environment construction
├── client_launch_args.go        # claude CLI argument mapping
├── client_transport.go          # stdio JSON transport
├── control_protocol.go          # initialize/control/mcp callback routing
├── doc.go                       # package-level documentation
├── errors.go                    # CLI and message parsing error types
├── hooks.go                     # Hook and CanUseTool dispatch
├── mcp.go                       # in-process MCP tools and servers
├── mcp_external.go              # stdio, SSE, and HTTP MCP server configs
├── mcp_status.go                # typed MCP status responses
├── message_parser.go            # stream-JSON message discriminator
├── options.go                   # Options, cloning, validation, skills coupling
├── permission_result.go         # CanUseTool result and context types
├── permission_update.go         # permission update wire variants
├── plugins.go                   # Plugin config
├── public_types.go              # Message, content block, hook, and permission types
├── query.go                     # package-level one-shot Query helper
├── sandbox.go                   # SandboxSettings and --settings merge logic
├── session_store.go             # SessionStore interface + in-memory implementation
├── setting_sources.go           # SettingSource constants
├── system_prompt.go             # SystemPromptSource variants
├── task_budget.go               # TaskBudget
├── thinking.go                  # ThinkingConfig and EffortLevel
├── examples/                    # Runnable examples (require real claude CLI)
├── internal/cmd/capture-fakecli-fixtures/  # real-CLI fixture capture tool
├── internal/fakecli/            # Hermetic FakeCLI test transport
├── internal/version/            # Pinned dependency versions
├── testdata/stream/             # captured stream-JSON fixtures
└── testing/sessionstoreconformance/  # Reusable conformance harness
```

## Examples

Runnable examples live under `pkg/claude/examples`. They require a real `claude` binary and `RUN_REAL_CLAUDE_TESTS=1`; without that opt-in, examples exit 0 so hermetic tests stay offline.

```sh
RUN_REAL_CLAUDE_TESTS=1 go run ./pkg/claude/examples/quick_start
```

Hermetic parity coverage for examples lives in `pkg/claude/examples_parity_test.go`:

```sh
go test -v -race -count=1 -shuffle=on -run TestExampleParity ./pkg/claude
```

## Testing

```sh
# Unit tests (no real CLI required)
go test -v -race -count=1 -shuffle=on ./pkg/claude

# Package tree, including examples that self-skip without RUN_REAL_CLAUDE_TESTS
go test -v -race -count=1 -shuffle=on ./pkg/claude/...

# Integration tests (real claude binary required)
RUN_REAL_CLAUDE_TESTS=1 go test -v -race -count=1 -shuffle=on -timeout 120s ./pkg/claude/...
```

## License

Apache 2.0 — see [LICENSE](../../LICENSE).
