# 03 — Versioning

## Connection-Level Version

MVCP and VPP each use a connection-level wire version byte in their
handshake prefix:

```
MVCP prefix (5 bytes):   "MVCP" + 0x01, followed by a HELLO frame
VPP  prefix (4 bytes):   "VPP"  + 0x01
```

The guest agent sends the appropriate prefix immediately after accept,
depending on which protocol the port expects. The host validates it,
then the MVCP handshake continues with a bidirectional HELLO exchange.
The prefix version byte is the **single source of truth** for the wire
format (frame layout, headers, primitive encodings, HELLO encoding);
there is no version field inside HELLO.

## Capability Negotiation

Wire compatibility is a floor, not a policy. Core (host) and vhandler
(guest) may run different software versions; feature compatibility is
decided **per connection** by the HELLO capability exchange: each side
advertises the capability revision ranges it supports, both compute the
same intersection, and each service enforces its own requirements. A
connection can be rejected with a specific `ERROR` code when
requirements cannot be met.

See [06-negotiation.md](06-negotiation.md) for the full design.

## Compatibility Policy

- **v1**: Wire format as specified in this document. Unified transport
  frame (4B length), MVCP inner header (6B), VPP inner header (1B).
  The HELLO handshake was added in-place — there is no stable public
  wire version to preserve, so the version byte stays `0x01`.
- **v2+**: When the wire format evolves (see below), the prefix version
  byte changes. All message types on that connection use the new
  version's wire format.

The following **will NOT** trigger a wire version bump:
- Adding new message types (existing types unchanged)
- Adding new error codes
- Adding new event types
- Adding new flags (reserved bits → defined)
- Adding new VPP message types
- Adding new capabilities or extending capability revision ranges

The following **WILL** trigger a wire version bump:
- Changing the transport frame header layout
- Changing MVCP or VPP inner header layout
- Changing primitive encoding sizes
- Changing `length` field semantics
- Removing or renumbering existing message types
- Changing the HELLO wire encoding
- Changing port allocation conventions (breaking existing deployments)

## Backward Compatibility

Compatibility between peers is decided by **capability negotiation**,
not version matching. Because unknown capability IDs are ignored,
adding a new capability is always backward compatible: an old peer
simply does not negotiate it.

The HELLO handshake change is intentionally incompatible with the
previous fire-and-forget exchange. This is acceptable during early
development: no stable public wire version exists to preserve, and
core/vhandler ship together in practice. Mixed old/new peers fail the
handshake with a timeout or close.

## Version History

| Version | Date | Changes |
|---------|------|---------|
| `0x01` | — | Unified transport frame (4B). MVCP inner header (type+flags+msg_id). VPP companion protocol (type-only). Port-agnostic design. |
| `0x01` (in-place) | — | Handshake extended: prefix + bidirectional HELLO capability negotiation (role, software version, capability revision ranges). No version bump — no stable public wire version existed. See [06-negotiation.md](06-negotiation.md). |

---

See also:
- [01-transport.md](01-transport.md) for the handshake wire format and transport frame.
- [02-wire-format.md](02-wire-format.md) for the MVCP and VPP frame layouts.
- [06-negotiation.md](06-negotiation.md) for the handshake and capability negotiation design.
