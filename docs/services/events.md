# Events Service (Port 9002)

> Message type range `0x80`–`0x8F`. G→H with application-level ACK.

Port 9002 carries asynchronous event notifications from guest to host.
Every event carries `WANT_ACK` (`0x04`) and a non-zero `msg_id`. The
host MUST respond with an `MVCP_ACK` (type `0xFB`) after dispatching
the event to its handler.

## Message Types

| Type | Name | Flags | Payload |
|------|------|-------|---------|
| `0x80` | `EVENT_READY` | `WANT_ACK` | `string version` |
| `0x81` | `EVENT_FILE_RECEIVED` | `WANT_ACK` | `string path`, `uint64 size` |
| `0x82` | `EVENT_MOUNT` | `WANT_ACK` | `string path`, `string fstype` |
| `0x83` | `EVENT_ERROR` | `WANT_ACK` | `uint16 code`, `string message` |
| `0x84` | `EVENT_LOG` | `WANT_ACK` | `uint8 level`, `string message`, `uint64 ts_ns` |
| `0x85`–`0x8F` | *(reserved)* | — | — |

## Acknowledgment Flow

Every event is acknowledged by the host. The guest allocates a monotonic
`msg_id` per connection and sets `WANT_ACK`:

```
Guest → EVENT_READY (msg_id=1, WANT_ACK, version)   → Host
Host  → MVCP_ACK   (msg_id=1, IS_RESPONSE, ok)      → Guest

Guest → EVENT_LOG  (msg_id=2, WANT_ACK, level, msg)  → Host
Host  → MVCP_ACK   (msg_id=2, IS_RESPONSE, ok)      → Guest
```

If the host cannot process the event (ring buffer full, no handler),
it sends `MVCP_ACK` with a non-zero status:

```
Guest → EVENT_READY (msg_id=1, WANT_ACK)             → Host
Host  → MVCP_ACK   (msg_id=1, status=0x02, "ring buffer full") → Guest
```

The guest SHOULD implement a timeout when waiting for `MVCP_ACK`. If
the host does not acknowledge within the timeout, the guest treats
the event as undelivered and may retry or drop the event.

## EVENT_READY (`0x80`)

Sent once when the guest finishes boot and all services are listening.

| Field | Encoding | Description |
|-------|----------|-------------|
| `version` | `string` | vhandler version string |

## EVENT_FILE_RECEIVED (`0x81`)

Sent when a file import completes successfully via the File Transfer
service (port 9004).

| Field | Encoding | Description |
|-------|----------|-------------|
| `path` | `string` | File path inside the guest |
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
