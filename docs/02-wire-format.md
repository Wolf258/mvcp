# 02 — Wire Format

## Transport Frame (all ports, all protocols)

Every frame after the handshake is a length-prefixed binary message.
The transport layer is shared across all ports and both protocols
(MVCP and VPP):

```
┌──────────┬─────────────────────────────┐
│ length   │ payload                     │
│ (4B BE)  │ (N bytes)                   │
└──────────┴─────────────────────────────┘

length = N    (uint32 big-endian, max 16 MB = 0x01000000)
```

| Field    | Size    | Description                                                  |
|----------|---------|--------------------------------------------------------------|
| `length` | 4 bytes | Total bytes in `payload`. Big-endian uint32. Max 16 MB.       |
| `payload`| N bytes | Opaque byte sequence — interpreted by the protocol layer.    |

### Parser (Go)

```go
var lenBuf [4]byte
io.ReadFull(conn, lenBuf[:])
length := binary.BigEndian.Uint32(lenBuf[:])
if length > 16*1024*1024 {
    // frame too large
}
buf := make([]byte, length)
io.ReadFull(conn, buf)
// buf is the transport payload — hand it to the protocol layer
```

This layout allows reading an entire frame in a single allocation.
`ReadFrame`/`WriteFrame` in `mvcp/protocol/frame.go` implement
this for both host and guest.

## MVCP Protocol Layer

On ports using MVCP (9000, 9002, 9003, 9004, 9005 under Shifty conventions), the
transport payload is structured as:

```
┌──────┬───────┬──────────┬──────────┐
│ type │ flags │ msg_id   │ body     │
│ (1B) │ (1B)  │ (4B BE)  │ (M)      │
└──────┴───────┴──────────┴──────────┘

payload = type(1) + flags(1) + msg_id(4) + body(M)
length  = 6 + M
```

| Field    | Size    | Description                                                                |
|----------|---------|----------------------------------------------------------------------------|
| `type`   | 1 byte  | Message type identifier (see type registry).                               |
| `flags`  | 1 byte  | Bitfield: `0x01` = `IS_RESPONSE`, `0x02` = `IS_STREAM_MORE`.              |
| `msg_id` | 4 bytes | Request/response correlation token. 0 for one-way messages. Big-endian.   |
| `body`   | M bytes | Type-specific binary encoding.                                             |

Port 9000 carries an **RPC layer** on top of MVCP that formalises
`msg_id` semantics for request/response, pipelining, and streaming.
See [services/rpc.md](services/rpc.md) for the full specification.

### MVCP Parser (Go)

```go
// buf is the transport payload (from ReadFrame)
typ    := buf[0]
flags  := buf[1]
msgID  := binary.BigEndian.Uint32(buf[2:6])
body   := buf[6:]
```

## VPP Protocol Layer

On port 9001 (console), VPP uses a thinner inner header — just a
1-byte type — inside the same transport frame:

```
┌──────┬──────────┐
│ type │ body     │
│ (1B) │ (M)      │
└──────┴──────────┘

payload = type(1) + body(M)
length  = 1 + M
```

See [services/console.md](services/console.md) for the full VPP specification.

## Flags (MVCP only)

| Bit          | Name              | Meaning                                                                                            |
|--------------|-------------------|----------------------------------------------------------------------------------------------------|
| `0x01`       | `IS_RESPONSE`     | This frame is a response to a previous request (msg_id matches the request).                       |
| `0x02`       | `IS_STREAM_MORE`  | More frames follow for this logical message.                                                       |
| `0x04`       | `WANT_ACK`        | Sender expects an `MVCP_ACK` (type `0xFB`) frame with matching `msg_id` from the receiver.         |
| `0x08`–`0x80`| *(reserved)*      | Must be 0.                                                                                         |

Frames with `IS_STREAM_MORE` set are part of a logical message split
across multiple frames. The final frame clears the flag. `msg_id` is
constant across all frames of a single logical message.

An error is signaled by `IS_RESPONSE` + `type=0xFE` — no separate error
flag is needed.

## Application-Level Acknowledgment (WANT_ACK)

The vsock `SOCK_STREAM` guarantees transport-level reliability (ordered
delivery, retransmission, no duplicates), but it does **not** confirm
application-level processing. The `WANT_ACK` flag bridges this gap: the
sender sets it when it needs the receiver to confirm that the message
was received and dispatched.

### MVCP_ACK (`0xFB`)

When a receiver processes a frame with `WANT_ACK` set, it MUST respond
with an `MVCP_ACK` frame:

```
┌──────┬───────┬──────────┬──────────────┐
│ type │ flags │ msg_id   │ body         │
│ 0xFB │ 0x01  │ (match)  │ ack payload  │
└──────┴───────┴──────────┴──────────────┘
```

| Field  | Value                                              |
|--------|----------------------------------------------------|
| `type` | `0xFB`                                             |
| `flags`| `0x01` (`IS_RESPONSE`)                              |
| `msg_id`| Matches the original message's `msg_id`            |
| `body` | `uint8 status` + `string error_msg` (empty if ok)  |

`status` values:

