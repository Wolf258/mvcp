# Changelog

## Unreleased

### v1 (0x01) — Refined wire format

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
