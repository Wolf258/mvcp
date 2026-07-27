# Error Codes

> Part of the MVCP specification. See [SPEC.md](../SPEC.md) for the
> complete protocol specification.

## Error Message

Errors are carried by MVCP frames with type `0xFE` and the `IS_ERROR`
flag (`0x04`) set. The payload encodes:

| Field | Encoding |
|-------|----------|
| `uint16 code` | Error code (see table below) |
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
