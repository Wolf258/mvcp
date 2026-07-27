# File Transfer Service (Port 9000)

> Message type range `0x20`–`0x2F`. Carried on port 9000 (Control).

The file transfer service enables sending files between host and guest
over the vsock channel. Transfer is chunked with SHA256 verification.

## Message Types

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x20` | `FILE_EXPORT_REQ` | G→H | `string path`, `uint32 chunk_size` |
| `0x21` | `FILE_EXPORT_CHUNK` | G→H | `uint32 seq`, `bytes data` |
| `0x22` | `FILE_EXPORT_END` | G→H | `uint32 total_chunks`, `bytes sha256` (32 bytes) |
| `0x23` | `FILE_IMPORT_REQ` | H→G | `string path`, `uint32 total_size` |
| `0x24` | `FILE_IMPORT_CHUNK` | H→G | `uint32 seq`, `bytes data` |
| `0x25` | `FILE_IMPORT_RESULT` | G→H | `uint64 written_bytes`, `bytes sha256` (32 bytes), `bool ok` |
| `0x26`–`0x2F` | *(reserved)* | — | — |

## File Export (Guest → Host)

The guest sends a file to the host. Guest initiates.

1. **Guest** sends `FILE_EXPORT_REQ` with path and chunk size
2. **Guest** sends `FILE_EXPORT_CHUNK` frames with `IS_STREAM_MORE` flag
3. Each chunk has a `seq` counter (0, 1, 2, …)
4. **Guest** sends `FILE_EXPORT_END` with total_chunks and SHA256 of the
   complete file. `IS_RESPONSE` set, `IS_STREAM_MORE` cleared.

### FILE_EXPORT_REQ (`0x20`)

| Field | Encoding | Description |
|-------|----------|-------------|
| `path` | `string` | File path inside the guest |
| `chunk_size` | `uint32` | Maximum bytes per chunk |

### FILE_EXPORT_CHUNK (`0x21`)

| Field | Encoding | Description |
|-------|----------|-------------|
| `seq` | `uint32` | Chunk sequence number, starts at 0 |
| `data` | `bytes` | Raw file data for this chunk |

### FILE_EXPORT_END (`0x22`)

| Field | Encoding | Description |
|-------|----------|-------------|
| `total_chunks` | `uint32` | Total number of chunks sent |
| `sha256` | `bytes` (32) | SHA256 hash of the complete file |

## File Import (Host → Guest)

The host sends a file to the guest. Host initiates.

1. **Host** sends `FILE_IMPORT_REQ` with destination path and total size
2. **Host** sends `FILE_IMPORT_CHUNK` frames with `IS_STREAM_MORE` flag
3. Each chunk has a `seq` counter (0, 1, 2, …)
4. After all chunks, `IS_STREAM_MORE` is cleared on the last chunk
5. **Guest** verifies SHA256 and sends `FILE_IMPORT_RESULT` with
   `IS_RESPONSE` set

### FILE_IMPORT_REQ (`0x23`)

| Field | Encoding | Description |
|-------|----------|-------------|
| `path` | `string` | Destination path inside the guest |
| `total_size` | `uint32` | Total file size in bytes |

### FILE_IMPORT_CHUNK (`0x24`)

| Field | Encoding | Description |
|-------|----------|-------------|
| `seq` | `uint32` | Chunk sequence number, starts at 0 |
| `data` | `bytes` | Raw file data for this chunk |

### FILE_IMPORT_RESULT (`0x25`)

| Field | Encoding | Description |
|-------|----------|-------------|
| `written_bytes` | `uint64` | Total bytes written to disk |
| `sha256` | `bytes` (32) | SHA256 hash of the received file |
| `ok` | `bool` | `true` if file was written and hash matched |

## Streaming Semantics

- All chunks share the same `msg_id` as the initiating request.
- `IS_STREAM_MORE` is set on every chunk except the last (for import)
  or the `FILE_EXPORT_END` / `FILE_IMPORT_RESULT` frame (for import).
- Chunks must arrive in `seq` order. Missing or out-of-order chunks
  trigger an `ERROR` response.

## Wire Example (Export, 64KB file, 16KB chunks)

**FILE_EXPORT_REQ** (guest → host):

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
  flags: 0x02             (IS_STREAM_MORE)
 msg_id: 0x00_00_00_03    (matches request)
payload:
  uint32 0                → seq=0
  bytes  [16384 raw bytes] → 0x00_00_40_00 + raw data
```

**FILE_EXPORT_END** (guest → host):

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
- MVCP: ~65,619 bytes
- JSON base64: ~87,227 bytes
- **MVCP saves ~25% on wire**, plus no base64 CPU overhead.

---

See also:
- [filesystem.md](filesystem.md) for filesystem metadata operations (STAT, LIST_DIR, READ/WRITE).
- [examples/file-export.md](../examples/file-export.md) for the wire-level example.
