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
│ ports 9000/9002/9003 │ port 9001                     │
├──────────────────────┴───────────────────────────────┤
│ TRANSPORT (shared by all ports)                       │
│ ReadFrame / WriteFrame — length(4B BE) + payload      │
│ max 16 MB                                             │
├───────────────────────────────────────────────────────┤
│ HANDSHAKE (per protocol)                              │
│ MVCP: "MVCP"+0x01 (5B)  |  VPP: "VPP"+0x01 (4B)     │
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

| Port | Service      | Protocol | Direction     |
|------|------------- |----------|-------------- |
| 9000 | Control/RPC  | MVCP     | Bidirectional |
| 9001 | Console      | VPP      | Bidirectional |
| 9002 | Events       | MVCP     | Guest → Host  |
| 9003 | Heartbeat    | MVCP     | Guest → Host  |

These are conventions. The protocol places no constraints on port
numbers. An implementation may re-map services to different ports.

## vsock Guarantees

`SOCK_STREAM` provides:

- Ordered delivery
- No duplicates
- Broken-connection detection
- Retransmission
- Flow control

Because the transport guarantees these, MVCP does **not** need: checksums,
ACKs, retransmission, or sequence numbers for ordering. Only framing +
type dispatch + encoding.

## Host-Side Transport (Shifty)

The host connects to Firecracker's per-VM vsock Unix socket:

1. Connect to `AF_UNIX` socket at `<jail>/root/vsock.sock`
2. Send `CONNECT <port>\n`
3. Receive `OK <assigned_port>\n`
4. Read protocol handshake from guest (see below)
5. Validate magic + version
6. Proceed to `ReadFrame` / `WriteFrame` loop

## Guest-Side Transport (Shifty)

1. Open `AF_VSOCK` `SOCK_STREAM` listeners on ports 9000–9003
2. `Accept()` connection
3. Immediately write protocol handshake (see below)
4. Enter `ReadFrame` / dispatch loop

## Connection Handshake

Every connection starts with a handshake sent by the guest immediately
after accept. The magic string tells the host which protocol to speak on
this connection.

### MVCP handshake (ports 9000, 9002, 9003 — 5 bytes)

```
┌─────────────────┬─────────┐
│ magic (4B)      │ version │
│ 'M' 'V' 'C' 'P' │  0x01   │
└─────────────────┴─────────┘
```

| Field    | Size    | Value                                      |
|----------|---------|--------------------------------------------|
| `magic`  | 4 bytes | `0x4D 0x56 0x43 0x50` ("MVCP")             |
| `version`| 1 byte  | Protocol version number. Currently `0x01`.  |

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

### Validation (host side)

The host reads `len(magic)` bytes, validates magic and version:

- **Version supported** → proceeds to read transport frames
- **Version not supported** → closes the connection

Fire-and-forget. No response from host, no round-trip.

## Multiple Connections

Multiple connections to the same port are allowed. Each connection is an
independent request/response stream. This enables concurrent EXEC and
FILE_EXPORT from different host-side goroutines without head-of-line
blocking.

Port 9001 (console) uses VPP — a separate binary protocol with its own
4-byte handshake and a thinner 1-byte inner header. One connection = one
interactive session. See [services/console.md](services/console.md).

---

See also:
- [02-wire-format.md](02-wire-format.md) for the transport frame + MVCP wire format.
- [03-versioning.md](03-versioning.md) for protocol version compatibility.
- [05-concurrency.md](05-concurrency.md) for the concurrency model.
- [services/console.md](services/console.md) for the VPP protocol on port 9001.
