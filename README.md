# MVCP — MicroVM Communication Protocol

A compact binary protocol purpose-built for guest↔host communication
over [Firecracker](https://firecracker-microvm.github.io/)'s vsock device.

## Features

- **No dependencies** — pure Go standard library
- **Compact encoding** — ~88% wire-size reduction vs JSON (for the EXEC case)
- **Streaming** — native chunked transfer with `IS_STREAM_MORE` flag (file I/O, exec stdout/stderr)
- **Pipelining** — 4-byte `msg_id` correlation token enables concurrent request/response
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

| Doc | Topic |
|-----|-------|
| [SPEC.md](SPEC.md) | Complete protocol specification — wire format, message types, encoding, ports, events, concurrency, adoption guide |
| [docs/wire-format.md](docs/wire-format.md) | Wire format, primitive encodings, and wire examples (detailed reference) |
| [docs/error-codes.md](docs/error-codes.md) | Error type and error code registry |
| [docs/versioning.md](docs/versioning.md) | Protocol versioning strategy (TBD) |

## Quick Reference

### Frame Layout

```
┌──────────────────┬──────┬───────┬──────────┬──────────────────────┐
│ length (4B BE)   │ type │ flags │ msg_id   │ payload (N bytes)    │
│                  │ (1B) │ (1B)  │ (4B BE)  │                      │
└──────────────────┴──────┴───────┴──────────┴──────────────────────┘
```

### Service Ports

| Port | Service | Protocol |
|------|---------|----------|
| 9000 | Control | MVCP binary |
| 9001 | Console | Raw bytes |
| 9002 | Events | MVCP binary |
| 9003 | Heartbeat | JSON |

### Message Categories

| Range | Category | Count |
|-------|----------|-------|
| `0x00–0x0F` | Control | 7 types (PING, PONG, SHUTDOWN, STATUS, …) |
| `0x10–0x1F` | Execution | 4 types (EXEC, EXEC_RESULT, EXEC_STDOUT, EXEC_STDERR) |
| `0x20–0x2F` | File Transfer | 6 types (EXPORT_REQ, EXPORT_CHUNK, IMPORT_REQ, …) |
| `0x30–0x3F` | Filesystem | 10 types (STAT, LIST_DIR, READ_FILE, WRITE_FILE, …) |
| `0x80–0x8F` | Events | 5 types (EVENT_READY, EVENT_FILE_RECEIVED, EVENT_LOG, …) |
| `0xFE` | Error | 1 type |

## Status

Planned — protocol specification complete, implementation pending.

## License

MIT
