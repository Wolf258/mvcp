# 06 — Handshake & Capability Negotiation

**Status**: design contract. Replaces the fire-and-forget magic+version
handshake on MVCP ports (9000, 9002, 9003, 9004). Code implementation
is tracked separately; this document is the normative design.

## 1. Goals

- Core (host) and vhandler (guest) may run **different software
  versions**. Compatibility is decided by **capability negotiation**,
  not by version matching.
- Keep the protocol simple: no full SemVer exchange, no feature
  bitmasks for now. Capability **revisions** are the granularity of
  compatibility.
- No backward-compatibility burden: there is no stable public wire
  version yet, so the handshake changes **in place** while the wire
  version stays `0x01`.

## 2. Wire Version Policy

The handshake prefix stays unchanged:

```
MVCP:  "MVCP" + 0x01   (5 bytes, ports 9000/9002/9003/9004)
VPP:   "VPP"  + 0x01   (4 bytes, port 9001)
```

- The prefix version byte is the **single source of truth** for the
  wire format: transport frame layout, MVCP/VPP inner headers,
  primitive encodings, and the HELLO encoding itself.
- **There is no version field inside HELLO.**
- The handshake change is incompatible with the previous one but does
  **not** bump the wire version: there is no public/stable version to
  preserve. Mixed old/new peers fail the handshake (timeout or close),
  which is acceptable during early development.
- A future change to frame layout, header layout, primitive sizes, the
  HELLO encoding, or anything else needed to parse messages **will**
  bump the prefix version byte.

## 3. Handshake Flow

Invariant preserved: **the vhandler (guest agent) writes first** on
every MVCP port, regardless of who initiated the connection. Core and
shiftyctl are always the reading side.

```
vhandler (ServerHandshake):            core / shiftyctl (ClientHandshake):

  write prefix "MVCP" + 0x01             read prefix
  write frame(HELLO, own table)          │ bad magic/version → close (no ERROR)
                                         read frame(HELLO)
                                         │ malformed / role ∉ expected →
                                         │   close without replying (own prefix
                                         │   not yet written, so an ERROR frame
                                         │   would be wire-invalid)
                                         write prefix "MVCP" + 0x01
                                         write frame(HELLO, own table)

  read prefix
  │ bad magic/version → close (no ERROR)
  read frame(HELLO)
  │ decode fails → send ERROR(BAD_PAYLOAD) → close
  compute negotiation
  │ role ∉ accepted → send ERROR(UNEXPECTED_ROLE) → close
  │ requirements fail → send ERROR(NO_COMMON_CAPABILITY) → close
  → connection established
  (client: requirements fail after writing its hello →
   send ERROR(NO_COMMON_CAPABILITY) → close)
```

- HELLO is the **first MVCP frame after the prefix**:
  `type = 0x00`, `flags = 0x00`, `msg_id = 0`.
- Handshake ERROR replies use the standard error frame:
  `type = 0xFE`, `flags = IS_RESPONSE (0x01)`, `msg_id = 0` (echoing
  HELLO), body = `uint16 code` + `string message`.
