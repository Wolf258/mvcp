# MVCP — MicroVM Communication Protocol (SPEC)

> A compact binary protocol purpose-built for guest↔host communication
> over Firecracker's vsock device, replacing the current JSON RPC
> messages on the vsock channel.

## Why MVCP

JSON-based RPC on vsock has fundamental problems that become acute
once file transfer and command execution are fully wired:

| Problem | JSON impact | MVCP solution |
|---------|-------------|---------------|
| Base64 overhead | +33% per byte for every file chunk | Raw bytes, no encoding |
| Repeated string keys | `"command"`, `"exit_code"`, `"stdout"` in every message | 1-byte type + positional payload |
| No request correlation | Responses are order-dependent on the stream | 4-byte `msg_id` enables pipelining |
| No streaming primitives | Each chunk is a full JSON parse round-trip | `is_stream_more` flag in frame header |
| Parse overhead | `json.Unmarshal` per frame | `binary.Read` with pre-allocated structs |
| Runtime type safety | String match on `"type"` field | `uint8` switch, compile-time exhaustiveness |

## Module Structure

MVCP is a standalone Go module with zero external dependencies beyond
the standard library.

```
mvcp/
  go.mod              # module github.com/Wolf258/mvcp
  protocol/
    wire.go           # frame format, type constants, flag constants
    encode.go         # binary write primitives (string, bytes, uint*, int32)
    decode.go         # binary read primitives
    message.go        # typed message structs + Encode/Decode per message type
    frame.go          # ReadFrame / WriteFrame (length-prefix layer)
  sdk/
    go/               # Go SDK
    c/                # C SDK
    rust/             # Rust SDK
  docs/
    wire-format.md    # Wire format and encoding reference
    error-codes.md    # Error code registry
    versioning.md     # Versioning strategy
```

---

## Transport

MVCP rides on top of the vsock transport:

- **Guest side:** `AF_VSOCK` `SOCK_STREAM` listeners on ports 9000–9003
- **Host side:** `AF_UNIX` connection to Firecracker's per-VM vsock UDS
  with `CONNECT <port>\n` / `OK <assigned_port>\n` handshake
- **No multiplexing layer:** service discovery remains port-based

The transport provides a reliable, ordered, bidirectional byte stream.
MVCP adds message framing and binary encoding on top.

---

## Frame Wire Format

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

### Flags

| Bit | Name | Meaning |
|-----|------|---------|
| `0x01` | `IS_RESPONSE` | This frame is a response to a previous request (msg_id matches the request). |
| `0x02` | `IS_STREAM_MORE` | More frames follow for this message (used by file chunks, exec streaming stdout/stderr). |
| `0x04` | `IS_ERROR` | This frame carries an error payload (type `0xFE`). |

Frames with `IS_STREAM_MORE` set are part of a logical message split
across multiple frames. The final frame clears the flag. `msg_id` is
constant across all frames of a single logical message.

### msg_id Semantics

- **Request:** sender allocates a non-zero `msg_id`. Responses echo the
  same id with `IS_RESPONSE` set.
- **One-way message** (heartbeat, event, notification): `msg_id = 0`.
- **Streaming:** all chunks of a message share the same `msg_id`.
  The first frame carries the request type; subsequent frames carry the
  chunk type. The last frame clears `IS_STREAM_MORE`.

---

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

### Encoding Functions (in `protocol/encode.go`)

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

### Decoding Functions (in `protocol/decode.go`)

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

## Message Type Registry

