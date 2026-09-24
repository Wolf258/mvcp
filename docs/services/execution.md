# Execution Service (Port 9000)

> Message type range `0x10`–`0x1F`. Carried on port 9000 via the RPC layer.

The execution service allows the host to run commands inside the guest
VM and receive results (exit code, stdout, stderr, duration). It uses
the RPC layer for request/response correlation and streaming.

See [rpc.md](rpc.md) for the request/response contract (pipelining,
streaming, timeouts, error handling).

## Message Types

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x10` | `EXEC` | H→G | JSON body: `command` (string), `workdir` (string absoluto), `env` (objeto, opcional), `timeout_ms` (entero positivo, opcional; ausente = default de política; `0`/negativo = `BAD_PAYLOAD`), `spec` (reservado; v1 lo rechaza) |
| `0x11` | `EXEC_STREAM` | G→H | `uint8 channel`, `uint32 sequence`, `bytes data` |
| `0x12` | `EXEC_RESULT` | G→H | `int32 exit_code`, `bytes stdout`, `bytes stderr`, `uint32 duration_ms` |
| `0x13`–`0x1F` | *(reserved)* | — | — |

## EXEC Request (`0x10`)

### EXEC body (JSON)

    {
      "command": "pytest -q",
      "workdir": "/work/pkg/foo",
      "env": {"FOO": "1"},
      "timeout_ms": 120000
    }

`workdir` siempre llega absoluto: core resuelve relativos bajo `/work` y
rechaza escapes; el guest rechaza cualquier path no absoluto o no limpio.
`spec` es el punto de extensión de `ExecSpec`: v1 no lo emite y el guest
responde `BAD_PAYLOAD` si viene con payload (fail-closed). `EXECSTREAM`
(0x11) y `EXEC_RESULT` (0x12) no cambian: exit code, stdout, stderr y
duración siguen binarios.

### Flags

| Flag | Bit | Description |
|------|-----|-------------|
| `FlagExecStreaming` | `0x04` | Host requests incremental output delivery via `EXEC_STREAM`. Without this flag, all output is buffered and returned inline in `EXEC_RESULT`. |

## EXEC_STREAM (`0x11`)

Streaming output chunks sent by the guest while the command runs.
All chunks share the original request's `msg_id` and carry `IS_STREAM_MORE`.
The guest always uses pipes internally; the flag only controls whether
chunks are sent immediately or buffered for the final result.

| Field | Encoding | Description |
|-------|----------|-------------|
| `channel` | `uint8` | Output channel identifier (see below) |
| `sequence` | `uint32` | Monotonic counter per-request, for observability and replay |
| `data` | `bytes` | Raw output chunk |

### ExecChannel Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `ExecChannelStdout` | `0x00` | Standard output |
| `ExecChannelStderr` | `0x01` | Standard error |
| *(reserved)* | `0x02`–`0x0F` | Future standard I/O channels |
| `ExecChannelProgress` | `0x10` | Progress updates |
| `ExecChannelLog` | `0x11` | Structured logs |
| `ExecChannelMetric` | `0x12` | Metrics |
| `ExecChannelDebug` | `0x13` | Debug output |
| *(reserved)* | `0x14`–`0x7F` | Future standard channels |
| *(vendor)* | `0x80`–`0xFF` | Implementation-defined |

## EXEC_RESULT (`0x12`)

Sent as a response after the command completes. `IS_RESPONSE` flag set,
`msg_id` matches the `EXEC` request.

| Field | Encoding | Description |
|-------|----------|-------------|
| `exit_code` | `int32` | Process exit code. -1 if killed by signal. |
| `stdout` | `bytes` | Captured standard output (empty when streaming was used) |
| `stderr` | `bytes` | Captured standard error (empty when streaming was used) |
| `duration_ms` | `uint32` | Wall-clock execution time in milliseconds |

The wire format is always the same (all four fields). When streaming
(`FlagExecStreaming`) was requested, `stdout` and `stderr` are empty
bytes — the output was already delivered via `EXEC_STREAM` chunks.

## Streaming Flow

### Without Streaming (`FlagExecStreaming` not set)

```
Host                           Guest
  │                              │
  │── EXEC ────────────────────→│  (flags=0x00)
  │                              │  cmd.Start()
  │←─ STARTED ──────────────────│  (stream=false)
  │                              │  ... read pipes, buffer output ...
  │                              │  cmd.Wait()
  │←─ EXEC_RESULT ──────────────│  (IS_RESPONSE, stdout+stderr inline)
