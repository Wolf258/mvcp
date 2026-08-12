# 05 — Concurrency Model

## Request/Response Pipelining

MVCP supports pipelined request/response on a single connection via
`msg_id` correlation. The RPC layer (port 9000) formalises this:

- **Client** allocates non-zero `msg_id` values (1, 2, 3, …) and stores
  each in a pending map.
- **Server** processes messages and sends responses with matching
  `msg_id` and `IS_RESPONSE` flag.
- The stream is ordered — frames arrive in order on the same connection.
- Responses are correlated by `msg_id`, not by position — enabling
  out-of-order delivery if the server processes in parallel.

See [services/rpc.md](services/rpc.md) for the full pipelining contract.

## Streaming and Concurrency

A streaming RPC (exec stdout/stderr) does **not** block the connection.
Requests and streams are multiplexed on a single connection:

- Every frame is correlated by `msg_id`; an active stream and other
  in-flight requests — including further streams — coexist on the same
  connection.
- Each frame is a complete, self-delimiting unit (length-prefixed), so
  frames of different streams never merge. Implementations must write
  each frame atomically when multiple handlers write concurrently.
- Ordering is preserved **within** a stream: chunks of the same `msg_id`
  arrive in the order written (vsock is ordered), and the last chunk
  clears `IS_STREAM_MORE`. There is no ordering guarantee **across**
  requests or streams — responses are correlated by `msg_id`, not by
  position.
- Stream consumers must drain promptly; a slow consumer causes
  backpressure, never silent frame loss.

## Multiple Connections to Port 9000

Multiple connections to port 9000 are allowed, each with its own `msg_id`
counter. They are not required for concurrency — a single connection
already multiplexes concurrent requests and streams — but they provide
session isolation: each connection can be torn down or re-handshaken
independently (e.g. one connection per agent conversation).

## One-Way Messages

Ports 9002 (events), 9003 (status), and 9004 (file transfer) use
MVCP directly without the RPC layer. Events are fire-and-forget
(`msg_id=0`). File transfer uses `STARTED` (`0xFA`) for handler
confirmation. Heartbeat (`msg_id=0`) remains fire-and-forget
with no ack.

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
- [services/rpc.md](services/rpc.md) for the RPC layer concurrency contract (pipelining, streaming multiplexing, timeouts).
- [services/execution.md](services/execution.md) for streaming EXEC semantics.
