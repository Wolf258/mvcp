# Heartbeat Service (Port 9003)

> Message type `0x07`. One-way G→H.

Port 9003 carries a periodic heartbeat from guest to host for liveness
detection. The heartbeat is a one-way frame (`msg_id = 0`) with a
monotonic sequence counter.

## Message Type

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x07` | `HEARTBEAT` | G→H | `uint64 seq` |

## Payload

| Field | Encoding | Description |
|-------|----------|-------------|
| `seq` | `uint64` | Monotonic sequence counter, starts at 1 on VM boot |

## Interval

The guest emits one `HEARTBEAT` frame every **1 second**.

- Fast enough for crash detection (host can declare the VM dead after
  3–5 missed heartbeats).
- Slow enough to be negligible overhead: **18 bytes/s**.

## Liveness Detection (Host Side)

The host uses `time.Now()` at reception to measure "how long since the
last heartbeat". This is more reliable than trusting the guest's clock
(which can drift, especially under load).

```go
type Liveness struct {
    LastBeat time.Time
    Seq      uint64
}
```

- `LastBeat` is updated on every received heartbeat.
- If `time.Since(LastBeat) > 5s`, the VM is considered unresponsive.
- `Seq` is checked for monotonicity (gaps indicate lost frames, skips
  indicate VM restart).

## Sequence Counter

- `uint64` — ~585 billion years before wrap at 1 Hz. Practically
  infinite.
- If wrap occurs (or the counter resets), it resets to 1.
- The host interprets a `seq` lower than the previously seen value as
  a VM restart (counter reset).

## Wire Example

**HEARTBEAT** (guest → host, 14 bytes on wire):

```
 length: 0x00_00_00_0E   (14 = 6 header + 8 payload)
   type: 0x07             (HEARTBEAT)
  flags: 0x00
 msg_id: 0x00_00_00_00    (one-way)
payload:
  uint64 42               → 0x00_00_00_00_00_00_00_2A
```

**Total wire bytes: 18** (4 length + 14 frame). JSON equivalent:
~80 bytes. **~78% reduction.**

## Why Binary Instead of JSON

| Metric | JSON | MVCP binary |
|--------|------|-------------|
| Wire size | ~80 bytes | 18 bytes |
| Parse overhead | `json.Unmarshal` per beat | `binary.Read` 8 bytes |
| Encoding overhead | String keys every frame | 1-byte type + positional payload |

---

See also:
- [control.md](control.md) for other control-plane messages like PING/PONG and STATUS.
- [01-transport.md](../01-transport.md) for the vsock connection model.
- [02-wire-format.md](../02-wire-format.md) for the frame layout and encoding primitives.
- [examples/heartbeat.md](../examples/heartbeat.md) for the wire-level example.
