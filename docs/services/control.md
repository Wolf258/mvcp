# Control Service (Port 9000)

> Message type range `0x00`–`0x0F`. Bidirectional H↔G.
> Part of the RPC layer. See [rpc.md](rpc.md) for the request/response contract.

The control service carries request/response RPC on port 9000 via the
RPC layer. Every control message uses `msg_id` correlation.

## Message Types

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x00` | *(reserved)* | — | Invalid, must never appear on wire. |
| `0x01` | `PING` | H→G | *(none)* |
| `0x02` | `PONG` | G→H | *(none)* |
| `0x03` | `SHUTDOWN` | H→G | *(none)* |
| `0x04` | `SHUTDOWN_ACK` | G→H | *(none)* |
| `0x07` | `HEARTBEAT` | G→H | `uint64 seq` — *(documented in [heartbeat.md](heartbeat.md))* |
| `0x05`–`0x06` | *(reserved)* | — | — |
| `0x08`–`0x0F` | *(reserved)* | — | — |

> `GET_STATUS` (`0x05`) and `STATUS` (`0x06`) use port 9003, not port 9000. See [status.md](status.md).

## PING / PONG

Liveness check. Host sends `PING`, guest responds with `PONG`.

- `PING` payload: none
- `PONG` payload: none
- `PONG` carries `IS_RESPONSE` flag with matching `msg_id`

## SHUTDOWN / SHUTDOWN_ACK

Graceful VM shutdown sequence.

1. Host sends `SHUTDOWN`
2. Guest begins shutdown (reaps children, syncs filesystems)
3. Guest sends `SHUTDOWN_ACK`
4. Guest exits PID 1, kernel halts

Both messages have empty payload. `SHUTDOWN_ACK` carries `IS_RESPONSE`.

---

See also:
- [rpc.md](rpc.md) for the RPC layer contract (pipelining, streaming, timeouts, error handling).
- [status.md](status.md) for GET_STATUS/STATUS and heartbeat (port 9003).
- [examples/ping-pong.md](../examples/ping-pong.md) for PING/PONG wire example.