### Control (`0x00` – `0x0F`)

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x00` | *(reserved)* | — | Invalid, must never appear on wire. |
| `0x01` | `PING` | H→G | *(none)* |
| `0x02` | `PONG` | G→H | *(none)* |
| `0x03` | `SHUTDOWN` | H→G | *(none)* |
| `0x04` | `SHUTDOWN_ACK` | G→H | *(none)* |
| `0x05` | `GET_STATUS` | H→G | *(none)* |
| `0x06` | `STATUS` | G→H | `string version`, `uint32 pid`, `bool shutting_down` |

### Execution (`0x10` – `0x1F`)

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x10` | `EXEC` | H→G | `string command`, `string cwd`, `map env`, `uint32 timeout_ms` |
| `0x11` | `EXEC_RESULT` | G→H | `int32 exit_code`, `bytes stdout`, `bytes stderr`, `uint32 duration_ms` |
| `0x12` | `EXEC_STDOUT` | G→H | `bytes data` (streaming, `IS_STREAM_MORE` set until last chunk) |
| `0x13` | `EXEC_STDERR` | G→H | `bytes data` (streaming, `IS_STREAM_MORE` set until last chunk) |

`EXEC_STDOUT` and `EXEC_STDERR` are optional streaming frames emitted
*while* the command runs. When streaming is enabled, `EXEC_RESULT`
arrives after the last stream chunk with `IS_STREAM_MORE` cleared and
`IS_RESPONSE` set.

### File Transfer (`0x20` – `0x2F`)

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x20` | `FILE_EXPORT_REQ` | G→H | `string path`, `uint32 chunk_size` |
| `0x21` | `FILE_EXPORT_CHUNK` | G→H | `uint32 seq`, `bytes data` |
| `0x22` | `FILE_EXPORT_END` | G→H | `uint32 total_chunks`, `bytes sha256` (32 bytes) |
| `0x23` | `FILE_IMPORT_REQ` | H→G | `string path`, `uint32 total_size` |
| `0x24` | `FILE_IMPORT_CHUNK` | H→G | `uint32 seq`, `bytes data` |
| `0x25` | `FILE_IMPORT_RESULT` | G→H | `uint64 written_bytes`, `bytes sha256` (32 bytes), `bool ok` |

Streaming semantics:
- `FILE_EXPORT_REQ` starts a stream; the guest sends chunks
  (`IS_STREAM_MORE` set) until the last chunk, then an
  `FILE_EXPORT_END` with `IS_RESPONSE`.
- `FILE_IMPORT_REQ` starts a stream; the host sends chunks
  (`IS_STREAM_MORE` set). The guest acknowledges with
  `FILE_IMPORT_RESULT`.

### Filesystem (`0x30` – `0x3F`)

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x30` | `GET_CWD` | H→G | *(none)* |
| `0x31` | `CWD_RESULT` | G→H | `string cwd` |
| `0x32` | `STAT` | H→G | `string path` |
| `0x33` | `STAT_RESULT` | G→H | `uint64 size`, `uint64 mode`, `int64 mtime_ns`, `bool exists` |
| `0x34` | `LIST_DIR` | H→G | `string path` |
| `0x35` | `LIST_DIR_RESULT` | G→H | `uint16 count` + N × (`string name`, `bool is_dir`) |
| `0x36` | `READ_FILE` | H→G | `string path`, `uint64 offset`, `uint32 max_bytes` |
| `0x37` | `READ_FILE_RESULT` | G→H | `bytes data`, `bool eof` |
| `0x38` | `WRITE_FILE` | H→G | `string path`, `bytes data`, `uint64 offset`, `bool truncate` |
| `0x39` | `WRITE_FILE_RESULT` | G→H | `uint64 written_bytes`, `bool ok` |

