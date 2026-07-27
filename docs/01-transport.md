# 01 — Transport

MVCP rides on top of Firecracker's vsock device over `SOCK_STREAM`.

## Service Ports

Service discovery is port-based — no multiplexing layer.

| Port | Service | Protocol | Direction |
|------|---------|----------|-----------|
| 9000 | Control (RPC) | MVCP binary | Bidirectional H↔G |
| 9001 | Console | Raw bytes (PTY ↔ shell) | Bidirectional H↔G |
| 9002 | Events | MVCP binary (one-way) | Guest → Host |
| 9003 | Heartbeat | MVCP binary (one-way) | Guest → Host |

## vsock Guarantees

`SOCK_STREAM` provides:

- Ordered delivery
- No duplicates
- Broken-connection detection
- Retransmission
- Flow control

Because the transport guarantees these, MVCP does **not** need: checksums,
ACKs, retransmission, or sequence numbers for ordering. Only framing + type
dispatch + encoding.

## Host-Side Transport (Shifty)

The host connects to Firecracker's per-VM vsock Unix socket:

1. Connect to `AF_UNIX` socket at `<jail>/root/vsock.sock`
2. Send `CONNECT <port>\n`
3. Receive `OK <assigned_port>\n`
4. Read 5-byte MVCP handshake from guest
5. Validate magic (`MVCP`) + version (`0x01`)
6. Proceed to `ReadFrame` / `WriteFrame` loop

## Guest-Side Transport (Shifty)

1. Open `AF_VSOCK` `SOCK_STREAM` listeners on ports 9000–9003
2. `Accept()` connection
3. Immediately write MVCP handshake: `MVCP` + `0x01` (5 bytes)
4. Enter `ReadFrame` / dispatch loop

## Connection Handshake

Every connection starts with a 5-byte handshake sent by the guest
immediately after accept:

```
┌─────────────────┬─────────┐
│ magic (4B)      │ version │
│ 'M' 'V' 'C' 'P' │  0x01   │
└─────────────────┴─────────┘
```

| Field | Size | Value |
|-------|------|-------|
| `magic` | 4 bytes | `0x4D 0x56 0x43 0x50` ("MVCP") — protocol identifier |
| `version` | 1 byte | Protocol version number. Currently `0x01`. |

The host reads 5 bytes, validates magic and version:
- **Version supported** → proceeds to read frames
- **Version not supported** → closes the connection

Fire-and-forget. No response from host, no round-trip.

## Multiple Connections

Multiple connections to the same port are allowed. Each connection is an
independent request/response stream. This enables concurrent EXEC and
FILE_EXPORT from different host-side goroutines without head-of-line
blocking.

Port 9001 (console) uses a single persistent connection for the lifetime
of the PTY session.

---

See also:
- [02-wire-format.md](02-wire-format.md) for the frame layout and encoding.
- [03-versioning.md](03-versioning.md) for protocol version compatibility.
- [05-concurrency.md](05-concurrency.md) for the concurrency model.
