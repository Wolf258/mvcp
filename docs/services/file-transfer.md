# File Transfer Service (Port 9004)

> Message type range `0x20`–`0x2F`. Dedicated port 9004.
> Uses MVCP wire directly — **not** the RPC layer. No `msg_id`.

The file transfer service enables sending files between host and guest
over a dedicated vsock channel. Transfer is chunked, host-initiated,
and uses a clean sender-push model with a single ack from the receiver.

## Design Principles

- **Host always initiates** the connection — the host decides whether to
  import a file to the guest or export a file from the guest.
- **Streaming chunks** — chunks are sent sequentially with
  `IS_STREAM_MORE`. No per-chunk `msg_id` or ACK; the transport layer
  handles ordering and integrity.
- **No cryptographic verification** — simple and fast. Integrity is
  handled at the transport layer.
- **Three-phase transfer** — INIT (acknowledged) → CHUNKS (stream) →
  DONE (verification). The `XFER_INIT` carries `WANT_ACK` so the
  receiver confirms readiness. `XFER_DONE` carries verification fields
  so the sender can confirm what was actually received.

## Service Architecture

```
Port 9004 → MVCP wire (type + flags) → Transport frame (4B BE)
            (no RPC layer, no msg_id)
```

## Port Assignment

| Port | Service | Protocol | Direction |
|------|---------|----------|-----------|
| 9004 | File Transfer | MVCP (no RPC) | Host-initiated, bidirectional data |

File transfer is isolated from port 9000 (RPC). Long file transfers never
head-of-line block control or execution messages.

## Message Types

| Type | Name | Flags | Direction | Payload |
|------|------|-------|-----------|---------|
| `0x20` | `XFER_INIT` | `WANT_ACK` | H→G | `string path`, `uint32 total_size`, `uint8 dir` |
| `0x21` | `XFER_CHUNK` | — | H↔G | `uint32 seq`, `bytes data` |
| `0x22` | `XFER_DONE` | `IS_RESPONSE` | receiver→sender | `bool ok`, `uint32 chunks_received`, `uint64 bytes_written` |
| `0x23`–`0x2F` | *(reserved)* | — | — | — |

### XFER_INIT (`0x20`)

Sent by the host to initiate a transfer. Carries `WANT_ACK` — the
receiver MUST respond with `MVCP_ACK` (type `0xFB`) before the sender
begins streaming chunks. The `msg_id` is non-zero for ack correlation.

| Field | Encoding | Description |
|-------|----------|-------------|
| `path` | `string` | File path inside the guest (destination for import, source for export) |
| `total_size` | `uint32` | Total file size in bytes. `0` if unknown (export case — guest knows). |
| `dir` | `uint8` | `0x00` = **import** (host → guest), `0x01` = **export** (guest → host) |

Frame flags: `0x04` (`WANT_ACK`).

### XFER_CHUNK (`0x21`)

Sent by the **data sender** (host for import, guest for export). Each
chunk carries sequential data.

| Field | Encoding | Description |
|-------|----------|-------------|
| `seq` | `uint32` | Chunk sequence number, starts at 0 |
| `data` | `bytes` | Raw file data for this chunk |

Frame flag semantics:

| Chunk | Flags | Meaning |
|-------|-------|---------|
| Non-final | `0x02` (`IS_STREAM_MORE`) | More chunks follow |
| Final | `0x00` | Last chunk — data stream ends |

### XFER_DONE (`0x22`)

Sent by the **receiver** after all chunks are consumed. Carries
verification fields so the sender can confirm what was actually received.

| Field | Encoding | Description |
|-------|----------|-------------|
| `ok` | `bool` | `true` if file was written to disk successfully, `false` on error |
| `chunks_received` | `uint32` | Number of chunks consumed by the receiver |
| `bytes_written` | `uint64` | Total bytes written to disk |

Flags: `0x01` (`IS_RESPONSE`). `msg_id` matches the `XFER_INIT` message.

The sender SHOULD compare `total_size` from `XFER_INIT` against
`bytes_written` and the highest `seq` sent against `chunks_received`
to detect mismatches.

## Flows

### Import (Host → Guest)

The host sends a file to the guest.

```
Host  →   INIT(path="/tmp/script.sh", size=2048, dir=0x00)     [flags=0x04] WANT_ACK, msg_id=1
Guest →   ACK(msg_id=1, status=0x00)                            [flags=0x01] MVCP_ACK — ready

Host  →   CHUNK(seq=0, data=<1024B>)                            [flags=0x02] MORE
Host  →   CHUNK(seq=1, data=<1024B>)                            [flags=0x00] ← last

Guest →   DONE(ok=true, chunks=2, bytes=2048)                   [flags=0x01] verification
```

### Export (Guest → Host)

The host requests a file from the guest. Guest pushes data back.

```
Host  →   INIT(path="/app/result.json", size=0, dir=0x01)      [flags=0x04] WANT_ACK, msg_id=1
Guest →   ACK(msg_id=1, status=0x00)                            [flags=0x01] MVCP_ACK — ready

Guest →   CHUNK(seq=0, data=<16KB>)                             [flags=0x02] MORE
Guest →   CHUNK(seq=1, data=<8KB>)                              [flags=0x00] ← last

Host  →   DONE(ok=true, chunks=2, bytes=24576)                  [flags=0x01] verification
```

### Export — File Not Found

```
Host  →   INIT(path="/nonexistent", dir=0x01)                   [flags=0x04] WANT_ACK, msg_id=1
Guest →   DONE(ok=false, chunks=0, bytes=0)                     [flags=0x01]
```

No chunks are sent. The guest responds immediately with failure.

### Mid-Transfer Error

