# Example: EXEC

Execute a command inside the guest VM and receive the result.

## EXEC (Host → Guest)

Command: `ls -la` in `/home/user`, no env vars, 30s timeout.

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

### Payload Breakdown

| Offset | Bytes | Field | Value |
|--------|-------|-------|-------|
| 0 | `00 06` | command length | 6 |
| 2 | `6c 73 20 2d 6c 61` | command | "ls -la" |
| 8 | `00 0b` | cwd length | 11 |
| 10 | `2f 68 6f 6d 65 2f 75 73 65 72` | cwd | "/home/user" |
| 20 | `00 00` | env entries | 0 |
| 22 | `00 00 75 30` | timeout_ms | 30000 |

## EXEC_RESULT (Guest → Host)

Command succeeded (exit 0), empty stdout/stderr, took 5ms.

```
 length: 0x00_00_00_1C   (28 = 6 + 22 payload)
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
var buf bytes.Buffer
protocol.WriteString(&buf, "ls -la")     // command
protocol.WriteString(&buf, "/home/user") // cwd
protocol.WriteStringMap(&buf, nil)       // env
protocol.WriteUint32(&buf, 30000)        // timeout_ms

frame := &protocol.Frame{
    Type:    protocol.TypeEXEC,
    Flags:   0,
    MsgID:   2,
    Body:    buf.Bytes(),
}
frame.WriteTo(conn)
```

## Go: Reading EXEC_RESULT

```go
frame, _ := protocol.ReadFrame(conn)
r := bytes.NewReader(frame.Payload)

exitCode, _  := protocol.ReadInt32(r)   // 0
stdout, _    := protocol.ReadBytes(r)
stderr, _    := protocol.ReadBytes(r)
duration, _  := protocol.ReadUint32(r)  // 5
```

## Wire Size Comparison

| Format | Bytes |
|--------|-------|
| MVCP binary | 28 |
| JSON (with empty stdout/stderr) | ~230 |
| **Reduction** | **~88%** |

For commands with actual output, the saving is even larger since MVCP
transmits stdout/stderr as raw bytes (no base64 overhead).
