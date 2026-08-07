# Events Service (Port 9002)

> Message type range `0x80`–`0x8F`. G→H, fire-and-forget.

Port 9002 carries asynchronous event notifications from guest to host.
Events are one-way (`msg_id = 0`): the guest pushes them and the host
reads and dispatches to subscribers. No per-event acknowledgment is
required — the host is expected to keep the events connection drained.

## Message Types

| Type | Name | Flags | Payload |
|------|------|-------|---------|
| `0x80` | `EVENT_READY` | `0x00` | `string version` |
| `0x81` | `EVENT_FILE_RECEIVED` | `0x00` | `string path`, `uint64 size` |
| `0x82` | `EVENT_MOUNT` | `0x00` | `string path`, `string fstype` |
| `0x83` | `EVENT_ERROR` | `0x00` | `uint16 code`, `string message` |
| `0x84` | `EVENT_LOG` | `0x00` | `uint8 level`, `string message`, `uint64 ts_ns` |
| `0x86` | `EVENT_INIT_FAILED` | `0x00` | `string version`, `string reason` |
| `0x85`, `0x87`–`0x8F` | *(reserved)* | — | — |

## Delivery Model

Events are fire-and-forget from the guest side: the guest pushes each
event as a single `msg_id=0` frame and does not wait for acknowledgment.
The host is expected to drain the events connection promptly. If the
host's event ring buffer fills up, the guest logs the lost event and
continues.

```
Guest → EVENT_READY (msg_id=0, version)   → Host  (fire-and-forget)
Guest → EVENT_LOG    (msg_id=0, level, msg) → Host
Guest → EVENT_ERROR  (msg_id=0, code, message) → Host
```

> Earlier drafts of the spec used `WANT_ACK` + `MVCP_ACK` for event
> acknowledgment. That was removed in favour of a simpler fire-and-forget
> model. See [SPEC.md](../SPEC.md) for the design decision.

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
[`shifty-vhandler/docs/architecture.md`](../../shifty-vhandler/docs/architecture.md#channel-gating-matrix))
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
- [Channel gating matrix](../../shifty-vhandler/docs/architecture.md#channel-gating-matrix)
  in `shifty-vhandler/docs/architecture.md` for which vsock ports are
  gated by the init broadcast and which require consumer-side
  enforcement.
