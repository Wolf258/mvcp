# Execution Service (Port 9000)

> Message type range `0x10`–`0x1F`. Carried on port 9000 (Control).

The execution service allows the host to run commands inside the guest
VM and receive results (exit code, stdout, stderr, duration).

## Message Types

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x10` | `EXEC` | H→G | `string command`, `string cwd`, `map env`, `uint32 timeout_ms` |
| `0x11` | `EXEC_RESULT` | G→H | `int32 exit_code`, `bytes stdout`, `bytes stderr`, `uint32 duration_ms` |
| `0x12` | `EXEC_STDOUT` | G→H | `bytes data` (streaming, `IS_STREAM_MORE` set until last chunk) |
| `0x13` | `EXEC_STDERR` | G→H | `bytes data` (streaming, `IS_STREAM_MORE` set until last chunk) |
| `0x14`–`0x1F` | *(reserved)* | — | — |

## EXEC Request (`0x10`)

| Field | Encoding | Description |
|-------|----------|-------------|
| `command` | `string` | Shell command to execute (passed to `/bin/sh -c`) |
| `cwd` | `string` | Working directory for the command |
| `env` | `map[string]string` | Environment variables (merged with guest defaults) |
| `timeout_ms` | `uint32` | Maximum execution time in milliseconds. 0 = no timeout. |

## EXEC_RESULT (`0x11`)

Sent as a response after the command completes. `IS_RESPONSE` flag set,
`msg_id` matches the `EXEC` request.

| Field | Encoding | Description |
|-------|----------|-------------|
| `exit_code` | `int32` | Process exit code. -1 if killed by signal. |
| `stdout` | `bytes` | Captured standard output |
| `stderr` | `bytes` | Captured standard error |
| `duration_ms` | `uint32` | Wall-clock execution time in milliseconds |

When the command output is small, `stdout` and `stderr` are included
inline in `EXEC_RESULT`. For large output, use streaming (below).

## Streaming: EXEC_STDOUT / EXEC_STDERR

For commands that produce large output, the guest can stream stdout
and stderr in chunks while the command runs:

1. Guest starts the command
2. As output is produced, guest sends `EXEC_STDOUT` and/or `EXEC_STDERR`
   frames with `IS_STREAM_MORE` flag set
3. All stream frames share the original request's `msg_id`
4. After the command exits, guest sends `EXEC_RESULT` with `IS_RESPONSE`
   and `IS_STREAM_MORE` cleared

**EXEC_STDOUT / EXEC_STDERR payload:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `data` | `bytes` | Chunk of output data |

## Wire Example

**EXEC** (host → guest):

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

**EXEC_RESULT** (guest → host):

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

**Total wire bytes: ~28.** JSON equivalent: ~230 bytes. ~88% reduction.

---

See also:
- [control.md](control.md) for the control service (port 9000) and other RPC messages.
- [examples/exec.md](../examples/exec.md) for the wire-level example.
- [examples/exec-streaming.md](../examples/exec-streaming.md) for the streaming example.