| Status | Meaning                                                        |
|--------|----------------------------------------------------------------|
| `0x00` | OK — message received and dispatched to the handler.           |
| `0x01` | Generic error — details in `error_msg`.                        |
| `0x02` | Resource exhausted (ring buffer full, no memory).              |
| `0x03` | Handler not registered — no handler exists for this type.      |

### Semantics

`MVCP_ACK` confirms **dispatch**, not **execution**:

- For an `EXEC` command: `MVCP_ACK` means "command received, process
  spawned" — the result (`EXEC_RESULT`) arrives later.
- For an `EVENT_READY`: `MVCP_ACK` means "event queued/enqueued" — the
  host has accepted the notification.
- For `XFER_INIT`: `MVCP_ACK` means "ready to receive chunks."

The ack is a fire-and-forget confirmation — the sender does **not** wait
for the ack before sending subsequent messages (except in file transfer
INIT, where the sender must wait before streaming chunks).

### What Does Not Use WANT_ACK

- **Streaming chunks** with `IS_STREAM_MORE` set — the final response
  frame serves as the cumulative ack.
- **Console (VPP)** — interactive terminal I/O on port 9001.
- **Heartbeat** — periodic liveness signal on port 9003 (`msg_id=0`).
- **Messages that already carry `IS_RESPONSE`** — the response is
  implicitly the ack.

### Wire Example

**MVCP_ACK** (receiver → sender, 14 bytes on wire):

```
 length: 0x00_00_00_0E   (14 = 6 header + 8 body)
   type: 0xFB             (MVCP_ACK)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches the acknowledged message)
   body:
    uint8 0x00             → OK
    string ""              → empty (no error)
```

## msg_id Semantics (MVCP only)

- **Request:** sender allocates a non-zero `msg_id`. Responses echo the
  same id with `IS_RESPONSE` set.
- **One-way message with ack:** events (`0x80`–`0x8F`) and `XFER_INIT`
  carry a non-zero `msg_id` with `WANT_ACK`. The receiver echoes the
  `msg_id` in the `MVCP_ACK` response.
- **One-way message without ack:** heartbeat uses `msg_id = 0`.
- **Streaming:** all chunks of a message share the same `msg_id`.
  The first frame carries the request type; subsequent frames carry the
  chunk type. The last frame clears `IS_STREAM_MORE`.
- **Monotonic per connection:** `msg_id` starts at 1 on a new connection
  and increments. 32 bits → ~4 billion messages before wrap; resets to 1
  after wrap or on new connection.

## Primitive Encodings

All multi-byte integers are **big-endian**.

| Type      | Wire encoding                                    |
|-----------|--------------------------------------------------|
| `uint8`   | 1 byte                                           |
| `uint16`  | 2 bytes, big-endian                              |
| `uint32`  | 4 bytes, big-endian                              |
| `uint64`  | 8 bytes, big-endian                              |
| `int32`   | 4 bytes, big-endian (two's complement)           |
| `int64`   | 8 bytes, big-endian (two's complement)           |
| `bool`    | 1 byte (`0x00` = false, `0x01` = true)           |
| `string`  | `uint16` length prefix + UTF-8 bytes             |
| `bytes`   | `uint32` length prefix + raw bytes               |
| `map[k]v` | `uint16` entry count + N × (encoded k, encoded v)|

Shared by both MVCP and VPP — implemented once in
`mvcp/protocol/encode.go` and `mvcp/protocol/decode.go`.

## Encoding Functions (Go — `mvcp/protocol/encode.go`)

```go
func WriteUint8(w io.Writer, v uint8) error
func WriteUint16(w io.Writer, v uint16) error
func WriteUint32(w io.Writer, v uint32) error
func WriteUint64(w io.Writer, v uint64) error
func WriteInt32(w io.Writer, v int32) error
func WriteInt64(w io.Writer, v int64) error
func WriteBool(w io.Writer, v bool) error
func WriteString(w io.Writer, s string) error
func WriteBytes(w io.Writer, b []byte) error
func WriteStringMap(w io.Writer, m map[string]string) error
```

## Decoding Functions (Go — `mvcp/protocol/decode.go`)

```go
func ReadUint8(r io.Reader) (uint8, error)
func ReadUint16(r io.Reader) (uint16, error)
func ReadUint32(r io.Reader) (uint32, error)
func ReadUint64(r io.Reader) (uint64, error)
func ReadInt32(r io.Reader) (int32, error)
func ReadInt64(r io.Reader) (int64, error)
func ReadBool(r io.Reader) (bool, error)
func ReadString(r io.Reader) (string, error)
func ReadBytes(r io.Reader) ([]byte, error)
func ReadStringMap(r io.Reader) (map[string]string, error)
```

---

See also:
- [01-transport.md](01-transport.md) for the vsock transport layer and connection handshake.
- [05-concurrency.md](05-concurrency.md) for the concurrency model (streaming, pipelining).
- [04-error-codes.md](04-error-codes.md) for the error envelope.
- [services/console.md](services/console.md) for the VPP wire format on port 9001.
- [examples/](../examples/) for wire-level examples of each message type.
