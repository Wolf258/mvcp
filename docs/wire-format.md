# Wire Format Reference

> Part of the MVCP specification. See [SPEC.md](../SPEC.md) for the
> complete protocol specification, message type registry, and adoption guide.

## Frame Format

Every MVCP frame is a length-prefixed binary message:

```
┌──────────────────┬──────┬───────┬──────────┬──────────────────────┐
│ length (4B BE)   │ type │ flags │ msg_id   │ payload (N bytes)    │
│                  │ (1B) │ (1B)  │ (4B BE)  │                      │
└──────────────────┴──────┴───────┴──────────┴──────────────────────┘
```

| Field | Size | Description |
|-------|------|-------------|
| `length` | 4 bytes | Total bytes after this field: `1 + 1 + 4 + len(payload)`. Big-endian uint32. |
| `type` | 1 byte | Message type identifier (see registry below). |
| `flags` | 1 byte | Bitfield: `0x01` = is_response, `0x02` = is_stream_more, `0x04` = is_error. |
| `msg_id` | 4 bytes | Request/response correlation token. 0 for one-way messages. Big-endian uint32. |
| `payload` | N bytes | Type-specific binary encoding. |

## Flags

| Bit | Name | Meaning |
|-----|------|---------|
| `0x01` | `IS_RESPONSE` | This frame is a response to a previous request (msg_id matches the request). |
| `0x02` | `IS_STREAM_MORE` | More frames follow for this message (used by file chunks, exec streaming stdout/stderr). |
| `0x04` | `IS_ERROR` | This frame carries an error payload (type `0xFE`). |

Frames with `IS_STREAM_MORE` set are part of a logical message split
across multiple frames. The final frame clears the flag. `msg_id` is
constant across all frames of a single logical message.

## msg_id Semantics

- **Request:** sender allocates a non-zero `msg_id`. Responses echo the
  same id with `IS_RESPONSE` set.
- **One-way message** (heartbeat, event, notification): `msg_id = 0`.
- **Streaming:** all chunks of a message share the same `msg_id`.
  The first frame carries the request type; subsequent frames carry the
  chunk type. The last frame clears `IS_STREAM_MORE`.

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

## Encoding Functions

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

## Decoding Functions

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

## Wire Examples

### Example 1: PING / PONG

**Request** (host → guest, 11 bytes on wire):
```
 length: 0x00_00_00_06   (1 + 1 + 4 + 0 = 6)
   type: 0x01             (PING)
  flags: 0x00
 msg_id: 0x00_00_00_01
payload: (empty)
```

**Response** (guest → host, 11 bytes):
```
 length: 0x00_00_00_06
   type: 0x02             (PONG)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches request)
payload: (empty)
```

### Example 2: EXEC

**Request** (host → guest):
```
 length: 0x00_00_00_2A   (42 = 6 + 36 payload)
   type: 0x10             (EXEC)
  flags: 0x00
 msg_id: 0x00_00_00_02
payload:
  string "ls -la"         → 0x0006 "ls -la"
  string "/home/user"     → 0x000B "/home/user"
  map<string,string> {}   → 0x0000
  uint32 30000            → 0x00_00_75_30
```

**Response** (guest → host):
```
 length: 0x00_00_00_1C
   type: 0x11             (EXEC_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_02    (matches request)
payload:
  int32  0                → 0x00_00_00_00 (exit_code)
  bytes  ""               → 0x00_00_00_00 (stdout)
  bytes  ""               → 0x00_00_00_00 (stderr)
  uint32 5                → 0x00_00_00_05 (duration_ms)
```

**Total wire bytes: ~28.** JSON equivalent: ~230 bytes (with base64 for
stdout/stderr placeholders). ~88% reduction for this example.

### Example 3: FILE_EXPORT (streaming, 64KB file, 16KB chunks)

**Request** (guest → host):
```
 length: 0x00_00_00_16
   type: 0x20             (FILE_EXPORT_REQ)
  flags: 0x00
 msg_id: 0x00_00_00_03
payload:
  string "/tmp/result.bin" → 0x0010 "/tmp/result.bin"
  uint32 16384             → 0x00_00_40_00
```

**Chunk 0** (guest → host):
```
 length: 0x00_00_40_10   (6 + 4 + 16384 = 16394)
   type: 0x21             (FILE_EXPORT_CHUNK)
  flags: 0x02             (IS_STREAM_MORE — more chunks follow)
 msg_id: 0x00_00_00_03    (matches request)
payload:
  uint32 0                → seq=0
  bytes  [16384 raw bytes] → 0x00_00_40_00 + raw data
```

**Chunks 1–3** — same pattern, seq 1–3, `IS_STREAM_MORE`.

**Final chunk** (guest → host):
```
 length: 0x00_00_00_2B   (6 + 4 + 32 + 1 = 43)
   type: 0x22             (FILE_EXPORT_END)
  flags: 0x01             (IS_RESPONSE, no more stream)
 msg_id: 0x00_00_00_03
payload:
  uint32 4                → total_chunks=4
  bytes  [32-byte SHA256]
```

**Total wire bytes for a 64KB file:**
- MVCP: ~43 + 4 × 16394 = ~65,619 bytes
- JSON base64: ~43 + 4 × (16384 × 1.33) = ~87,227 bytes
- **MVCP saves ~25% on wire alone**, plus no base64 CPU overhead.