### Error

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0xFE` | `ERROR` | Both | `uint16 code`, `string message` |

Error codes:

| Code | Name |
|------|------|
| `0x0001` | `UNKNOWN_TYPE` — message type not recognized |
| `0x0002` | `BAD_PAYLOAD` — payload decoding failed |
| `0x0003` | `FILE_NOT_FOUND` |
| `0x0004` | `PERMISSION_DENIED` |
| `0x0005` | `EXEC_FAILED` — command could not be started |
| `0x0006` | `TIMEOUT` — operation exceeded its deadline |
| `0x0007` | `NOT_A_DIRECTORY` |

### Reserved

| Type | Description |
|------|-------------|
| `0xFF` | Protocol extension marker. Reserved for future use. |

---

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

**Total wire bytes: ~28.** JSON equivalent: ~230 bytes (with base64 for stdout/stderr
placeholders). ~88% reduction for this example.

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

---

## Service Ports

MVCP keeps the existing port-based service discovery unchanged:

| Port | Service | Protocol | Direction |
|------|---------|----------|-----------|
| 9000 | Control | **MVCP** (length-prefixed binary) | Bidirectional |
| 9001 | Console | Raw bytes (PTY ↔ shell) | Bidirectional |
| 9002 | Events | **MVCP** (length-prefixed binary, one-way G→H) | Guest → Host |
| 9003 | Heartbeat | Length-prefixed JSON *(unchanged)* | Guest → Host |

**Port 9001 (console)** is deliberately kept as raw bytes — there is
no benefit to framing console I/O through the protocol.

**Port 9002 (events)** migrates from JSONL to MVCP frames for
consistency. Event types are MVCP message types in the `0x80–0x8F`
range.

**Port 9003 (heartbeat)** keeps its current JSON payload (~80 bytes).
The overhead of converting to binary is not worth the complexity for a
~80-byte message sent every 500ms.

### Event Messages (`0x80` – `0x8F`)

Each event is an MVCP frame with `msg_id = 0` (one-way).

| Type | Name | Payload |
|------|------|---------|
| `0x80` | `EVENT_READY` | `string version` |
| `0x81` | `EVENT_FILE_RECEIVED` | `string path`, `bytes sha256`, `uint64 size` |
| `0x82` | `EVENT_MOUNT` | `string path`, `string fstype` |
| `0x83` | `EVENT_ERROR` | `uint16 code`, `string message` |
| `0x84` | `EVENT_LOG` | `uint8 level`, `string message`, `uint64 ts_ns` |

Log levels: `0x00` = debug, `0x01` = info, `0x02` = warn, `0x03` = error.

---

## Protocol Concurrency

- **Request/response:** one request, one response, correlated by `msg_id`.
  The stream is ordered — responses arrive in request order on the same
  connection.
- **Streaming:** once a stream starts, no other request can be sent on
  the same connection until the stream ends. For concurrent access, the
  host opens multiple connections to the same port.
- **Multiple connections to port 9000 are allowed.** Each connection is
  an independent request/response stream. This enables concurrent EXEC
  and FILE_EXPORT from different host-side goroutines without head-of-line
  blocking.

---

## Adoption Guide (example: Shifty)

This is an example migration path for a consumer that currently uses JSON
RPC over vsock. The console service (port 9001, raw bytes) and heartbeat
service (port 9003, tiny JSON) are **not** part of the migration — they
stay as-is.

| Step | What | Where |
|------|------|-------|
| 1 | Create `mvcp/` module with `protocol/` package | Repo root |
| 2 | Add `use ./mvcp` to `go.work` | `go.work` |
| 3 | Implement `mvcp/protocol/{wire,encode,decode,message,frame}.go` | `mvcp/` |
| 4 | Write round-trip tests for every message type | `mvcp/protocol/*_test.go` |
| 5 | Refactor `shared/infrastructure/vsock/` to use `mvcp/protocol` | `shared/` |
| 6 | Replace `shifty-vhandler/rpc.go` JSON dispatcher with MVCP dispatcher | `shifty-vhandler/` |
| 7 | Migrate `shifty-vhandler/events.go` JSONL → MVCP frames | `shifty-vhandler/` |
| 8 | Update host-side clients (`VMStream`, `HealthCheckWatcher`) | `shifty-core/` |
| 9 | Update `shifty-vhandler/shiftyctl/` to speak MVCP | `shifty-vhandler/shiftyctl/` |
| 10 | Remove legacy JSON RPC types and dead code | All modules |

---

See also:
- [docs/wire-format.md](docs/wire-format.md) for the detailed wire encoding reference.
- [docs/error-codes.md](docs/error-codes.md) for the error code registry.
- [docs/versioning.md](docs/versioning.md) for protocol versioning strategy.