If the sender encounters an error (disk I/O, read failure), it **closes
the connection**. The receiver detects EOF and discards the partial file.

No error frames are sent mid-stream — tearing down the connection is the
error signal.

If the receiver rejects the `XFER_INIT` (permission denied, disk full),
it responds with `MVCP_ACK` with a non-zero status. No chunks are sent.

## Flags Summary

| Frame | Flags | Meaning |
|-------|-------|---------|
| `XFER_INIT` | `0x04` | `WANT_ACK` — receiver must respond with `MVCP_ACK` |
| `XFER_CHUNK` (more) | `0x02` | `IS_STREAM_MORE` — more chunks follow |
| `XFER_CHUNK` (last) | `0x00` | End of data stream |
| `XFER_DONE` | `0x01` | `IS_RESPONSE` — transfer result + verification |

## Wire Example (Export, 24KB file, 16KB chunk)

**XFER_INIT** (host → guest, `WANT_ACK`, msg_id=1):

```
 length: 0x00_00_00_1D   (29 = 6 + 23 payload)
   type: 0x20             (XFER_INIT)
  flags: 0x04             (WANT_ACK)
 msg_id: 0x00_00_00_01
 payload:
   string "/tmp/result.bin" → 0x0010 "/tmp/result.bin"
   uint32 0                → 0x00_00_00_00   (size unknown for export)
   uint8  0x01             → 0x01             (dir=export)
```

**MVCP_ACK** (guest → host, confirms readiness):

```
 length: 0x00_00_00_0E   (14 = 6 + 8 body)
   type: 0xFB             (MVCP_ACK)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches INIT)
 payload:
   uint8 0x00             → OK
   string ""              → empty
```

**Chunk 0** (guest → host):

```
 length: 0x00_00_40_10   (16394 = 6 + 4 + 16384)
   type: 0x21             (XFER_CHUNK)
  flags: 0x02             (IS_STREAM_MORE)
  payload:
   uint32 0               → seq=0
   bytes  [16384 raw bytes]
```

**Chunk 1** (guest → host, last):

```
 length: 0x00_00_20_0E   (8206 = 6 + 4 + 8196)
   type: 0x21             (XFER_CHUNK)
  flags: 0x00             (no MORE — last chunk)
  payload:
   uint32 1               → seq=1
   bytes  [8196 raw bytes]
```

**XFER_DONE** (host → guest, verification):

```
 length: 0x00_00_00_14   (20 = 6 + 14 body)
   type: 0x22             (XFER_DONE)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches INIT)
   body:
    bool true             → 0x01 (ok)
    uint32 2              → 0x00_00_00_02 (chunks_received)
    uint64 24576          → 0x00_00_00_00_00_00_60_00 (bytes_written)
```

## Go Example: Host Receiving an Export

```go
func receiveExport(conn net.Conn, outPath string) error {
    // read XFER_INIT
    frame, _ := mvcp.ReadMVCPFrame(conn)
    path, _ := mvcp.ReadString(frame.Body)
    totalSize, _ := mvcp.ReadUint32(frame.Body)
    dir, _ := mvcp.ReadUint8(frame.Body)

    // respond with MVCP_ACK to confirm readiness
    ackBody := mvcp.EncodeAck(mvcp.AckOK, "")
    ackFrame := mvcp.Frame{
        Type:   mvcp.TypeAck,
        Flags:  mvcp.FlagResponse,
        MsgID:  frame.MsgID,
        Body:   ackBody,
    }
    mvcp.WriteMVCPFrame(conn, ackFrame)

    out, err := os.Create(outPath)
    if err != nil {
        return err
    }
    defer out.Close()

    var chunksReceived uint32
    var bytesWritten uint64

    // read chunks
    for {
        frame, _ := mvcp.ReadMVCPFrame(conn)
        buf := bytes.NewReader(frame.Body)
        seq, _ := mvcp.ReadUint32(buf)
        data, _ := mvcp.ReadBytes(buf)
        n, _ := out.Write(data)
        chunksReceived++
        bytesWritten += uint64(n)
        if frame.Flags&mvcp.FlagStreamMore == 0 {
            break // last chunk
        }
    }

    // send XFER_DONE with verification
    body := new(bytes.Buffer)
    mvcp.WriteBool(body, true)     // ok
    mvcp.WriteUint32(body, chunksReceived)
    mvcp.WriteUint64(body, bytesWritten)
    doneFrame := mvcp.Frame{
        Type:   mvcp.TypeXferDone,
        Flags:  mvcp.FlagResponse,
        MsgID:  1, // matches XFER_INIT
        Body:   body.Bytes(),
    }
    mvcp.WriteMVCPFrame(conn, doneFrame)
    return nil
}
```

## Differences from JSON RPC

| Aspect | Legacy JSON | MVCP File Transfer |
|--------|------------|-------------------|
| Encoding | JSON + base64 | Raw binary |
| Overhead per chunk | Full JSON parse | 6B header + 4B seq |
| Base64 bloat | +33% | 0% |
| Connection | Shares RPC port 9000 | Dedicated port 9004 |
| Init ack | None (fire-and-forget) | `MVCP_ACK` via `WANT_ACK` |
| Completion verification | JSON error objects | `XFER_DONE` with chunk count + byte count |
| Error signaling | JSON error objects | Connection close or `MVCP_ACK` with error status |

---

See also:
- [rpc.md](rpc.md) for the RPC layer (port 9000) — file transfer is explicitly excluded.
- [tools.md](tools.md) for filesystem metadata operations (read_file, write_file, stat, list_dir).
- [02-wire-format.md](../02-wire-format.md) for the MVCP header layout and flags.
