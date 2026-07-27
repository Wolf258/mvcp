# Events Service (Port 9002)

> Message type range `0x80`–`0x8F`. One-way G→H.

Port 9002 carries asynchronous event notifications from guest to host.
All events are one-way frames (`msg_id = 0`). No response is expected.

## Message Types

| Type | Name | Payload |
|------|------|---------|
| `0x80` | `EVENT_READY` | `string version` |
| `0x81` | `EVENT_FILE_RECEIVED` | `string path`, `bytes sha256`, `uint64 size` |
| `0x82` | `EVENT_MOUNT` | `string path`, `string fstype` |
| `0x83` | `EVENT_ERROR` | `uint16 code`, `string message` |
| `0x84` | `EVENT_LOG` | `uint8 level`, `string message`, `uint64 ts_ns` |
| `0x85`–`0x8F` | *(reserved)* | — |

## EVENT_READY (`0x80`)

Sent once when the guest finishes boot and all services are listening.

| Field | Encoding | Description |
|-------|----------|-------------|
| `version` | `string` | vhandler version string |

## EVENT_FILE_RECEIVED (`0x81`)

Sent when a file import completes successfully.

| Field | Encoding | Description |
|-------|----------|-------------|
| `path` | `string` | File path inside the guest |
| `sha256` | `bytes` (32) | SHA256 hash of the received file |
| `size` | `uint64` | File size in bytes |

## EVENT_MOUNT (`0x82`)

Sent when an overlay or bind mount is created inside the guest.

| Field | Encoding | Description |
|-------|----------|-------------|
| `path` | `string` | Mount point path |
| `fstype` | `string` | Filesystem type (e.g. "overlay", "tmpfs") |

## EVENT_ERROR (`0x83`)

Sent when a guest-side error occurs that is not tied to a specific
request `msg_id` (request errors use `type=0xFE` with `IS_RESPONSE`).

| Field | Encoding | Description |
|-------|----------|-------------|
| `code` | `uint16` | Error code from the [error registry](../04-error-codes.md) |
| `message` | `string` | Human-readable description |

## EVENT_LOG (`0x84`)

Structured log event from inside the guest.

| Field | Encoding | Description |
|-------|----------|-------------|
| `level` | `uint8` | Log level: `0x00` = debug, `0x01` = info, `0x02` = warn, `0x03` = error |
| `message` | `string` | Log message text |
| `ts_ns` | `uint64` | Unix timestamp in nanoseconds |

---

See also:
- [04-error-codes.md](../04-error-codes.md) for the error code registry used by `EVENT_ERROR`.
