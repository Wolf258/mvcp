# Example: File Export

Export a 64KB file from guest to host in 16KB chunks.

## FILE_EXPORT_REQ (Guest → Host)

Request to export `/tmp/result.bin` with 16KB chunks.

```
 length: 0x00_00_00_16   (22 = 6 + 16 payload)
   type: 0x20             (FILE_EXPORT_REQ)
  flags: 0x00
 msg_id: 0x00_00_00_03
payload:
  string "/tmp/result.bin" → 0x0010 "/tmp/result.bin"
  uint32 16384             → 0x00_00_40_00
```

## Chunk 0 (Guest → Host)

```
 length: 0x00_00_40_10   (16394 = 6 + 4 + 16384)
   type: 0x21             (FILE_EXPORT_CHUNK)
  flags: 0x02             (IS_STREAM_MORE)
 msg_id: 0x00_00_00_03    (matches request)
payload:
  uint32 0                → seq=0
  bytes  [16384 raw bytes] → 0x00_00_40_00 + raw data
```

## Chunks 1–3

Same pattern, seq 1–3, `IS_STREAM_MORE` flag set on chunks 1–2,
cleared on chunk 3 (the last data chunk).

## FILE_EXPORT_END (Guest → Host)

```
 length: 0x00_00_00_2B   (43 = 6 + 4 + 32)
   type: 0x22             (FILE_EXPORT_END)
  flags: 0x01             (IS_RESPONSE, IS_STREAM_MORE cleared)
 msg_id: 0x00_00_00_03
payload:
  uint32 4                → total_chunks=4
  bytes  [32-byte SHA256]
```

## Go: Receiving a File Export

```go
var buf bytes.Buffer
var totalChunks uint32

// read FILE_EXPORT_REQ
req, _ := protocol.ReadFrame(conn)
r := bytes.NewReader(req.Payload)
path, _ := protocol.ReadString(r)
chunkSize, _ := protocol.ReadUint32(r)

// read chunks
for {
    frame, _ := protocol.ReadFrame(conn)
    r := bytes.NewReader(frame.Payload)

    switch frame.Type {
    case protocol.TypeFileExportChunk:
        seq, _ := protocol.ReadUint32(r)
        data, _ := protocol.ReadBytes(r)
        buf.Write(data)

    case protocol.TypeFileExportEnd:
        totalChunks, _ = protocol.ReadUint32(r)
        sha256sum, _ := protocol.ReadBytes(r)
        break
    }
}

// verify SHA256
actual := sha256.Sum256(buf.Bytes())
if !bytes.Equal(actual[:], sha256sum) {
    // hash mismatch
}
```

## Wire Size Comparison (64KB file)

| Format | Bytes |
|--------|-------|
| MVCP binary | ~65,619 |
| JSON base64 | ~87,227 |
| **Saving** | **~25%** |

Plus no base64 encode/decode CPU overhead on either side.
