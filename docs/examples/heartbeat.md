# Example: Heartbeat

Periodic liveness heartbeat from guest to host on port 9003.

## HEARTBEAT (Guest → Host, 14 bytes on wire)

```
 length: 0x00_00_00_0E   (14 = 6 + 8 payload)
   type: 0x07             (HEARTBEAT)
  flags: 0x00
 msg_id: 0x00_00_00_00    (one-way, no response)
payload:
  uint64 42               → 0x00_00_00_00_00_00_00_2A
```

### Total Wire Bytes: 18 (4 length + 14 frame)

### Payload Breakdown

| Offset | Bytes | Field | Value |
|--------|-------|-------|-------|
| 0 | `00 00 00 00 00 00 00 2A` | seq | 42 |

## Sequence Progression

```
Beat 1:  seq=1   → 0x00_00_00_00_00_00_00_01
Beat 2:  seq=2   → 0x00_00_00_00_00_00_00_02
Beat 3:  seq=3   → 0x00_00_00_00_00_00_00_03
...
```

## Go: Sending Heartbeats (Guest)

```go
seq := uint64(1)
ticker := time.NewTicker(1 * time.Second)
defer ticker.Stop()

for range ticker.C {
    var buf bytes.Buffer
    protocol.WriteUint64(&buf, seq)

    frame := &protocol.Frame{
        Type:    protocol.TypeHeartbeat,
        MsgID:   0, // one-way
        Payload: buf.Bytes(),
    }
    frame.WriteTo(conn)
    seq++
}
```

## Go: Monitoring Heartbeats (Host)

```go
type Liveness struct {
    LastBeat time.Time
    Seq      uint64
}

func (l *Liveness) Feed(seq uint64) {
    if seq < l.Seq {
        log.Println("VM restarted (seq reset)")
    }
    l.LastBeat = time.Now()
    l.Seq = seq
}

func (l *Liveness) Alive() bool {
    return time.Since(l.LastBeat) < 5*time.Second
}
```

## Wire Size Comparison

| Format | Bytes per heartbeat | Bytes per minute |
|--------|---------------------|-------------------|
| JSON | ~80 | ~4,800 |
| MVCP binary | 18 | 1,080 |
| **Saving** | **~78%** | **~78%** |
