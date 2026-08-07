# VM Commands Service

> **Planned — not yet implemented.** Reserved range `0x40`–`0x4F`.
> Carried on port 9000 (Control port).
>
> Status: no message types defined, no wire format specified, no
> implementation. This is a placeholder for future VM lifecycle
> operations.

Reserved for VM-specific control operations beyond the generic
execution and tools primitives.

## Planned Operations

This range is reserved for commands that are specific to the VM
lifecycle and guest environment, such as:

| Tentative type | Name | Direction | Purpose |
|----------------|------|-----------|---------|
| `0x40` | — | H→G | Snapshot creation trigger |
| `0x41` | — | H→G | Overlay reset / discard |
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
