# 01 — Transport

MVCP rides on top of Firecracker's vsock device over `SOCK_STREAM`.

## Port-Agnostic Design

The protocol is **port-agnostic** — any service can run on any port. The
port assignments below are **Shifty conventions**, not protocol
requirements. An implementation is free to use different port numbers as
long as both sides agree.

## Architecture Layers

```
┌──────────────────────────────────────────────────────┐
│ APPLICATION                                           │
│ Control / Events / Heartbeat / Console / ...          │
├──────────────────────┬───────────────────────────────┤
│ MVCP wire format     │ VPP wire format               │
│ type+flags+msg_id(6B)│ type(1B)                      │
│ ports 9000/9002/9003/9004 │ port 9001                     │
├──────────────────────┴───────────────────────────────┤
│ TRANSPORT (shared by all ports)                       │
│ ReadFrame / WriteFrame — length(4B BE) + payload      │
│ max 16 MB                                             │
├───────────────────────────────────────────────────────┤
│ HANDSHAKE (per protocol)                              │
│ MVCP: "MVCP"+0x01 + HELLO | VPP: "VPP"+0x01 (4B)     │
├───────────────────────────────────────────────────────┤
│ VSOCK TRANSPORT (Firecracker)                         │
│ CONNECT <port>\n / OK <port>\n over AF_UNIX          │
└───────────────────────────────────────────────────────┘
```

All ports use the same **transport framing** (`ReadFrame`/`WriteFrame`)
and the same **encoding primitives**. MVCP and VPP differ only in how
they structure the transport payload: MVCP adds a 6-byte header
(`type`+`flags`+`msg_id`), VPP adds a 1-byte header (`type`).

## Transport Frame (common)

Every frame after the handshake uses the same length-prefixed format,
regardless of port or protocol:

```
┌──────────┬─────────────────────────────┐
│ length   │ payload                     │
│ (4B BE)  │ (N bytes)                   │
└──────────┴─────────────────────────────┘

length = N    (uint32 big-endian, max 16 MB = 0x01000000)
```

The transport does **not** interpret the payload — it is an opaque byte
sequence. The receiver decides how to interpret it based on the port
(and the handshake magic that preceded it).

### Why 4 bytes for all ports

Previously the VPP console protocol used a 2-byte length prefix to save
overhead per keystroke. With a shared transport layer the frame header
is always 4 bytes. The 2-byte saving per frame is negligible on local
vsock, and sharing `ReadFrame`/`WriteFrame` eliminates duplicated code
on both host and guest sides.

For interactive terminal I/O, the sender should **batch small writes**
into fewer frames (e.g. flush every 5 ms or when a buffer reaches 4 KB)
to amortise the fixed header cost.

## Service Ports (Shifty convention)

| Port | Service      | Protocol | Direction            |
|------|------------- |----------|--------------------- |
| 9000 | Control/RPC  | MVCP     | Bidirectional        |
| 9001 | Console      | VPP      | Bidirectional        |
| 9002 | Events       | MVCP     | Guest → Host         |
| 9003 | Heartbeat    | MVCP     | Guest → Host         |
| 9004 | File Transfer| MVCP     | Host-initiated, bidir|

These are conventions. The protocol places no constraints on port
numbers. An implementation may re-map services to different ports.

## vsock Guarantees

`SOCK_STREAM` provides **transport-level** reliability:

- Ordered delivery
- No duplicates
- Broken-connection detection
- Retransmission
- Flow control

Because the transport guarantees these, MVCP does **not** need: checksums,
retransmission, or sequence numbers for ordering. Only framing +
type dispatch + encoding.

## Application-Level Acknowledgment

Transport guarantees ensure data reaches the other side, but they do
**not** confirm that the receiver successfully processed the payload (e.g.
wrote a chunk to disk, enqueued an event). MVCP provides optional
application-level acknowledgment via the `WANT_ACK` flag (`0x04`) and
`MVCP_ACK` type (`0xFB`).

