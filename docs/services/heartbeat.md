# Heartbeat Service (Port 9003)

> **LEGACY — superseded by [status.md](status.md).** Port 9003 is now the
> bidirectional Status service (GET_STATUS/STATUS + HEARTBEAT). This page
> is kept for reference; it documents only the heartbeat message.

> Message type `0x07`. One-way G→H. Periodic liveness with extensible
> TLV extensions.

Port 9003 carries a periodic heartbeat from guest to host for liveness
detection. The heartbeat is a one-way frame (`msg_id = 0`) with a
20-byte fixed header and optional TLV extensions.

## Message Type

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x07` | `HEARTBEAT` | G→H | 20B header + TLV extensions |

## Fixed Header (20 bytes)

| Field | Offset | Encoding | Description |
|-------|--------|----------|-------------|
| `boot_id` | 0 | `uint64` | Boot identifier (UnixNano timestamp at VM boot) |
| `seq` | 8 | `uint64` | Monotonic counter, starts at 1 on VM boot |
| `state` | 16 | `uint8` | VM state (see below) |
| `flags` | 17 | `uint8` | Bitmask flags |
| `payload_length` | 18 | `uint16` | Length of TLV extension data that follows (0 = none) |

## States

| Value | Name | Description |
|-------|------|-------------|
| 0 | Unknown | State unknown / not set |
| 1 | Booting | VM is booting (first heartbeat) |
| 2 | Running | VM is running normally |
| 3 | Stopping | Shutdown sequence in progress |
| 4 | Stopped | Shutdown complete |
| 5 | Failed | VM has failed |

Health is **not** a VM state: it is orthogonal to the lifecycle and
carried in the `ExtHealth` TLV extension (type 4), 1 byte: `0` = Healthy,
`1` = Degraded. Default `Healthy` if the TLV is absent.

## Flags (bitmask)

| Bit | Name | Description |
|-----|------|-------------|
| 0 | Busy | VM is processing a request |
| 1 | Maintenance | VM is in maintenance mode |
| 2 | ReadOnly | Filesystem is read-only |
| 3 | LowResources | VM is low on resources |
| 4–7 | *(reserved)* | Must be zero |

## TLV Extensions

After the 20-byte header come `payload_length` bytes of
Type-Length-Value entries. Each entry is 4 bytes minimum:

| Field | Encoding | Description |
|-------|----------|-------------|
| `type` | `uint16` | Extension type identifier |
| `length` | `uint16` | Value length in bytes |
| `value` | `[]byte` | Extension value (`length` bytes) |

Parsers **skip unknown extension types** by reading `length` bytes
forward. This makes the heartbeat future-proof — new extensions can
be added without breaking existing receivers.

### Standard Extension Types

| Type | Name | Value encoding |
|------|------|----------------|
| 1 | CPU Usage | `uint32` percentage × 100 |
| 2 | Memory Usage | `uint32` MiB |
| 3 | Queue Depth | `uint32` pending tool/exec requests |
| 4 | Health | `uint8`: 0 = Healthy, 1 = Degraded |
| 5 | Failure Reason | variable-length string (State=Failed only) |
| 6–99 | *(reserved)* | — |
| 100+ | Custom | Application-specific |

## Interval

The guest emits one `HEARTBEAT` frame every **500 ms** (Shifty implementation).

## Liveness Detection (Host Side)

The host uses `time.Now()` at reception to measure "how long since the
last heartbeat".

```go
type HeartbeatInfo struct {
    BootID uint64
    Seq    uint64
    State  uint8
    Flags  uint8
}
```

- `BootID` is checked — a change means the VM restarted.
- `State` transitions are logged (Booting→Running, Running→Stopping).
- If `time.Since(LastBeat) > 5s`, the VM is considered unresponsive.
- `Seq` is checked for monotonicity (gaps indicate lost frames, skips
  indicate VM restart).

## Sequence Counter

- `uint64` — ~585 billion years before wrap at 1 Hz. Practically
  infinite.
- If wrap occurs, resets to 1.
- The host interprets a `seq` lower than the previously seen value as
  a VM restart.

## Wire Example

**HEARTBEAT with state=Running, no extensions** — 34 bytes on wire:

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

**HEARTBEAT with 3 extensions (CPU=21%, Mem=712 MiB, Queue=3)** — 58 bytes:

```
 length: 0x00_00_00_3A   (58 = 6 + 20 + 32)
 ...
    pay_len  32           → 0x00_20
    [ext 1: type=1, len=4, value=0x00_00_00_15]
    [ext 2: type=2, len=4, value=0x00_00_02_C8]
    [ext 3: type=3, len=4, value=0x00_00_00_03]
```

## Why Binary Instead of JSON

| Metric | JSON | MVCP binary (no ext) |
|--------|------|----------------------|
| Wire size | ~80 bytes | 34 bytes |
| Parse overhead | `json.Unmarshal` per beat | `binary.Read` 20 bytes |
| Encoding overhead | String keys every frame | 1-byte type + positional payload |
| Extensibility | JSON keys | TLV extensions (skip unknown) |

---

See also:
- [status.md](status.md) for the full status service (GET_STATUS/STATUS RPC + heartbeat).
- [control.md](control.md) for other control-plane messages like PING/PONG and SHUTDOWN.
- [01-transport.md](../01-transport.md) for the vsock connection model.
- [02-wire-format.md](../02-wire-format.md) for the frame layout and encoding primitives.
- [examples/heartbeat.md](../examples/heartbeat.md) for the wire-level example.
