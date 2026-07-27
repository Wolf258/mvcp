# MVCP — MicroVM Communication Protocol (SPEC)

> A compact binary protocol purpose-built for guest↔host communication
> over Firecracker's vsock device, replacing JSON RPC on the vsock channel.

## Why MVCP

| Problem | JSON impact | MVCP solution |
|---------|-------------|---------------|
| Base64 overhead | +33% per byte for every file chunk | Raw bytes, no encoding |
| Repeated string keys | `"command"`, `"exit_code"` in every message | 1-byte type + positional payload |
| No request correlation | Responses are order-dependent on the stream | 4-byte `msg_id` enables pipelining |
| No streaming primitives | Each chunk is a full JSON parse round-trip | `IS_STREAM_MORE` flag in frame header |
| Parse overhead | `json.Unmarshal` per frame | `binary.Read` with pre-allocated structs |

## Quick Reference

### Service Ports

| Port | Service | Protocol | Direction |
|------|---------|----------|-----------|
| 9000 | Control (RPC) | MVCP binary | Bidirectional H↔G |
| 9001 | Console | Raw bytes (PTY ↔ shell) | Bidirectional H↔G |
| 9002 | Events | MVCP binary (one-way) | Guest → Host |
| 9003 | Heartbeat | MVCP binary (one-way) | Guest → Host |

### Message Categories

| Range | Category | Types | Port |
|-------|----------|-------|------|
| `0x00`–`0x0F` | Control | PING, PONG, SHUTDOWN, STATUS, HEARTBEAT | 9000 / 9003 |
| `0x10`–`0x1F` | Execution | EXEC, EXEC_RESULT, EXEC_STDOUT, EXEC_STDERR | 9000 |
| `0x20`–`0x2F` | File Transfer | FILE_EXPORT/IMPORT (chunked) | 9000 |
| `0x30`–`0x3F` | Filesystem | STAT, LIST_DIR, READ/WRITE, CWD | 9000 |
| `0x40`–`0x4F` | VM Commands | *(reserved — planned)* | 9000 |
| `0x80`–`0x8F` | Events | EVENT_READY, EVENT_LOG, EVENT_ERROR, … | 9002 |
| `0xFE` | Error | ERROR | 9000 |

## Documentation Index

### Reference

| Doc | Topic |
|-----|-------|
| [docs/01-transport.md](docs/01-transport.md) | vsock transport, connection handshake, service ports, multiple connections |
| [docs/02-wire-format.md](docs/02-wire-format.md) | Frame layout, flags, msg_id semantics, primitive encodings, encoding/decoding functions |
| [docs/03-versioning.md](docs/03-versioning.md) | Protocol versioning strategy, compatibility policy |
| [docs/04-error-codes.md](docs/04-error-codes.md) | Error envelope (`0xFE`) and error code registry |
| [docs/05-concurrency.md](docs/05-concurrency.md) | Pipelining, streaming, head-of-line blocking, multiple connections |

### Services (by port / function)

| Doc | Topic |
|-----|-------|
| [docs/services/control.md](docs/services/control.md) | Port 9000: PING/PONG, SHUTDOWN, GET_STATUS/STATUS |
| [docs/services/console.md](docs/services/console.md) | Port 9001: PTY ↔ shell raw byte bridge |
| [docs/services/events.md](docs/services/events.md) | Port 9002: Asynchronous event notifications |
| [docs/services/heartbeat.md](docs/services/heartbeat.md) | Port 9003: Periodic liveness heartbeat |
| [docs/services/execution.md](docs/services/execution.md) | Command execution: EXEC, EXEC_RESULT, streaming stdout/stderr |
| [docs/services/file-transfer.md](docs/services/file-transfer.md) | Chunked file export/import with SHA256 |
| [docs/services/filesystem.md](docs/services/filesystem.md) | Filesystem metadata: STAT, LIST_DIR, READ/WRITE, CWD |
| [docs/services/vm-commands.md](docs/services/vm-commands.md) | *(Planned)* VM-specific control operations |

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

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| `length` includes header bytes (type+flags+msg_id) | Enables single `make([]byte, length)` + single `ReadFull` for the entire frame |
| Handshake: magic + version, fire-and-forget from guest | No round-trip needed; host validates and proceeds or closes |
| `IS_ERROR` flag removed | Error is signaled by `IS_RESPONSE` + `type=0xFE` — the type already indicates error |
| Heartbeat: binary, `uint64 seq` only | Host uses `time.Now()` at reception for liveness; version doesn't change during VM lifetime |
| Heartbeat interval: 1 second | Fast enough for crash detection, slow enough to be negligible overhead (18 bytes/s) |
| Strings: `uint16` length prefix | 64KB max covers all current use cases; file data uses `bytes` with `uint32` length |
| No per-frame version byte | Version is per-connection (handshake). When the protocol evolves, the handshake version changes for all message types on that connection |
| Head-of-line blocking during streaming | Accepted tradeoff. Multiple connections provide concurrency when needed without frame-interleaving complexity |

## Module Structure

```
mvcp/
  go.mod              # module github.com/Wolf258/mvcp
  protocol/           # Go protocol implementation
    wire.go           # frame format, type constants, flag constants
    encode.go         # binary write primitives
    decode.go         # binary read primitives
    message.go        # typed message structs + per-type Encode/Decode
    frame.go          # ReadFrame / WriteFrame (length-prefix layer)
  sdk/                # Multi-language SDKs (planned)
    go/
    c/
    rust/
  docs/               # Protocol documentation (this directory)
```

## Adoption Guide (Shifty)

| Step | What | Where |
|------|------|-------|
| 1 | Create `mvcp/` module with `protocol/` package | Repo root |
| 2 | Add `use ./mvcp` to `go.work` | `go.work` |
| 3 | Implement `mvcp/protocol/{wire,encode,decode,message,frame}.go` | `mvcp/` |
| 4 | Write round-trip tests for every message type | `mvcp/protocol/*_test.go` |
| 5 | Refactor `shared/infrastructure/vsock/` to use `mvcp/protocol` | `shared/` |
| 6 | Replace `shifty-vhandler/rpc.go` JSON dispatcher with MVCP dispatcher | `shifty-vhandler/` |
| 7 | Migrate `shifty-vhandler/events.go` JSONL → MVCP frames | `shifty-vhandler/` |
| 8 | Migrate `shifty-vhandler/heartbeat.go` JSON → MVCP binary | `shifty-vhandler/` |
| 9 | Update host-side clients (`VMStream`, `HealthCheckWatcher`) | `shifty-core/` |
| 10 | Update `shifty-vhandler/shiftyctl/` to speak MVCP | `shifty-vhandler/shiftyctl/` |
| 11 | Remove legacy JSON RPC types and dead code | All modules |

---

See also:
- [CHANGELOG.md](CHANGELOG.md) for version history.
- [../docs/agents/05-mvcp-protocol.md](../docs/agents/05-mvcp-protocol.md) for Shifty-specific integration notes.
