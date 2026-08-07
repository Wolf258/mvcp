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

On ports using MVCP (9000, 9002, 9003, 9004 under Shifty conventions), the
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
| `0x04`       | `FLAG_EXEC_STREAMING` | Request streaming output (EXEC). Was `WANT_ACK` in early drafts — removed in favour of `STARTED` (0xFA). |
| `0x08`–`0x80`| *(reserved)*      | Must be 0.                                                                                         |

Frames with `IS_STREAM_MORE` set are part of a logical message split
across multiple frames. The final frame clears the flag. `msg_id` is
constant across all frames of a single logical message.

An error is signaled by `IS_RESPONSE` + `type=0xFE` — no separate error
flag is needed.

## Dispatch Acknowledgment (STARTED)

`WANT_ACK` and `MVCP_ACK` (type `0xFB`) were removed in the final
protocol design. Their replacement is `STARTED` (type `0xFA`, see
[services/rpc.md](services/rpc.md)): the server sends a `STARTED` frame
to confirm it accepted the request for processing. Unlike the former
`MVCP_ACK` which confirmed mere dispatch, `STARTED` confirms the handler
has begun processing (process spawned, tool invoked, file opened).

- `STARTED` carries a single `bool stream` field (`EncodeStarted`).
- The frame uses `IS_RESPONSE` with the matching `msg_id`.
- If the handler cannot start, it sends `ERROR` (`0xFE`) instead.

See [services/rpc.md](services/rpc.md) for the full `STARTED` life-cycle
and [SPEC.md](../SPEC.md) for the design decision that removed
`WANT_ACK`.

## msg_id Semantics (MVCP only)

- **Request:** sender allocates a non-zero `msg_id`. Responses echo the
  same id with `IS_RESPONSE` set.
- **One-way message:** events (`0x80`–`0x8F`) use `msg_id = 0` (fire-and-forget).
  XFER_INIT uses a non-zero `msg_id` for correlation with XFER_DONE.
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
