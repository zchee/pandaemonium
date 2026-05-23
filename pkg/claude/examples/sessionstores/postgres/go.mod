module github.com/zchee/pandaemonium/pkg/claude/examples/sessionstores/postgres

go 1.27

// replace points to the repository root so this example module can import
// pkg/claude without a published release.
replace github.com/zchee/pandaemonium => ../../../../..

require (
	github.com/go-json-experiment/json v0.0.0-20260520185125-572e7c383686
	github.com/jackc/pgx/v5 v5.7.4
	github.com/zchee/pandaemonium v0.0.0
)

require (
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/modelcontextprotocol/go-sdk v1.6.0 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.21.0 // indirect
)
