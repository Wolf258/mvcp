# 05 — Concurrency Model

## Request/Response Pipelining

MVCP supports pipelined request/response on a single connection via
`msg_id` correlation:

- Host sends requests with incrementing `msg_id` values (1, 2, 3, …).
- Guest processes in order and sends responses with matching `msg_id`
  and `IS_RESPONSE` flag.
- The stream is ordered — responses arrive in request order on the same
  connection.
- Responses are correlated by `msg_id`, not by position — enabling
  future out-of-order delivery if needed.

## Streaming and Head-of-Line Blocking

Once a streaming message starts (file transfer, exec stdout/stderr):

- **No other request can be sent** on the same connection until the
  stream ends (last frame with `IS_STREAM_MORE` cleared).
- This is an accepted tradeoff — avoiding frame-interleaving complexity.

For concurrent operations while a stream is active, open **multiple
connections** to the same port (see below).

## Multiple Connections to Port 9000

Multiple connections to port 9000 are allowed. Each connection is an
independent request/response stream. This enables concurrent EXEC and
FILE_EXPORT from different host-side goroutines without head-of-line
blocking.

Example: start a long-running file export on connection A, while
sending a STAT request on connection B.

## One-Way Messages

Ports 9002 (events) and 9003 (heartbeat) use `msg_id = 0` for all
frames — they are fire-and-forget with no response correlation.

## Connection Lifecycle

| Event | Behavior |
|-------|----------|
| Connection accepted | Guest sends handshake, host validates |
| Host disconnects | Guest closes connection, cleans up any in-flight state |
| Guest disconnects | Host detects broken connection, marks VM accordingly |
| Frame decode error | Receiver sends `ERROR` frame with `BAD_PAYLOAD` code |

---

See also:
- [01-transport.md](01-transport.md) for the vsock connection model and multiple-connection semantics.
- [02-wire-format.md](02-wire-format.md) for `msg_id` semantics and `IS_STREAM_MORE` flag details.
- [services/execution.md](services/execution.md) for streaming EXEC semantics.
- [services/file-transfer.md](services/file-transfer.md) for chunked streaming semantics.
