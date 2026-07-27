# Example: PING / PONG

Liveness check between host and guest.

## PING (Host → Guest, 10 bytes on wire)

```
 length: 0x00_00_00_06   (1 + 1 + 4 + 0 = 6)
   type: 0x01             (PING)
  flags: 0x00
 msg_id: 0x00_00_00_01
payload: (empty)
```

## PONG (Guest → Host, 10 bytes on wire)

```
 length: 0x00_00_00_06
   type: 0x02             (PONG)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches request)
payload: (empty)
```

## Go: Writing a PING

```go
// host side
frame := protocol.NewFrame(protocol.TypePING, 0, nil)
frame.MsgID = 1
frame.WriteTo(conn)

// read PONG
resp, err := protocol.ReadFrame(conn)
// resp.Type == protocol.TypePONG
// resp.Flags & protocol.FlagResponse != 0
// resp.MsgID == 1
```

## Go: Handling PING (guest side)

```go
// guest side
frame, _ := protocol.ReadFrame(conn)
if frame.Type == protocol.TypePING {
    pong := protocol.NewFrame(protocol.TypePONG, protocol.FlagResponse, nil)
    pong.MsgID = frame.MsgID
    pong.WriteTo(conn)
}
```
