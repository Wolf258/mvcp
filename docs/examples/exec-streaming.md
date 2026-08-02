# Example: EXEC with Streaming

The host sets `FlagExecStreaming` on the `EXEC` request to request
incremental output delivery. The guest streams output via `EXEC_STREAM`
frames while the command runs.

## Flow

```
Host                           Guest
  │                              │
  │── EXEC ────────────────────→│  (msg_id=2, flags=0x04 FlagExecStreaming)
  │                              │  starts "find / -name '*.log'"
  │                              │
  │←─ STARTED ──────────────────│  stream=true
  │←─ EXEC_STREAM (stdout) ─────│  IS_STREAM_MORE, msg_id=2, channel=0x00, seq=0
  │←─ EXEC_STREAM (stdout) ─────│  IS_STREAM_MORE, msg_id=2, channel=0x00, seq=1
  │←─ EXEC_STREAM (stderr) ─────│  IS_STREAM_MORE, msg_id=2, channel=0x01, seq=2
  │←─ EXEC_STREAM (stdout) ─────│  IS_STREAM_MORE, msg_id=2, channel=0x00, seq=3
  │                              │  process exits
  │←─ EXEC_RESULT ──────────────│  IS_RESPONSE, msg_id=2, stdout/stderr empty
```

## EXEC Request

Same as the non-streaming case, with `FlagExecStreaming` (0x04) set in flags:

```
 length: 0x00_00_00_2A   (42 = 6 + 36 payload)
   type: 0x10             (EXEC)
  flags: 0x04             (FlagExecStreaming)
 msg_id: 0x00_00_00_02
payload:
  string "find / -name '*.log'"
  string "/"
  map<string,string> {}
  uint32 0                (no timeout)
```

## EXEC_STREAM Chunk (Guest → Host)

Stdout chunk with sequence 0:

```
 length: 0x00_00_04_15   (1045 = 6 + 9 + 1030 payload)
   type: 0x11             (EXEC_STREAM)
  flags: 0x02             (IS_STREAM_MORE — more chunks follow)
 msg_id: 0x00_00_00_02    (matches EXEC request)
payload:
  uint8  0x00             → ExecChannelStdout
  uint32 0x00_00_00_00    → sequence 0
  bytes  [1024 raw bytes] → raw stdout data
```

Stderr chunk with sequence 2:

```
 length: 0x00_00_00_6F   (111 = 6 + 9 + 96 payload)
   type: 0x11             (EXEC_STREAM)
  flags: 0x02             (IS_STREAM_MORE)
 msg_id: 0x00_00_00_02    (matches EXEC request)
payload:
  uint8  0x01             → ExecChannelStderr
  uint32 0x00_00_00_02    → sequence 2
  bytes  [90 raw bytes]   → raw stderr data
```

## Final EXEC_RESULT

The last chunk has `IS_STREAM_MORE` cleared. `EXEC_RESULT` arrives
with `IS_RESPONSE` and no `IS_STREAM_MORE`. Since streaming was used,
stdout and stderr fields are empty:

```
 length: 0x00_00_00_1C   (28 = 6 + 22 payload)
   type: 0x12             (EXEC_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_02
payload:
  int32  0                → exit_code: 0
  bytes  ""               → stdout: empty (delivered via chunks)
  bytes  ""               → stderr: empty (delivered via chunks)
  uint32 1234             → duration_ms: 1234
```

## Go: Receiving Streaming Output

```go
var stdout, stderr bytes.Buffer

for {
    frame, _ := protocol.ReadMVCPFrame(conn)
    r := bytes.NewReader(frame.Body)

    switch frame.Type {
    case protocol.TypeEXECSTREAM:
        var s messages.ExecStream
        s.UnmarshalBinary(frame.Body)
        switch s.Channel {
        case protocol.ExecChannelStdout:
            stdout.Write(s.Data)
        case protocol.ExecChannelStderr:
            stderr.Write(s.Data)
        }
    case protocol.TypeEXECRESULT:
        exitCode, _ := protocol.ReadInt32(r)
        protocol.ReadBytes(r) // stdout (empty when streaming)
        protocol.ReadBytes(r) // stderr (empty when streaming)
        duration, _ := protocol.ReadUint32(r)
        // streaming complete
    }
}
```

## Notes

- `seq` is a monotonic counter shared across all channels for the
  request. It aids observability, debug, and replay — not ordering
  (SOCK_STREAM preserves that).
- All chunks share the original request's `msg_id`.
- The guest sets `IS_STREAM_MORE` on every chunk. `EXEC_RESULT`
  clears it and sets `IS_RESPONSE`.
- Without `FlagExecStreaming`, the guest buffers all output and
  returns it inline in `EXEC_RESULT` (up to a 16 MB limit).
