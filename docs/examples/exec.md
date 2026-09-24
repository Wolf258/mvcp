# Example: EXEC

Execute a command inside the guest VM and receive the result.

## EXEC (Host → Guest)

Command: `ls -la` in `/work`, no env vars, 120s timeout.

```
 length: 0x00_00_00_40   (64 = 6 + 58 body)
   type: 0x10             (EXEC)
  flags: 0x00
 msg_id: 0x00_00_00_02
 body: {"command":"ls -la","workdir":"/work","timeout_ms":120000}
```

### Payload Breakdown

The body is UTF-8 JSON (58 bytes, no length prefix):

| Field | JSON | Value |
|-------|------|-------|
| `command` | `"command":"ls -la"` | `"ls -la"` |
| `workdir` | `"workdir":"/work"` | `"/work"` |
| `timeout_ms` | `"timeout_ms":120000` | 120000 (120s) |

`workdir` is always absolute on the wire: core resolves relative paths
under `/work` and rejects escapes. `env` is omitted here (optional), and
`spec` is reserved — v1 rejects any non-empty payload.

## EXEC_RESULT (Guest → Host)

Command succeeded (exit 0), empty stdout/stderr, took 5ms.

```
 length: 0x00_00_00_16   (22 = 6 + 16 payload)
   type: 0x12             (EXEC_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_02    (matches request)
payload:
  int32  0                → 0x00_00_00_00 (exit_code)
  bytes  ""               → 0x00_00_00_00 (stdout)
  bytes  ""               → 0x00_00_00_00 (stderr)
  uint32 5                → 0x00_00_00_05 (duration_ms)
```

## Go: Sending EXEC

```go
timeoutMs := int64(120000)
body, _ := (&messages.ExecCmd{
    Command:   "ls -la",
    Workdir:   "/work",
    TimeoutMs: &timeoutMs,
}).MarshalBinary()
_ = protocol.WriteMVCPFrame(conn, &protocol.Frame{
    Type: protocol.TypeEXEC, Flags: 0, MsgID: 2, Body: body,
})
```

## Go: Reading EXEC_RESULT

```go
frame, _ := protocol.ReadMVCPFrame(conn)
r := bytes.NewReader(frame.Body)

exitCode, _  := protocol.ReadInt32(r)   // 0
stdout, _    := protocol.ReadBytes(r)
stderr, _    := protocol.ReadBytes(r)
duration, _  := protocol.ReadUint32(r)  // 5
```

## EXEC_RESULT Wire Size Comparison

| Format | Bytes |
|--------|-------|
| MVCP binary | 28 |
| JSON (with empty stdout/stderr) | ~230 |
| **Reduction** | **~88%** |

For commands with actual output, the saving is even larger since MVCP
transmits stdout/stderr as raw bytes (no base64 overhead).
