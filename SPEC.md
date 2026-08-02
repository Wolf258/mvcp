# MVCP — MicroVM Communication Protocol (SPEC)

> A compact binary protocol purpose-built for guest↔host communication
> over Firecracker's vsock device, replacing JSON RPC on the vsock channel.

## Why MVCP

| Problem                   | JSON impact                                  | MVCP solution                                 |
|---------------------------|--------------------------------------------- |-----------------------------------------------|
| Base64 overhead           | +33% per byte for every file chunk            | Raw bytes, no encoding                        |
| Repeated string keys      | `"command"`, `"exit_code"` in every message  | 1-byte type + positional payload              |
| No request correlation    | Responses are order-dependent on the stream   | 4-byte `msg_id` enables pipelining            |
| No streaming primitives   | Each chunk is a full JSON parse round-trip    | `IS_STREAM_MORE` flag in frame header         |
| Parse overhead            | `json.Unmarshal` per frame                   | `binary.Read` with pre-allocated structs       |

## Quick Reference

### Protocol Architecture

MVCP has four layers:

```
┌──────────────────────────────────────────────────────────┐
│ SERVICE LAYER                                             │
│ Control / Execution / Events / Status / Console / …  │
├──────────────────────────────────────────────────────────┤
│ RPC LAYER (port 9000)                                    │
│ Request/Response │ Pipelining │ Streaming │ Timeouts     │
├──────────────────────┬───────────────────────────────────┤
│ MVCP wire format     │ VPP wire format                   │
│ type+flags+msg_id(6B)│ type(1B)                          │
├──────────────────────┴───────────────────────────────────┤
│ TRANSPORT (shared by all ports)                           │
│ ReadFrame / WriteFrame — length(4B BE) + payload          │
├───────────────────────────────────────────────────────────┤
│ HANDSHAKE (per protocol)                                  │
│ MVCP: "MVCP"+0x01 (5B)  |  VPP: "VPP"+0x01 (4B)         │
└───────────────────────────────────────────────────────────┘
```

The transport frame is identical for every port. The RPC layer is a
**software abstraction** on top of MVCP (not a separate wire format) that
formalises `msg_id` correlation and request/response semantics. Events
use MVCP directly. Status uses MVCP with periodic heartbeats + request/response
GET_STATUS/STATUS RPC. File transfer
uses MVCP with STARTED on INIT and verification on DONE.

### Service Ports (Shifty conventions)

The protocol is **port-agnostic** — ports are conventions, not
requirements. Any service can run on any port.

| Port | Service      | Protocol       | Direction              |
|------|------------- |----------------|----------------------- |
| 9000 | RPC          | MVCP + RPC     | Bidirectional          |
| 9001 | Console      | VPP            | Bidirectional          |
| 9002 | Events       | MVCP           | Guest → Host           |
| 9003 | Status       | MVCP           | Bidirectional          |
| 9004 | File Transfer| MVCP           | Host-initiated, bidir  |

### Message Categories (MVCP)

| Range         | Category     | Types                                                | Port          |
|--------------|-------------|------------------------------------------------------|-------------- |
| `0x00`–`0x0F` | Control      | PING, PONG, SHUTDOWN                                 | 9000          |
| `0x05`–`0x07` | Status       | GET_STATUS, STATUS, HEARTBEAT                        | 9003          |
| `0x10`–`0x1F` | Execution    | EXEC, EXEC_STREAM, EXEC_RESULT                  | 9000          |
| `0x20`–`0x2F` | File Transfer| XFER_INIT, XFER_CHUNK, XFER_DONE                      | 9004          |
| `0x30`–`0x3F` | Tools        | TOOL_CALL, TOOL_RESULT, LIST_TOOLS                    | 9000          |
| `0x40`–`0x4F` | VM Commands  | *(reserved)*                                          | —             |
| `0x80`–`0x8F` | Events       | EVENT_READY, EVENT_LOG, EVENT_ERROR, …               | 9002          |
| `0xFA`        | STARTED      | STARTED                                            | 9000/9004     |
| `0xFE`        | Error        | ERROR                                               | 9000/9004     |

## Documentation Index

### Reference

| Doc                                                            | Topic                                                                                          |
|----------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| [docs/01-transport.md](docs/01-transport.md)                   | vsock transport, unified transport frame, connection handshake (MVCP+VPP), service ports        |
| [docs/02-wire-format.md](docs/02-wire-format.md)               | Transport frame, MVCP header (type/flags/msg_id), VPP header, primitive encodings               |
| [docs/03-versioning.md](docs/03-versioning.md)                 | Protocol versioning strategy, compatibility policy                                              |
| [docs/04-error-codes.md](docs/04-error-codes.md)               | Error envelope (`0xFE`) and error code registry                                                |
| [docs/05-concurrency.md](docs/05-concurrency.md)               | Pipelining, streaming, head-of-line blocking, multiple connections                             |

