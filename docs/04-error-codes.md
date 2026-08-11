# 04 — Error Codes

## Error Message

Errors are carried by MVCP frames with `IS_RESPONSE` flag (`0x01`) and
type `0xFE`. The payload encodes:

| Field | Encoding |
|-------|----------|
| `uint16 code` | Error code (see registry below) |
| `string message` | Human-readable error description |

Direction: both host-to-guest and guest-to-host.

## Error Code Registry

| Code | Name | Description |
|------|------|-------------|
| `0x0001` | `UNKNOWN_TYPE` | Message type not recognized by the receiver |
| `0x0002` | `BAD_PAYLOAD` | Payload decoding failed |
| `0x0003` | `FILE_NOT_FOUND` | Requested file does not exist |
| `0x0004` | `PERMISSION_DENIED` | Insufficient permissions for the operation |
| `0x0005` | `EXEC_FAILED` | Command could not be started |
| `0x0006` | `TIMEOUT` | Operation exceeded its deadline |
| `0x0007` | `NOT_A_DIRECTORY` | Path expected to be a directory but was not |
| `0x0008` | `BAD_VERSION` | Protocol version not supported by the receiver |
| `0x0009` | `UNKNOWN_TOOL` | Tool name not registered in the guest |
| `0x000A` | `TOOL_FAILED` | Tool executed but returned an error (details in error_msg) |
| `0x000B` | `UNEXPECTED_ROLE` | Handshake: peer role not accepted by this endpoint |
| `0x000C` | `NO_COMMON_CAPABILITY` | Handshake: a required capability is missing or has no common revision (details in error_msg) |

Handshake errors use `msg_id = 0` (echoing HELLO) and `IS_RESPONSE`.
`BAD_VERSION` (0x0008) remains defined but unused: wire version
mismatches close the connection without an ERROR frame.

## Wire Example

**ERROR** (guest → host, unknown message type):

```
 length: 0x00_00_00_14   (6 + 14 payload)
   type: 0xFE             (ERROR)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches the unknown request)
payload:
  uint16 0x0001           (UNKNOWN_TYPE)
  string "unknown message type"
```

---

See also:
- [02-wire-format.md](02-wire-format.md) for the frame layout and `IS_RESPONSE` flag.
- [examples/error.md](examples/error.md) for additional error wire examples.
