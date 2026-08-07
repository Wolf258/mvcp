# Virtual PTY Protocol — VPP (Port 9001)

> Binary-framed interactive terminal protocol. Bidirectional H↔G over
> a single vsock connection. Replaces the legacy raw-bytes stream.

Port 9001 carries a framed binary protocol between a host-side terminal
and a guest-side PTY + shell. VPP uses the same **transport frame** as
MVCP (`length(4B BE)` + opaque payload) but a thinner **inner header**:
just a 1-byte type, no flags and no msg_id.

## Comparison: MVCP vs VPP

| Property              | MVCP                                | VPP (this spec)                     |
|-----------------------|-------------------------------------|-------------------------------------|
| Transport frame       | 4 bytes (`uint32` length prefix)    | 4 bytes (`uint32` length prefix) — **same as MVCP** |
| Inner header          | 6 bytes (`type`+`flags`+`msg_id`)   | 1 byte (`type` only)                |
| Total overhead/frame  | 10 bytes                            | 5 bytes                             |
| Request correlation   | `msg_id` + `IS_RESPONSE` flag       | Not needed (one session = one conn)  |
| Streaming             | `IS_STREAM_MORE` flag               | Implicit — `DATA` frames are the stream |
| Max frame             | 16 MB                               | 64 KB (VPP-enforced limit)          |
| Encoding primitives   | `WriteUint16`, `WriteString`, ...   | Same — shared from `mvcp/protocol/` |

Both protocols share the same transport framing (`ReadFrame`/`WriteFrame`
from `mvcp/protocol/frame.go`) and the same encoding primitives
(`encode.go`/`decode.go`).

## Architecture

```
Host                                    Guest
┌──────────┐                            ┌───────────┐
│ Terminal │ ←→ VPP frames ←→ vsock ←→  │ PTY+Shell │
│ (client) │        port 9001           │ (vhandler)│
└──────────┘                            └───────────┘
```

The guest manages a single PTY+shell pair per connection. Only one
session exists at a time; a new `ATTACH` destroys the previous session.

## Transport Frame (shared with MVCP)

VPP uses the same length-prefixed transport as all other ports:

```
┌──────────┬─────────────────────────────┐
│ length   │ payload                     │
│ (4B BE)  │ (N bytes)                   │
└──────────┴─────────────────────────────┘

length = N    (uint32 big-endian)
```

## VPP Protocol Layer (inner header)

Inside the transport payload, VPP adds a minimal 1-byte type dispatch:

```
┌──────┬──────────┐
│ type │ body     │
│ (1B) │ (N-1)    │
└──────┴──────────┘

payload = type(1) + body(N-1)
length  = 1 + len(body)
```

VPP enforces its own frame limit: **max 64 KB** (65535 bytes) for the
transport payload. This is enforced at the VPP layer — the transport
allows up to 16 MB but console frames larger than 64 KB are rejected.

| Field    | Size    | Description                                                  |
|----------|---------|--------------------------------------------------------------|
| `length` | 4 bytes | Transport payload size = `1 + len(body)`. Big-endian uint32. |
| `type`   | 1 byte  | Message type (see type registry below).                      |
| `body`   | N bytes | Type-specific binary encoding. `N = length - 1`.             |

**DATA frames carry raw bytes with no internal length prefix.** The
body size is `length - 1` — encoding a redundant length inside the
body wastes bytes on every frame.

### Parser (Go)

```go
// ReadFrame is mvcp/protocol.ReadFrame (shared transport)
payload, err := protocol.ReadFrame(conn)
if len(payload) < 1 || len(payload) > 65535 {
    // invalid VPP frame
}
typ    := payload[0]
body   := payload[1:]
```

## Connection Handshake

Every VPP connection starts with a 4-byte handshake sent by the guest
immediately after accept:

```
┌──────┬──────┬──────┬─────────┐
│ 'V'  │ 'P'  │ 'P'  │ version │
│ 0x56 │ 0x50 │ 0x50 │  0x01   │
└──────┴──────┴──────┴─────────┘
```

| Byte | Value  | Meaning           |
|------|--------|-------------------|
| 0    | `0x56` | 'V'               |
| 1    | `0x50` | 'P'               |
| 2    | `0x50` | 'P'               |
| 3    | `0x01` | Protocol version  |

The host reads 4 bytes after the vsock transport handshake, validates
the magic bytes and version, then proceeds to transport frame I/O.

Fire-and-forget — no host response needed. If the magic or version
is unrecognised the host closes the connection.

Host-side integration with the existing vsock dial flow:

