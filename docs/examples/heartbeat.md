# Example: Heartbeat

Periodic liveness heartbeat from guest to host on port 9003.

The heartbeat carries two orthogonal dimensions on the wire:

- **Lifecycle** (1 byte in the fixed header, field `State`): the VM's
  position in the boot/run/stop pipeline. Mutually exclusive.
- **Health** (1-byte TLV in the body, type `ExtHealth=4`): the VM's
  resource health. Independent of lifecycle. Default `Healthy` if
  the TLV is absent.

A `State=Failed` heartbeat MAY carry an additional string TLV
`ExtFailureReason=5` with a short machine-readable reason
(`init_timeout`, `mount_failed`, etc.).

## HEARTBEAT with state=Running, health=Healthy, no other extensions

```
 length: 0x00_00_00_27   (39 = 6 + 20 body + 5 bytes of extensions)
   type: 0x07             (HEARTBEAT)
  flags: 0x00
 msg_id: 0x00_00_00_00    (one-way, no response)
payload:
  boot_id     42           → 0x00_00_00_00_00_00_00_2A
  seq         991          → 0x00_00_00_00_00_00_03_DF
  state       2 (Running)  → 0x02
  flags       0            → 0x00
  pay_length  5            → 0x00_05

  Extensions (5 bytes):
  [type=4, len=1] 0x00       → Health = Healthy
```

### Total Wire Bytes: 43 (4 length + 39 frame)

### Payload Breakdown

| Offset | Bytes | Field | Value |
|--------|-------|-------|-------|
| 0 | `00 00 00 00 00 00 00 2A` | boot_id | 42 |
| 8 | `00 00 00 00 00 00 03 DF` | seq | 991 |
| 16 | `02` | state | Running |
| 17 | `00` | flags | (none) |
| 18 | `00 05` | payload_length | 5 |
| 20 | `00 04 00 01 00` | ext: Health=Healthy | TLV[4,1,0] |

## HEARTBEAT with state=Failed, reason=init_timeout

```
 length: 0x00_00_00_2F   (47 = 6 + 20 + 21 bytes of extensions)
   type: 0x07
  flags: 0x00
 msg_id: 0x00_00_00_00
payload:
  boot_id     42           → 0x00_00_00_00_00_00_00_2A
  seq         1000         → 0x00_00_00_00_00_00_03_E8
  state       5 (Failed)   → 0x05
  flags       0            → 0x00
  pay_length  21           → 0x00_15

  Extensions (21 bytes):
  [type=4, len=1] 0x00                              → Health = Healthy
  [type=5, len=12] "init_timeout"                   → FailureReason
```

## HEARTBEAT with extensions (CPU=21%, Mem=712 MiB, Queue=3, Health=Degraded)

```
 length: 0x00_00_00_43   (67 = 6 + 20 + 41 bytes of extensions)
   type: 0x07
  flags: 0x00
 msg_id: 0x00_00_00_00
payload:
  boot_id     42           → 0x00_00_00_00_00_00_00_2A
  seq         995          → 0x00_00_00_00_00_00_03_E3
  state       2 (Running)  → 0x02
  flags       0x01 (Busy)  → 0x01
  pay_length  37           → 0x00_25

  Extensions (37 bytes):
  [type=1, len=4] 0x00_00_00_15  → CPU = 21%
  [type=2, len=4] 0x00_00_02_C8  → Mem = 712 MiB
  [type=3, len=4] 0x00_00_00_03  → Queue = 3 pending
  [type=4, len=1] 0x01            → Health = Degraded
```

## Sequence Progression

```
Beat 1:   seq=1,  state=Booting,  health=Healthy    (VM booting)
Beat 2:   seq=2,  state=Running,  health=Healthy    (VM ready)
Beat 3:   seq=3,  state=Running,  health=Healthy    (Busy processing)
...
Beat K:   seq=K,  state=Running,  health=Degraded   (resource pressure)
...
Beat N:   seq=N,  state=Stopping, health=Healthy    (Shutdown initiated)
Beat N+1: (last) — vsock closed, no further heartbeats
```

## Go: Sending Heartbeats (Guest)

```go
var bootID = uint64(time.Now().UnixNano())
var heartbeatState = protocol.HeartbeatStateRunning
var healthState = protocol.HealthHealthy

func encodeHeartbeatFrame(seq uint64) []byte {
    body := &messages.HeartbeatMsg{
        BootID: bootID,
        Seq:    seq,
        State:  heartbeatState,
        Health: healthState,
    }
    raw, _ := body.MarshalBinary()
    // ... wrap in MVCP frame ...
}
```

## Go: Parsing Heartbeats (Host)

```go
type HeartbeatInfo struct {
    BootID        uint64
    Seq           uint64
    State         uint8
    Health        uint8
    Flags         uint8
    FailureReason messages.HeartbeatFailureReason
}

func parseHeartbeat(body []byte) (HeartbeatInfo, error) {
    msg := &messages.HeartbeatMsg{}
    if err := msg.UnmarshalBinary(body); err != nil {
        return HeartbeatInfo{}, err
    }
    return HeartbeatInfo{
        BootID:        msg.BootID,
        Seq:           msg.Seq,
        State:         msg.State,
        Health:        msg.Health,
        Flags:         msg.Flags,
        FailureReason: msg.FailureReason,
    }, nil
}
```

## Future-Proof Extensions

Within 2 years the heartbeat may carry:

```
CPU, Memory, GPU, Containers, Filesystem, Network, AgentStatus, UserServices
```

Old parsers read the 20-byte fixed header and skip unknown extension
types by reading `length` bytes forward. Zero code changes needed on
the receiver for new extensions.

## Wire Size Comparison

| Format | Bytes per heartbeat | Bytes per minute (at 1 Hz) |
|--------|---------------------|----------------------------|
| JSON | ~80 | ~4,800 |
| MVCP binary (no ext) | 38 | 2,280 |
| MVCP binary (3 ext) | 62 | 3,720 |
| MVCP binary (Health + 3 ext) | 67 | 4,020 |
| **Saving (no ext)** | **~53%** | **~53%** |
