# 03 — Versioning

## Connection-Level Version

MVCP uses a connection-level version byte in the handshake:

```
┌─────────────────┬─────────┐
│ magic (4B)      │ version │
│ 'M' 'V' 'C' 'P' │  0x01   │
└─────────────────┴─────────┘
```

The guest sends its protocol version immediately after accept. The host
validates it and either proceeds or closes the connection.

## Version Negotiation

- **Current version: `0x01`**
- Guest and host in Shifty are compiled from the same repository, so
  they are always in sync. No runtime negotiation is needed.
- If a version mismatch occurs (e.g., old guest binary with new host),
  the host closes the connection. The guest can detect this and emit a
  log event.

## Compatibility Policy

- **v1**: Initial wire format as specified in this document.
- **v2+**: When the protocol evolves, the version byte in the handshake
  changes. All message types on that connection use the new version's
  wire format.

The following **will NOT** trigger a major version bump:
- Adding new message types (existing types unchanged)
- Adding new error codes
- Adding new event types
- Adding new flags (reserved bits → defined)

The following **WILL** trigger a major version bump:
- Changing the frame header layout
- Changing primitive encoding sizes
- Changing `length` field semantics
- Removing or renumbering existing message types
- Changing port allocations

## Backward Compatibility

MVCP is designed for greenfield deployment where both peers are updated
atomically. Backward compatibility is not a goal for v1 — the guest
binary is shipped with the rootfs, and the host is part of the same
build.

If backward compatibility is needed in the future, the handshake can be
extended to allow the host to respond with its supported version range,
and the guest can select a compatible version. This would be a v2
feature.

## Version History

| Version | Date | Changes |
|---------|------|---------|
| `0x01` | — | Initial wire format. Connection handshake (magic + version). Frame: length(type+flags+msg_id+payload). `IS_ERROR` flag removed. Heartbeat migrated to binary. |

---

See also:
- [01-transport.md](01-transport.md) for the handshake wire format.
- [02-wire-format.md](02-wire-format.md) for the frame layout.
