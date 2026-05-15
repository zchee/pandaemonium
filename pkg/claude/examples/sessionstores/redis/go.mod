module github.com/zchee/pandaemonium/pkg/claude/examples/sessionstores/redis

go 1.26

require (
	github.com/redis/go-redis/v9 v9.7.3
	github.com/zchee/pandaemonium v0.0.0
)

// replace points to the repository root so this example module can import
// pkg/claude without a published release.
replace github.com/zchee/pandaemonium => ../../../../../..
