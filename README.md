# MVCP — MicroVM Communication Protocol

A compact binary protocol purpose-built for guest↔host communication
over [Firecracker](https://firecracker-microvm.github.io/)'s vsock device.

## Features

- **No dependencies** — pure Go standard library
- **Compact encoding** — ~88% wire-size reduction vs JSON (for the EXEC case)
- **Streaming** — native chunked transfer with `IS_STREAM_MORE` flag (file I/O, exec stdout/stderr)
- **Pipelining** — 4-byte `msg_id` correlation token enables concurrent request/response
- **Versioned** — connection-level handshake with magic + version byte
- **Extensible** — 256 message type space, reserved extension marker `0xFF`
- **SDK-ready** — encoding/decoding primitives designed for straightforward C/Rust bindings
- **Port-agnostic** — any service can run on any port; port numbers are conventions
- **Unified transport** — single `ReadFrame`/`WriteFrame` for all ports and both protocols (MVCP + VPP)

## Structure

```
mvcp/
  go.mod              # module github.com/Wolf258/mvcp
  protocol/           # Go protocol implementation
    frame.go          # ReadFrame / WriteFrame (transport, 4B BE — shared by all ports)
    encode.go         # binary write primitives (shared by MVCP and VPP)
    decode.go         # binary read primitives (shared by MVCP and VPP)
    conn.go           # WriteHandshake / ValidateHandshake
    mvcp.go           # MVCP wire format: type/flags/msg_id constants, Frame struct
    message.go        # Message interface + decode registry
    messages/         # MVCP message structs + per-type Encode/Decode
    vpp/              # Virtual PTY Protocol (companion wire format)
      vpp.go          # VPP types, ReadFrame/WriteFrame (uses transport frame)
      types.go        # AttachMsg, SessionMsg, WinchMsg, DetachMsg
  sdk/                # Multi-language SDKs (planned)
    go/
    c/
    rust/
  docs/               # Protocol documentation
```

## Documentation

### Reference

| Doc | Topic |
|-----|-------|
| [SPEC.md](SPEC.md) | Protocol hub: quick reference, documentation index, design decisions, adoption guide |
| [docs/01-transport.md](docs/01-transport.md) | vsock transport, unified transport frame, handshake (MVCP + VPP), service ports |
| [docs/02-wire-format.md](docs/02-wire-format.md) | Transport frame, MVCP + VPP inner headers, flags, msg_id, primitive encodings |
| [docs/03-versioning.md](docs/03-versioning.md) | Protocol versioning strategy, compatibility policy |
| [docs/04-error-codes.md](docs/04-error-codes.md) | Error envelope (`0xFE`) and error code registry |
| [docs/05-concurrency.md](docs/05-concurrency.md) | Pipelining, streaming, head-of-line blocking, multiple connections |

### Services

| Doc | Service | Protocol |
|-----|---------|----------|
| [docs/services/control.md](docs/services/control.md) | Control: PING, PONG, SHUTDOWN, STATUS | MVCP |
| [docs/services/console.md](docs/services/console.md) | Console: VPP binary terminal protocol | VPP |
| [docs/services/events.md](docs/services/events.md) | Events: asynchronous notifications | MVCP |
| [docs/services/heartbeat.md](docs/services/heartbeat.md) | Heartbeat: periodic liveness | MVCP |
| [docs/services/execution.md](docs/services/execution.md) | Execution: EXEC, streaming stdout/stderr | MVCP |
| [docs/services/file-transfer.md](docs/services/file-transfer.md) | File transfer: chunked export/import | MVCP |
| [docs/services/filesystem.md](docs/services/filesystem.md) | *(Obsolete)* Superseded by Tools | MVCP |
| [docs/services/tools.md](docs/services/tools.md) | Tools: generic TOOL_CALL, read_file/write_file/bash/glob/grep | MVCP |
| [docs/services/vm-commands.md](docs/services/vm-commands.md) | VM-specific operations (`SYNC_FILESYSTEMS`) | MVCP |

### Examples

| Doc | Scenario |
|-----|----------|
| [docs/examples/handshake.md](docs/examples/handshake.md) | Connection handshake (MVCP 5B + VPP 4B) |
| [docs/examples/ping-pong.md](docs/examples/ping-pong.md) | PING/PONG liveness check |
| [docs/examples/exec.md](docs/examples/exec.md) | EXEC command with result |
| [docs/examples/exec-streaming.md](docs/examples/exec-streaming.md) | EXEC with streaming stdout/stderr |
| [docs/examples/file-export.md](docs/examples/file-export.md) | Chunked file export (host-initiated, 3-type protocol) |
| [docs/examples/heartbeat.md](docs/examples/heartbeat.md) | Heartbeat sequence |
| [docs/examples/error.md](docs/examples/error.md) | Error responses |
| [docs/examples/tool-calls.md](docs/examples/tool-calls.md) | Tool call wire examples (read_file, write_file, bash, glob, grep) |

## Quick Reference

### Architecture

```
┌──────────────────────────────────────────────────────┐
│ APPLICATION     Control / Console / Events / ...      │
├──────────────────────┬───────────────────────────────┤
│ MVCP wire format     │ VPP wire format               │
│ type+flags+msg_id(6B)│ type(1B)                      │
├──────────────────────┴───────────────────────────────┤
│ TRANSPORT (shared by all ports)                       │
│ length(4B BE) + payload                               │
└───────────────────────────────────────────────────────┘
```

### Transport Frame (all ports)

```
┌──────────┬─────────────────────────────┐
│ length   │ payload                     │
│ (4B BE)  │ (N bytes)                   │
└──────────┴─────────────────────────────┘
```

### MVCP Inner Header (ports 9000, 9002, 9003, 9004)

```
┌──────┬───────┬──────────┬──────────┐
│ type │ flags │ msg_id   │ body     │
│ (1B) │ (1B)  │ (4B BE)  │ (M)      │
└──────┴───────┴──────────┴──────────┘

payload = type(1) + flags(1) + msg_id(4) + body(M)
```

### VPP Inner Header (port 9001)

```
┌──────┬──────────┐
│ type │ body     │
│ (1B) │ (M)      │
└──────┴──────────┘

payload = type(1) + body(M)
```

### Connection Handshakes

```
MVCP (5 bytes, guest → host):  "MVCP" + 0x01
VPP  (4 bytes, guest → host):  "VPP"  + 0x01
```

### Service Ports (Shifty conventions)

The protocol is **port-agnostic**. These are conventions, not requirements.

| Port | Service | Protocol |
|------|---------|----------|
| 9000 | Control/RPC | MVCP |
| 9001 | Console | VPP |
| 9002 | Events | MVCP |
| 9003 | Heartbeat | MVCP |
| 9004 | File Transfer | MVCP |

### Message Categories (MVCP)

| Range | Category | Count |
|-------|----------|-------|
| `0x00–0x0F` | Control | 8 types (PING, PONG, SHUTDOWN, STATUS, HEARTBEAT, …) |
| `0x10–0x1F` | Execution | 4 types (EXEC, EXEC_RESULT, EXEC_STDOUT, EXEC_STDERR) |
| `0x20–0x2F` | File Transfer | 3 types (XFER_INIT, XFER_CHUNK, XFER_DONE) |
| `0x30–0x3F` | Tools | 4 types (TOOL_CALL, TOOL_RESULT, LIST_TOOLS, LIST_TOOLS_RESULT) |
| `0x80–0x8F` | Events | 5 types (EVENT_READY, EVENT_FILE_RECEIVED, EVENT_LOG, …) |
| `0xFE` | Error | 1 type |

## Status

Specification and Go implementation complete. Integrated in `shifty-core` (host) and
`shifty-vhandler` (guest). SDKs for Go, C, and Rust are planned.
Missing: comprehensive round-trip tests for all message types.

## License

MIT
