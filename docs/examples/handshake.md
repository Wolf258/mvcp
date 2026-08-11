# Example: Connection Handshake

Every MVCP connection starts with a 5-byte **wire prefix** sent by the
guest agent (vhandler) immediately after accept, followed by a **HELLO
frame** that negotiates identity and capabilities. The magic string
tells the peer which protocol to speak on this connection. The full
contract — HELLO layout, validation limits, capability table,
negotiation algorithm, per-port requirements, and timeout — lives in
[06-negotiation.md](../06-negotiation.md).

## MVCP Handshake (ports 9000, 9002, 9003, 9004)

### 1. Wire prefix (5 bytes, Guest → Host)

```
magic:   0x4D 0x56 0x43 0x50  ("MVCP")
version: 0x01
```

### 2. HELLO frame (bidirectional)

The guest agent sends `HELLO` (`type=0x00`, `flags=0x00`, `msg_id=0`)
immediately after the prefix, announcing its role, software version,
and supported capability revision ranges. The host validates the
prefix and HELLO, computes the negotiation, and replies with its own
prefix + `HELLO` — or an `ERROR` frame, then closes.

### Host-Side Validation (conceptual)

```go
// 1. Read + validate the 5-byte prefix (magic + wire version).
var prefix [5]byte
io.ReadFull(conn, prefix[:])
if string(prefix[:4]) != "MVCP" || prefix[4] != 0x01 {
    conn.Close()
    return fmt.Errorf("unsupported MVCP wire version: %x", prefix[4])
}

// 2. Read the peer HELLO frame, validate, negotiate, and reply with
//    our own prefix + HELLO. This is handled by
//    protocol.ClientHandshake / protocol.ServerHandshake in the
//    implementation; see 06-negotiation.md for the algorithm.
```

## VPP Handshake (port 9001 — 4 bytes, Guest → Host)

```
magic:   0x56 0x50 0x50        ("VPP")
version: 0x01
```

VPP is unchanged: fire-and-forget, no HELLO exchange, no negotiation.

### Host Validation

```go
var handshake [4]byte
io.ReadFull(conn, handshake[:])

magic := string(handshake[:3])  // "VPP"
version := handshake[3]          // 0x01

if magic != "VPP" || version != 0x01 {
    conn.Close()
    return fmt.Errorf("unsupported VPP version: %x", version)
}
// proceed to ReadFrame loop (transport) + VPP dispatch
```

## Notes

- The MVCP handshake is **no longer fire-and-forget**: both sides
  exchange HELLO and independently compute the negotiated capability
  set.
- Wire incompatibility (bad magic or version) closes **without** an
  ERROR frame. Post-acceptance failures (unexpected role, malformed
  HELLO, unsatisfied requirements) close **with** an ERROR frame.
- The handshake is performed on every connection, including ports
  9002, 9003, and 9004; each connection negotiates independently.
- The magic string tells the peer which protocol to speak:
  `"MVCP"` → MVCP message dispatch (type+flags+msg_id).
  `"VPP"`  → VPP frame dispatch (type-only).
- Port numbers are conventions — an implementation could run MVCP on
  port 9001 or VPP on port 9000 as long as both sides agree.

After the handshake, both sides enter the transport frame read/write
loop. All ports use the same 4-byte length-prefixed `ReadFrame`/
`WriteFrame` from `mvcp/protocol/frame.go`.
