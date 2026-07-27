# Console Service (Port 9001)

> Raw bytes. No MVCP framing. Bidirectional H↔G.

Port 9001 carries a raw byte stream between a host-side PTY and the
guest's shell process. It is deliberately **not** wrapped in MVCP
frames — there is no benefit to framing interactive terminal I/O.

## Architecture

```
Host                           Guest
┌──────────┐                  ┌───────────┐
│  PTY     │ ←→ vsock ←→      │ /bin/sh   │
│ (master) │   port 9001      │ (slave)   │
└──────────┘                  └───────────┘
```

The guest opens a PTY pair:
- Master: connected to the vsock listener on port 9001
- Slave: connected to `stdin`/`stdout`/`stderr` of `/bin/sh -i`

Every byte written by the host is forwarded directly to the shell's
stdin. Every byte written by the shell is forwarded directly to the
host.

## Connection Lifecycle

- A single persistent connection on port 9001.
- The connection is established when the VM boots and the console
  service starts.
- When the shell exits (or the VM shuts down), the vsock connection
  is closed.
- The host can detach (close its side) without stopping the VM. The
  guest keeps the shell running.

## Window Size

Terminal window resize is **not yet handled** by the protocol. The
initial PTY size is fixed (80×24). Future versions may add a control
channel or inline escape sequences for SIGWINCH.

## No Protocol Overhead

This service has zero framing overhead:
- No handshake (handshake happens at vsock connection level)
- No length prefix
- No message types
- Just raw bytes, both directions

---

See also:
- [01-transport.md](../01-transport.md) for the vsock transport layer.
- [07-vhandler.md](../../../docs/agents/07-vhandler.md) for the guest-side PTY implementation.
