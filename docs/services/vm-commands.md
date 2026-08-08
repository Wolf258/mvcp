# VM Commands Service

> Range `0x40`–`0x4F`. Carried on port 9000 (Control port).

Reserved for VM-specific control operations beyond the generic
execution and tools primitives.

## SyncFilesystems

`SYNC_FILESYSTEMS` is the first VM-control operation in this range. It lets
the host request an in-guest filesystem flush before it pauses or stops
Firecracker to capture an overlay, filesystem snapshot, or checkpoint.

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x40` | `SYNC_FILESYSTEMS` | H→G | none |
| `0x41` | `SYNC_FILESYSTEMS_ACK` | G→H | none |

The guest performs `sync(2)` across its mounted filesystems and sends the ACK
only after that call returns. The request accepts no command, path, or other
caller-controlled payload; it must not be implemented by executing a guest
shell command.

## Reserved Operations

This range is reserved for commands that are specific to the VM
lifecycle and guest environment, such as:

| Tentative type | Name | Direction | Purpose |
|----------------|------|-----------|---------|
| `0x42` | — | H→G | Network reconfiguration |
| `0x43` | — | H→G | Resource limit adjustment |
| `0x44`–`0x4F` | — | — | Reserved for future VM-specific ops |

## Design Notes

- These commands are distinct from generic execution (`0x10`) because
  they target the VM runtime itself, not a user process.
- Payload formats will follow the same encoding primitives defined in
  [02-wire-format.md](../02-wire-format.md).
- Each command will be documented in its own service doc once
  finalized.

---

See also:
- [control.md](control.md) for generic control-plane messages (PING, SHUTDOWN).
- [execution.md](execution.md) for user-command execution inside the VM.
- [status.md](status.md) for VM status queries (GET_STATUS, STATUS) on port 9003.