```

### With Streaming (`FlagExecStreaming` set)

```
Host                           Guest
  │                              │
  │── EXEC ────────────────────→│  (flags=0x04, FlagExecStreaming)
  │                              │  cmd.Start()
  │←─ STARTED ──────────────────│  (stream=true)
  │←─ EXEC_STREAM (stdout) ─────│  IS_STREAM_MORE, channel=0x00, seq=0
  │←─ EXEC_STREAM (stderr) ─────│  IS_STREAM_MORE, channel=0x01, seq=1
  │←─ EXEC_STREAM (stdout) ─────│  IS_STREAM_MORE, channel=0x00, seq=2
  │                              │  cmd.Wait()
  │←─ EXEC_RESULT ──────────────│  IS_RESPONSE, stdout/stderr empty
```

## Guest Behavior

The guest **always** uses `StdoutPipe()` and `StderrPipe()` with fixed-size
chunk reads (32 KB). The execution path is identical regardless of the
streaming flag — the only difference is the destination of each chunk:

- **Streaming:** chunk is sent immediately via `EXEC_STREAM`.
- **Buffered:** chunk is accumulated in memory. If the combined buffer
  exceeds 16 MB, execution is aborted with an error.

This guarantees the command never blocks due to a full pipe buffer,
even without streaming.

## Managed vs One-Shot Execution

`EXEC` is a **one-shot** primitive — it runs a command and returns the result.
There is no identity, no lifecycle, and no management surface (inspect, logs,
kill, restart, attach).

For **managed processes** (agent-spawned workloads with identity, PTY,
triggers, and persistence), see the [workload model](../workload-model.md).
Managed processes are built on top of the execution primitives but add
lifecycle supervision, observability, and ownership tracking in the vhandler.

## Wire Examples

**EXEC** (host → guest):

```
 length: 0x00_00_00_40   (64 = 6 + 58 body)
   type: 0x10             (EXEC)
  flags: 0x00
 msg_id: 0x00_00_00_02
 body: {"command":"ls -la","workdir":"/work","timeout_ms":120000}
```

**EXEC_STREAM** (guest → host), stdout chunk:

```
 length: 0x00_00_04_0F   (1039 = 6 + 1033 payload)
   type: 0x11             (EXECSTREAM)
  flags: 0x02             (IS_STREAM_MORE)
 msg_id: 0x00_00_00_02    (matches request)
payload:
  uint8  0x00             → ExecChannelStdout
  uint32 0x00_00_00_00    → sequence 0
  bytes  [1024 bytes]     → raw stdout data
```

**EXEC_RESULT** (guest → host), non-streaming:

```
 length: 0x00_00_00_1C   (28 = 6 + 22 payload)
   type: 0x12             (EXEC_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_02    (matches request)
payload:
  int32  0                → exit_code: 0
  bytes  ""               → stdout (empty)
  bytes  ""               → stderr (empty)
  uint32 5                → duration_ms: 5
```

**EXEC_RESULT total wire bytes: ~28.** JSON equivalent: ~230 bytes. ~88% reduction.

---

See also:
- [rpc.md](rpc.md) for the RPC layer contract (pipelining, streaming, timeouts, error handling).
- [control.md](control.md) for the control service (port 9000) and other RPC messages.
- [examples/exec.md](../examples/exec.md) for the wire-level example.
- [examples/exec-streaming.md](../examples/exec-streaming.md) for the streaming example.
