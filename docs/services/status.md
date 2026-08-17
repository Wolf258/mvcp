# Status Service (Port 9003)

> Message types `0x05`–`0x07`. Bidirectional H↔G.
> Carries periodic heartbeats (G→H) and status RPC (H→G→H).

Port 9003 is the **status channel** — a bidirectional MVCP stream that
carries two categories of traffic on a single connection:

1. **Periodic heartbeat** (G→H, `msg_id = 0`): unidirectional liveness
   ticks emitted at a fixed interval.
2. **Status RPC** (H→G→H, correlated `msg_id`): request/response pairs
   for querying guest runtime state.

The host opens **one persistent connection** to port 9003 and holds it
for the VM lifetime. Both heartbeat frames and status responses arrive
interleaved on the same vsock stream. The `type` byte disambiguates
them — `0x07` is a heartbeat tick, `0x06` is a STATUS response.

## Message Types

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x05` | `GET_STATUS` | H→G | *(none)* |
| `0x06` | `STATUS` | G→H | `string version`, `uint32 pid`, `bool shutting_down` |
| `0x07` | `HEARTBEAT` | G→H | `uint64 seq` |

### GET_STATUS / STATUS

Query guest runtime state. The host sends `GET_STATUS` with a `msg_id`;
the guest responds with `STATUS` carrying `IS_RESPONSE` and the matching
`msg_id`.

**STATUS** payload:

| Field | Encoding | Description |
|-------|----------|-------------|
| `version` | `string` | vhandler version string |
| `pid` | `uint32` | Guest PID 1 process ID |
| `shutting_down` | `bool` | `true` if shutdown sequence is in progress |

**Wire example** — GET_STATUS (H→G, 12 bytes):

```
 length: 0x00_00_00_06   (6 = type+flags+msg_id, no body)
   type: 0x05             (GET_STATUS)
  flags: 0x00
 msg_id: 0x00_00_00_01    (request)
```

**Wire example** — STATUS response (G→H, 25 bytes with version "0.1.0"):

```
 length: 0x00_00_00_0D   (13 = 6 header + 7 body)
   type: 0x06             (STATUS)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches request)
   body:
   string "0.1.0"         → 0x00_05 + "0.1.0"  (len=5)
   uint32 1               → 0x00_00_00_01
   bool false             → 0x00
```

### HEARTBEAT

Periodic liveness pulse with VM state and extensible TLV extensions.
Emitted G→H every **500 ms** (Shifty implementation) with `msg_id = 0x00000000` (one-way,
no response expected). The host uses `time.Now()` at reception to
measure "time since last beat."

**Fixed header** — 20 bytes:

| Field | Encoding | Description |
|-------|----------|-------------|
| `boot_id` | `uint64` | Boot identifier (UnixNano timestamp at VM boot) |
| `seq` | `uint64` | Monotonic counter, starts at 1 on VM boot |
| `state` | `uint8` | VM state (see state table below) |
| `flags` | `uint8` | Bitmask flags (see flags table below) |
| `payload_length` | `uint16` | Total length of TLV extension data that follows |

After the 20-byte header, `payload_length` bytes of **TLV extensions** follow.
Each extension is:

| Field | Encoding | Description |
|-------|----------|-------------|
| `type` | `uint16` | Extension type identifier |
| `length` | `uint16` | Value length in bytes |
| `value` | `[]byte` | Extension value |

Parsers **skip unknown extension types** by reading `length` bytes
forward. This makes the heartbeat future-proof — new extensions can be
added without breaking existing receivers.

**States**:

| Value | Name | Description |
|-------|------|-------------|
| 0 | Unknown | State unknown / not set |
| 1 | Booting | VM is booting (first heartbeat only) |
| 2 | Running | VM is running normally |
| 3 | Stopping | Shutdown sequence in progress |
| 4 | Stopped | Shutdown complete |
| 5 | Failed | VM has failed |

Health is **not** a VM state: it is orthogonal to the lifecycle and
carried in the `ExtHealth` TLV extension (type 4), 1 byte: `0` = Healthy,
`1` = Degraded. Default `Healthy` if the TLV is absent.

**Flags** (bitmask):

| Bit | Name | Description |
|-----|------|-------------|
| 0 | Busy | VM is processing a request |
| 1 | Maintenance | VM is in maintenance mode |
| 2 | ReadOnly | Filesystem is read-only |
| 3 | LowResources | VM is low on resources |

**Extension types** (standard range: 1–99, custom: 100+):

| Type | Name | Value encoding |
|------|------|----------------|
| 1 | CPU Usage | `uint32` percentage × 100 |
| 2 | Memory Usage | `uint32` MiB |
| 3 | Queue Depth | `uint32` pending requests |
| 4 | Health | `uint8`: 0 = Healthy, 1 = Degraded |
| 5 | Failure Reason | variable-length string (State=Failed only) |

**Wire example** — 38 bytes total (34 payload + 4 transport), no extensions:

```
 length: 0x00_00_00_22   (34 = 6 header + 20 body + 0 ext)
   type: 0x07             (HEARTBEAT)
  flags: 0x00
 msg_id: 0x00_00_00_00    (one-way)
   body:
    boot_id  42           → 0x00_00_00_00_00_00_00_2A
    seq      991          → 0x00_00_00_00_00_00_03_DF
    state    2 (Running)  → 0x02
    flags    1 (Busy)     → 0x01
    pay_len  0            → 0x00_00
