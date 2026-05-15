module github.com/zchee/pandaemonium/pkg/claude/examples/sessionstores/s3

go 1.26

require (
	github.com/aws/aws-sdk-go-v2/config v1.29.14
	github.com/aws/aws-sdk-go-v2/service/s3 v1.78.0
	github.com/zchee/pandaemonium v0.0.0
)

// replace points to the repository root so this example module can import
// pkg/claude without a published release.
replace github.com/zchee/pandaemonium => ../../../../../..