- **Rejection visibility**: a side sends an ERROR frame only once its
  own prefix is on the wire (always true for the vhandler; true for
  core only after it has accepted the peer's HELLO). The rejecting
  side returns a `HandshakeError` locally; the peer either receives
  the ERROR or observes the closed connection on its next I/O.
- **Timeout**: the whole handshake (prefix + HELLO, both directions)
  must complete within `HandshakeTimeout = 2s` — the same value as the
  existing vsock dial deadline convention
  (`shifty-core/internal/infrastructure/vsock/dial.go`). The deadline
  is cleared as soon as the handshake completes, so it never
  contaminates normal connection traffic.

## 4. HELLO Message

Type `0x00` — the first free ID in the control range `0x00`–`0x0F`.

### 4.1 Wire layout (big-endian, existing primitives)

```
uint8  role
string software_version     (uint16 length prefix + bytes; MUST be ≤ 128)
uint16 capability_count     (MUST be ≤ 64)
repeat count ×:
  uint8  capability_id
  uint16 min_revision
  uint16 max_revision        (MUST be ≥ min_revision)
```

Capability entries are **sorted by ID ascending** on the wire
(deterministic output, easy diffing in tests).

### 4.2 Go types

```go
type PeerRole uint8

const (
    RoleUnknown  PeerRole = 0 // reserved — never valid on the wire
    RoleCore     PeerRole = 1 // shifty-core (host daemon)
    RoleVHandler PeerRole = 2 // vhandler (guest agent)
    RoleCLI      PeerRole = 3 // shiftyctl (in-guest CLI)
)

type CapabilityID uint8

const (
    CapabilityExec         CapabilityID = 0x01
    CapabilityTools        CapabilityID = 0x02
    CapabilityEvents       CapabilityID = 0x03
    CapabilityFileTransfer CapabilityID = 0x04
    CapabilitySyncFS       CapabilityID = 0x05
)

type CapabilitySupport struct {
    MinRevision uint16
    MaxRevision uint16
}

type AdvertisedCapabilities map[CapabilityID]CapabilitySupport

type Capability struct {
    ID          CapabilityID
    MinRevision uint16
    MaxRevision uint16
}

type Hello struct {
    Role            PeerRole
    SoftwareVersion string       // informational only — never used for compatibility
    Capabilities    []Capability // sorted by ID ascending
}
```

### 4.3 Semantics

- **Role** identifies the component, not "host"/"guest": Core,
  VHandler, CLI. Each endpoint validates which peer roles it accepts
  (section 8).
- **SoftwareVersion** is free-form (`"0.8.2"`, `"0.8.2-dev+17a39d"`).
  It is for logging and triage only and is **explicitly excluded from
  compatibility decisions**. Encoders must reject versions longer than
  128 bytes — the `WriteString` primitive truncates silently, and HELLO
  must never truncate.
- **Revisions** represent the full contract of a capability. There are
  no feature bitmasks: everything a capability can do is described by
  its revision. If truly orthogonal features appear later, add
  capabilities or reconsider a bitmask then.

### 4.4 Validation (decoder, strict)

- `capability_count ≤ 64` (checked before allocating)
- `software_version ≤ 128` bytes (checked before allocating)
- `min_revision ≤ max_revision`
- capability IDs unique (duplicate → malformed)
- `role ∈ {1, 2, 3}`; unknown enum values → malformed

Malformed HELLO → reject with `ERROR(BAD_PAYLOAD, …)`. `RoleUnknown`
(0) is well-formed but never accepted → reject with
`ERROR(UNEXPECTED_ROLE, …)`.

## 5. Baseline Message Set (not negotiable)

These messages are part of MVCP v1 itself — every connection supports
them, no capability needed. They are **never** gated by negotiation:

| Type          | ID   | Notes                                                        |
|---------------|------|--------------------------------------------------------------|
| PING / PONG   | 0x01 / 0x02 | control liveness                                    |
| SHUTDOWN / SHUTDOWNACK | 0x03 / 0x04 | control lifecycle                          |
| GETSTATUS / STATUS | 0x05 / 0x06 | port 9003 status RPC — drives the VM lifecycle healthcheck; must never be gated |
| HEARTBEAT     | 0x07 | port 9003 periodic heartbeat                                  |
| STARTED       | 0xFA | RPC-layer "request accepted" marker — used by exec streaming **and** file-transfer import; cross-family, so baseline |
| ERROR         | 0xFE | universal error envelope                                      |
| HELLO         | 0x00 | the handshake itself                                          |

An **empty capability intersection is not a wire incompatibility**.
Whether the negotiated set is sufficient is application/service policy
(section 8).

## 6. Capability Table

Grounded in the actual message catalog (`mvcp/protocol/mvcp.go` +
`mvcp/protocol/messages/`). A shared default table
(`protocol.DefaultCapabilities`) covers the common case where core and
vhandler ship from the same build; a side may advertise a narrower or
wider table.

| ID | Capability   | Messages                                       | Current | Contract |
|----|--------------|------------------------------------------------|---------|----------|
| 0x01 | Exec        | EXEC 0x10, EXECSTREAM 0x11, EXECRESULT 0x12    | 1..2    | rev 1: EXEC → EXECRESULT (non-streaming). rev 2: streaming — `FlagExecStreaming` + `STARTED(stream=true)` + EXECSTREAM chunks + final EXECRESULT. |
| 0x02 | Tools       | TOOLCALL 0x30, TOOLRESULT 0x31, LISTTOOLS 0x32, LISTTOOLSRESULT 0x33 | 1..1 | full current contract, including schemas in LISTTOOLSRESULT |
| 0x03 | Events      | EVENTREADY 0x80, EVENTFILERECEIVED 0x81, EVENTMOUNT 0x82, EVENTERROR 0x83, EVENTLOG 0x84, EVENTINITFAILED 0x86 | 1..1 | full event family (only READY/INIT_FAILED have producers today) |
| 0x04 | FileTransfer| XFERINIT 0x20, XFERCHUNK 0x21, XFERDONE 0x22   | 1..1    | chunked import/export, both directions |
| 0x05 | SyncFS      | SYNCFILESYSTEMS 0x40, SYNCFILESYSTEMSACK 0x41  | 1..1    | wired end-to-end (lifecycle stop / snapshot / checkpoint flush) |

Properties:

- **Adding a new capability ID is always backward compatible**: unknown
  IDs are ignored by the negotiation (section 7), so an old peer simply
  does not negotiate it.
- **Revisions are monotonic**: supporting revision N implies supporting
  every revision in `[Min, N]`. If a future revision ever breaks this,
  introduce a new capability instead of a non-monotonic range.

## 7. Negotiation Algorithm

```go
// NegotiatedCapabilities maps capability ID → negotiated revision.
//
// Negotiate is symmetric: Negotiate(A, B) == Negotiate(B, A).
func Negotiate(peer, local AdvertisedCapabilities) NegotiatedCapabilities {
    out := make(NegotiatedCapabilities, min(len(peer), len(local)))
    for id, p := range peer {
        l, ok := local[id]
        if !ok {
            continue // unknown capability IDs are ignored, never rejected
        }
        lo := max(p.MinRevision, l.MinRevision)
        hi := min(p.MaxRevision, l.MaxRevision)
        if lo <= hi {
            out[id] = hi // always the highest common revision
        }
        // no common range → capability is simply not negotiated
    }
    return out
}
```

Rules:

- **Highest common revision wins.** Both sides then speak the
  negotiated revision's contract.
- **Unknown IDs: ignored** — never a rejection reason.
- **No common revision: not negotiated** — never an automatic global
  rejection. Whether that matters is decided by per-service
  requirements (section 8).
- Both sides run the same function over the same two tables, so both
  arrive at the identical negotiated set without any trust asymmetry.

## 8. Per-Service Requirements

```go
// Requirements: capability → minimum required revision.
type Requirements map[CapabilityID]uint16

// Check returns an error describing the first unsatisfied requirement.
func (r Requirements) Check(n NegotiatedCapabilities) error
```

Handshake helpers enforce requirements internally, so callers never
duplicate the logic:

```go
const HandshakeTimeout = 2 * time.Second

// NewHello builds the local HELLO (role, informational software
// version, capability table) that the helpers advertise on the wire.
func NewHello(role PeerRole, softwareVersion string, caps AdvertisedCapabilities) *Hello

// Server side (vhandler sessions): writes prefix + HELLO first, then
// reads the peer's prefix + HELLO. Rejects with ERROR on failure.
func ServerHandshake(rw io.ReadWriter, local *Hello,
    acceptRoles []PeerRole, reqs Requirements) (*Hello, NegotiatedCapabilities, error)

// Client side (core dials, shiftyctl): reads the peer's prefix + HELLO
// first, then writes its own. Rejects with ERROR on failure.
func ClientHandshake(rw io.ReadWriter, local *Hello,
    expectRoles []PeerRole, reqs Requirements) (*Hello, NegotiatedCapabilities, error)
```

Both helpers require a deadline-capable connection
(`SetDeadline(time.Time) error`), apply `HandshakeTimeout` for the
handshake phase only, and clear it before returning. Rejections
surface as `*HandshakeError{Code, Message}` (see section 9); callers
must close the connection after a rejection.

### 8.1 Per-port requirements (Shifty conventions)

Each MVCP connection negotiates **independently**; the result on port
9000 is never reused for 9002/9003/9004. After the handshake, each
service enforces its own requirements. Both sides enforce the same
table (symmetric enforcement catches misconfiguration):

| Port | Service              | Requirements                          |
|------|----------------------|---------------------------------------|
| 9000 | RPC / control        | Exec ≥ 1, Tools ≥ 1, SyncFS ≥ 1       |
| 9002 | Events               | Events ≥ 1                            |
| 9003 | Status / heartbeat   | *none* (baseline only)                |
| 9004 | File transfer        | FileTransfer ≥ 1                      |

Port 9003 deliberately has **no requirements**: the healthcheck must
work even against a completely empty capability intersection. The
baseline status/heartbeat contract is what drives the VM lifecycle.

### 8.2 Role expectations

| Endpoint                        | Accepts / expects peer role          |
|---------------------------------|--------------------------------------|
| vhandler sessions (9000/9002/9003/9004) | Core, CLI                    |
| core dial endpoints (all ports) | VHandler                             |
| shiftyctl                       | VHandler                             |

An unexpected role is rejected during the handshake with
`ERROR(UNEXPECTED_ROLE, …)` — a cheap sanity check that catches
mis-wired ports and wrong endpoints.

## 9. Error Semantics

Two distinct categories:

**1. Wire incompatibility** — bad magic, or wire version ≠ `0x01`.
The peer may not be able to parse an ERROR frame, so: **close
immediately, no ERROR frame**. Local errors:
`ErrBadMVCPMagic` / `ErrUnsupportedMVCPVersion` (replacing the current
single `ErrBadMVCPHandshake`).

**2. Post-wire-acceptance failures** — once both peers have exchanged
valid prefixes, the ERROR frame is safe to use:

| Code    | Name                  | Use                                          |
|---------|-----------------------|----------------------------------------------|
| 0x0002  | BAD_PAYLOAD (existing) | malformed HELLO (bad role enum, count > 64, duplicate ID, min > max, version > 128) |
| 0x000B  | UNEXPECTED_ROLE (new) | peer role not accepted by this endpoint, or RoleUnknown |
| 0x000C  | NO_COMMON_CAPABILITY (new) | a required capability is missing or has no common revision; the message carries detail (e.g. `"exec: peer 3..4, local 1..2"`) |

The existing `ErrorMsg` (uint16 code + string message) is sufficient —
no error-system redesign needed. `BAD_VERSION` (0x0008) stays defined
but unused: wire version mismatches close without an ERROR frame.

## 10. VPP (Console, port 9001)

**Unchanged.** Port 9001 keeps the fire-and-forget `"VPP" + 0x01`
handshake and no capability negotiation. Console is a thin interactive
channel that does not need feature negotiation.

## 11. Implementation Status

Implemented. Deviations from the original checklist are noted.

Protocol (`mvcp/protocol`):
- `TypeHELLO = 0x00`; `ErrorCodeUnexpectedRole 0x000B`,
  `ErrorCodeNoCommonCapability 0x000C` in `mvcp.go`
- `Hello`, `PeerRole`, `CapabilityID`, `CapabilitySupport`,
  `AdvertisedCapabilities`, `DefaultCapabilities`, `NewHello` in
  `hello.go` with strict validation (was planned as
  `messages/hello.go`; moved to package `protocol` to avoid a
  circular import with the handshake helpers)
- `Negotiate`, `Requirements.Check`, `HandshakeTimeout` in
  `capabilities.go`
- `ServerHandshake`, `ClientHandshake`, `ErrBadMVCPMagic`,
  `ErrUnsupportedMVCPVersion`, `HandshakeError` in `handshake.go`
- `conn.go` keeps only the VPP handshake (`ErrBadVPPHandshake`
  unchanged)

shifty-core:
- `vsock/dial.go`: `DialMVCP` returns
  `(*Conn, NegotiatedCapabilities, error)` and runs
  `ClientHandshake` with the per-port `portRequirements` table
  (9000: Exec/Tools/SyncFS; 9002: Events; 9004: FileTransfer);
  `CoreHello()` builds the advertised HELLO
- `app/vm/healthcheck.go` `dialStatus` (port 9003): `ClientHandshake`
  with no requirements; the 2s deadline convention is kept
- `vsock/rpc.go`: `RPCClient.Negotiated()` exposes the negotiated set

shifty-vhandler:
- `handshake.go`: `vhandlerHello()` (role VHandler, `buildVersion`),
  `sessionReqs` per port (9003 absent), `doServerHandshake()`; all
  four MVCP sessions use it
- `server.go` `fdReadWriter.SetDeadline`: SO_RCVTIMEO/SO_SNDTIMEO;
  `console.go` fdReader maps EAGAIN to `os.ErrDeadlineExceeded`
- shiftyctl dials with `ClientHandshake` (role CLI, expects VHandler)
  via its own `vsockRW` deadline wrapper. Peer-wiring caveat: per
  docs shiftyctl talks to the vhandler; verify against the real peer
  when the host-side vsock path exists.

Tests (all green under `make test`, race detector on):
- `mvcp/protocol/negotiate_test.go`: symmetry property over random
  tables, empty/unknown/no-common-range/partial edge cases
- `mvcp/protocol/hello_test.go`: round-trip, sorted wire order,
  rejection limits (count, version, min>max, duplicates, role enum,
  truncation, trailing bytes)
- `mvcp/protocol/handshake_test.go`: both directions over `net.Pipe`
  (with a deadline wrapper), role/requirements/malformed rejections
  on both sides, wire-version mismatch, mute-peer timeout ≈ 2s,
  deadline cleared after the exchange
- `shifty-core/internal/infrastructure/vsock/dial_test.go`: MVCP
  handshake against a fake vsock server (happy path, requirements
  fail, role rejected); the VPP coalescing regression test is kept

---

See also:
- [01-transport.md](01-transport.md) for the wire prefix and transport frame.
- [03-versioning.md](03-versioning.md) for the wire version policy.
- [04-error-codes.md](04-error-codes.md) for the error code registry.