### Services (by port / function)

| Doc                                                            | Topic                                                                                          |
|----------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| [docs/services/rpc.md](docs/services/rpc.md)                   | Port 9000: RPC layer — request/response, pipelining, streaming, timeouts                       |
| [docs/services/control.md](docs/services/control.md)           | Port 9000: Control — PING/PONG, SHUTDOWN                                    |
| [docs/services/status.md](docs/services/status.md)             | Port 9003: Status — GET_STATUS, STATUS, HEARTBEAT                           |
| [docs/services/execution.md](docs/services/execution.md)       | Port 9000: Execution — EXEC, EXEC_RESULT, streaming stdout/stderr                              |
| [docs/services/console.md](docs/services/console.md)           | Port 9001: VPP binary — interactive terminal protocol                                           |
| [docs/services/events.md](docs/services/events.md)             | Port 9002: Asynchronous event notifications                    |
| [docs/services/heartbeat.md](docs/services/heartbeat.md)       | *(legacy)* Port 9003: heartbeat spec — superseded by [status.md](docs/services/status.md) |
| [docs/services/vm-commands.md](docs/services/vm-commands.md)   | *(Planned)* VM-specific control operations                     |
| [docs/workload-model.md](docs/workload-model.md)               | Workload model: tools vs processes vs services, lifecycles, ownership, cleanup |
| [docs/services/file-transfer.md](docs/services/file-transfer.md)| Port 9004: Chunked file export/import — dedicated data plane, not part of RPC layer          |
| [docs/services/filesystem.md](docs/services/filesystem.md)     | *(Obsolete)* Superseded by Tools service on port 9000                                           |
| [docs/services/tools.md](docs/services/tools.md)               | Port 9000: LLM tools — generic TOOL_CALL dispatch, `read_file`/`write_file`/`bash`/`glob`/`grep`/`edit_file` |

### Examples

| Doc                                                            | Scenario                                                                                       |
|----------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| [docs/examples/handshake.md](docs/examples/handshake.md)       | Connection handshake (MVCP 5B + VPP 4B)                                                        |
| [docs/examples/ping-pong.md](docs/examples/ping-pong.md)       | PING/PONG liveness check                                                                       |
| [docs/examples/exec.md](docs/examples/exec.md)                 | EXEC command with result                                                                       |
| [docs/examples/exec-streaming.md](docs/examples/exec-streaming.md)| EXEC with streaming stdout/stderr                                                           |
| [docs/examples/file-export.md](docs/examples/file-export.md)   | Chunked file export (host-initiated, 3-type protocol)                                          |
| [docs/examples/heartbeat.md](docs/examples/heartbeat.md)       | Heartbeat sequence                                                                             |
| [docs/examples/error.md](docs/examples/error.md)               | Error responses                                                                                |
| [docs/examples/tool-calls.md](docs/examples/tool-calls.md)     | Tool call wire examples (read_file, write_file, bash, glob, grep, list_tools)                  |

## Design Decisions

| Decision                                                      | Rationale                                                                                                    |
|---------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------|
| Transport frame: 4B BE length prefix, shared by all ports      | Single `ReadFrame`/`WriteFrame` implementation on both host and guest. No duplicated framing code.            |
| MVCP and VPP share transport but differ in inner header        | VPP's 1B type header suits interactive keystrokes; MVCP's 6B header (type+flags+msg_id) suits structured RPC. |
| RPC layer on top of MVCP, not a separate wire format           | MVCP already provides `msg_id`, `IS_RESPONSE`, and `IS_STREAM_MORE`. RPC formalises how to use them.          |
| `length` includes all post-length bytes                       | Enables single `make([]byte, length)` + single `ReadFull` for the entire transport payload.                  |
| Handshake: magic + version, fire-and-forget from guest         | No round-trip needed; host validates and proceeds or closes.                                                 |
| MVCP handshake: "MVCP" (4B), VPP handshake: "VPP" (3B)        | Self-describing — the magic tells the host which protocol to speak on this connection.                        |
| `IS_ERROR` flag removed                                        | Error is signaled by `IS_RESPONSE` + `type=0xFE` — the type already indicates error.                         |
| `WANT_ACK` flag removed — replaced by `STARTED` (0xFA) | `STARTED` confirms the handler accepted the request for processing (not just dispatch). Sent after validation and process/tool start. |
| Events are fire-and-forget from guest                              | Guest pushes events; host reads and dispatches to subscribers. Events no longer carry `msg_id`.                           |
| File transfer: dedicated data plane (port 9004), separate from control plane (port 9000) | Large-file streaming should not compete with command/response traffic. Port 9004 has exclusive bandwidth for chunks. |
| Heartbeat: binary, 20B fixed header + TLV extensions | `boot_id` (uint64), `seq` (uint64), `state` (uint8), `flags` (uint8), `payload_length` (uint16) + optional TLV extensions. Future-proof — parsers skip unknown extension types. |
| Heartbeat interval: 1 second                                   | Fast enough for crash detection, slow enough to be negligible overhead (18 bytes/s).                           |
| GET_STATUS tuned for monitoring                                | Separate from operational RPC (port 9000). Status queries don't contend with execution streams.                |
| Strings: `uint16` length prefix                                | 64KB max covers all current use cases; file data uses `bytes` with `uint32` length.                           |
| No per-frame version byte                                      | Version is per-connection (handshake). When the protocol evolves, the handshake version changes.              |
| Head-of-line blocking during streaming                         | Accepted tradeoff. Multiple connections provide concurrency when needed without frame-interleaving complexity. |
| Port-agnostic design                                           | Any service can run on any port. Port numbers in these docs are Shifty conventions.                           |
| VPP enforces 64 KB frame limit                                 | Reasonable maximum for interactive terminal data; transport allows 16 MB but VPP rejects larger frames.       |
| Timeouts via context.Context, not wire-level fields            | Clients already carry deadlines via contexts. No need to duplicate in the protocol.                           |

