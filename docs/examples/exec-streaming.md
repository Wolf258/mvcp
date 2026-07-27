# Example: EXEC with Streaming

For commands that produce large output, stdout and stderr are streamed
in chunks while the command runs.

## Flow

```
Host                          Guest
  │                             │
  │── EXEC ────────────────────→│  (msg_id=2)
  │                             │  starts "find / -name '*.log'"
  │                             │
  │←─ EXEC_STDOUT (chunk 1) ───│  IS_STREAM_MORE, msg_id=2
  │←─ EXEC_STDOUT (chunk 2) ───│  IS_STREAM_MORE, msg_id=2
  │←─ EXEC_STDERR (chunk 1) ───│  IS_STREAM_MORE, msg_id=2
  │←─ EXEC_STDOUT (chunk 3) ───│  IS_STREAM_MORE, msg_id=2
  │                             │  process exits
  │←─ EXEC_RESULT ─────────────│  IS_RESPONSE, STREAM_MORE=0, msg_id=2
```

## EXEC Request

Same as the non-streaming case — the guest decides whether to stream
based on output size.

## EXEC_STDOUT Chunk (Guest → Host)

```
 length: 0x00_00_04_12   (1042 = 6 + 4 + 1032 payload)
   type: 0x12             (EXEC_STDOUT)
  flags: 0x02             (IS_STREAM_MORE — more chunks follow)
 msg_id: 0x00_00_00_02    (matches EXEC request)
payload:
  bytes  [1032 raw bytes] → 0x00_00_04_08 + raw stdout data
```

## EXEC_STDERR Chunk (Guest → Host)

```
 length: 0x00_00_00_6C   (108 = 6 + 4 + 98 payload)
   type: 0x13             (EXEC_STDERR)
  flags: 0x02             (IS_STREAM_MORE)
 msg_id: 0x00_00_00_02    (matches EXEC request)
payload:
  bytes  [98 raw bytes]  → 0x00_00_00_62 + raw stderr data
```

## Final EXEC_RESULT

The last chunk has `IS_STREAM_MORE` cleared. `EXEC_RESULT` arrives
with `IS_RESPONSE` and no `IS_STREAM_MORE`:

```
 length: 0x00_00_00_1C
   type: 0x11             (EXEC_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_02
payload:
  int32  0                → exit_code: 0
  bytes  ""               → stdout: empty in result (was streamed)
  bytes  ""               → stderr: empty in result (was streamed)
  uint32 1234             → duration_ms: 1234
```

## Go: Receiving Streaming Output

```go
var stdout, stderr bytes.Buffer

for {
    frame, _ := protocol.ReadFrame(conn)
    r := bytes.NewReader(frame.Payload)

    switch frame.Type {
    case protocol.TypeExecStdout:
        data, _ := protocol.ReadBytes(r)
        stdout.Write(data)
    case protocol.TypeExecStderr:
        data, _ := protocol.ReadBytes(r)
        stderr.Write(data)
    case protocol.TypeExecResult:
        exitCode, _ := protocol.ReadInt32(r)
        // streaming complete
        break
    }
}
```

## Notes

- Chunks arrive in order (SOCK_STREAM guarantee, no seq needed).
- `msg_id` ties all chunks to the original request.
- The guest sets `IS_STREAM_MORE` on every chunk except the last.
- `EXEC_RESULT` arrives after the last chunk with `IS_RESPONSE`.
- `EXEC_RESULT.stdout` and `EXEC_RESULT.stderr` are empty when
  streaming was used (output was delivered via chunks).
