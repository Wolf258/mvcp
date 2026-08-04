# Example: Connection Handshake

The handshake is sent by the guest immediately after accepting a
vsock connection, before any framed messages. Two handshake variants
exist — one per protocol.

## MVCP Handshake (ports 9000, 9002, 9003, 9004 — 5 bytes, Guest → Host)

```
magic:   0x4D 0x56 0x43 0x50  ("MVCP")
version: 0x01
```

### Host Validation

```go
var handshake [5]byte
io.ReadFull(conn, handshake[:])

magic := string(handshake[:4])  // "MVCP"
version := handshake[4]          // 0x01

if magic != "MVCP" || version != 0x01 {
    conn.Close()
    return fmt.Errorf("unsupported protocol version: %x", version)
}
// proceed to ReadFrame loop (transport)
```

## VPP Handshake (port 9001 — 4 bytes, Guest → Host)

```
magic:   0x56 0x50 0x50        ("VPP")
version: 0x01
```

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

## Unified Host-Side Flow

The host can use a shared handshake validator that accepts both
magic strings:

```go
// mvcp/protocol/conn.go
func ValidateHandshake(r io.Reader, expectedMagic string) error {
    n := len(expectedMagic) + 1 // magic + version
    buf := make([]byte, n)
    if _, err := io.ReadFull(r, buf); err != nil {
        return err
    }
    if string(buf[:len(expectedMagic)]) != expectedMagic {
        return fmt.Errorf("bad magic: expected %q", expectedMagic)
    }
    if buf[len(expectedMagic)] != Version1 {
        return fmt.Errorf("unsupported version: %x", buf[len(expectedMagic)])
    }
    return nil
}
```

## Notes

- Fire-and-forget: no host response, no round-trip.
- Handshake is sent on every connection, including ports 9002, 9003, and 9004.
- The magic string tells the host which protocol to speak:
  `"MVCP"` → MVCP message dispatch (type+flags+msg_id).
  `"VPP"`  → VPP frame dispatch (type-only).
- Port numbers are conventions — an implementation could run MVCP on
  port 9001 or VPP on port 9000 as long as both sides agree.

After the handshake, both sides enter the transport frame read/write
loop. All ports use the same 4-byte length-prefixed `ReadFrame`/
`WriteFrame` from `mvcp/protocol/frame.go`.
