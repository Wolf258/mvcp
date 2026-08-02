# Example: File Export

Export a 24KB file from guest to host in 16KB chunks. Host initiates.

## XFER_INIT (Host → Guest)

Host requests the guest to export `/tmp/result.bin`. Sets `WANT_ACK`.

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

## MVCP_ACK (Guest → Host)

Guest confirms readiness before sending chunks.

```
 length: 0x00_00_00_0E   (14 = 6 + 8 body)
   type: 0xFB             (MVCP_ACK)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches INIT)
   body:
    uint8 0x00             → OK
    string ""              → empty
```

## Chunk 0 (Guest → Host)

```
 length: 0x00_00_40_10   (16394 = 6 + 4 + 16384)
   type: 0x21             (XFER_CHUNK)
  flags: 0x02             (IS_STREAM_MORE)
 payload:
   uint32 0               → seq=0
   bytes  [16384 raw bytes] → 0x00_00_40_00 + raw data
```

## Chunk 1 — Last (Guest → Host)

```
 length: 0x00_00_20_0E   (8206 = 6 + 4 + 8196)
   type: 0x21             (XFER_CHUNK)
  flags: 0x00             (no MORE — last chunk)
 payload:
   uint32 1               → seq=1
   bytes  [8196 raw bytes] → 0x00_00_20_04 + raw data
```

## XFER_DONE (Host → Guest)

Host confirms with verification fields.

```
 length: 0x00_00_00_14   (20 = 6 + 14 payload)
   type: 0x22             (XFER_DONE)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches INIT)
 payload:
   bool true              → 0x01 (ok)
   uint32 2               → 0x00_00_00_02 (chunks_received)
   uint64 24576           → 0x00_00_00_00_00_00_60_00 (bytes_written)
```

## Go: Host Receiving an Export

```go
func receiveExport(conn net.Conn, outPath string) error {
    frame, err := mvcp.ReadFrame(conn)
    if err != nil {
        return err
    }
    path, _ := mvcp.ReadString(frame.Body)
    _, _ = mvcp.ReadUint32(frame.Body)  // totalSize (unknown for export)
    _, _ = mvcp.ReadUint8(frame.Body)   // dir

    // respond with MVCP_ACK to confirm readiness
    ackBody := mvcp.EncodeAck(mvcp.AckOK, "")
    ackFrame := mvcp.Frame{
        Type: mvcp.TypeAck, Flags: mvcp.FlagResponse,
        MsgID: frame.MsgID, Body: ackBody,
    }
    mvcp.WriteFrame(conn, ackFrame)

    out, err := os.Create(outPath)
    if err != nil {
        return err
    }
    defer out.Close()

    var chunksReceived uint32
    var bytesWritten uint64
    for {
        frame, err := mvcp.ReadFrame(conn)
        if err != nil {
            out.Close()
            os.Remove(outPath)
            return err
        }
        buf := bytes.NewReader(frame.Body)
        _, _ = mvcp.ReadUint32(buf) // seq
        data, _ := mvcp.ReadBytes(buf)
        n, _ := out.Write(data)
        chunksReceived++
        bytesWritten += uint64(n)
        if frame.Flags&mvcp.FlagStreamMore == 0 {
            break
        }
    }

    // send XFER_DONE with verification
    body := new(bytes.Buffer)
    mvcp.WriteBool(body, true)
    mvcp.WriteUint32(body, chunksReceived)
    mvcp.WriteUint64(body, bytesWritten)
    done := mvcp.Frame{
        Type: mvcp.TypeXferDone, Flags: mvcp.FlagResponse,
        MsgID: 1, Body: body.Bytes(),
    }
    return mvcp.WriteFrame(conn, done)
}
```


## Go: Guest Sending an Export

```go
func sendExport(conn net.Conn, filePath string) error {
    // wait for XFER_INIT (host → guest)
    initFrame, err := mvcp.ReadFrame(conn)
    if err != nil {
        return err
    }
    _, _ = mvcp.ReadString(initFrame.Body)   // path
    _, _ = mvcp.ReadUint32(initFrame.Body)   // totalSize
    _, _ = mvcp.ReadUint8(initFrame.Body)    // dir

    f, err := os.Open(filePath)
    if err != nil {
        // send MVCP_ACK with error
        ackBody := mvcp.EncodeAck(mvcp.AckGenericError, err.Error())
        ackFrame := mvcp.Frame{
            Type: mvcp.TypeAck, Flags: mvcp.FlagResponse,
            MsgID: initFrame.MsgID, Body: ackBody,
        }
        mvcp.WriteFrame(conn, ackFrame)
        return err
    }
    defer f.Close()

    // send MVCP_ACK — ready
    ackBody := mvcp.EncodeAck(mvcp.AckOK, "")
    ackFrame := mvcp.Frame{
        Type: mvcp.TypeAck, Flags: mvcp.FlagResponse,
        MsgID: initFrame.MsgID, Body: ackBody,
    }
    mvcp.WriteFrame(conn, ackFrame)

    buf := make([]byte, 16*1024)
    seq := uint32(0)
    for {
        n, _ := f.Read(buf)
        if n == 0 {
            break
        }
        more := uint8(mvcp.FlagStreamMore)
        peek := make([]byte, 1)
        f.Read(peek)
        if peek[0] == 0 {
            more = 0x00
        }
        payload := encodeSeqAndData(seq, buf[:n])
        chunk := mvcp.Frame{Type: mvcp.TypeXferChunk, Flags: more, Body: payload}
        if err := mvcp.WriteFrame(conn, chunk); err != nil {
            return err
        }
        seq++
    }

    // read XFER_DONE from host
    doneFrame, _ := mvcp.ReadFrame(conn)
    ok, _ := mvcp.ReadBool(doneFrame.Body)
    chunksReceived, _ := mvcp.ReadUint32(doneFrame.Body)
    bytesWritten, _ := mvcp.ReadUint64(doneFrame.Body)
    _ = ok
    _ = chunksReceived
    _ = bytesWritten
    return nil
}
```

## Wire Size Comparison (24KB file)

| Format | Bytes |
|--------|-------|
| MVCP binary | ~24,625 |
| JSON base64 | ~32,800 |
| **Saving** | **~25%** |

Plus no base64 encode/decode CPU overhead on either side.
