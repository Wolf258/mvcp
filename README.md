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

## Structure

```
mvcp/
  go.mod              # module github.com/Wolf258/mvcp
  protocol/           # Go protocol implementation (planned)
    wire.go           # frame format, type constants, flag constants
    encode.go         # binary write primitives
    decode.go         # binary read primitives
    message.go        # typed message structs + per-type Encode/Decode
    frame.go          # ReadFrame / WriteFrame (length-prefix layer)
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
| [docs/01-transport.md](docs/01-transport.md) | vsock transport, connection handshake, service ports, multiple connections |
| [docs/02-wire-format.md](docs/02-wire-format.md) | Frame layout, flags, msg_id, primitive encodings, encoding/decoding functions |
| [docs/03-versioning.md](docs/03-versioning.md) | Protocol versioning strategy, compatibility policy |
| [docs/04-error-codes.md](docs/04-error-codes.md) | Error envelope (`0xFE`) and error code registry |
| [docs/05-concurrency.md](docs/05-concurrency.md) | Pipelining, streaming, head-of-line blocking, multiple connections |

### Services

| Doc | Service | Port |
|-----|---------|------|
| [docs/services/control.md](docs/services/control.md) | Control: PING, PONG, SHUTDOWN, STATUS | 9000 |
| [docs/services/console.md](docs/services/console.md) | Console: PTY ↔ shell raw bytes | 9001 |
| [docs/services/events.md](docs/services/events.md) | Events: asynchronous notifications | 9002 |
| [docs/services/heartbeat.md](docs/services/heartbeat.md) | Heartbeat: periodic liveness | 9003 |
| [docs/services/execution.md](docs/services/execution.md) | Execution: EXEC, streaming stdout/stderr | 9000 |
| [docs/services/file-transfer.md](docs/services/file-transfer.md) | File transfer: chunked export/import | 9000 |
| [docs/services/filesystem.md](docs/services/filesystem.md) | Filesystem: STAT, LIST_DIR, READ/WRITE, CWD | 9000 |
| [docs/services/vm-commands.md](docs/services/vm-commands.md) | *(Planned)* VM-specific operations | 9000 |

### Examples

| Doc | Scenario |
|-----|----------|
| [docs/examples/handshake.md](docs/examples/handshake.md) | Connection handshake (5 bytes) |
| [docs/examples/ping-pong.md](docs/examples/ping-pong.md) | PING/PONG liveness check |
| [docs/examples/exec.md](docs/examples/exec.md) | EXEC command with result |
| [docs/examples/exec-streaming.md](docs/examples/exec-streaming.md) | EXEC with streaming stdout/stderr |
| [docs/examples/file-export.md](docs/examples/file-export.md) | Chunked file export with SHA256 |
| [docs/examples/heartbeat.md](docs/examples/heartbeat.md) | Heartbeat sequence |
| [docs/examples/error.md](docs/examples/error.md) | Error responses |

## Quick Reference

### Connection Handshake (5 bytes, guest → host)

```
┌─────────────────┬─────────┐
│ magic (4B)      │ version │
│ 'M' 'V' 'C' 'P' │  0x01   │
└─────────────────┴─────────┘
```

### Frame Layout

```
┌──────────┬──────┬───────┬──────────┬──────────┐
│ length   │ type │ flags │ msg_id   │ payload  │
│ (4B BE)  │ (1B) │ (1B)  │ (4B BE)  │ (N)      │
└──────────┴──────┴───────┴──────────┴──────────┘

length = 1(type) + 1(flags) + 4(msg_id) + len(payload)
```

### Service Ports

| Port | Service | Protocol |
|------|---------|----------|
| 9000 | Control (RPC) | MVCP binary |
| 9001 | Console | Raw bytes |
| 9002 | Events | MVCP binary |
| 9003 | Heartbeat | MVCP binary |

### Message Categories

| Range | Category | Count |
|-------|----------|-------|
| `0x00–0x0F` | Control | 8 types (PING, PONG, SHUTDOWN, STATUS, HEARTBEAT, …) |
| `0x10–0x1F` | Execution | 4 types (EXEC, EXEC_RESULT, EXEC_STDOUT, EXEC_STDERR) |
| `0x20–0x2F` | File Transfer | 6 types (EXPORT_REQ, EXPORT_CHUNK, IMPORT_REQ, …) |
| `0x30–0x3F` | Filesystem | 10 types (STAT, LIST_DIR, READ_FILE, WRITE_FILE, …) |
| `0x80–0x8F` | Events | 5 types (EVENT_READY, EVENT_FILE_RECEIVED, EVENT_LOG, …) |
| `0xFE` | Error | 1 type |

## Status

Planned — protocol specification complete, implementation pending.

## License

MIT
