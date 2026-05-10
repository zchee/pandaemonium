package codexappserver

//go:generate go run ./internal/cmd/generate-protocol-types -schema https://raw.githubusercontent.com/openai/codex/refs/tags/rust-v0.131.0-alpha.5/codex-rs/app-server-protocol/schema/json/codex_app_server_protocol.v2.schemas.json -out ./protocol/protocol_gen.go -package protocol
