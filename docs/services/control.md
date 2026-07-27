# Control Service (Port 9000)

> Message type range `0x00`–`0x0F`. Bidirectional H↔G.

The control service carries request/response RPC on port 9000. Every
frame on this port uses `msg_id` correlation (except `HEARTBEAT`, which
lives on its own port 9003).

## Message Types

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x00` | *(reserved)* | — | Invalid, must never appear on wire. |
| `0x01` | `PING` | H→G | *(none)* |
| `0x02` | `PONG` | G→H | *(none)* |
| `0x03` | `SHUTDOWN` | H→G | *(none)* |
| `0x04` | `SHUTDOWN_ACK` | G→H | *(none)* |
| `0x05` | `GET_STATUS` | H→G | *(none)* |
| `0x06` | `STATUS` | G→H | `string version`, `uint32 pid`, `bool shutting_down` |
| `0x07` | `HEARTBEAT` | G→H | `uint64 seq` — *(documented in [heartbeat.md](heartbeat.md))* |
| `0x08`–`0x0F` | *(reserved)* | — | — |

> `HEARTBEAT` (`0x07`) uses port 9003, not port 9000. See [heartbeat.md](heartbeat.md).

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

## GET_STATUS / STATUS

Query guest runtime status.

**STATUS** payload:

| Field | Encoding | Description |
|-------|----------|-------------|
| `version` | `string` | vhandler version string |
| `pid` | `uint32` | Guest PID 1 process ID |
| `shutting_down` | `bool` | `true` if shutdown sequence is in progress |

---

See also:
- [heartbeat.md](heartbeat.md) for the heartbeat service (port 9003).
- [examples/ping-pong.md](../examples/ping-pong.md) for PING/PONG wire example.