1. Connect to Firecracker's AF_UNIX vsock socket
2. Send `CONNECT 9001\n`
3. Receive `OK <port>\n`
4. **Read 4-byte VPP handshake** (`VPP` + `0x01`)
5. Validate magic + version
6. Send `ATTACH` frame to initialise the session
7. Enter frame read/write loop

## Type Registry

| Type         | Name      | Direction | Payload                                      | Wire bytes |
|-------------|-----------|-----------|-----------------------------------------------|------------|
| `0x00`      | `DATA`    | H↔G       | `[]byte` — raw terminal I/O, length implicit   | `6 + N`    |
| `0x01`      | `WINCH`   | H→G       | `uint16 cols` + `uint16 rows`                  | `9`        |
| `0x02`      | `DETACH`  | H↔G       | `uint32 exit_code`                             | `9`        |
| `0x03`      | `ATTACH`  | H→G       | `string term` + `uint16 cols` + `uint16 rows`   | `7+2+len(term)+4` |
| `0x04`      | `SESSION` | G→H       | `uint32 session_id` + `uint32 pid` + `uint16 cols` + `uint16 rows` | `17` |
| `0x05`–`0xFF` | *(reserved)* | —       | —                                             | —          |

All multi-byte integers are **big-endian**. Strings use `uint16` length
prefix + UTF-8 bytes (same encoding as MVCP, from `mvcp/protocol/encode.go`).

Wire bytes include the 4-byte transport length prefix and the 1-byte
inner type. VPP frame bodies use the shared encoding primitives.

### `DATA` (0x00)

Bidirectional. Carries stdin bytes (H→G) and stdout/stderr bytes (G→H).
The PTY combines stdout and stderr — they are not split.

A single keystroke `'a'` fits in a 6-byte frame:
`[length=0x02] [type=0x00] [body=0x61]`.

The sender should **batch small writes** into fewer, larger frames to
amortise the 5-byte overhead. Implementations may use a buffered flush
with a short deadline (e.g. 5 ms) or a buffer threshold (e.g. 4 KB).

### `WINCH` (0x01)

Host → Guest only. Signals the guest to resize the PTY via `TIOCSWINSZ`.

| Offset | Size | Field                       |
|--------|------|-----------------------------|
| 0      | 2    | `cols` (uint16, BE)          |
| 2      | 2    | `rows` (uint16, BE)          |

The guest applies the new size immediately. No response frame is sent.

### `DETACH` (0x02)

Bidirectional. Signals end of session.

| Offset | Size | Field                       |
|--------|------|-----------------------------|
| 0      | 4    | `exit_code` (uint32, BE)     |

- **Host → Guest:** host sends `exit_code = 0`. Guest kills the shell
  (`SIGKILL`), closes the PTY, then closes the vsock connection.
- **Guest → Host:** shell exited. Guest sends the real exit code, then
  closes the vsock connection after the host has read the frame.

In both cases the connection is closed after `DETACH`. A new attach
requires a new `vsock dial`.

### `ATTACH` (0x03)

Host → Guest only. Initialises a new interactive session.

| Offset | Size   | Field                                    |
|--------|--------|------------------------------------------|
| 0      | 2 + N  | `term` (string, uint16-prefixed UTF-8)    |
| 2+N    | 2      | `cols` (uint16, BE)                       |
| 4+N    | 2      | `rows` (uint16, BE)                       |

The `term` string (e.g. `"xterm-256color"`) is set as the `TERM`
environment variable for the shell process. `cols` and `rows` set the
initial PTY dimensions.

On receiving `ATTACH` the guest:
1. Destroys any existing PTY+shell session
2. Opens a new PTY with the requested dimensions
3. Spawns `/bin/sh -i` with `TERM=<term>`, stdin/stdout/stderr connected
   to the PTY slave
4. Sends a `SESSION` frame to the host
5. Begins bidirectional `DATA` forwarding

### `SESSION` (0x04)

Guest → Host only. Confirms session creation after `ATTACH`.

| Offset | Size | Field                        |
|--------|------|------------------------------|
| 0      | 4    | `session_id` (uint32, BE)     |
| 4      | 4    | `pid` (uint32, BE)            |
| 8      | 2    | `cols` (uint16, BE)           |
| 10     | 2    | `rows` (uint16, BE)           |

- `session_id` — opaque identifier. Allocated by the guest. Currently
  starts at 1 and increments per session; reserved for future multi-session
  support.
- `pid` — shell process PID inside the guest.
- `cols` / `rows` — confirmed PTY dimensions (should match the `ATTACH`
  request; may differ if the guest enforces limits).

## Session Lifecycle

