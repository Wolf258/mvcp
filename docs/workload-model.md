# Workload Model

> Conceptual model for workloads running inside a Shifty VM: Tools,
> Processes, and Services. Defines lifecycles, ownership, cleanup
> policies, and the vhandler's role as workload supervisor.

## Overview

The vhandler (guest PID 1) manages three categories of workloads. Each
has different semantics, lifecycle, and ownership — the protocol
channels reflect these distinctions.

```
┌───────────────────────────────────────────┐
│                Shifty VM                  │
│                                           │
│               vhandler (PID 1)            │
│                    │                      │
│       ┌────────────┼────────────┐         │
│       │            │            │         │
│   Services      Processes      Tools      │
│       │            │            │         │
│   declarative   dynamic      ephemeral    │
│       │            │            │         │
│       └────────────┼────────────┘         │
│                    │                      │
│              Observability                │
│          ┌─────────┼─────────┐            │
│          │         │         │            │
│        logs      events     PTY           │
└───────────────────────────────────────────┘
```

## Comparison

| Property     | Tool                    | Process                    | Service                    |
|-------------|-------------------------|----------------------------|----------------------------|
| **Created by** | LLM (one-shot RPC)    | LLM (spawn request)        | VM manifest / init config  |
| **Lifetime**   | Single invocation     | Agent session or persistent| VM lifetime                |
| **Channel**    | Port 9000 (RPC)       | Port 9000 (RPC)            | Port 9000 (RPC)            |
| **Identity**   | None                  | `process_id`               | `service_id`               |
| **PTY**        | No                    | Optional                   | No (stdout/stderr only)    |
| **Streaming**  | Unary only            | Yes (buffered + eventing)  | Yes (buffered)             |
| **Triggers**   | No                    | Yes (configured at spawn)  | No (service-level events)  |
| **Owner**      | —                     | agent / user               | system                     |
| **Persistent** | N/A                   | Configurable               | Always                     |
| **Example**    | glob, grep, bash      | `npm run dev`, `go test`   | `postgres`, `./my-server`  |

## 1. Tools

### Semantics

Tools are one-shot, unary RPCs. The LLM invokes a tool, the guest
executes it, and the result is returned in a single response. There is
no ongoing state — every call is independent.

```
REQUESTED → RUNNING → COMPLETED
```

- **Channel**: port 9000 (MVCP + RPC).
- **Wire**: `TOOL_CALL` (type `0x30`) → `TOOL_RESULT` (type `0x31`).
- **No identity**: there is no `tool_id`. The tool is the function, not
  an entity.
- **No PTY, no triggers, no persistence.** Tools are pure functions
  over the guest environment.

### Tool contract

Every tool accepts opaque `params` bytes and returns opaque `result`
bytes. The protocol only enforces the `TOOL_CALL` / `TOOL_RESULT`
envelope; each tool defines its own binary layout for params and result
using MVCP primitives. See [tools.md](services/tools.md) for the full
spec and built-in tool definitions.

## 2. Processes

### Semantics

A process is a workload **spawned by the agent** that has identity,
lifecycle, and observability. It is not a one-shot command — it is an
entity the agent can inspect, stream logs from, attach to, kill, and
restart.

```
                 ┌──────────┐
                 │  CREATED │
                 └────┬─────┘
                      │ start
                      ▼
                 ┌──────────┐    signal / exit
                 │ RUNNING  │─────────────────┐
                 └────┬─────┘                 │
                      │                       ▼
                      │                ┌───────────┐
                      └───────────────→│  EXITED   │
                                       └───────────┘
```

### Process identity

| Field        | Type     | Description                                   |
|-------------|----------|-----------------------------------------------|
| `process_id` | string   | Unique identifier assigned by vhandler         |
| `pid`        | uint32   | Guest OS PID                                   |
| `command`    | string   | Full command line                              |
| `cwd`        | string   | Working directory                              |
| `env`        | map      | Environment variables                          |
| `owner`      | enum     | `agent` or `user`                              |
| `state`      | enum     | `created` → `running` → `exited`               |
| `started_at` | uint64   | Unix nanosecond timestamp                      |
| `exit_code`  | int32    | Exit code. `-1` if killed by signal.           |
| `persistent` | bool     | Survives agent disconnect                      |
| `pty`        | bool     | Allocated a pseudo-terminal                    |

### Triggers

Configured at spawn time. Each trigger is a regex pattern matched
against process stdout/stderr output. When a trigger matches, the
vhandler emits a `PROC_EVENT` on the events channel (port 9002)
carrying the `process_id` and matched pattern.

**Example triggers:**
```
"listening on :3000"
"ERROR"
"Compiled successfully"
```

Triggers are a process property, not a bash property. They are part of
the workload's observability contract.

### PTY

When `pty=true`, a pseudo-terminal is allocated for the process. PTY
enables interactive attachment via the VPP console (port 9001).
Processes with PTY have the same console semantics as the default shell
— the agent can `attach` to them and interact directly.

### Ownership and cleanup

| Owner | Disconnect policy                  | VM shutdown policy           |
|-------|-----------------------------------|------------------------------|
| agent | kill (ephemeral) / keep (persistent) | terminate                   |
| user  | policy-dependent                  | policy-dependent             |

