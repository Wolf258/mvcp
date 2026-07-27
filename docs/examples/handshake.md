# Example: Connection Handshake

The handshake is sent by the guest immediately after accepting a
vsock connection, before any MVCP frames.

## Wire Bytes (5 bytes, Guest → Host)

```
magic:   0x4D 0x56 0x43 0x50  ("MVCP")
version: 0x01
```

## Host Validation

```go
var handshake [5]byte
io.ReadFull(conn, handshake[:])

magic := string(handshake[:4])  // "MVCP"
version := handshake[4]          // 0x01

if magic != "MVCP" || version != 0x01 {
    conn.Close()
    return fmt.Errorf("unsupported protocol version: %x", version)
}
// proceed to ReadFrame loop
```

## Notes

- Fire-and-forget: no host response, no round-trip.
- Handshake is sent on every connection, including ports 9002 and 9003.
- Port 9001 (console) does **not** send a handshake — it's raw bytes.
