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
| `0x86` | `EVENT_INIT_FAILED` | `WANT_ACK` | `string version`, `string reason` |
| `0x85`, `0x87`–`0x8F` | *(reserved)* | — | — |

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

Sent once when the guest finishes boot **and** the init
sequence (`/init.sh` or `defaultInit`) returns successfully.
A late subscriber that connects to port 9002 before init
completes will block inside the events session until init
resolves — either to `EVENT_READY` (success) or to
`EVENT_INIT_FAILED` (failure/panic/timeout). The event is
emitted at most once per VM lifetime.

| Field | Encoding | Description |
|-------|----------|-------------|
| `version` | `string` | vhandler version string |

### Consumer contract

`EVENT_READY` is the canonical "init is complete" signal
emitted by the guest. Hosts that initiate host→guest actions
on vsock ports **not** explicitly designated as "always
available" in the vhandler's channel gating matrix (see
[`docs/agents/07-vhandler.md`](../../../docs/agents/07-vhandler.md#channel-gating-matrix))
MUST wait for this event before sending their request. The
vhandler applies the broadcast as a defense-in-depth gate only
on the console port (9001) and on its own event emission; the
host remains the authoritative enforcer for RPC (9000) and
file transfer (9004).

## EVENT_INIT_FAILED (`0x86`)

Sent once when the guest's init sequence fails, panics, or
exceeds the `initTimeout` budget. The `reason` string is the
same machine-readable vocabulary the heartbeat's
`ExtFailureReason` TLV uses (`init_timeout`, `init_panic`,
`internal`, `mount_failed`, `exec_failed`) so a host can
reuse its reason→error mapping for either channel. Emitted
in place of `EVENT_READY`; the two are mutually exclusive
within a single VM lifetime.

| Field | Encoding | Description |
|-------|----------|-------------|
| `version` | `string` | vhandler version string |
| `reason` | `string` | failure reason (see `messages.HeartbeatFailureReason`) |

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
- [Channel gating matrix](../../../docs/agents/07-vhandler.md#channel-gating-matrix)
  in `docs/agents/07-vhandler.md` for which vsock ports are
  gated by the init broadcast and which require consumer-side
  enforcement.