When the agent disconnects or its session ends:
- **Ephemeral processes** (`persistent=false`) are killed immediately.
- **Persistent processes** (`persistent=true`) continue running. The
  agent can re-attach to them in a future session.

On VM shutdown:
- **System services** stop gracefully first.
- **Agent processes** are terminated.
- **User processes** follow configured policy.

### RPC operations (port 9000)

Process management is exposed through the RPC channel on port 9000
using message types in a dedicated range (see [execution.md](services/execution.md)).
Operations include:

```
process.spawn(process_id, command, cwd, env, pty, persistent, owner, triggers) → process_id
process.inspect(process_id)        → ProcInfo
process.logs(process_id, stream)   → data stream
process.kill(process_id, signal)   → exit_code
process.restart(process_id)        → ProcInfo
process.attach(process_id)         → (VPP redirect)
process.wait(process_id, timeout)  → exit_code
process.list()                      → []ProcInfo
```

## 3. Services

### Semantics

A service is a workload that **belongs to the VM environment**, not to
any agent. Services are declared in the VM configuration (manifest or
init script) and start at boot. They run for the VM's entire lifetime
and have no agent dependency.

```
            ┌────────────┐
            │  DEFINED   │
            └─────┬──────┘
                  │ boot / start
                  ▼
            ┌────────────┐
            │  STARTING  │
            └─────┬──────┘
                  │
            ┌─────▼──────┐     crash     ┌────────────┐
            │  RUNNING   │──────────────→│   FAILED   │
            └─────┬──────┘               └─────┬──────┘
                  │                    restart │
                  │                    ┌───────▼──────┐
                  │                    │ RESTARTING   │
                  │                    └───────┬──────┘
                  │                            │
                  └────────────────────────────┘
```

### Service identity

| Field          | Type     | Description                                 |
|---------------|----------|---------------------------------------------|
| `service_id`   | string   | Unique identifier from VM manifest           |
| `command`      | string   | Command to execute                           |
| `pid`          | uint32   | Current guest OS PID                         |
| `state`        | enum     | `defined` / `starting` / `running` / `failed` / `restarting` |
| `restart_policy` | enum   | `never` / `always` / `on-failure`            |
| `restart_count` | uint32   | Number of restarts since boot                |
| `started_at`   | uint64   | Last start timestamp (Unix ns)               |
| `exit_code`    | int32    | Last exit code                               |

### Ownership

Services are always `owner = system`. They are not tied to any agent
session. The vhandler manages their lifecycle independently of agent
connectivity.

### RPC operations (port 9000)

```
service.list()                    → []SvcInfo
service.inspect(service_id)       → SvcInfo
service.logs(service_id, stream)  → data stream
service.restart(service_id)       → SvcInfo
```

Start and stop for services are typically driven by the VM boot/shutdown
sequence, not by RPC. The RPC layer provides introspection and
administrative control.

## Ownership Summary

```
PID  TYPE      OWNER    PERSISTENT  CLEANUP ON DISCONNECT
────────────────────────────────────────────────────────
42   service   system   yes         (unaffected)
71   process   agent    no          kill
82   process   agent    yes         keep
91   process   user     policy      policy-dependent
```

## VM Boot Sequence

```
VM BOOT
   │
   ├── 1. Mount filesystems (overlayfs)
   │
   ├── 2. Start system services (declared in manifest)
   │       └── service-01  ./my-server     → running
   │       └── service-02  postgres        → running
   │
   ├── 3. vhandler enters main loop
   │       ├── Port 9000: RPC (control + execution + process + service mgmt)
   │       ├── Port 9001: VPP console
   │       ├── Port 9002: Events
   │       ├── Port 9003: Status (heartbeat + query)
│       └── Port 9004: File Transfer
   │
   └── 4. Agent connects → spawns processes → tools → disconnect
```

## VM Shutdown Sequence

```
VM SHUTDOWN (host-initiated or guest-initiated)
   │
   ├── 1. Signal agent disconnection
   │       └── Cleanup agent processes per policy
   │           ├── ephemeral  → kill
   │           └── persistent → keep (or policy)
   │
   ├── 2. Stop system services gracefully
   │       └── SIGTERM → wait → SIGKILL
   │
   ├── 3. Stop user processes per policy
   │
   └── 4. Unmount filesystems, halt
```

## Observability

All three workload types contribute to the VM's observability model:

| Source    | Channel                  | Content                                   |
|-----------|-------------------------|-------------------------------------------|
| Tools     | Port 9000 (TOOL_RESULT) | Inline result data                        |
| Tools     | Port 9002 (log events)  | Tool execution attempt + outcome          |
| Processes | Port 9002 (events)      | PROC_EVENT on trigger match               |
| Processes | Port 9001 (VPP/PTY)     | Interactive console attachment            |
| Services  | Port 9002 (events)      | SVC_EVENT on state transitions            |
| Services  | Port 9002 (log events)  | Service start/stop/restart                |

---

See also:
- [tools.md](services/tools.md) — Tool service (port 9000) spec.
- [execution.md](services/execution.md) — Execution service (one-shot EXEC, port 9000).
- [events.md](services/events.md) — Event notifications (port 9002).
- [console.md](services/console.md) — VPP interactive terminal (port 9001).
- [rpc.md](services/rpc.md) — RPC layer contract (port 9000).
