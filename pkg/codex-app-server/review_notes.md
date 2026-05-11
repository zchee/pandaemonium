# codex-app-server review notes

Scope: task 5 docs/review lane.

## Routing semantics

- `Client.NextNotification` returns the next notification exactly as received
  from the transport.
- `TurnHandle.Stream` and `TurnHandle.Run` share the same client notification
  consumer guard, so only one active turn consumer may drain a `Client` at a
  time.
- Unknown notification methods remain available through the raw `Notification`
  value; they are not silently rewritten.

## Generated rename policy

- The generated root package uses prefixed compatibility names such as
  `ProtocolConfig` and `ProtocolThread` when the schema would otherwise collide
  with public SDK names.
- The generator tests already cover the collision case and reject accidental
  reintroduction of plain `Config` or `Thread` root types.

## Review findings

- `notification.go` still treats the registry as an explicit allowlist. Any new
  upstream method not added to `notificationDecoders` will remain raw and
  untyped until the registry is updated.
- Existing tests cover the registry round-trip, unknown payload preservation,
  turn-stream consumer release, and generated rename collision handling. No
  additional runtime fix was required for this lane.
