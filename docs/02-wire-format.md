# 02 — Wire Format

## Frame Layout

Every frame after the handshake is a length-prefixed binary message:

```
┌──────────┬──────┬───────┬──────────┬──────────┐
│ length   │ type │ flags │ msg_id   │ payload  │
│ (4B BE)  │ (1B) │ (1B)  │ (4B BE)  │ (N)      │
└──────────┴──────┴───────┴──────────┴──────────┘

length = 6 + len(payload) = 1(type) + 1(flags) + 4(msg_id) + N(payload)
```

| Field | Size | Description |
|-------|------|-------------|
| `length` | 4 bytes | Total bytes after the length field: `1 + 1 + 4 + len(payload)`. Big-endian uint32. Maximum: 16 MB (`0x01000000`). |
| `type` | 1 byte | Message type identifier (see type registry). |
| `flags` | 1 byte | Bitfield: `0x01` = `IS_RESPONSE`, `0x02` = `IS_STREAM_MORE`. |
| `msg_id` | 4 bytes | Request/response correlation token. 0 for one-way messages. Big-endian uint32. |
| `payload` | N bytes | Type-specific binary encoding. |

This layout allows reading an entire frame in a single allocation.

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
typ    := buf[0]
flags  := buf[1]
msgID  := binary.BigEndian.Uint32(buf[2:6])
payload := buf[6:]
```

## Flags

| Bit | Name | Meaning |
|-----|------|---------|
| `0x01` | `IS_RESPONSE` | This frame is a response to a previous request (msg_id matches the request). |
| `0x02` | `IS_STREAM_MORE` | More frames follow for this message (used by file chunks, exec streaming stdout/stderr). |
| `0x04`–`0x80` | *(reserved)* | Must be 0. |

Frames with `IS_STREAM_MORE` set are part of a logical message split
across multiple frames. The final frame clears the flag. `msg_id` is
constant across all frames of a single logical message.

An error is signaled by `IS_RESPONSE` + `type=0xFE` — no separate error
flag is needed.

## msg_id Semantics

- **Request:** sender allocates a non-zero `msg_id`. Responses echo the
  same id with `IS_RESPONSE` set.
- **One-way message** (heartbeat, event, notification): `msg_id = 0`.
- **Streaming:** all chunks of a message share the same `msg_id`.
  The first frame carries the request type; subsequent frames carry the
  chunk type. The last frame clears `IS_STREAM_MORE`.
- **Monotonic per connection:** `msg_id` starts at 1 on a new connection
  and increments. 32 bits → ~4 billion messages before wrap; resets to 1
  after wrap or on new connection.

## Primitive Encodings

All multi-byte integers are **big-endian**.

| Type | Wire encoding |
|------|---------------|
| `uint8` | 1 byte |
| `uint16` | 2 bytes, big-endian |
| `uint32` | 4 bytes, big-endian |
| `uint64` | 8 bytes, big-endian |
| `int32` | 4 bytes, big-endian (two's complement) |
| `int64` | 8 bytes, big-endian (two's complement) |
| `bool` | 1 byte (`0x00` = false, `0x01` = true) |
| `string` | `uint16` length prefix + UTF-8 bytes |
| `bytes` | `uint32` length prefix + raw bytes |
| `map[k]v` | `uint16` entry count + N × (encoded k, encoded v) |

## Encoding Functions (Go — `protocol/encode.go`)

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

## Decoding Functions (Go — `protocol/decode.go`)

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
- [01-transport.md](01-transport.md) for the vsock transport layer.
- [05-concurrency.md](05-concurrency.md) for the concurrency model (streaming, pipelining).
- [04-error-codes.md](04-error-codes.md) for the error envelope.
- [examples/](../examples/) for wire-level examples of each message type.