See [02-wire-format.md](02-wire-format.md) for the full `WANT_ACK`
specification.

## Host-Side Transport (Shifty)

The host connects to Firecracker's per-VM vsock Unix socket:

1. Connect to `AF_UNIX` socket at `<jail>/root/vsock.sock`
2. Send `CONNECT <port>\n`
3. Receive `OK <assigned_port>\n`
4. Read wire prefix + HELLO from guest (see below)
5. Validate prefix (magic + wire version), decode HELLO, negotiate
6. Reply with own prefix + HELLO — or `ERROR` and close
7. Proceed to `ReadFrame` / `WriteFrame` loop

## Guest-Side Transport (Shifty)

1. Open `AF_VSOCK` `SOCK_STREAM` listeners on ports 9000–9004
2. `Accept()` connection
3. Immediately write wire prefix + HELLO (see below)
4. Enter `ReadFrame` / dispatch loop

## Connection Handshake

Every connection starts with a **wire prefix** sent by the guest agent
(vhandler) immediately after accept, followed by a **HELLO frame** that
negotiates identity and capabilities. The magic string tells the host
which protocol to speak on this connection.

### MVCP handshake (ports 9000, 9002, 9003, 9004)

```
┌─────────────────┬─────────┐
│ magic (4B)      │ version │
│ 'M' 'V' 'C' 'P' │  0x01   │
└─────────────────┴─────────┘
```

| Field    | Size    | Value                                      |
|----------|---------|--------------------------------------------|
| `magic`  | 4 bytes | `0x4D 0x56 0x43 0x50` ("MVCP")             |
| `version`| 1 byte  | Wire format version. Currently `0x01`.      |

Immediately after the prefix, the guest agent sends a **HELLO frame**
(`type=0x00`, `flags=0x00`, `msg_id=0`) announcing its role, software
version and supported capability revision ranges. The host validates,
negotiates, and replies with its own prefix + HELLO — or an `ERROR`
frame followed by close.

See [06-negotiation.md](06-negotiation.md) for the full handshake
contract: HELLO layout, validation limits, capability table,
negotiation algorithm, per-port requirements, and timeout.

### VPP handshake (port 9001 — 4 bytes)

```
┌──────┬──────┬──────┬─────────┐
│ 'V'  │ 'P'  │ 'P'  │ version │
│ 0x56 │ 0x50 │ 0x50 │  0x01   │
└──────┴──────┴──────┴─────────┘
```

| Byte | Value  | Meaning           |
|------|--------|-------------------|
| 0    | `0x56` | 'V'               |
| 1    | `0x50` | 'P'               |
| 2    | `0x50` | 'P'               |
| 3    | `0x01` | Protocol version  |

### Validation

Two failure categories:

- **Wire incompatibility** (bad magic, or wire version ≠ `0x01`):
  close immediately, no `ERROR` frame — the peer may not be able to
  parse it.
- **Post-wire-acceptance failures** (unexpected role, malformed HELLO,
  unsatisfied capability requirements): the detecting side sends an
  `ERROR` frame with a specific code, then closes.

The handshake is **no longer fire-and-forget**: both sides exchange
HELLO and independently compute the negotiated capability set.

## Multiple Connections

Multiple connections to the same port are allowed. Each connection is an
independent request/response stream. This enables concurrent EXEC from
different host-side goroutines without head-of-line blocking.
File transfers use a dedicated port (9004) and never block RPC traffic.

Port 9001 (console) uses VPP — a separate binary protocol with its own
4-byte handshake and a thinner 1-byte inner header. One connection = one
interactive session. See [services/console.md](services/console.md).

---

See also:
- [02-wire-format.md](02-wire-format.md) for the transport frame + MVCP wire format.
- [03-versioning.md](03-versioning.md) for protocol version compatibility.
- [05-concurrency.md](05-concurrency.md) for the concurrency model.
- [services/console.md](services/console.md) for the VPP protocol on port 9001.
