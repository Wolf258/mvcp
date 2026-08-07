# Changelog

## Unreleased

### Documentation — removed WANT_ACK / MVCP_ACK references

- **WANT_ACK + MVCP_ACK removed from the spec**: Already replaced by
  `STARTED` (0xFA) in the design decisions. All docs now consistently
  reflect this: `02-wire-format.md`, `services/rpc.md`,
  `services/events.md`, `services/file-transfer.md`, and
  `examples/file-export.md`.
- **Flag collision clarified**: `FlagExecStreaming = 0x04` was `WANT_ACK`
  in early drafts. The code comment now documents this history.
- **Known gaps documented**: Test coverage (~15%), empty SDK stubs,
  empty `examples/` directory, unimplemented VM Commands service.

### Status Service — port 9003 becomes bidirectional

- **Port 9003 renamed**: Heartbeat → Status. The port now carries both periodic
  heartbeats (G→H, `msg_id=0`, type `0x07`) and status RPC (H→G→H, correlated
  `msg_id`).
- **GET_STATUS/STATUS moved**: `0x05`/`0x06` move from port 9000 (Control) to
  port 9003 (Status). Port 9000 is now purely operational (PING, SHUTDOWN,
  EXEC, TOOL_CALL); port 9003 is the monitoring/status channel.
- **Heartbeat unchanged**: still type `0x07`, `uint64 seq`, 1s interval,
  `msg_id=0`. The host still uses `time.Now()` for liveness detection.
- **Bidirectional multiplexing**: a single vsock connection carries both
  unidirectional heartbeat ticks and request/response status queries.
  The `type` byte disambiguates: `0x07` = heartbeat, `0x06` = STATUS response.
- **STATUS payload unchanged**: `string version`, `uint32 pid`, `bool shutting_down`.
- **Docs**: new `docs/services/status.md` supersedes `docs/services/heartbeat.md`;
  `control.md` cross-references status.md for `GET_STATUS`/`STATUS`.

### Tools Service — generic tool dispatch (port 9000)

- **Port 9000**: LLM-facing tool calls share the control plane with full RPC semantics (msg_id pipelining).
- **Generic dispatch model**: `TOOL_CALL`/`TOOL_RESULT` carry a `tool_name` string and
  opaque `params`/`result` bytes — custom tools can be registered without protocol changes.
- **Built-in tools**: `read_file`, `write_file`, `edit_file`, `glob`, `grep`, `bash`.
- **Introspection**: `LIST_TOOLS`/`LIST_TOOLS_RESULT` lets the host discover available tools.
- **Unary only**: all tool calls are request→response, no streaming. Head-of-line blocking
  never applies.
- **Error model**: `TOOL_RESULT(ok=false)` for tool-level failures; error envelope `0xFE`
  with `UNKNOWN_TOOL` (`0x0009`) for unregistered tools.
- **Range repurposed**: `0x30`–`0x3F` was "Filesystem (reserved)" — now "Tools" on port 9000.
  The `bash` tool internally forwards to the Execution service (port 9000 EXEC).

### File Transfer Service — simplified design (port 9004)

- **Separated from RPC layer**: File Transfer moves to dedicated port 9004.
  No `msg_id`, no RPC abstraction — pure MVCP wire with sender-push streaming.
- **Host always initiates**: the host decides whether to import or export.
  `XFER_INIT` carries a `dir` field (`0x00` import, `0x01` export).
- **Reduced message types**: 3 types (`XFER_INIT`, `XFER_CHUNK`, `XFER_DONE`)
  replace the previous 6 (`FILE_EXPORT_REQ/CHUNK/END`, `FILE_IMPORT_REQ/CHUNK/RESULT`).
- **No SHA256 verification**: simple and fast. Integrity at transport layer.
- **Error model**: mid-transfer failures signal by closing the connection.
  `XFER_DONE(ok=false)` for immediate failures (e.g. file not found).

### v1 (0x01) — Unified transport + VPP companion protocol

- **Unified transport frame**: all ports use a single `ReadFrame`/`WriteFrame`
  with 4-byte big-endian uint32 length prefix. MVCP and VPP are sibling
  wire formats that share the same transport framing and encoding primitives.
  Eliminates duplicated framing code between host and guest.
- **VPP now uses the 4-byte transport frame** (was 2-byte). The VPP inner
  header is just a 1-byte type — 5 bytes total overhead vs MVCP's 10.
  Interactive senders should batch small writes to amortise the header cost.
- **Port-agnostic design**: the protocol places no constraints on port
   numbers. Port assignments (9000–9004) are Shifty conventions.
- **Connection handshake**: MVCP ports use 5-byte handshake (`MVCP`+`0x01`);
  VPP ports use 4-byte handshake (`VPP`+`0x01`). The magic string tells
  the host which protocol to speak.
- **Shared encoding primitives**: `WriteString`, `ReadUint32`, etc. are
  implemented once in `mvcp/protocol/encode.go` and `decode.go`, used by
  both MVCP message structs and VPP message structs.
- **VPP frame limit**: enforced at 64 KB (transport allows 16 MB but
  VPP rejects console frames larger than 65535 bytes).
- **Module layout**: `mvcp/protocol/vpp/` is the VPP companion protocol
  package, reusing `protocol.ReadFrame`/`WriteFrame` from the parent.

### v1 (0x01) — Initial wire format

- **Connection handshake**: magic (`MVCP` 4B) + version (1B) at the start of every connection, sent by guest after accept. Host validates and proceeds or closes.
- **Frame header**: `length` now consistently equals `type(1) + flags(1) + msg_id(4) + payload(N)` — enables single `make + ReadFull` per frame.
- **`IS_ERROR` flag removed**. Errors are `IS_RESPONSE` + `type=0xFE`. The type already indicates an error.
- **Heartbeat migrated to binary**: type `0x07` with `uint64 seq` payload. 18 bytes vs ~80 JSON. Interval: 1 second.
- **New error code**: `0x0008` `BAD_VERSION` for version mismatch.
- **Max frame length**: 16 MB (`0x01000000`).
- **msg_id wrap**: resets to 1 on wrap or new connection.
- **Heartbeat seq wrap**: `uint64` is practically infinite; resets to 1 if wrap occurs.

### Initial scaffolding

- Repository structure: `mvcp/` module, `protocol/`, `sdk/`, `docs/`.
- Protocol specification: wire format, message type registry, encoding, service ports, events, concurrency.
