# 03 — Versioning

## Connection-Level Version

MVCP and VPP each use a connection-level version byte in their handshake:

```
MVCP handshake (5 bytes):   "MVCP" + 0x01
VPP  handshake (4 bytes):   "VPP"  + 0x01
```

The guest sends the appropriate handshake immediately after accept,
depending on which protocol the port expects. The host validates it
and either proceeds or closes the connection.

## Version Negotiation

- **Current version: `0x01`**
- Guest and host in Shifty are compiled from the same repository, so
  they are always in sync. No runtime negotiation is needed.
- If a version mismatch occurs (e.g., old guest binary with new host),
  the host closes the connection. The guest can detect this and emit a
  log event.

## Compatibility Policy

- **v1**: Initial wire format as specified in this document. Unified
  transport frame (4B length), MVCP inner header (6B), VPP inner header
  (1B).
- **v2+**: When the protocol evolves, the version byte in the handshake
  changes. All message types on that connection use the new version's
  wire format.

The following **will NOT** trigger a major version bump:
- Adding new message types (existing types unchanged)
- Adding new error codes
- Adding new event types
- Adding new flags (reserved bits → defined)
- Adding new VPP message types

The following **WILL** trigger a major version bump:
- Changing the transport frame header layout
- Changing MVCP or VPP inner header layout
- Changing primitive encoding sizes
- Changing `length` field semantics
- Removing or renumbering existing message types
- Changing port allocation conventions (breaking existing deployments)

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
| `0x01` | — | Unified transport frame (4B). MVCP inner header (type+flags+msg_id). VPP companion protocol (type-only). Port-agnostic design. |

---

See also:
- [01-transport.md](01-transport.md) for the handshake wire format and transport frame.
- [02-wire-format.md](02-wire-format.md) for the MVCP and VPP frame layouts.