## Module Structure

```
mvcp/
  go.mod              # module github.com/Wolf258/mvcp
  protocol/           # Go protocol implementation
    frame.go          # ReadFrame / WriteFrame (transport, 4B BE — shared by all ports)
    encode.go         # binary write primitives (shared by MVCP and VPP)
    decode.go         # binary read primitives (shared by MVCP and VPP)
    conn.go           # WriteHandshake / ValidateHandshake (MVCP + VPP)
    mvcp.go           # MVCP wire format: type/flags/msg_id constants, Frame struct, ReadMVCPFrame/WriteMVCPFrame
    message.go        # Message interface + decode registry
      messages/         # MVCP message structs + per-type Encode/Decode
        control.go
        exec.go
        file.go
        events.go
        status.go          # HEARTBEAT (0x07) + GET_STATUS (0x05) + STATUS (0x06)
        error.go
    rpc/              # RPC layer (request/response abstraction on MVCP)
      client.go       # RPC Client: Call, Stream, pending map, msg_id allocation
      server.go       # RPC Server: Handle, Serve, handler dispatch
      types.go        # Request, Response, StreamFrame types
    vpp/              # Virtual PTY Protocol (companion wire format)
      vpp.go          # VPP types/constants, ReadFrame/WriteFrame (uses transport + thin type header)
      types.go        # AttachMsg, SessionMsg, WinchMsg, DetachMsg + Encode/Decode
  sdk/                # Multi-language SDKs (planned)
    go/
    c/
    rust/
  docs/               # Protocol documentation (this directory)
```

## Adoption Guide (Shifty)

| Step | What                                                            | Where                    |
|------|-----------------------------------------------------------------|--------------------------|
| 1    | Implement `mvcp/protocol/{frame,encode,decode,conn}.go`        | `mvcp/protocol/`         |
| 2    | Implement `mvcp/protocol/{mvcp,message}.go` + `messages/`      | `mvcp/protocol/`         |
| 3    | Implement `mvcp/protocol/rpc/{client,server,types}.go`         | `mvcp/protocol/rpc/`     |
| 4    | Implement `mvcp/protocol/vpp/{vpp,types}.go`                   | `mvcp/protocol/vpp/`     |
| 5    | Write round-trip tests for every message type                  | `mvcp/protocol/*_test.go`|
| 6    | Remove `shared/vsock/frame.go` — use `mvcp/protocol/frame.go`  | `shared/`                |
| 7    | Remove `vhandler/frame.go` — use `mvcp/protocol/frame.go`      | `shifty-vhandler/`       |
| 8    | Replace `vhandler/rpc.go` JSON dispatcher with MVCP RPC server | `shifty-vhandler/`       |
| 9    | Migrate `vhandler/console.go` raw bytes → VPP frames            | `shifty-vhandler/`       |
| 10   | Migrate `vhandler/events.go` JSONL → MVCP frames                | `shifty-vhandler/`       |
| 11   | Migrate `vhandler/heartbeat.go` → MVCP status service (port 9003, bidirectional) | `shifty-vhandler/`       |
| 12   | Update `shared/vsock/rpc.go` → MVCP RPC client                  | `shared/`                |
| 13   | Update `shared/vsock/events.go` → MVCP event reader             | `shared/`                |
| 14   | Update `shiftyctl` to speak MVCP                                | `shifty-vhandler/shiftyctl/`|
| 15   | Remove legacy JSON RPC types and dead code                      | All modules              |

---

See also:
- [CHANGELOG.md](CHANGELOG.md) for version history.
- [../docs/agents/05-mvcp-protocol.md](../docs/agents/05-mvcp-protocol.md) for Shifty-specific integration notes.
