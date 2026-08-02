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

## Streaming and Head-of-Line Blocking

Once a streaming RPC call starts on port 9000 (exec stdout/stderr):

- **No other request can be sent** on the same connection until the
  stream ends (last frame with `IS_STREAM_MORE` cleared).
- This is an accepted tradeoff — avoiding frame-interleaving complexity.

For concurrent operations while a stream is active, open **multiple
connections** to port 9000.

## Multiple Connections to Port 9000

Multiple connections to port 9000 are allowed. Each connection is an
independent RPC stream with its own `msg_id` counter. This enables
concurrent EXEC calls from different host-side goroutines without
head-of-line blocking.

## One-Way Messages

Ports 9002 (events), 9003 (heartbeat), and 9004 (file transfer) use
MVCP directly without the RPC layer. Events and `XFER_INIT` carry
`WANT_ACK` (`0x04`) — the receiver responds with `MVCP_ACK` (`0xFB`)
to confirm dispatch. Heartbeat (`msg_id=0`) remains fire-and-forget
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
- [services/rpc.md](services/rpc.md) for the RPC layer concurrency contract (pipelining, head-of-line, timeouts).
- [services/execution.md](services/execution.md) for streaming EXEC semantics.
