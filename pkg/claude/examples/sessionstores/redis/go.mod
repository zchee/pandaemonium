module github.com/zchee/pandaemonium/pkg/claude/examples/sessionstores/redis

go 1.27

// replace points to the repository root so this example module can import
// pkg/claude without a published release.
replace github.com/zchee/pandaemonium => ../../../../..

require (
	github.com/go-json-experiment/json v0.0.0-20260520185125-572e7c383686
	github.com/redis/go-redis/v9 v9.7.3
	github.com/zchee/pandaemonium v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/modelcontextprotocol/go-sdk v1.6.0 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)