```

**Wire example** — with 3 extensions (CPU=21%, Mem=712 MiB, Queue=3):

```
 length: 0x00_00_00_3A   (58 = 6 + 20 + 32)
 ...
    pay_len  32           → 0x00_20
    [ext 1: type=1, len=4, value=0x00_00_00_15]
    [ext 2: type=2, len=4, value=0x00_00_02_C8]
    [ext 3: type=3, len=4, value=0x00_00_00_03]
   = 3 × (2+2+4) = 24 bytes + 8 unused = 32 bytes total
```

**Future-proofing** — within 2 years the heartbeat might carry:

```
CPU, Memory, GPU, Containers, Filesystem, Network, AgentStatus, UserServices
```

Old parsers parse the 20-byte fixed header and skip unknown extension
types. Zero code changes required on the receiver side for new
extension types. The sender simply appends new TLV entries.

## Connection Model

```
Host                                Guest (port 9003)
  |                                       |
  |--- CONNECT 9003 -------------------> |
  |<-- OK 9003 --------------------------|
  |<-- MVCP handshake (5B) --------------|
  |                                       |
  |<-- HEARTBEAT(seq=1) [1s] ------------|
  |                                       |
  |--- GET_STATUS(msg_id=1) ------------> |
  |<-- STATUS(msg_id=1, IS_RESPONSE) ----|
  |                                       |
  |<-- HEARTBEAT(seq=2) [1s] ------------|
  |<-- HEARTBEAT(seq=3) [1s] ------------|
  |                                       |
  |--- GET_STATUS(msg_id=2) ------------> |
  |<-- STATUS(msg_id=2, IS_RESPONSE) ----|
  |                                       |
  ... (heartbeats continue indefinitely) ...
```

Heartbeats and status RPC are **multiplexed** on the same connection.
The host read loop checks the `type` byte:
- `0x07` → update liveness tracker
- `0x06` → route to the pending request by `msg_id`

## Liveness Detection (Host Side)

Same as the legacy heartbeat model but with MVCP binary frames and
extended header:

- `BootID` is checked — a change means the VM restarted.
- `State` transitions are logged (Booting→Running, Running→Stopping, etc.).
- If `time.Since(LastBeat) > 5s`, the VM is considered unresponsive.
- `Seq` is checked for monotonicity (gaps = lost frames, reset = VM restart).

## Sequence Counter

- `uint64` — practically infinite at 1 Hz.
- If wrap occurs, resets to 1.
- Host interprets `seq < previous` as a VM restart.

---

See also:
- [control.md](control.md) for other control-plane messages (PING/PONG, SHUTDOWN).
- [01-transport.md](../01-transport.md) for the vsock connection model.
- [02-wire-format.md](../02-wire-format.md) for the frame layout and encoding primitives.
- [examples/heartbeat.md](../examples/heartbeat.md) for the wire-level heartbeat example.
