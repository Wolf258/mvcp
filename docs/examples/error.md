# Example: ERROR

Error response to an unknown message type.

## ERROR (Guest → Host)

```
 length: 0x00_00_00_14   (20 = 6 + 14 payload)
   type: 0xFE             (ERROR)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches the unknown request)
payload:
  uint16 0x0001           (UNKNOWN_TYPE)
  string "unknown message type: 0xAB"
```

### Payload Breakdown

| Offset | Bytes | Field | Value |
|--------|-------|-------|-------|
| 0 | `00 01` | error code | 1 (UNKNOWN_TYPE) |
| 2 | `00 1C` | message length | 28 |
| 4 | `75 6e 6b 6e 6f 77 6e 20 6d 65 73 73 61 67 65 20 74 79 70 65 3a 20 30 78 41 42` | message | "unknown message type: 0xAB" |

## Other Error Examples

### BAD_PAYLOAD (0x0002)

```
 length: 0x00_00_00_17
   type: 0xFE             (ERROR)
  flags: 0x01
 msg_id: 0x00_00_00_03
payload:
  uint16 0x0002           (BAD_PAYLOAD)
  string "string length exceeds remaining buffer"
```

### FILE_NOT_FOUND (0x0003)

```
 length: 0x00_00_00_14
   type: 0xFE             (ERROR)
  flags: 0x01
 msg_id: 0x00_00_00_04
payload:
  uint16 0x0003           (FILE_NOT_FOUND)
  string "/nonexistent/path"
```

### TIMEOUT (0x0006)

```
 length: 0x00_00_00_10
   type: 0xFE             (ERROR)
  flags: 0x01
 msg_id: 0x00_00_00_05
payload:
  uint16 0x0006           (TIMEOUT)
  string "command exceeded 30s"
```

## Go: Sending an Error

```go
var buf bytes.Buffer
protocol.WriteUint16(&buf, protocol.ErrUnknownType)
protocol.WriteString(&buf, "unknown message type: 0xAB")

frame := &protocol.Frame{
    Type:    protocol.TypeError,
    Flags:   protocol.FlagResponse,
    MsgID:   requestMsgID,
    Payload: buf.Bytes(),
}
frame.WriteTo(conn)
```

## Go: Handling an Error

```go
frame, _ := protocol.ReadFrame(conn)

if frame.Type == protocol.TypeError {
    r := bytes.NewReader(frame.Payload)
    code, _ := protocol.ReadUint16(r)
    msg, _ := protocol.ReadString(r)
    return fmt.Errorf("MVCP error %04x: %s", code, msg)
}
```
