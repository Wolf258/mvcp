# Changelog

## Unreleased

### v1 (0x01) — Unified transport + VPP companion protocol

- **Unified transport frame**: all ports use a single `ReadFrame`/`WriteFrame`
  with 4-byte big-endian uint32 length prefix. MVCP and VPP are sibling
  wire formats that share the same transport framing and encoding primitives.
  Eliminates duplicated framing code between host and guest.
- **VPP now uses the 4-byte transport frame** (was 2-byte). The VPP inner
  header is just a 1-byte type — 5 bytes total overhead vs MVCP's 10.
  Interactive senders should batch small writes to amortise the header cost.
- **Port-agnostic design**: the protocol places no constraints on port
  numbers. Port assignments (9000–9003) are Shifty conventions.
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