```
HOST                                    GUEST
  |                                       |
  | --- vsock dial 9001 --->              | accept()
  |                    <--- [VPP\x01] ---- handshake (4 bytes)
  |                                       |
  | --- ATTACH("xterm-256color",120,40) -->|  openPTY(120,40)
  |                                       |  fork /bin/sh -i
  |                    <--- SESSION(1,42,120,40) ---
  |                                       |
  | <============ DATA ====================>  bidirectional I/O
  |                                       |
  | --- WINCH(100,30) --->                |  ioctl(TIOCSWINSZ, 100, 30)
  |                                       |
  | --- DETACH(0) --->                    |  SIGKILL shell, close PTY
  |   (cierra conn)     <--- close -------|  close vsock fd
  |                                       |
  | ... or ...                            |
  |                                       |  shell exit(1)
  |                    <--- DETACH(1) -----|
  |   (lee exit_code)   <--- close -------|  close vsock fd
  |                                       |
  | (new dial para otra sesión)           |
```

**Rules:**

- **One session = one connection.** `DETACH` always closes the vsock
  connection from the guest side. A new session starts with a fresh
  vsock dial, a new handshake, and a new `ATTACH`.
- **ATTACH is destructive.** If a session is already running and a new
  connection sends `ATTACH`, the guest kills the old shell and PTY
  before creating the new one. Sessions are never handed off between
  connections.
- **WINCH is fire-and-forget.** No response from guest. The host may
  send `WINCH` frames at any time during a session.
- **DETACH host→guest means "disconnect me".** The host sends `exit_code=0`
  and the guest terminates the shell. Used as a clean replacement for
  the legacy Ctrl+\ (0x1C) escape.

## Encoding Primitives

VPP reuses MVCP's encoding primitives (the same `ReadUint16`,
`WriteString`, etc. functions in `mvcp/protocol/encode.go` and
`decode.go`).

| Type           | Wire                                       |
|----------------|--------------------------------------------|
| `uint16`       | 2 bytes, big-endian                        |
| `uint32`       | 4 bytes, big-endian                        |
| `string`       | `uint16` length prefix + UTF-8 bytes       |
| `bytes` (raw)  | Implicit — body is `length − 1` raw bytes  |

## Module Layout

```
mvcp/protocol/
  frame.go            # ReadFrame / WriteFrame — transport (4B BE) — shared by MVCP and VPP
  encode.go           # WriteUint16, WriteString, ... (shared)
  decode.go           # ReadUint16, ReadString, ... (shared)
  conn.go             # WriteHandshake / ValidateHandshake
  mvcp.go             # MVCP type/flags/msg_id constants, ReadMVCPFrame/WriteMVCPFrame
  message.go          # Message interface + decode registry
  messages/           # MVCP message structs (control, exec, file, fs, events, heartbeat, error)
  vpp/                # Virtual PTY Protocol (this spec)
    vpp.go            # ReadFrame / WriteFrame — uses transport ReadFrame + VPP type dispatch
    types.go          # Type constants (DATA=0x00, WINCH=0x01, ...)
    messages.go       # AttachMsg, SessionMsg, WinchMsg, DetachMsg + Encode/Decode
```

VPP's `ReadFrame`/`WriteFrame` call `protocol.ReadFrame`/`protocol.WriteFrame`
from the parent package (transport) and then add/interpret the 1-byte
type header. The VPP package does **not** duplicate transport framing code.

## Frame Size Examples

Wire sizes include 4-byte transport length prefix + 1-byte inner type.

```
keystroke 'a':      [00 00 00 02] [00] [61]                        = 6 bytes
WINCH 120×40:       [00 00 00 05] [01] [00 78] [00 28]             = 9 bytes
DETACH(exit=1):     [00 00 00 05] [02] [00 00 00 01]               = 9 bytes
ATTACH xterm...:    [00 00 00 12] [03] [00 0E] xterm-256color
                      [00 78] [00 28]                                = 22 bytes
SESSION:            [00 00 00 0D] [04] [00 00 00 01] [00 00 00 2A]
                      [00 78] [00 28]                                = 17 bytes
ls -la output (4K): [00 00 10 05] [00] <4096 raw bytes>             = 4101 bytes
```

---

See also:
- [01-transport.md](../01-transport.md) for the shared transport frame and vsock dial flow.
- [02-wire-format.md](../02-wire-format.md) for MVCP's frame layout and shared encoding primitives.
- [../SPEC.md](../SPEC.md) for the protocol hub and service port table.
- [shifty-vhandler/docs/architecture.md](../../shifty-vhandler/docs/architecture.md) for the guest-side VPP service implementation.
