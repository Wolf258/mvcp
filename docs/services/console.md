# Virtual PTY Protocol — VPP (Port 9001)

> **Wire status:** frames `0x00`–`0x05` implemented (phases 0–2, frozen):
> `KILL` (0x05) and `AttachMsg.SessionID` landed with the phase-2 session
> registry (see `docs/tmux-console.md` §6–8). The wire is frozen after
> phase 2 — no further additions are planned.

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
| Request correlation   | `msg_id` + `IS_RESPONSE` flag       | Not needed (join-or-create; `session_id` in `ATTACH`) |
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

The guest keeps a **session registry**. Each `ConsoleSession` owns one
PTY+shell pair and may have **0..N attached connections** (multi-client).
`ATTACH` is **join-or-create** and never destructive: a session only dies
via an explicit `KILL`, the shell exiting, or the VM going away.

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

VPP enforces its own frame limit: **max 64 KB** (65536 bytes) for the
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
if len(payload) < 1 || len(payload) > 65536 {
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
6. Send `ATTACH` frame to create or join the session
7. Enter frame read/write loop

## Type Registry

| Type         | Name      | Direction | Payload                                      | Wire bytes |
|-------------|-----------|-----------|-----------------------------------------------|------------|
| `0x00`      | `DATA`    | H↔G       | `[]byte` — raw terminal I/O, length implicit   | `5 + N`    |
| `0x01`      | `WINCH`   | H→G       | `uint16 cols` + `uint16 rows`                  | `9`        |
| `0x02`      | `DETACH`  | H↔G       | `uint32 exit_code`                             | `9`        |
| `0x03`      | `ATTACH`  | H→G       | `string term` + `uint16 cols` + `uint16 rows` + `uint32 session_id` | `15 + len(term)` |
| `0x04`      | `SESSION` | G→H       | `uint32 session_id` + `uint32 pid` + `uint16 cols` + `uint16 rows` | `17` |
| `0x05`      | `KILL`    | H→G       | *(empty)*                                      | `5`        |
| `0x06`–`0xFF` | *(reserved)* | —     | —                                              | —          |

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

Bidirectional. H→G: detaches **this connection** — the session keeps
running. G→H: the shell exited and the session ends.

| Offset | Size | Field                       |
|--------|------|-----------------------------|
| 0      | 4    | `exit_code` (uint32, BE)     |

- **Host → Guest:** the host detaches **this connection** and sends
  `exit_code=0`, which the guest ignores. The guest removes the
  connection from its session and keeps the session running (DETACHED if
  it was the last connection); the connection closes after the frame. To
  destroy the session instead, send `KILL` (0x05).
- **Guest → Host:** the shell exited. Guest broadcasts the real exit code
  to every attached connection, then closes them.

After a host→guest `DETACH` the connection is closed; re-attaching to the
same (still running) session requires a new `vsock dial` + `ATTACH`.

### `ATTACH` (0x03)

Host → Guest only. Creates or joins an interactive session.

| Offset | Size   | Field                                    |
|--------|--------|------------------------------------------|
| 0      | 2 + N  | `term` (string, uint16-prefixed UTF-8)    |
| 2+N    | 2      | `cols` (uint16, BE)                       |
| 4+N    | 2      | `rows` (uint16, BE)                       |
| 6+N    | 4      | `session_id` (uint32, BE)                 |

The `term` string (e.g. `"xterm-256color"`) is set as the `TERM`
environment variable for the shell process. `cols` and `rows` set the
initial PTY dimensions.

`session_id` selects the target session: `0` (or absent — old hosts) means
**join-or-create** — join the existing session for this guest, or create
one; a non-zero id targets that specific session, creating it with the
requested id if absent. The field is appended at the end of the payload so
old decoders still read `term`/`cols`/`rows` correctly (trailing bytes are
ignored) and old hosts read as 0.

On receiving `ATTACH` the guest:
1. Resolves the target session per `session_id` — **never destructive**:
   existing sessions are joined, not replaced
2. If the session is new: opens a PTY with the requested dimensions,
   spawns the console shell with `TERM=<term>` — `/bin/bash -i` when the
   image provides it, `/bin/sh -i` otherwise (bash is the supported
   console shell, see `docs/tmux-console.md` §5) — stdin/stdout/stderr
   connected to the PTY slave (`TERM` and size are fixed at creation;
   later joins only apply their carried size)
3. Sends a `SESSION` frame to the host — the same `session_id` for every
   connection attached to the same session
4. Begins bidirectional `DATA` forwarding

The guest waits for the client's **first frame** before spawning the
shell, so a spec-compliant host sends `ATTACH` immediately after the
handshake — it determines the shell's `TERM` and the initial PTY size.
Clients that send `DATA`/`WINCH` first or stay silent fall back to
`TERM=linux` and an 80x24 window after a short timeout; a first frame
of `DETACH` closes the connection without creating or joining anything.

### `SESSION` (0x04)

Guest → Host only. Confirms session creation after `ATTACH`.

| Offset | Size | Field                        |
|--------|------|------------------------------|
| 0      | 4    | `session_id` (uint32, BE)     |
| 4      | 4    | `pid` (uint32, BE)            |
| 8      | 2    | `cols` (uint16, BE)           |
| 10     | 2    | `rows` (uint16, BE)           |

- `session_id` — opaque identifier allocated by the guest (increments per
  session). Stable for the lifetime of the session; every connection
  attached to the same session receives the same id.
- `pid` — shell process PID inside the guest.
- `cols` / `rows` — confirmed PTY dimensions (should match the `ATTACH`
  request; may differ if the guest enforces limits).

### `KILL` (0x05)

Host → Guest only. Destroys the session this connection is attached to:
SIGKILL the shell, close the PTY, remove the session from the registry,
broadcast `DETACH{exit_code}` to the remaining connections, then close.
The body is empty.

Guards:

- A `KILL` received on a connection that is not attached to a live
  session (or targeting an already-dead session) is a **no-op**: the
  guest simply closes the connection.
- A dropped connection (queue full) is closed **immediately**; in-flight
  frames from a dropped connection are ignored — a `KILL` racing a drop
  cannot destroy the session.

## Session Lifecycle

```
HOST                                    GUEST
  |                                       |
  | --- vsock dial 9001 --->              | accept()
  |                    <--- [VPP\x01] ---- handshake (4 bytes)
  |                                       |
  | --- ATTACH(0,"xterm-256color",120,40) ->|  no session → create PTY + console shell
  |                    <--- SESSION(1,42,120,40) ---
  |                                       |
  | <============ DATA ====================>  bidirectional I/O
  |                                       |
  | --- WINCH(100,30) --->                |  ioctl(TIOCSWINSZ, 100, 30)
  |                                       |
  | --- DETACH(0) --->                    |  remove connection; session keeps
  |   (closes conn)                       |  running (DETACHED until re-attach)
  |                                       |
  | ... or ...                            |
  | --- KILL --->                         |  SIGKILL shell, close PTY, delete
  |   (closes conn)                       |  session; broadcast DETACH to the rest
  |                                       |
  | ... or ...                            |
  |                                       |  shell exit(1)
  |                    <--- DETACH(1) -----|
  |   (reads exit_code)   <--- close -----|  close vsock fd
  |                                       |
  | second client, same session:          |
  | --- ATTACH(1,"tmux-256color",120,40) -> join existing session (never destructive)
  |                    <--- SESSION(1,42,120,40) ---  (same id, same pid)
```

**Rules:**

- **One session = one PTY+shell; 0..N connections.** `ATTACH` joins or
  creates — it never destroys. New connections start with a fresh
  vsock dial, a new handshake, and a new `ATTACH`.
- **A connection leaving never kills the session.** `DETACH` (H→G) and
  unexpected EOF both just remove that connection; the session keeps
  running and can be re-attached (DETACHED state). Only `KILL`, the
  shell exiting, or the VM going away destroys a session.
- **`KILL` (H→G) destroys the session** this connection is attached to:
  SIGKILL the shell, close the PTY, remove the session from the registry,
  broadcast `DETACH{exit_code}` to the remaining connections, then close.
- **A dropped connection is closed immediately.** When a connection's
  bounded queue overflows (slow client), the guest closes its fd right
  away and ignores any in-flight frames from it — a `KILL` racing a drop
  cannot destroy the session.
- **`KILL` to an unattached/unknown session is a no-op.** A connection
  that is not attached to a live session cannot destroy anything — the
  guest simply closes the connection.
- **WINCH is fire-and-forget.** No response from guest. The host may
  send `WINCH` frames at any time during a session. The PTY has a single
  shared size: the last `WINCH` (or the size carried by a joining
  `ATTACH`) wins for all connections.
- **DETACH host→guest means "detach this connection".** The session
  survives. Clean replacement for the legacy Ctrl+\ (0x1C) escape on the
  raw path.

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
  handshake.go        # MVCP handshake: "MVCP"+0x01 + HELLO exchange (ServerHandshake/ClientHandshake)
  hello.go            # HELLO: roles, capabilities, strict Encode/Decode
  capabilities.go     # Negotiate, per-port Requirements, HandshakeTimeout (2s)
  conn.go             # VPP handshake only: "VPP"+0x01 (WriteVPPHandshake / ValidateVPPHandshake)
  mvcp.go             # MVCP type/flags/msg_id constants, ReadMVCPFrame/WriteMVCPFrame
  message.go          # Message interface + decode registry
  messages/           # MVCP message structs (control, exec, file, events, status, tools, started, error)
  vpp/                # Virtual PTY Protocol (this spec)
    vpp.go            # ReadFrame / WriteFrame — uses transport ReadFrame + VPP type dispatch
    types.go          # Type constants (DATA=0x00, WINCH=0x01, ...) + AttachMsg, SessionMsg, WinchMsg, DetachMsg + Encode/Decode
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
KILL:               [00 00 00 01] [05]                              = 5 bytes
ATTACH xterm...:    [00 00 00 19] [03] [00 0E] xterm-256color
                      [00 78] [00 28] [00 00 00 00]                  = 29 bytes
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
