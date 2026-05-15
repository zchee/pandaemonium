module github.com/zchee/pandaemonium/pkg/claude/examples/sessionstores/postgres

go 1.26

require (
	github.com/jackc/pgx/v5 v5.7.4
	github.com/zchee/pandaemonium v0.0.0
)

// replace points to the repository root so this example module can import
// pkg/claude without a published release.
replace github.com/zchee/pandaemonium => ../../../../../..
